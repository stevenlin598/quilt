package stitch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ditashi/jsbeautifier-go/jsbeautifier"
)

var machineConstructor = objectConstructor{
	optionalArgs: []string{machineProviderKey, machineRoleKey,
		machineSizeKey, machineCPUKey, machineRAMKey,
		machineDiskSizeKey, machineRegionKey, machineSSHKeysKey},
}

var objects = []object{
	{
		name:        "Machine",
		constructor: machineConstructor,
		methods: []method{
			{
				name: "clone",
				body: machineConstructor.cloneBody(),
			},
			{
				name: "withRole",
				args: []string{"role"},
				body: fmt.Sprintf(`var copy = this.clone();
				copy.%s = role;
				return copy;`, machineRoleKey),
			},
		},
	},
	{
		name: "MachineRule",
		constructor: objectConstructor{
			requiredArgs: []string{placementExclusiveKey},
			optionalArgs: []string{machineProviderKey, machineSizeKey,
				machineRegionKey},
		},
	},
	{
		name: "Label",
		constructor: objectConstructor{
			requiredArgs: []string{labelNameKey, labelContainersKey},
			defaultValues: map[string]string{
				labelAnnotationsKey: "[]",
			},
			body: "ctx.labels.push(this);",
		},
		methods: []method{
			{
				name: "hostname",
				body: fmt.Sprintf(`return this.%s + ".q";`, labelNameKey),
			},
			{
				name: "children",
				body: fmt.Sprintf(`
				var _containers = this.%s;
				var _name = this.%s;
				var res = [];
				for (var i = 1 ; i < _containers.length + 1 ; i++) {
					res.push(i + "." + _name + ".q");
				}
				return res;`, labelContainersKey, labelNameKey),
			},
			{
				name: "annotate",
				args: []string{"annotation"},
				body: fmt.Sprintf(`
				this.%s.push(annotation);
				return this;`, labelAnnotationsKey),
			},
		},
	},
	{
		name: "LabelRule",
		constructor: objectConstructor{
			requiredArgs: []string{placementExclusiveKey, otherLabelKey},
		},
	},
	{
		name: "Docker",
		constructor: objectConstructor{
			requiredArgs: []string{containerImageKey},
			optionalArgs: []string{containerArgsKey, containerEnvKey},
			defaultValues: map[string]string{
				containerEnvKey: "{}",
				containerIDKey:  "++" + idCounterKey,
			},
			body: "ctx.containers.push(this)",
		},
		methods: []method{
			{
				// XXX: Have users directly manipulate docker.env?
				name: "setEnv",
				args: []string{"key", "val"},
				body: fmt.Sprintf(`
				this.%s[key] = val;
				return this;`, containerEnvKey),
			},
		},
	},
	{
		name: "Placement",
		constructor: objectConstructor{
			requiredArgs: []string{placementTargetKey, placementRuleKey},
		},
	},
	{
		name: "Connection",
		constructor: objectConstructor{
			requiredArgs: []string{connectionRangeKey, connectionFromKey,
				connectionToKey},
		},
	},
	{
		name: "Range",
		constructor: objectConstructor{
			requiredArgs: []string{rangeMinKey, rangeMaxKey},
		},
	},
	{
		name: "Reachable",
		constructor: objectConstructor{
			requiredArgs: []string{invariantFromKey, invariantToKey},
			defaultValues: map[string]string{
				invariantTypeKey: strconv.Quote(reachInvariant),
			},
		},
	},
	{
		name: "ACLReachable",
		constructor: objectConstructor{
			requiredArgs: []string{invariantFromKey, invariantToKey},
			defaultValues: map[string]string{
				invariantTypeKey: strconv.Quote(reachACLInvariant),
			},
		},
	},
	{
		name: "Neighborship",
		constructor: objectConstructor{
			requiredArgs: []string{invariantFromKey, invariantToKey},
			defaultValues: map[string]string{
				invariantTypeKey: strconv.Quote(neighborInvariant),
			},
		},
	},
	{
		name: "Enough",
		constructor: objectConstructor{
			defaultValues: map[string]string{
				invariantTypeKey: strconv.Quote(schedulabilityInvariant),
			},
		},
	},
	{
		name: "Between",
		constructor: objectConstructor{
			requiredArgs: []string{invariantFromKey, invariantBetweenKey,
				invariantToKey},
			defaultValues: map[string]string{
				invariantTypeKey: strconv.Quote(betweenInvariant),
			},
		},
	},
	{
		name: "Assertion",
		constructor: objectConstructor{
			requiredArgs: []string{invariantKey, invariantDesiredKey},
		},
	},
}

