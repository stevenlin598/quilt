package stitch

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/afero"

	"github.com/NetSys/quilt/util"
)

func TestMachine(t *testing.T) {
	t.Parallel()

	checkMachines(t, `deployMachines([new Machine({
	role: "Worker",
	provider: "Amazon",
	region: "us-west-2",
	size: "m4.large",
	cpu: new Range(2, 4),
	ram: new Range(4, 8),
	diskSize: 32,
	keys: ["key1", "key2"]})])`,
		[]Machine{
			{
				Role:     "Worker",
				Provider: "Amazon",
				Region:   "us-west-2",
				Size:     "m4.large",
				CPU:      Range{2, 4},
				RAM:      Range{4, 8},
				DiskSize: 32,
				SSHKeys:  []string{"key1", "key2"},
			}})

	checkMachines(t, `var baseMachine = new Machine({provider: "Amazon"});
	deployMachines([baseMachine.withRole("Master")])`,
		[]Machine{
			{
				Role:     "Master",
				Provider: "Amazon",
			}})

	checkMachines(t, `var baseMachine = new Machine({provider: "Amazon"});
	deployMachines(_(2).times(function() {
		return baseMachine.withRole("Master");
	}))`,
		[]Machine{
			{
				Role:     "Master",
				Provider: "Amazon",
			},
			{
				Role:     "Master",
				Provider: "Amazon",
			}})
}

func TestDocker(t *testing.T) {
	t.Parallel()

	checkContainers(t, `new Docker("image",
	{
		args: ["arg1", "arg2"],
		env: {"foo": "bar"}
	})`,
		[]Container{
			{
				ID:      1,
				Image:   "image",
				Command: []string{"arg1", "arg2"},
				Env:     map[string]string{"foo": "bar"},
			},
		})

	checkContainers(t, `new Docker("image",
	{
		args: ["arg1", "arg2"]
	})`,
		[]Container{
			{
				ID:      1,
				Image:   "image",
				Command: []string{"arg1", "arg2"},
				Env:     map[string]string{},
			},
		})

	checkContainers(t, `new Docker("image", {})`,
		[]Container{
			{
				ID:    1,
				Image: "image",
				Env:   map[string]string{},
			},
		})

	checkContainers(t, `var c = new Docker("image", {});
	c.setEnv("foo", "bar")`,
		[]Container{
			{
				ID:    1,
				Image: "image",
				Env:   map[string]string{"foo": "bar"},
			},
		})
}

func TestPlacement(t *testing.T) {
	t.Parallel()

	pre := `var target = new Label("target", []);
	var other = new Label("other", []);`
	checkPlacements(t, pre+`place(target, new LabelRule(true, other))`,
		[]Placement{
			{
				TargetLabel: "target",
				OtherLabel:  "other",
				Exclusive:   true,
			},
		})

	checkPlacements(t, pre+`place(target, new MachineRule(true,
	{size: "m4.large",
	region: "us-west-2",
	provider: "Amazon"}))`,
		[]Placement{
			{
				TargetLabel: "target",
				Exclusive:   true,
				Region:      "us-west-2",
				Provider:    "Amazon",
				Size:        "m4.large",
			},
		})

	checkPlacements(t, pre+`place(target, new MachineRule(true,
	{size: "m4.large",
	provider: "Amazon"}))`,
		[]Placement{
			{
				TargetLabel: "target",
				Exclusive:   true,
				Provider:    "Amazon",
				Size:        "m4.large",
			},
		})
}

func TestLabel(t *testing.T) {
	t.Parallel()

	checkLabels(t, `new Label("web_tier", [new Docker("nginx", {})])`,
		map[string]Label{
			"public": {
				Name:        "public",
				Annotations: []string{},
			},
			"web_tier": {
				Name:        "web_tier",
				IDs:         []int{1},
				Annotations: []string{},
			},
		})

	checkLabels(t, `new Label("web_tier",
		[new Docker("nginx", {}), new Docker("nginx", {})])`,
		map[string]Label{
			"public": {
				Name:        "public",
				Annotations: []string{},
			},
			"web_tier": {
				Name:        "web_tier",
				IDs:         []int{1, 2},
				Annotations: []string{},
			},
		})

	expHostname := "foo.q"
	checkJavascript(t, `var foo = new Label("foo", []);
	return foo.hostname();`, expHostname)

	expChildren := []string{"1.foo.q", "2.foo.q"}
	checkJavascript(t, `
	var foo = new Label("foo",
		[new Docker("bar", {}), new Docker("baz", {})]);
	return foo.children();`, expChildren)
}

