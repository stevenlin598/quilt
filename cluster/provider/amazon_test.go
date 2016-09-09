package provider

import (
	"encoding/base64"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/NetSys/quilt/cluster/provider/mocks"
	"github.com/NetSys/quilt/db"
)

const testNamespace = "namespace"

func TestList(t *testing.T) {
	t.Parallel()

	mockClient := new(mocks.EC2Client)
	instances := []*ec2.Instance{
		// A booted spot instance (with a matching spot tag).
		{
			InstanceId:            aws.String("inst1"),
			SpotInstanceRequestId: aws.String("spot1"),
			PublicIpAddress:       aws.String("publicIP"),
			PrivateIpAddress:      aws.String("privateIP"),
			InstanceType:          aws.String("size"),
		},
		// A booted spot instance (with a lost spot tag).
		{
			InstanceId:            aws.String("inst2"),
			SpotInstanceRequestId: aws.String("spot2"),
			InstanceType:          aws.String("size2"),
		},
		// A reserved instance.
		{
			InstanceId: aws.String("inst3"),
		},
	}
	mockClient.On("DescribeInstances", mock.Anything).Return(
		&ec2.DescribeInstancesOutput{
			Reservations: []*ec2.Reservation{
				{
					Instances: instances,
				},
			},
		}, nil,
	)
	mockClient.On("DescribeSpotInstanceRequests", mock.Anything).Return(
		&ec2.DescribeSpotInstanceRequestsOutput{
			SpotInstanceRequests: []*ec2.SpotInstanceRequest{
				// A spot request with tags and a corresponding instance.
				{
					SpotInstanceRequestId: aws.String("spot1"),
					State: aws.String(ec2.SpotInstanceStateActive),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("namespace"),
							Value: aws.String(testNamespace),
						},
					},
				},
				// A spot request without tags, but with
				// a corresponding instance.
				{
					SpotInstanceRequestId: aws.String("spot2"),
					State: aws.String(ec2.SpotInstanceStateActive),
				},
				// A spot request that hasn't been booted yet.
				{
					SpotInstanceRequestId: aws.String("spot3"),
					State: aws.String(ec2.SpotInstanceStateOpen),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("namespace"),
							Value: aws.String(testNamespace),
						},
					},
				},
				// A spot request in another namespace.
				{
					SpotInstanceRequestId: aws.String("spot4"),
					State: aws.String(ec2.SpotInstanceStateOpen),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("namespace"),
							Value: aws.String("notOurs"),
						},
					},
				},
			},
		}, nil,
	)

	emptyClient := new(mocks.EC2Client)
	emptyClient.On("DescribeInstances", mock.Anything).Return(
		&ec2.DescribeInstancesOutput{}, nil,
	)
	emptyClient.On("DescribeSpotInstanceRequests", mock.Anything).Return(
		&ec2.DescribeSpotInstanceRequestsOutput{}, nil,
	)

	amazonCluster := newAmazonCluster(func(region string) EC2Client {
		if region == "us-west-1" {
			return mockClient
		}
		return emptyClient
	})

	amazonCluster.namespace = testNamespace
	spotCluster := &amazonSpot{
		amazonCluster,
	}
	spots, err := spotCluster.List()

	assert.Nil(t, err)
	assert.Equal(t, []Machine{
		{
			ID:       "spot3",
			Provider: db.AmazonSpot,
			Region:   "us-west-1",
		},
		{
			ID:        "spot1",
			Provider:  db.AmazonSpot,
			PublicIP:  "publicIP",
			PrivateIP: "privateIP",
			Size:      "size",
			Region:    "us-west-1",
		},
		{
			ID:       "spot2",
			Provider: db.AmazonSpot,
			Region:   "us-west-1",
			Size:     "size2",
		},
	}, spots)

	reservedCluster := &amazonReserved{
		amazonCluster,
	}
	reserved, err := reservedCluster.List()

	assert.Nil(t, err)
	assert.Equal(t, []Machine{
		{
			ID:       "inst3",
			Provider: db.AmazonReserved,
			Region:   "us-west-1",
		},
	}, reserved)
}

