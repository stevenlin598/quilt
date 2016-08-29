package minion

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/stitch"
	"github.com/NetSys/quilt/util"
	"github.com/davecgh/go-spew/spew"
)

const testImage = "alpine"

func TestContainerTxn(t *testing.T) {
	conn := db.New()
	trigg := conn.Trigger(db.ContainerTable).C

	spec := ""
	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if fired(trigg) {
		t.Error("Unexpected Database Change")
	}

	spec = `new Label("a", [new Docker("alpine", {args: ["tail"]})]);`
	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}

	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if fired(trigg) {
		t.Error("Unexpected Database Change")
	}

	spec = `var b = new Docker("alpine", {args: ["tail"]});
	new Label("b", [b]);
	new Label("a", [b, new Docker("alpine", {args: ["tail"]})]);`
	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}

	spec = `var b = new Label("b", [new Docker("alpine", {args: ["cat"]})]);
	new Label("a", b.containers.concat([new Docker("alpine", {args: ["tail"]})]));`
	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}

	spec = `var b = new Label("b", [new Docker("ubuntu", {args: ["cat"]})]);
	new Label("a", b.containers.concat([new Docker("alpine", {args: ["tail"]})]));`
	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}

	spec = `new Label("a", [new Docker("alpine", {args: ["cat"]}),
	new Docker("alpine", {args: ["cat"]})]);`
	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}

	spec = `new Label("a", [new Docker("alpine", {})]);`
	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}

	spec = `var b = new Label("b", [new Docker("alpine", {})]);
	var c = new Label("c", [new Docker("alpine", {})]);
	new Label("a", b.containers.concat(c.containers));`
	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}

	if err := testContainerTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if fired(trigg) {
		t.Error("Unexpected Database Change")
	}
}

func testContainerTxn(conn db.Conn, spec string) string {
	var containers []db.Container
	conn.Transact(func(view db.Database) error {
		updatePolicy(view, db.Master, spec)
		containers = view.SelectFromContainer(nil)
		return nil
	})

	compiled, err := stitch.New(spec, stitch.DefaultImportGetter)
	if err != nil {
		return err.Error()
	}

	for _, e := range queryContainers(compiled) {
		found := false
		for i, c := range containers {
			if e.Image == c.Image &&
				reflect.DeepEqual(e.Command, c.Command) &&
				util.EditDistance(c.Labels, e.Labels) == 0 {
				containers = append(containers[:i], containers[i+1:]...)
				found = true
				break
			}
		}

		if found == false {
			return fmt.Sprintf("Missing expected label set: %v\n%v",
				e, containers)
		}
	}

	if len(containers) > 0 {
		return spew.Sprintf("Unexpected containers: %s", containers)
	}

	return ""
}

func TestConnectionTxn(t *testing.T) {
	conn := db.New()
	trigg := conn.Trigger(db.ConnectionTable).C

	spec := ""
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if fired(trigg) {
		t.Error("Unexpected Database Change")
	}

	spec = `var a = new Label("a", [new Docker("alpine", {})]);
	connect(new Port(80), a, a)`
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if fired(trigg) {
		t.Error("Unexpected Database Change")
	}

	spec = `var a = new Label("a", [new Docker("alpine", {})]);
	connect(new Port(90), a, a)`
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if fired(trigg) {
		t.Error("Unexpected Database Change")
	}

	spec = `var a = new Label("a", [new Docker("alpine", {})]);
	var b = new Label("b", [new Docker("alpine", {})]);
	var c = new Label("c", [new Docker("alpine", {})]);
	connect(new Port(90), b, a)
	connect(new Port(90), b, c)
	connect(new Port(100), b, b)
	connect(new Port(101), c, a)`
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if fired(trigg) {
		t.Error("Unexpected Database Change")
	}

	spec = `var a = new Label("a", [new Docker("alpine", {})]);
	var b = new Label("b", [new Docker("alpine", {})]);
	var c = new Label("c", [new Docker("alpine", {})]);`
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if !fired(trigg) {
		t.Error("Expected Database Change")
	}
	if err := testConnectionTxn(conn, spec); err != "" {
		t.Error(err)
	}
	if fired(trigg) {
		t.Error("Unexpected Database Change")
	}
}

