package network

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/minion/ovsdb"
)

type lportslice []ovsdb.LPort

func (lps lportslice) Len() int {
	return len(lps)
}

func (lps lportslice) Less(i, j int) bool {
	return lps[i].Name < lps[j].Name
}

func (lps lportslice) Swap(i, j int) {
	lps[i], lps[j] = lps[j], lps[i]
}

func TestRunMaster(t *testing.T) {
	client := ovsdb.NewFakeOvsdbClient()
	client.CreateLogicalSwitch(lSwitch)
	conn := db.New()
	ovsdb.Open = func() (ovsdb.Client, error) {
		return client, nil
	}

	expPorts := []ovsdb.LPort{}
	conn.Transact(func(view db.Database) error {
		etcd := view.InsertEtcd()
		etcd.Leader = true
		view.Commit(etcd)

		for i := 0; i < 3; i++ {
			si := strconv.Itoa(i)
			l := view.InsertLabel()
			l.IP = fmt.Sprintf("0.0.0.%s", si)
			l.MultiHost = true
			view.Commit(l)
			expPorts = append(expPorts, ovsdb.LPort{
				Bridge:    lSwitch,
				Name:      l.IP,
				Addresses: []string{labelMac, l.IP},
			})
		}

		for i := 3; i < 5; i++ {
			si := strconv.Itoa(i)
			c := view.InsertContainer()
			c.IP = fmt.Sprintf("0.0.0.%s", si)
			c.Mac = fmt.Sprintf("00:00:00:00:00:0%s", si)
			view.Commit(c)
			expPorts = append(expPorts, ovsdb.LPort{
				Bridge:    lSwitch,
				Name:      c.IP,
				Addresses: []string{c.Mac, c.IP},
			})
		}

		return nil
	})

	for i := 1; i < 6; i++ {
		si := strconv.Itoa(i)
		mac := fmt.Sprintf("00:00:00:00:00:0%s", si)
		if i < 3 {
			mac = labelMac
		}
		ip := fmt.Sprintf("0.0.0.%s", si)
		client.CreateLogicalPort(lSwitch, ip, mac, ip)
	}

	runMaster(conn)

	lports, err := client.ListLogicalPorts(lSwitch)
	if err != nil {
		t.Fatal("failed to fetch logical ports from mock client")
	}

	if len(lports) != len(expPorts) {
		t.Fatalf("wrong number of logical ports. Got %d, expected %d.",
			len(lports), len(expPorts))
	}

	sort.Sort(lportslice(lports))
	sort.Sort(lportslice(expPorts))
	for i, port := range expPorts {
		lport := lports[i]
		if lport.Bridge != port.Bridge || lport.Name != port.Name {
			t.Fatalf("Incorrect port %v, expected %v.", lport, port)
		}
	}
}

func checkACLCount(t *testing.T, client ovsdb.Client,
	connections []db.Connection, expCount int) {

	updateACLs(connections, allLabels, allContainers)
	if acls, _ := client.ListACLs(lSwitch); len(acls) != expCount {
		t.Errorf("Wrong number of ACLs: expected %d, got %d.",
			expCount, len(acls))
	}
}

func TestACLUpdate(t *testing.T) {
	client := ovsdb.NewFakeOvsdbClient()
	client.CreateLogicalSwitch(lSwitch)
	ovsdb.Open = func() (ovsdb.Client, error) {
		return client, nil
	}
	redBlueConnection := db.Connection{
		From:    "red",
		To:      "blue",
		MinPort: 80,
		MaxPort: 80,
	}
	redYellowConnection := db.Connection{
		From:    "redBlue",
		To:      "redBlue",
		MinPort: 80,
		MaxPort: 81,
	}
	checkACLCount(t, client, []db.Connection{redBlueConnection}, 10)
	checkACLCount(t, client,
		[]db.Connection{redBlueConnection, redYellowConnection}, 26)
	checkACLCount(t, client, []db.Connection{redYellowConnection}, 18)
	checkACLCount(t, client, nil, 2)
}