func TestConnect(t *testing.T) {
	t.Parallel()

	pre := `
	var foo = new Label("foo", []);
	var bar = new Label("bar", []);`

	checkConnections(t, pre+`connect(new Port(80), foo, bar)`,
		[]Connection{
			{
				From:    "foo",
				To:      "bar",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkConnections(t, pre+`connect(new PortRange(80, 85), foo, bar)`,
		[]Connection{
			{
				From:    "foo",
				To:      "bar",
				MinPort: 80,
				MaxPort: 85,
			},
		})

	checkConnections(t, pre+`connect(new Port(80), foo, publicInternet)`,
		[]Connection{
			{
				From:    "foo",
				To:      "public",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkError(t, pre+`connect(new Port(80), publicInternet, publicInternet);`,
		"cannot connect public internet to itself")
	checkError(t, pre+`connect(new PortRange(80, 81), foo, publicInternet);`,
		"public internet cannot connect on port ranges")
}

func TestRequire(t *testing.T) {
	util.AppFs = afero.NewMemMapFs()

	// Import returning a primitive.
	util.WriteFile("math.spec", []byte(`
	exports.square = function(x) {
		return x*x;
	};`), 0644)
	checkJavascript(t, `math = require('math');
	return math.square(5)`, float64(25))

	// Import returning a type.
	util.WriteFile("testImport.spec", []byte(`
	exports.getLabel = function() {
		return new Label("foo", []);
	};`), 0644)
	checkJavascript(t, `testImport = require('testImport');
	return testImport.getLabel().hostname()`, "foo.q")

	// Import with an import
	util.WriteFile("square.spec", []byte(`
	exports.square = function(x) {
		return x*x;
	};`), 0644)
	util.WriteFile("cube.spec", []byte(`
	var square = require("square");
	exports.cube = function(x) {
		return x * square.square(x);
	};`), 0644)
	checkJavascript(t, `cube = require('cube');
	return cube.cube(5)`, float64(125))

	// Directly assigned exports
	util.WriteFile("square.spec", []byte("module.exports = function(x) {"+
		"return x*x }"), 0644)
	checkJavascript(t, `var square = require('square');
	return square(5)`, float64(25))

	testSpec := `var square = require('square');
	square(5)`
	util.WriteFile("test.spec", []byte(testSpec), 0644)
	compiled, err := Compile("test.spec", ImportGetter{
		Path: ".",
	})
	if err != nil {
		t.Errorf(`Unexpected error: "%s".`, err.Error())
	}
	expCompiled := `importSources = {"square": "module.exports = ` +
		`function(x) {return x*x }",};` + testSpec
	if compiled != expCompiled {
		t.Errorf(`Bad compilation: expected "%s", got "%s".`,
			expCompiled, compiled)
	}
}

func checkJavascript(t *testing.T, code string, exp interface{}) {
	resultKey := "result"

	handle, err := New(fmt.Sprintf(`var %s = function() {
		%s
	}()`, resultKey, code), ImportGetter{
		Path: ".",
	})
	if err != nil {
		t.Errorf(`Unexpected error: "%s".`, err.Error())
		return
	}

	actualVal, err := handle.vm.Get(resultKey)
	if err != nil {
		t.Errorf(`Unexpected error retrieving result from VM: "%s".`,
			err.Error())
		return
	}

	actual, _ := actualVal.Export()
	if !reflect.DeepEqual(actual, exp) {
		t.Errorf("Bad javascript code: Expected %s, got %s.",
			spew.Sdump(exp), spew.Sdump(actual))
	}
}

func checkError(t *testing.T, code string, exp string) {
	_, err := New(code, DefaultImportGetter)
	if err == nil {
		t.Errorf(`Expected error "%s", but got nothing.`, exp)
		return
	}
	if actual := err.Error(); actual != exp {
		t.Errorf(`Expected error "%s", but got "%s".`, exp, actual)
	}
}

func queryChecker(
	queryFunc func(Stitch) interface{}) func(*testing.T, string, interface{}) {

	return func(t *testing.T, code string, exp interface{}) {
		handle, err := New(code, DefaultImportGetter)
		if err != nil {
			t.Errorf(`Unexpected error: "%s".`, err.Error())
			return
		}

		actual := queryFunc(handle)
		if !reflect.DeepEqual(actual, exp) {
			t.Errorf("Bad query: Expected %s, got %s.",
				spew.Sdump(exp), spew.Sdump(actual))
		}
	}
}

var checkMachines = queryChecker(func(s Stitch) interface{} {
	return s.QueryMachines()
})

var checkContainers = queryChecker(func(s Stitch) interface{} {
	return s.QueryContainers()
})

var checkPlacements = queryChecker(func(s Stitch) interface{} {
	return s.QueryPlacements()
})

var checkLabels = queryChecker(func(s Stitch) interface{} {
	// Convert the slice to a map because the ordering is non-deterministic.
	labelsMap := make(map[string]Label)
	for _, label := range s.QueryLabels() {
		labelsMap[label.Name] = label
	}
	return labelsMap
})

var checkConnections = queryChecker(func(s Stitch) interface{} {
	return s.QueryConnections()
})
