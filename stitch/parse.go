package stitch

import (
	"errors"
	"fmt"

	"github.com/robertkrimen/otto"
)

const (
	// Machine fields.
	machineProviderKey = "provider"
	machineRoleKey     = "role"
	machineSizeKey     = "size"
	machineCPUKey      = "cpu"
	machineRAMKey      = "ram"
	machineDiskSizeKey = "diskSize"
	machineRegionKey   = "region"
	machineSSHKeysKey  = "keys"

	// Connection fields.
	connectionRangeKey = "ports"
	connectionFromKey  = "from"
	connectionToKey    = "to"

	// Range fields.
	rangeMinKey = "min"
	rangeMaxKey = "max"

	// Placement fields.
	placementTargetKey = "target"
	placementRuleKey   = "rule"

	// Placement rule fields.
	placementExclusiveKey = "exclusive"
	otherLabelKey         = "otherLabel"

	// Label fields.
	labelNameKey        = "name"
	labelContainersKey  = "containers"
	labelAnnotationsKey = "annotations"

	// Container fields.
	containerImageKey = "image"
	containerArgsKey  = "args"
	containerEnvKey   = "env"
	containerIDKey    = "id"

	// Invariant fields.
	invariantTypeKey    = "type"
	invariantFromKey    = "from"
	invariantBetweenKey = "between"
	invariantToKey      = "to"
	invariantKey        = "invariant"
	invariantDesiredKey = "desired"
)

func parseRange(v otto.Value) (rng Range, err error) {
	err = forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case rangeMinKey:
			rng.Min, err = val.ToFloat()
		case rangeMaxKey:
			rng.Max, err = val.ToFloat()
		}
		return err
	})
	return rng, err
}

func parseContainer(v otto.Value) (c Container, err error) {
	err = forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case containerIDKey:
			c.ID, err = parseInt(val)
		case containerImageKey:
			c.Image, err = val.ToString()
		case containerArgsKey:
			c.Command, err = parseStringSlice(val)
		case containerEnvKey:
			c.Env, err = parseContainerEnv(val)
		}
		return err
	})
	return c, err
}

func parseContainerEnv(v otto.Value) (map[string]string, error) {
	env := make(map[string]string)
	err := forField(v, func(key string, val otto.Value) (err error) {
		env[key], err = val.ToString()
		return err
	})
	return env, err
}

func parseMachine(v otto.Value) (m Machine, err error) {
	err = forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case machineProviderKey:
			m.Provider, err = val.ToString()
		case machineRegionKey:
			m.Region, err = val.ToString()
		case machineSizeKey:
			m.Size, err = val.ToString()
		case machineDiskSizeKey:
			m.DiskSize, err = parseInt(val)
		case machineRoleKey:
			m.Role, err = val.ToString()
		case machineRAMKey:
			m.RAM, err = parseRange(val)
		case machineCPUKey:
			m.CPU, err = parseRange(val)
		case machineSSHKeysKey:
			m.SSHKeys, err = parseStringSlice(val)
		}
		return err
	})
	return m, err
}

func parsePlacement(v otto.Value) (Placement, error) {
	var p Placement
	var target string
	err := forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case placementRuleKey:
			p, err = parsePlacementRule(val)
		case placementTargetKey:
			target, err = parseLabelName(val)
		}
		return err
	})
	return Placement{
		TargetLabel: target,
		Exclusive:   p.Exclusive,
		OtherLabel:  p.OtherLabel,
		Provider:    p.Provider,
		Size:        p.Size,
		Region:      p.Region,
	}, err
}

func parsePlacementRule(v otto.Value) (p Placement, err error) {
	err = forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case placementExclusiveKey:
			p.Exclusive, err = val.ToBoolean()
		case machineProviderKey:
			p.Provider, err = val.ToString()
		case machineRegionKey:
			p.Region, err = val.ToString()
		case machineSizeKey:
			p.Size, err = val.ToString()
		case otherLabelKey:
			p.OtherLabel, err = parseLabelName(val)
		}
		return err
	})
	return p, err
}

func parseInvariant(v otto.Value) (invariant, error) {
	var form, from, to, between string
	err := forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case invariantFromKey:
			from, err = parseLabelName(val)
		case invariantBetweenKey:
			between, err = parseLabelName(val)
		case invariantToKey:
			to, err = parseLabelName(val)
		case invariantTypeKey:
			form, err = val.ToString()
		}
		return err
	})

	inv := invariant{form: invariantType(form)}
	switch inv.form {
	case reachInvariant, neighborInvariant, reachACLInvariant:
		inv.nodes = []string{from, to}
	case betweenInvariant:
		inv.nodes = []string{from, between, to}
	}
	return inv, err
}

func parseAssertion(v otto.Value) (invariant, error) {
	var inv invariant
	var desired bool
	err := forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case invariantKey:
			inv, err = parseInvariant(val)
		case invariantDesiredKey:
			desired, err = val.ToBoolean()
		}
		return err
	})
	return invariant{
		target: desired,
		form:   inv.form,
		nodes:  inv.nodes,
	}, err
}

