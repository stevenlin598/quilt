package provider

import (
	"reflect"
	"testing"

	"github.com/NetSys/quilt/constants"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/stitch"
)

func TestDefaultRegion(t *testing.T) {
	exp := "foo"
	m := db.Machine{Provider: "AmazonSpot", Region: exp}
	m = DefaultRegion(m)
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m = DefaultRegion(m)
	exp = "us-west-1"
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m.Provider = "AmazonReserved"
	exp = "us-west-1"
	m = DefaultRegion(m)
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m.Provider = "Google"
	exp = "us-east1-b"
	m = DefaultRegion(m)
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m.Provider = "Azure"
	exp = "centralus"
	m = DefaultRegion(m)
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m.Provider = "Vagrant"
	exp = ""
	m = DefaultRegion(m)
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m.Provider = "Panic"
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic")
		}
	}()

	m = DefaultRegion(m)
}

func TestConstraints(t *testing.T) {
	checkConstraint := func(descriptions []constants.Description, ram stitch.Range,
		cpu stitch.Range, maxPrice float64, exp string) {
		resSize := pickBestSize(descriptions, ram, cpu, maxPrice)
		if resSize != exp {
			t.Errorf("bad size picked. Expected %s, got %s", exp, resSize)
		}
	}

	// Test all constraints specified with valid price
	testDescriptions := []constants.Description{
		{Size: "size1", Price: 2, RAM: 2, CPU: 2},
	}
	checkConstraint(testDescriptions, stitch.Range{Min: 1, Max: 3},
		stitch.Range{Min: 1, Max: 3}, 2, "size1")

	// Test no max
	checkConstraint(testDescriptions, stitch.Range{Min: 1},
		stitch.Range{Min: 1}, 2, "size1")

	// Test exact match
	checkConstraint(testDescriptions, stitch.Range{Min: 2},
		stitch.Range{Min: 2}, 2, "size1")

	// Test no match
	checkConstraint(testDescriptions, stitch.Range{Min: 3},
		stitch.Range{Min: 2}, 2, "")

	// Test price too expensive
	checkConstraint(testDescriptions, stitch.Range{Min: 2},
		stitch.Range{Min: 2}, 1, "")

	// Test multiple matches (should pick cheapest)
	testDescriptions = []constants.Description{
		{Size: "size2", Price: 2, RAM: 8, CPU: 4},
		{Size: "size3", Price: 1, RAM: 4, CPU: 4},
		{Size: "size4", Price: 0.5, RAM: 3, CPU: 4},
	}
	checkConstraint(testDescriptions, stitch.Range{Min: 4},
		stitch.Range{Min: 3}, 2, "size3")

	// Test infinite price
	checkConstraint(testDescriptions, stitch.Range{Min: 4},
		stitch.Range{Min: 3}, 0, "size3")

	// Test default ranges (should pick cheapest)
	checkConstraint(testDescriptions, stitch.Range{},
		stitch.Range{}, 0, "size4")

	// Test one default range (should pick only on the specified range)
	checkConstraint(testDescriptions, stitch.Range{Min: 4},
		stitch.Range{}, 0, "size3")
	checkConstraint(testDescriptions, stitch.Range{Min: 3},
		stitch.Range{}, 0, "size4")
}

func TestNewProviderSuccess(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Error("provider.New panicked on valid provider")
		}
	}()
	New(db.Azure)
	New(db.AmazonSpot)
	New(db.AmazonReserved)
	New(db.Google)
	New(db.Vagrant)
}

func TestNewProviderFailure(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("provider.New did not panic on invalid provider")
		}
	}()
	New("FakeAmazon")
}

func TestGroupByProvider(t *testing.T) {
	machines := []Machine{
		{Provider: db.Google}, {Provider: db.AmazonSpot}, {Provider: db.Google},
		{Provider: db.Google}, {Provider: db.Azure},
	}
	grouped := GroupByProvider(machines)
	m := grouped[db.AmazonSpot]
	if len(m) != 1 || m[0].Provider != machines[1].Provider {
		t.Errorf("wrong Amazon machines: %v", m)
	}
	m = grouped[db.Azure]
	if len(m) != 1 || m[0].Provider != machines[4].Provider {
		t.Errorf("wrong Azure machines: %v", m)
	}
	m = grouped[db.Google]
	if len(m) != 3 {
		t.Errorf("wrong Google machines: %v", m)
	} else {
		for _, machine := range m {
			if machine.Provider != db.Google {
				t.Errorf("machine provider is not Google: %v", machine)
			}
		}
	}
	m = grouped[db.Vagrant]
	if len(m) != 0 {
		t.Errorf("unexpected Vagrant machines: %v", m)
	}
}

func TestGroupByRegion(t *testing.T) {
	machines := []Machine{
		{Provider: db.AmazonSpot, Region: "west"},
		{Provider: db.AmazonSpot, Region: "west"},
		{Provider: db.AmazonSpot, Region: "east"},
	}
	grouped := groupByRegion(machines)
	if len(grouped) != 2 {
		t.Errorf("Unexpected number of machine region groups: %d", len(grouped))
	}
	if wests := grouped["west"]; len(wests) != 2 {
		t.Errorf("wrong Amazon machines: %v", wests)
	}
	if easts := grouped["east"]; len(easts) != 1 {
		t.Errorf("wrong Amazon machines: %v", easts)
	}
}

func TestGetIDs(t *testing.T) {
	machines := []Machine{
		{ID: "foo"},
		{ID: "bar"},
	}
	exp := []string{"foo", "bar"}
	actual := getIDs(machines)
	if !reflect.DeepEqual(exp, actual) {
		t.Errorf("Bad ID extraction: expected %v, got %v.", exp, actual)
	}
}

func TestBootRequests(t *testing.T) {
	machines := []Machine{
		{Size: "m4.large", Region: "us-west-1", DiskSize: 32},
		{Size: "m4.large", Region: "us-west-1", DiskSize: 32},
		{Size: "m4.large", Region: "us-west-1"},
		{Size: "m4.large", Region: "us-west-2"},
	}
	cfg := cloudConfigUbuntu([]string{}, "wily")
	exp := map[bootRequest]int64{
		bootRequest{
			cfg:      cfg,
			size:     "m4.large",
			region:   "us-west-1",
			diskSize: 32,
		}: 2,
		bootRequest{
			cfg:    cfg,
			size:   "m4.large",
			region: "us-west-1",
		}: 1,
		bootRequest{
			cfg:    cfg,
			size:   "m4.large",
			region: "us-west-2",
		}: 1,
	}
	actual := bootRequests(machines)
	if !reflect.DeepEqual(exp, actual) {
		t.Errorf("Bad boot request batching: expected %v, got %v.", exp, actual)
	}
}

func TestCloudConfig(t *testing.T) {
	t.Parallel()

	cloudConfigFormat = "(%v) (%v) (%v)"

	res := cloudConfigUbuntu([]string{"a", "b"}, "1")
	exp := "(quilt/quilt:paper) (a\nb) (1)"
	if res != exp {
		t.Errorf("res: %s\nexp: %s", res, exp)
	}
}