func allowMatch(acls map[ovsdb.AclCore]struct{}, match string) {
	acls[ovsdb.AclCore{
		Priority:  1,
		Direction: "from-lport",
		Action:    "allow",
		Match:     match,
	}] = struct{}{}
	acls[ovsdb.AclCore{
		Priority:  1,
		Direction: "to-lport",
		Action:    "allow",
		Match:     match,
	}] = struct{}{}
}

func TestACLGeneration(t *testing.T) {
	exp := map[ovsdb.AclCore]struct{}{
		{
			Priority:  0,
			Direction: "from-lport",
			Match:     "ip",
			Action:    "drop",
		}: {},
		{
			Priority:  0,
			Direction: "to-lport",
			Match:     "ip",
			Action:    "drop",
		}: {},
	}
	allowMatch(exp, "ip4.src==8.8.8.8 && ip4.dst==9.9.9.9 "+
		"&& (80 <= udp.dst <= 80 || 80 <= tcp.dst <= 80)")
	allowMatch(exp, "ip4.src==8.8.8.8 && ip4.dst==9.9.9.9 && icmp")
	allowMatch(exp, "ip4.src==9.9.9.9 && ip4.dst==8.8.8.8 "+
		"&& (80 <= udp.src <= 80 || 80 <= tcp.src <= 80)")
	allowMatch(exp, "ip4.src==9.9.9.9 && ip4.dst==8.8.8.8 && icmp")
	allowMatch(exp, "ip4.src==10.10.10.10 && ip4.dst==12.12.12.12 "+
		"&& (80 <= udp.dst <= 81 || 80 <= tcp.dst <= 81)")
	allowMatch(exp, "ip4.src==10.10.10.10 && ip4.dst==12.12.12.12 && icmp")
	allowMatch(exp, "ip4.src==10.10.10.10 && ip4.dst==13.13.13.13 "+
		"&& (80 <= udp.dst <= 81 || 80 <= tcp.dst <= 81)")
	allowMatch(exp, "ip4.src==10.10.10.10 && ip4.dst==13.13.13.13 && icmp")
	allowMatch(exp, "ip4.src==11.11.11.11 && ip4.dst==12.12.12.12 "+
		"&& (80 <= udp.dst <= 81 || 80 <= tcp.dst <= 81)")
	allowMatch(exp, "ip4.src==11.11.11.11 && ip4.dst==12.12.12.12 && icmp")
	allowMatch(exp, "ip4.src==11.11.11.11 && ip4.dst==13.13.13.13 "+
		"&& (80 <= udp.dst <= 81 || 80 <= tcp.dst <= 81)")
	allowMatch(exp, "ip4.src==11.11.11.11 && ip4.dst==13.13.13.13 && icmp")
	allowMatch(exp, "ip4.src==12.12.12.12 && ip4.dst==10.10.10.10 "+
		"&& (80 <= udp.src <= 81 || 80 <= tcp.src <= 81)")
	allowMatch(exp, "ip4.src==12.12.12.12 && ip4.dst==10.10.10.10 && icmp")
	allowMatch(exp, "ip4.src==12.12.12.12 && ip4.dst==11.11.11.11 "+
		"&& (80 <= udp.src <= 81 || 80 <= tcp.src <= 81)")
	allowMatch(exp, "ip4.src==12.12.12.12 && ip4.dst==11.11.11.11 && icmp")
	allowMatch(exp, "ip4.src==13.13.13.13 && ip4.dst==10.10.10.10 "+
		"&& (80 <= udp.src <= 81 || 80 <= tcp.src <= 81)")
	allowMatch(exp, "ip4.src==13.13.13.13 && ip4.dst==10.10.10.10 && icmp")
	allowMatch(exp, "ip4.src==13.13.13.13 && ip4.dst==11.11.11.11 "+
		"&& (80 <= udp.src <= 81 || 80 <= tcp.src <= 81)")
	allowMatch(exp, "ip4.src==13.13.13.13 && ip4.dst==11.11.11.11 && icmp")

	actual := generateACLs(
		[]aclConnection{
			{
				fromIPs: []string{"8.8.8.8"},
				toIPs:   []string{"9.9.9.9"},
				minPort: 80,
				maxPort: 80,
			},
			{
				fromIPs: []string{"10.10.10.10", "11.11.11.11"},
				toIPs:   []string{"12.12.12.12", "13.13.13.13"},
				minPort: 80,
				maxPort: 81,
			},
		},
	)
	if !reflect.DeepEqual(actual, exp) {
		t.Errorf("Bad ACL generation: expected %v, got %v",
			exp, actual)
	}
}

