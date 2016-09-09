package provider

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	log "github.com/Sirupsen/logrus"

	"github.com/NetSys/quilt/constants"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/stitch"
)

// Ubuntu 15.10, 64-bit hvm-ssd
var amis = map[string]string{
	"ap-southeast-2": "ami-f599ba96",
	"us-west-1":      "ami-af671bcf",
	"us-west-2":      "ami-acd63bcc",
}

// amazonCluster implements some shared functionality between AmazonSpot and
// AmazonReserved. This is namely setting ACLs, listing instances, and choosing
// sizes.
type amazonCluster struct {
	sessionGetter func(string) EC2Client
	sessions      map[string]EC2Client

	namespace string
}

// EC2Client defines an interface that can be mocked out for interacting with EC2.
type EC2Client interface {
	AuthorizeSecurityGroupIngress(*ec2.AuthorizeSecurityGroupIngressInput) (
		*ec2.AuthorizeSecurityGroupIngressOutput, error)

	CancelSpotInstanceRequests(*ec2.CancelSpotInstanceRequestsInput) (
		*ec2.CancelSpotInstanceRequestsOutput, error)

	CreateSecurityGroup(*ec2.CreateSecurityGroupInput) (
		*ec2.CreateSecurityGroupOutput, error)

	CreateTags(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)

	DescribeSecurityGroups(*ec2.DescribeSecurityGroupsInput) (
		*ec2.DescribeSecurityGroupsOutput, error)

	DescribeInstances(*ec2.DescribeInstancesInput) (
		*ec2.DescribeInstancesOutput, error)

	DescribeSpotInstanceRequests(*ec2.DescribeSpotInstanceRequestsInput) (
		*ec2.DescribeSpotInstanceRequestsOutput, error)

	DescribeVolumes(*ec2.DescribeVolumesInput) (
		*ec2.DescribeVolumesOutput, error)

	RevokeSecurityGroupIngress(*ec2.RevokeSecurityGroupIngressInput) (
		*ec2.RevokeSecurityGroupIngressOutput, error)

	TerminateInstances(*ec2.TerminateInstancesInput) (
		*ec2.TerminateInstancesOutput, error)

	RequestSpotInstances(*ec2.RequestSpotInstancesInput) (
		*ec2.RequestSpotInstancesOutput, error)

	RunInstances(*ec2.RunInstancesInput) (*ec2.Reservation, error)

	WaitUntilInstanceExists(*ec2.DescribeInstancesInput) error

	WaitUntilInstanceTerminated(*ec2.DescribeInstancesInput) error

	WaitUntilSpotInstanceRequestFulfilled(
		*ec2.DescribeSpotInstanceRequestsInput) error
}

func newAmazonCluster(sessionGetter func(string) EC2Client) amazonCluster {
	return amazonCluster{
		sessions:      make(map[string]EC2Client),
		sessionGetter: sessionGetter,
	}
}

func (clst *amazonCluster) Connect(namespace string) error {
	clst.namespace = namespace
	if _, err := clst.listInstances(); err != nil {
		return errors.New("AWS failed to connect")
	}
	return nil
}

func newEC2Session(region string) EC2Client {
	session := session.New()
	session.Config.Region = aws.String(region)
	return ec2.New(session)
}

func (clst amazonCluster) getSession(region string) EC2Client {
	if _, ok := clst.sessions[region]; !ok {
		clst.sessions[region] = clst.sessionGetter(region)
	}

	return clst.sessions[region]
}

// blockDevice returns the block device we use for our AWS machines.
func blockDevice(diskSize int) *ec2.BlockDeviceMapping {
	return &ec2.BlockDeviceMapping{
		DeviceName: aws.String("/dev/sda1"),
		Ebs: &ec2.EbsBlockDevice{
			DeleteOnTermination: aws.Bool(true),
			VolumeSize:          aws.Int64(int64(diskSize)),
			VolumeType:          aws.String("gp2"),
		},
	}
}

type awsMachine struct {
	namespace  string
	instanceID string
	spotID     string

	region    string
	publicIP  string
	privateIP string
	size      string
	diskSize  int
}

func (m awsMachine) toSpotMachine() Machine {
	return Machine{
		Provider:  db.AmazonSpot,
		ID:        m.spotID,
		PublicIP:  m.publicIP,
		PrivateIP: m.privateIP,
		Region:    m.region,
		Size:      m.size,
		DiskSize:  m.diskSize,
	}
}

func (m awsMachine) toReservedMachine() Machine {
	return Machine{
		Provider:  db.AmazonReserved,
		ID:        m.instanceID,
		PublicIP:  m.publicIP,
		PrivateIP: m.privateIP,
		Region:    m.region,
		Size:      m.size,
		DiskSize:  m.diskSize,
	}
}

func resolveString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func parseDiskSize(session EC2Client, inst ec2.Instance) (int, error) {
	if len(inst.BlockDeviceMappings) == 0 {
		return 0, nil
	}

	volumeID := inst.BlockDeviceMappings[0].Ebs.VolumeId
	filters := []*ec2.Filter{
		{
			Name: aws.String("volume-id"),
			Values: []*string{
				aws.String(*volumeID),
			},
		},
	}

	volumeInfo, err := session.DescribeVolumes(
		&ec2.DescribeVolumesInput{
			Filters: filters,
		})
	if err != nil {
		return 0, err
	}

	if len(volumeInfo.Volumes) == 0 {
		return 0, nil
	}

	return int(*volumeInfo.Volumes[0].Size), nil
}

