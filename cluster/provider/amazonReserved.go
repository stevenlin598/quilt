package provider

import (
	"encoding/base64"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type amazonReserved struct {
	amazonCluster
}

func (clst amazonReserved) Boot(bootSet []Machine) error {
	waiter := newWaiter()
	defer close(waiter.waiters)

	for br, count := range bootRequests(bootSet) {
		session := clst.getSession(br.region)
		resp, err := session.RunInstances(&ec2.RunInstancesInput{
			ImageId:      aws.String(amis[br.region]),
			InstanceType: aws.String(br.size),
			UserData: aws.String(base64.StdEncoding.EncodeToString(
				[]byte(br.cfg))),
			SecurityGroups: []*string{&clst.namespace},
			BlockDeviceMappings: []*ec2.BlockDeviceMapping{
				blockDevice(br.diskSize)},
			MaxCount: &count,
			MinCount: &count,
		})
		if err != nil {
			return err
		}

		var bootedIDs []string
		for _, inst := range resp.Instances {
			bootedIDs = append(bootedIDs, *inst.InstanceId)
		}

		waiter.waiters <- instanceBooted{
			session: session,
			ids:     bootedIDs,
		}
	}

	return waiter.wait()
}

func (clst amazonReserved) Stop(machines []Machine) error {
	waiter := newWaiter()
	defer close(waiter.waiters)

	for region, regionMachines := range groupByRegion(machines) {
		session := clst.getSession(region)
		ids := getIDs(regionMachines)

		_, err := session.TerminateInstances(&ec2.TerminateInstancesInput{
			InstanceIds: aws.StringSlice(ids),
		})
		if err != nil {
			return err
		}

		waiter.waiters <- instanceStopped{
			session: session,
			ids:     ids,
		}
	}

	return waiter.wait()
}

func (clst amazonReserved) List() ([]Machine, error) {
	ourInsts, err := clst.listInstances()
	if err != nil {
		return nil, err
	}

	var machines []Machine
	for _, inst := range ourInsts {
		// A reserved machines is any machine in our namespace without a spot ID.
		if inst.spotID == "" {
			machines = append(machines, inst.toReservedMachine())
		}
	}
	return machines, nil
}