func TestNewACLs(t *testing.T) {
	t.Parallel()

	mockClient := new(mocks.EC2Client)
	mockClient.On("DescribeSecurityGroups", mock.Anything).Return(
		&ec2.DescribeSecurityGroupsOutput{
			SecurityGroups: []*ec2.SecurityGroup{
				{
					IpPermissions: []*ec2.IpPermission{
						{
							IpRanges: []*ec2.IpRange{
								{CidrIp: aws.String(
									"foo")},
								{CidrIp: aws.String(
									"deleteMe")},
							},
							IpProtocol: aws.String("-1"),
						},
						{
							// An extra permission group:
							// Should be deleted.
							IpProtocol: aws.String("0"),
						},
					},
				},
			},
		}, nil,
	)
	mockClient.On("RevokeSecurityGroupIngress", mock.Anything).Return(
		&ec2.RevokeSecurityGroupIngressOutput{}, nil,
	)
	mockClient.On("AuthorizeSecurityGroupIngress", mock.Anything).Return(
		&ec2.AuthorizeSecurityGroupIngressOutput{}, nil,
	)

	cluster := newAmazonCluster(func(region string) EC2Client {
		return mockClient
	})
	cluster.namespace = testNamespace

	err := cluster.SetACLs([]string{"foo", "bar"})

	assert.Nil(t, err)

	mockClient.AssertCalled(t, "AuthorizeSecurityGroupIngress",
		&ec2.AuthorizeSecurityGroupIngressInput{
			GroupName:               aws.String(testNamespace),
			SourceSecurityGroupName: aws.String(testNamespace),
		},
	)

	mockClient.AssertCalled(t, "AuthorizeSecurityGroupIngress",
		&ec2.AuthorizeSecurityGroupIngressInput{
			CidrIp:     aws.String("bar"),
			GroupName:  aws.String(testNamespace),
			IpProtocol: aws.String("-1"),
		},
	)

	mockClient.AssertCalled(t, "RevokeSecurityGroupIngress",
		&ec2.RevokeSecurityGroupIngressInput{
			GroupName: aws.String(testNamespace),
			IpPermissions: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("0"),
				},
			},
		},
	)

	mockClient.AssertCalled(t, "RevokeSecurityGroupIngress",
		&ec2.RevokeSecurityGroupIngressInput{
			GroupName:  aws.String(testNamespace),
			CidrIp:     aws.String("deleteMe"),
			IpProtocol: aws.String("-1"),
		},
	)
}

func TestSpotBoot(t *testing.T) {
	t.Parallel()

	mockClient := new(mocks.EC2Client)
	mockClient.On("RequestSpotInstances", mock.Anything).Return(
		&ec2.RequestSpotInstancesOutput{
			SpotInstanceRequests: []*ec2.SpotInstanceRequest{
				{
					SpotInstanceRequestId: aws.String("spotID1"),
				},
				{
					SpotInstanceRequestId: aws.String("spotID2"),
				},
			},
		}, nil,
	)
	mockClient.On("CreateTags", mock.Anything).Return(
		&ec2.CreateTagsOutput{}, nil,
	)
	mockClient.On("WaitUntilSpotInstanceRequestFulfilled", mock.Anything).Return(
		nil,
	)

	amazonCluster := newAmazonCluster(func(region string) EC2Client {
		return mockClient
	})
	amazonCluster.namespace = testNamespace
	spotCluster := &amazonSpot{
		amazonCluster,
	}

	err := spotCluster.Boot([]Machine{
		{
			Region:   "us-west-1",
			Size:     "m4.large",
			DiskSize: 32,
		},
		{
			Region:   "us-west-1",
			Size:     "m4.large",
			DiskSize: 32,
		},
	})
	assert.Nil(t, err)

	cfg := cloudConfigUbuntu(nil, "wily")
	mockClient.AssertCalled(t, "RequestSpotInstances",
		&ec2.RequestSpotInstancesInput{
			SpotPrice: aws.String(spotPrice),
			LaunchSpecification: &ec2.RequestSpotLaunchSpecification{
				ImageId:      aws.String(amis["us-west-1"]),
				InstanceType: aws.String("m4.large"),
				UserData: aws.String(base64.StdEncoding.EncodeToString(
					[]byte(cfg))),
				SecurityGroups: aws.StringSlice([]string{testNamespace}),
				BlockDeviceMappings: []*ec2.BlockDeviceMapping{
					blockDevice(32)},
			},
			InstanceCount: aws.Int64(2),
		},
	)
	mockClient.AssertCalled(t, "CreateTags",
		&ec2.CreateTagsInput{
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(namespaceTagKey),
					Value: aws.String(testNamespace),
				},
			},
			Resources: aws.StringSlice([]string{"spotID1", "spotID2"}),
		},
	)
}

