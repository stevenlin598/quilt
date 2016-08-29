package stitch

import (
	"fmt"
	"strings"
)

type invariantType string

const (
	// Reachability (reach): two arguments, <from> <to...>
	reachInvariant = "reach"
	// Neighborship (reach-direct): two arguments, <from> <to>
	neighborInvariant = "reachDirect"
	// Reachability, don't pass through ACL-annotated nodes (reachACL):
	// two arguments, <from> <to...>
	reachACLInvariant = "reachACL"
	// On-pathness (between): three arguments, <from> <to> <between>
	betweenInvariant = "between"
	// Schedulability (enough): zero arguments
	schedulabilityInvariant = "enough"
)

// Annotations.
const (
	aclAnnotation = "ACL"
)

type invariantError struct {
	failer invariant
}

func (invErr invariantError) Error() string {
	return fmt.Sprintf("invariant failed: %s", invErr.failer)
}

type invariant struct {
	form   invariantType
	target bool     // Desired answer to invariant question.
	nodes  []string // Nodes the invariant operates on.
}

func (inv invariant) String() string {
	tags := []string{string(inv.form)}
	tags = append(tags, fmt.Sprintf("%t", inv.target))
	for _, node := range inv.nodes {
		tags = append(tags, fmt.Sprintf("%q", node))
	}
	return strings.Join(tags, " ")
}

var formImpls map[invariantType]func(graph Graph, inv invariant) bool

func init() {
	formImpls = map[invariantType]func(graph Graph, inv invariant) bool{
		reachInvariant:          reachImpl,
		neighborInvariant:       neighborImpl,
		reachACLInvariant:       reachACLImpl,
		betweenInvariant:        betweenImpl,
		schedulabilityInvariant: schedulabilityImpl,
	}
}

func checkInvariants(graph Graph, invs []invariant) error {
	for _, asrt := range invs {
		if val := formImpls[asrt.form](graph, asrt); !val {
			return invariantError{asrt}
		}
	}

	return nil
}

func reachImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	for _, node := range graph.Nodes {
		if node.Label == inv.nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			if pass := contains(from.dfs(),
				to.Name) == inv.target; !pass {
				return false
			}
		}
	}

	return true
}

func neighborImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	for _, node := range graph.Nodes {
		if node.Label == inv.nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			_, ok := from.Connections[to.Name]
			if pass := ok == inv.target; !pass {
				return false
			}
		}
	}

	return true
}

func reachACLImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	for _, node := range graph.Nodes {
		if node.Label == inv.nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			if reachable := contains(from.dfsWithACL(),
				to.Name); reachable != inv.target {
				return false
			}
		}
	}

	return true
}

func betweenImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	var betweenNodes []Node
	for _, node := range graph.Nodes {
		switch node.Label {
		case inv.nodes[0]:
			fromNodes = append(fromNodes, node)
		case inv.nodes[1]:
			toNodes = append(toNodes, node)
		case inv.nodes[2]:
			betweenNodes = append(betweenNodes, node)
		}
	}

	allPassed := true
	for _, from := range fromNodes {
		for _, to := range toNodes {
			allPassed = allPassed && betweenPathsHelper(
				betweenNodes,
				from,
				to,
				inv.target,
			)
		}
	}
	return allPassed
}

func betweenPathsHelper(betweenNodes []Node, from Node, to Node, target bool) bool {
	paths, ok := paths(from, to)
	if !ok {
		// No path between source and dest.
		return !target
	}

	if target { // A betweenNode must be in all paths.
		allPaths := true
	pathsAll:
		for _, path := range paths {
			for _, between := range betweenNodes {
				if ok := contains(path, between.Name); ok {
					break
				} else {
					allPaths = false
					break pathsAll
				}
			}
		}
		return allPaths
	}
	// A betweenNode must not be in any path.
	noPaths := true
pathsAny:
	for _, path := range paths {
		for _, between := range betweenNodes {
			if ok := contains(path, between.Name); ok {
				noPaths = false
				break pathsAny
			}
		}
	}
	return noPaths
}

func schedulabilityImpl(graph Graph, inv invariant) bool {
	machines := graph.Machines
	avSets := graph.Availability
	if _, ok := graph.Nodes["public"]; ok {
		return len(machines) >= (len(avSets) - 1)
	}
	return len(machines) >= len(avSets)
}