func parseConnection(v otto.Value) (c Connection, err error) {
	err = forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case connectionRangeKey:
			var rng Range
			rng, err = parseRange(val)
			c.MinPort = int(rng.Min)
			c.MaxPort = int(rng.Max)
		case connectionFromKey:
			c.From, err = parseLabelName(val)
		case connectionToKey:
			c.To, err = parseLabelName(val)
		}
		return err
	})

	return c, err
}

func parseLabel(v otto.Value) (l Label, err error) {
	err = forField(v, func(key string, val otto.Value) (err error) {
		switch key {
		case labelNameKey:
			l.Name, err = val.ToString()
		case labelContainersKey:
			err = parseArray(val, parseContainerGeneric,
				func(containerIntf interface{}) {
					l.IDs = append(l.IDs,
						containerIntf.(Container).ID)
				})
		case labelAnnotationsKey:
			l.Annotations, err = parseStringSlice(val)
		}
		return err
	})
	return l, err
}

func parseLabelName(v otto.Value) (string, error) {
	label, err := parseLabel(v)
	if err != nil {
		return "", err
	}
	return label.Name, nil
}

func isEmptySlice(intf interface{}) bool {
	slice, ok := intf.([]interface{})
	return ok && len(slice) == 0
}

func forElem(v otto.Value, fn func(otto.Value) error) error {
	obj := v.Object()
	if obj.Class() != "Array" {
		return errors.New("not an array")
	}

	lengthValue, _ := obj.Get("length")
	length, _ := lengthValue.ToInteger()
	for index := int64(0); index < length; index++ {
		value, _ := obj.Get(fmt.Sprintf("%d", index))
		if err := fn(value); err != nil {
			return err
		}
	}

	return nil
}

type ottoParser func(otto.Value) (interface{}, error)

func parseArray(ottoArray otto.Value, parse ottoParser, fn func(interface{})) error {
	return forElem(ottoArray, func(elem otto.Value) error {
		parsed, err := parse(elem)
		if err != nil {
			return err
		}
		fn(parsed)
		return nil
	})
}

func forField(v otto.Value, fn func(string, otto.Value) error) error {
	obj := v.Object()
	if obj == nil {
		return nil
	}

	for _, k := range obj.Keys() {
		v, _ := obj.Get(k)
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}

func parseInt(v otto.Value) (int, error) {
	parsed, err := v.ToInteger()
	return int(parsed), err
}

func parseStringSlice(v otto.Value) ([]string, error) {
	sliceIntf, _ := v.Export()
	if isEmptySlice(sliceIntf) {
		return []string{}, nil
	}
	slice, ok := sliceIntf.([]string)
	if !ok {
		return []string{}, errors.New("not string slice")
	}
	return slice, nil
}

func parseContext(vm *otto.Otto) (evalCtx, error) {
	ctx := evalCtx{
		make(map[int]Container),
		make(map[string]Label),
		make(map[Connection]struct{}),
		make(map[Placement]struct{}),
		[]Machine{},
		[]invariant{},
	}

	vmCtx, err := vm.Get("ctx")
	if err != nil {
		return ctx, err
	}

	err = forField(vmCtx, func(key string, val otto.Value) error {
		var err error
		switch key {
		case "invariants":
			err = parseArray(val, parseAssertionGeneric,
				func(inv interface{}) {
					ctx.invariants = append(ctx.invariants,
						inv.(invariant))
				})
		case "connections":
			err = parseArray(val, parseConnectionGeneric,
				func(c interface{}) {
					ctx.connections[c.(Connection)] = struct{}{}
				})
		case "machines":
			err = parseArray(val, parseMachineGeneric,
				func(m interface{}) {
					ctx.machines = append(ctx.machines, m.(Machine))
				})
		case "labels":
			err = parseArray(val, parseLabelGeneric,
				func(l interface{}) {
					label := l.(Label)
					ctx.labels[label.Name] = label
				})
		case "containers":
			err = parseArray(val, parseContainerGeneric,
				func(c interface{}) {
					container := c.(Container)
					ctx.containers[container.ID] = container
				})
		case "placements":
			err = parseArray(val, parsePlacementGeneric,
				func(p interface{}) {
					ctx.placements[p.(Placement)] = struct{}{}
				})
		}
		return err
	})

	return ctx, err
}

func parseAssertionGeneric(v otto.Value) (interface{}, error) {
	return parseAssertion(v)
}

func parseConnectionGeneric(v otto.Value) (interface{}, error) {
	return parseConnection(v)
}

func parseMachineGeneric(v otto.Value) (interface{}, error) {
	return parseMachine(v)
}

func parseLabelGeneric(v otto.Value) (interface{}, error) {
	return parseLabel(v)
}

func parseContainerGeneric(v otto.Value) (interface{}, error) {
	return parseContainer(v)
}

func parsePlacementGeneric(v otto.Value) (interface{}, error) {
	return parsePlacement(v)
}