func TestSpotStop(t *testing.T) {
	t.Parallel()

	mockClient := new(mocks.EC2Client)
	mockClient.On("DescribeSpotInstanceRequests", mock.Anything).Return(
		&ec2.DescribeSpotInstanceRequestsOutput{
			SpotInstanceRequests: []*ec2.SpotInstanceRequest{
				{
					SpotInstanceRequestId: aws.String("spot1"),
					InstanceId:            aws.String("inst1"),
				},
				{
					SpotInstanceRequestId: aws.String("spot2"),
				},
			},
		}, nil,
	)
	mockClient.On("TerminateInstances", mock.Anything).Return(
		&ec2.TerminateInstancesOutput{}, nil,
	)
	mockClient.On("CancelSpotInstanceRequests", mock.Anything).Return(
		&ec2.CancelSpotInstanceRequestsOutput{}, nil,
	)
	mockClient.On("WaitUntilInstanceTerminated", mock.Anything).Return(
		nil,
	)

	spotCluster := &amazonSpot{
		newAmazonCluster(func(region string) EC2Client {
			return mockClient
		}),
	}

	err := spotCluster.Stop([]Machine{
		{
			Region: "us-west-1",
			ID:     "spot1",
		},
		{
			Region: "us-west-1",
			ID:     "spot2",
		},
	})
	assert.Nil(t, err)

	mockClient.AssertCalled(t, "TerminateInstances",
		&ec2.TerminateInstancesInput{
			InstanceIds: aws.StringSlice([]string{"inst1"}),
		},
	)

	mockClient.AssertCalled(t, "CancelSpotInstanceRequests",
		&ec2.CancelSpotInstanceRequestsInput{
			SpotInstanceRequestIds: aws.StringSlice(
				[]string{"spot1", "spot2"}),
		},
	)
}

func TestReservedBoot(t *testing.T) {
	t.Parallel()

	mockClient := new(mocks.EC2Client)
	mockClient.On("RunInstances", mock.Anything).Return(
		&ec2.Reservation{
			Instances: []*ec2.Instance{
				{
					InstanceId: aws.String("inst1"),
				},
				{
					InstanceId: aws.String("inst2"),
				},
			},
		}, nil,
	)
	mockClient.On("WaitUntilInstanceExists", mock.Anything).Return(
		nil,
	)

	amazonCluster := newAmazonCluster(func(region string) EC2Client {
		return mockClient
	})
	amazonCluster.namespace = testNamespace
	reservedCluster := &amazonReserved{
		amazonCluster,
	}

	err := reservedCluster.Boot([]Machine{
		{
			Region:   "us-west-1",
			Size:     "m4.large",
			DiskSize: 32,
		},
		{
			Region:   "us-west-1",
			Size:     "m4.large",
			DiskSize: 32,
		},
	})
	assert.Nil(t, err)

	cfg := cloudConfigUbuntu(nil, "wily")
	mockClient.AssertCalled(t, "RunInstances",
		&ec2.RunInstancesInput{
			ImageId:      aws.String(amis["us-west-1"]),
			InstanceType: aws.String("m4.large"),
			UserData: aws.String(base64.StdEncoding.EncodeToString(
				[]byte(cfg))),
			SecurityGroups: aws.StringSlice([]string{testNamespace}),
			BlockDeviceMappings: []*ec2.BlockDeviceMapping{
				blockDevice(32)},
			MaxCount: aws.Int64(2),
			MinCount: aws.Int64(2),
		},
	)
}

func TestReservedStop(t *testing.T) {
	t.Parallel()

	mockClient := new(mocks.EC2Client)
	mockClient.On("TerminateInstances", mock.Anything).Return(
		&ec2.TerminateInstancesOutput{}, nil,
	)
	mockClient.On("WaitUntilInstanceTerminated", mock.Anything).Return(
		nil,
	)

	reservedCluster := &amazonReserved{
		newAmazonCluster(func(region string) EC2Client {
			return mockClient
		}),
	}

	err := reservedCluster.Stop([]Machine{
		{
			Region: "us-west-1",
			ID:     "inst1",
		},
		{
			Region: "us-west-1",
			ID:     "inst2",
		},
	})
	assert.Nil(t, err)

	mockClient.AssertCalled(t, "TerminateInstances",
		&ec2.TerminateInstancesInput{
			InstanceIds: aws.StringSlice([]string{"inst1", "inst2"}),
		},
	)
}