var otherFunctions = idCounterKey + " = 0;" +
	fmt.Sprintf(`var publicInternet = new Label("%s", []);`, PublicInternetLabel) +
	`function Port(p) {
    return new Range(p, p);
}

function assert(rule, desired) {
	ctx.invariants.push(new Assertion(rule, desired));
}

function connect(range, from, to) {
	var fromPublic = from.name == publicInternet.name;
	var toPublic = to.name == publicInternet.name;
	if (fromPublic && toPublic) {
		throw "cannot connect public internet to itself";
	}
	if ((fromPublic || toPublic) && (range.min != range.max)) {
		throw "public internet cannot connect on port ranges";
	}
	ctx.connections.push(new Connection(range, from, to));
}

function deployMachines(machines) {
	ctx.machines = ctx.machines.concat(machines);
}

function deployMasters(n, machine) {
	deployMachines(_(n).times(function() {
		return machine.withRole("Master");
	}));
}

function deployWorkers(n, machine) {
	deployMachines(_(n).times(function() {
		return machine.withRole("Worker");
	}));
}

function place(tgt, rule) {
	ctx.placements.push(new Placement(tgt, rule));
}

PortRange = Range;`

type object struct {
	name        string
	constructor objectConstructor
	methods     []method
}

type objectConstructor struct {
	requiredArgs  []string
	optionalArgs  []string
	defaultValues map[string]string
	body          string
}

type method struct {
	name string
	args []string
	body string
}

const (
	idCounterKey          = "containerIDCounter"
	optionalParameterName = "optionalArgs"
)

func (c objectConstructor) constructorSignature(name string) string {
	argNames := c.requiredArgs
	if len(c.optionalArgs) != 0 {
		argNames = append(argNames, optionalParameterName)
	}
	return fmt.Sprintf("function %s(%s)", name, strings.Join(argNames, ", "))
}

func (c objectConstructor) constructorBody() string {
	var body string
	for fieldName, defaultVal := range c.defaultValues {
		body += fmt.Sprintf("this.%s = %s;", fieldName, defaultVal)
	}
	for _, requiredArg := range c.requiredArgs {
		body += fmt.Sprintf("this.%[1]s = %[1]s;", requiredArg)
	}
	for _, optionalArg := range c.optionalArgs {
		body += fmt.Sprintf(`if (%[1]s.%[2]s) {
			this.%[2]s = %[1]s.%[2]s;
		}`, optionalParameterName, optionalArg)
	}
	return body + c.body
}

func (c objectConstructor) code(name string) string {
	return fmt.Sprintf(`%s {
		%s
	}`, c.constructorSignature(name), c.constructorBody())
}

func (c objectConstructor) cloneBody() string {
	body := "var cloned = {};\n"
	for _, arg := range append(c.requiredArgs, c.optionalArgs...) {
		body += fmt.Sprintf(`if (this.%[1]s) {
			cloned.%[1]s = this.%[1]s;
		}`, arg)
	}
	return body + "return cloned;"
}

func (o object) code() string {
	code := o.constructor.code(o.name)
	for _, m := range o.methods {
		signature := fmt.Sprintf("function(%s)", strings.Join(m.args, ","))
		code += fmt.Sprintf(`%s.prototype.%s = %s {
			%s
		};`, o.name, m.name, signature, m.body)
	}
	return code
}

// JavascriptLibrary generates the Stitch Javascript bindings and
// returns them as a string.
func JavascriptLibrary() string {
	code := `ctx = {
		invariants: [],
		connections: [],
		machines: [],
		labels: [],
		containers: [],
		placements: [],
	};`
	for _, o := range objects {
		code += o.code() + "\n"
	}
	code += otherFunctions

	pretty, _ := jsbeautifier.Beautify(&code, jsbeautifier.DefaultOptions())
	return pretty
}
