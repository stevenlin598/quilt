package provider

import (
	"encoding/base64"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	log "github.com/Sirupsen/logrus"

	"github.com/NetSys/quilt/join"
)

const (
	spotPrice       = "0.5"
	namespaceTagKey = "namespace"
)

type amazonSpot struct {
	amazonCluster
}

func (clst amazonSpot) Boot(bootSet []Machine) error {
	waiter := newWaiter()
	defer close(waiter.waiters)

	for br, count := range bootRequests(bootSet) {
		session := clst.getSession(br.region)
		resp, err := session.RequestSpotInstances(&ec2.RequestSpotInstancesInput{
			SpotPrice: aws.String(spotPrice),
			LaunchSpecification: &ec2.RequestSpotLaunchSpecification{
				ImageId:      aws.String(amis[br.region]),
				InstanceType: aws.String(br.size),
				UserData: aws.String(base64.StdEncoding.EncodeToString(
					[]byte(br.cfg))),
				SecurityGroups: []*string{&clst.namespace},
				BlockDeviceMappings: []*ec2.BlockDeviceMapping{
					blockDevice(br.diskSize)},
			},
			InstanceCount: &count,
		})
		if err != nil {
			return err
		}

		var spotIDs []string
		for _, request := range resp.SpotInstanceRequests {
			spotIDs = append(spotIDs, *request.SpotInstanceRequestId)
		}

		if err := clst.tagSpotRequests(session, spotIDs); err != nil {
			return err
		}

		waiter.add(spotBooted{
			session: session,
			ids:     spotIDs,
		})
	}

	return waiter.wait()
}

func (clst amazonSpot) tagSpotRequests(session EC2Client, spotIDs []string) (err error) {
	for i := 0; i < 30; i++ {
		_, err = session.CreateTags(&ec2.CreateTagsInput{
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(namespaceTagKey),
					Value: aws.String(clst.namespace),
				},
			},
			Resources: aws.StringSlice(spotIDs),
		})
		if err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	log.Warn("Failed to tag spot requests: ", err)
	session.CancelSpotInstanceRequests(
		&ec2.CancelSpotInstanceRequestsInput{
			SpotInstanceRequestIds: aws.StringSlice(spotIDs),
		})

	return err
}

func (clst amazonSpot) Stop(machines []Machine) error {
	waiter := newWaiter()
	defer close(waiter.waiters)

	for region, regionMachines := range groupByRegion(machines) {
		session := clst.getSession(region)
		spotIDs := getIDs(regionMachines)

		spots, err := session.DescribeSpotInstanceRequests(
			&ec2.DescribeSpotInstanceRequestsInput{
				SpotInstanceRequestIds: aws.StringSlice(spotIDs),
			})
		if err != nil {
			return err
		}

		instIds := []string{}
		for _, spot := range spots.SpotInstanceRequests {
			if spot.InstanceId != nil {
				instIds = append(instIds, *spot.InstanceId)
			}
		}

		if len(instIds) > 0 {
			_, err = session.TerminateInstances(&ec2.TerminateInstancesInput{
				InstanceIds: aws.StringSlice(instIds),
			})
			if err != nil {
				return err
			}
			waiter.add(instanceStopped{
				session: session,
				ids:     instIds,
			})
		}

		_, err = session.CancelSpotInstanceRequests(
			&ec2.CancelSpotInstanceRequestsInput{
				SpotInstanceRequestIds: aws.StringSlice(spotIDs),
			})
		if err != nil {
			return err
		}
	}

	return waiter.wait()
}

func (clst amazonSpot) List() ([]Machine, error) {
	allSpots, err := clst.allSpots()
	if err != nil {
		return nil, err
	}
	ourInsts, err := clst.listInstances()
	if err != nil {
		return nil, err
	}

	spotIDKey := func(intf interface{}) interface{} {
		return intf.(awsMachine).spotID
	}
	bootedSpots, nonbootedSpots, _ :=
		join.HashJoin(awsMachineSlice(allSpots), awsMachineSlice(ourInsts),
			spotIDKey, spotIDKey)

	// Due to a race condition in the AWS API, it's possible that
	// spot requests might lose their Tags. If handled naively,
	// those spot requests would technically be without a namespace,
	// meaning the instances they create would be live forever as
	// zombies.
	//
	// To mitigate this issue, we rely not only on the spot request
	// tags, but additionally on the instance security group. If a
	// spot request has a running instance in the appropriate
	// security group, it is by definition in our namespace.
	// Thus, we only check the tags for spot requests without
	// running instances.
	var machines []Machine
	for _, mIntf := range nonbootedSpots {
		m := mIntf.(awsMachine)
		if m.namespace == clst.namespace {
			machines = append(machines, m.toSpotMachine())
		}
	}
	for _, pair := range bootedSpots {
		machines = append(machines, pair.R.(awsMachine).toSpotMachine())
	}
	return machines, nil
}

type awsMachineSlice []awsMachine

func (ams awsMachineSlice) Get(ii int) interface{} {
	return ams[ii]
}

func (ams awsMachineSlice) Len() int {
	return len(ams)
}

var trackedSpotStates = aws.StringSlice(
	[]string{ec2.SpotInstanceStateActive, ec2.SpotInstanceStateOpen})

// `allSpots` fetches and parses all spot requests into a list of `awsMachine`s.
func (clst amazonSpot) allSpots() ([]awsMachine, error) {
	var machines []awsMachine
	for region := range amis {
		session := clst.getSession(region)
		spotsResp, err := session.DescribeSpotInstanceRequests(
			&ec2.DescribeSpotInstanceRequestsInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("state"),
						Values: trackedSpotStates,
					},
				},
			})
		if err != nil {
			return nil, err
		}

		for _, spot := range spotsResp.SpotInstanceRequests {
			var namespace string
			for _, tag := range spot.Tags {
				if tag != nil &&
					resolveString(tag.Key) == namespaceTagKey {
					namespace = resolveString(tag.Value)
					break
				}
			}
			machines = append(machines, awsMachine{
				namespace: namespace,
				spotID:    resolveString(spot.SpotInstanceRequestId),
				region:    region,
			})
		}
	}
	return machines, nil
}
