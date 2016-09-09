package provider

import (
	"fmt"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/stitch"
)

// Machine represents an instance of a machine booted by a Provider.
type Machine struct {
	ID        string
	PublicIP  string
	PrivateIP string
	Size      string
	DiskSize  int
	SSHKeys   []string
	Provider  db.Provider
	Region    string
}

// Provider defines an interface for interacting with cloud providers.
type Provider interface {
	Connect(namespace string) error

	List() ([]Machine, error)

	Boot([]Machine) error

	Stop([]Machine) error

	SetACLs(acls []string) error

	ChooseSize(ram stitch.Range, cpu stitch.Range, maxPrice float64) string
}

// New returns an empty instance of the Provider represented by `dbp`
func New(dbp db.Provider) Provider {
	switch dbp {
	case db.AmazonSpot:
		return &amazonSpot{
			newAmazonCluster(newEC2Session),
		}
	case db.AmazonReserved:
		return &amazonReserved{
			newAmazonCluster(newEC2Session),
		}
	case db.Google:
		return &gceCluster{}
	case db.Azure:
		return &azureCluster{}
	case db.Vagrant:
		return &vagrantCluster{}
	default:
		panic("Unimplemented")
	}
}

// GroupByProvider transforms the `machines` into a map of `db.Provider` to the machines
// with that provider.
func GroupByProvider(machines []Machine) map[db.Provider][]Machine {
	machineMap := make(map[db.Provider][]Machine)
	for _, m := range machines {
		machineMap[m.Provider] = append(machineMap[m.Provider], m)
	}
	return machineMap
}

func groupByRegion(machines []Machine) map[string][]Machine {
	machineMap := make(map[string][]Machine)
	for _, m := range machines {
		machineMap[m.Region] = append(machineMap[m.Region], m)
	}
	return machineMap
}

type bootRequest struct {
	cfg      string
	size     string
	region   string
	diskSize int
}

func bootRequests(machines []Machine) map[bootRequest]int64 {
	bootReqMap := make(map[bootRequest]int64)
	for _, m := range machines {
		br := bootRequest{
			cfg:      cloudConfigUbuntu(m.SSHKeys, "wily"),
			size:     m.Size,
			region:   m.Region,
			diskSize: m.DiskSize,
		}
		bootReqMap[br] = bootReqMap[br] + 1
	}
	return bootReqMap
}

func getIDs(machines []Machine) []string {
	var ids []string
	for _, m := range machines {
		ids = append(ids, m.ID)
	}
	return ids
}

// DefaultRegion populates `m.Region` for the provided db.Machine if one isn't
// specified. This is intended to allow users to omit the cloud provider region when
// they don't particularly care where a system is placed.
func DefaultRegion(m db.Machine) db.Machine {
	if m.Region != "" {
		return m
	}

	region := ""
	switch m.Provider {
	case "AmazonSpot", "AmazonReserved":
		region = "us-west-1"
	case "Google":
		region = "us-east1-b"
	case "Azure":
		region = "centralus"
	case "Vagrant":
	default:
		panic(fmt.Sprintf("Unknown Cloud Provider: %s", m.Provider))
	}

	m.Region = region
	return m
}