func testConnectionTxn(conn db.Conn, spec string) string {
	var connections []db.Connection
	conn.Transact(func(view db.Database) error {
		updatePolicy(view, db.Master, spec)
		connections = view.SelectFromConnection(nil)
		return nil
	})

	compiled, err := stitch.New(spec, stitch.DefaultImportGetter)
	if err != nil {
		return err.Error()
	}

	exp := compiled.QueryConnections()
	for _, e := range exp {
		found := false
		for i, c := range connections {
			if e.From == c.From && e.To == c.To && e.MinPort == c.MinPort &&
				e.MaxPort == c.MaxPort {
				connections = append(
					connections[:i], connections[i+1:]...)
				found = true
				break
			}
		}

		if found == false {
			return fmt.Sprintf("Missing expected connection: %v", e)
		}
	}

	if len(connections) > 0 {
		return spew.Sprintf("Unexpected connections: %s", connections)
	}

	return ""
}

func fired(c chan struct{}) bool {
	time.Sleep(5 * time.Millisecond)
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func TestPlacementTxn(t *testing.T) {
	conn := db.New()
	checkPlacement := func(spec string, exp ...db.Placement) {
		placements := map[db.Placement]struct{}{}
		conn.Transact(func(view db.Database) error {
			updatePolicy(view, db.Master, spec)
			res := view.SelectFromPlacement(nil)

			// Set the ID to 0 so that we can use reflect.DeepEqual.
			for _, p := range res {
				p.ID = 0
				placements[p] = struct{}{}
			}

			return nil
		})

		if len(placements) != len(exp) {
			t.Errorf("Placement error in %s. Expected %v, got %v",
				spec, exp, placements)
		}

		for _, p := range exp {
			if _, ok := placements[p]; !ok {
				t.Errorf("Placement error in %s. Expected %v, got %v",
					spec, exp, placements)
				break
			}
		}
	}

	// Create an exclusive placement.
	spec := `var foo = new Label("foo", [new Docker("foo", {})]);
	var bar = new Label("bar", [new Docker("bar", {})]);
	place(bar, new LabelRule(true, foo));`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "foo",
		},
	)

	// Change the placement from "exclusive" to "on".
	spec = `var foo = new Label("foo", [new Docker("foo", {})]);
	var bar = new Label("bar", [new Docker("bar", {})]);
	place(bar, new LabelRule(false, foo));`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "bar",
			Exclusive:   false,
			OtherLabel:  "foo",
		},
	)

	// Add another placement constraint.
	spec = `var foo = new Label("foo", [new Docker("foo", {})]);
	var bar = new Label("bar", [new Docker("bar", {})]);
	place(bar, new LabelRule(false, foo));
	place(bar, new LabelRule(true, bar));`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "bar",
			Exclusive:   false,
			OtherLabel:  "foo",
		},
		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "bar",
		},
	)

	// Machine placement
	spec = `var foo = new Label("foo", [new Docker("foo", {})]);
	place(foo, new MachineRule(false, {size: "m4.large"}));`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "foo",
			Exclusive:   false,
			Size:        "m4.large",
		},
	)

	// Port placement
	spec = `var foo = new Label("foo", [new Docker("foo", {})]);
	connect(new Port(80), publicInternet, foo);
	connect(new Port(81), publicInternet, foo);`
	checkPlacement(spec,
		db.Placement{
			TargetLabel: "foo",
			Exclusive:   true,
			OtherLabel:  "foo",
		},
	)

	spec = `var foo = new Label("foo", [new Docker("foo", {})]);
	var bar = new Label("bar", [new Docker("bar", {})]);
	var baz = new Label("baz", [new Docker("baz", {})]);
	connect(new Port(80), publicInternet, foo);
	connect(new Port(80), publicInternet, bar);
	(function() {
		connect(new Port(81), publicInternet, bar)
		connect(new Port(81), publicInternet, baz)
	})()`

	checkPlacement(spec,
		db.Placement{
			TargetLabel: "foo",
			Exclusive:   true,
			OtherLabel:  "foo",
		},

		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "bar",
		},

		db.Placement{
			TargetLabel: "foo",
			Exclusive:   true,
			OtherLabel:  "bar",
		},

		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "foo",
		},

		db.Placement{
			TargetLabel: "baz",
			Exclusive:   true,
			OtherLabel:  "baz",
		},

		db.Placement{
			TargetLabel: "bar",
			Exclusive:   true,
			OtherLabel:  "baz",
		},

		db.Placement{
			TargetLabel: "baz",
			Exclusive:   true,
			OtherLabel:  "bar",
		},
	)
}