// `listInstances` fetches and parses all machines in the namespace into a list
// of `awsMachine`s
func (clst amazonCluster) listInstances() ([]awsMachine, error) {
	var instances []awsMachine
	for region := range amis {
		session := clst.getSession(region)

		insts, err := session.DescribeInstances(&ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("instance.group-name"),
					Values: []*string{aws.String(clst.namespace)},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		for _, res := range insts.Reservations {
			for _, inst := range res.Instances {
				diskSize, err := parseDiskSize(session, *inst)
				if err != nil {
					log.WithError(err).
						Warn("Error retrieving Amazon machine " +
							"disk information.")
				}

				instances = append(instances, awsMachine{
					region:     region,
					instanceID: resolveString(inst.InstanceId),
					spotID: resolveString(
						inst.SpotInstanceRequestId),
					publicIP:  resolveString(inst.PublicIpAddress),
					privateIP: resolveString(inst.PrivateIpAddress),
					size:      resolveString(inst.InstanceType),
					diskSize:  diskSize,
				})
			}
		}
	}
	return instances, nil
}

func (clst amazonCluster) ChooseSize(ram stitch.Range, cpu stitch.Range,
	maxPrice float64) string {
	return pickBestSize(constants.AwsDescriptions, ram, cpu, maxPrice)
}

func (clst amazonCluster) SetACLs(acls []string) error {
	for region := range amis {
		session := clst.getSession(region)

		resp, err := session.DescribeSecurityGroups(
			&ec2.DescribeSecurityGroupsInput{
				Filters: []*ec2.Filter{
					{
						Name: aws.String("group-name"),
						Values: []*string{
							aws.String(clst.namespace),
						},
					},
				},
			})

		if err != nil {
			return err
		}

		ingress := []*ec2.IpPermission{}
		groups := resp.SecurityGroups
		if len(groups) > 1 {
			return errors.New(
				"Multiple Security Groups with the same name: " +
					clst.namespace)
		} else if len(groups) == 0 {
			_, err := session.CreateSecurityGroup(
				&ec2.CreateSecurityGroupInput{
					Description: aws.String("Quilt Group"),
					GroupName:   aws.String(clst.namespace),
				})
			if err != nil {
				return err
			}
		} else {
			/* XXX: Deal with egress rules. */
			ingress = groups[0].IpPermissions
		}

		permMap := make(map[string]bool)
		for _, acl := range acls {
			permMap[acl] = true
		}

		groupIngressExists := false
		for i, p := range ingress {
			if (i > 0 || p.FromPort != nil || p.ToPort != nil ||
				*p.IpProtocol != "-1") && p.UserIdGroupPairs == nil {
				log.Debug("Amazon: Revoke ingress security group: ", *p)
				_, err = session.RevokeSecurityGroupIngress(
					&ec2.RevokeSecurityGroupIngressInput{
						GroupName: aws.String(
							clst.namespace),
						IpPermissions: []*ec2.IpPermission{p}})
				if err != nil {
					return err
				}

				continue
			}

			for _, ipr := range p.IpRanges {
				ip := *ipr.CidrIp
				if !permMap[ip] {
					log.Debug("Amazon: Revoke "+
						"ingress security group: ", ip)
					_, err = session.RevokeSecurityGroupIngress(
						&ec2.RevokeSecurityGroupIngressInput{
							GroupName: aws.String(
								clst.namespace),
							CidrIp:     aws.String(ip),
							FromPort:   p.FromPort,
							IpProtocol: p.IpProtocol,
							ToPort:     p.ToPort})
					if err != nil {
						return err
					}
				} else {
					permMap[ip] = false
				}
			}

			if len(groups) == 0 {
				continue
			}
			for _, grp := range p.UserIdGroupPairs {
				if *grp.GroupId != *groups[0].GroupId {
					log.Debug("Amazon: Revoke "+
						"ingress security group GroupID: ",
						*grp.GroupId)
					options := &ec2.RevokeSecurityGroupIngressInput{
						GroupName: aws.String(
							clst.namespace),
						SourceSecurityGroupName: grp.GroupName,
					}
					_, err = session.RevokeSecurityGroupIngress(
						options)
					if err != nil {
						return err
					}
				} else {
					groupIngressExists = true
				}
			}
		}

		if !groupIngressExists {
			log.Debug("Amazon: Add intragroup ACL")
			_, err = session.AuthorizeSecurityGroupIngress(
				&ec2.AuthorizeSecurityGroupIngressInput{
					GroupName: aws.String(
						clst.namespace),
					SourceSecurityGroupName: aws.String(
						clst.namespace)})
			if err != nil {
				return err
			}
		}

		for perm, install := range permMap {
			if !install {
				continue
			}

			log.Debug("Amazon: Add ACL: ", perm)
			_, err = session.AuthorizeSecurityGroupIngress(
				&ec2.AuthorizeSecurityGroupIngressInput{
					CidrIp:     aws.String(perm),
					GroupName:  aws.String(clst.namespace),
					IpProtocol: aws.String("-1")})

			if err != nil {
				return err
			}
		}
	}

	return nil
}