func checkConnectionConstruction(t *testing.T, connections []db.Connection,
	labels []db.Label, containers []db.Container, expected []aclConnection) {

	actual := getACLConnections(connections, labels, containers)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Bad connection deconstruction: expected %v, got %v",
			expected, actual)
	}
}

var redLabelIP = "8.8.8.8"
var blueLabelIP = "9.9.9.9"
var yellowLabelIP = "10.10.10.10"
var redBlueLabelIP = "13.13.13.13"
var redContainerIP = "100.1.1.1"
var blueContainerIP = "100.1.1.2"
var yellowContainerIP = "100.1.1.3"

var redLabel = db.Label{
	Label: "red",
	IP:    redLabelIP,
}
var blueLabel = db.Label{
	Label: "blue",
	IP:    blueLabelIP,
}
var yellowLabel = db.Label{
	Label: "yellow",
	IP:    yellowLabelIP,
}
var redBlueLabel = db.Label{
	Label: "redBlue",
	IP:    redBlueLabelIP,
}
var allLabels = []db.Label{redLabel, blueLabel, yellowLabel, redBlueLabel}

var redContainer = db.Container{
	IP:     redContainerIP,
	Labels: []string{"red", "redBlue"},
}
var blueContainer = db.Container{
	IP:     blueContainerIP,
	Labels: []string{"blue", "redBlue"},
}
var yellowContainer = db.Container{
	IP:     yellowContainerIP,
	Labels: []string{"yellow"},
}
var allContainers = []db.Container{redContainer, blueContainer, yellowContainer}

func TestConnectionBreakdown(t *testing.T) {

	// No connections should result in no ACLs but the default drop rules.
	checkConnectionConstruction(t,
		[]db.Connection{}, []db.Label{}, []db.Container{}, nil)

	// Test one connection (with range)
	checkConnectionConstruction(t,
		[]db.Connection{
			{
				From:    "red",
				To:      "blue",
				MinPort: 80,
				MaxPort: 81,
			},
		},
		allLabels,
		allContainers,
		[]aclConnection{
			{
				fromIPs: []string{redContainerIP},
				toIPs:   []string{blueLabelIP},
				minPort: 80,
				maxPort: 81,
			},
		},
	)

	// Test connecting from label with multiple containers
	checkConnectionConstruction(t,
		[]db.Connection{
			{
				From:    "redBlue",
				To:      "yellow",
				MinPort: 80,
				MaxPort: 80,
			},
		},
		allLabels,
		allContainers,
		[]aclConnection{
			{
				fromIPs: []string{redContainerIP, blueContainerIP},
				toIPs:   []string{yellowLabelIP},
				minPort: 80,
				maxPort: 80,
			},
		},
	)

	// Test multiple connections
	checkConnectionConstruction(t,
		[]db.Connection{
			{
				From:    "redBlue",
				To:      "yellow",
				MinPort: 80,
				MaxPort: 80,
			},
			{
				From:    "red",
				To:      "blue",
				MinPort: 80,
				MaxPort: 81,
			},
		},
		allLabels,
		allContainers,
		[]aclConnection{
			{
				fromIPs: []string{redContainerIP, blueContainerIP},
				toIPs:   []string{yellowLabelIP},
				minPort: 80,
				maxPort: 80,
			},
			{
				fromIPs: []string{redContainerIP},
				toIPs:   []string{blueLabelIP},
				minPort: 80,
				maxPort: 81,
			},
		},
	)

	// Test toLabel with multiple containers
	checkConnectionConstruction(t,
		[]db.Connection{
			{
				From:    "yellow",
				To:      "redBlue",
				MinPort: 80,
				MaxPort: 80,
			},
		},
		allLabels,
		allContainers,
		[]aclConnection{
			{
				fromIPs: []string{yellowContainerIP},
				toIPs:   []string{redBlueLabelIP},
				minPort: 80,
				maxPort: 80,
			},
		},
	)
}
