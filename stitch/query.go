package stitch

import (
	"fmt"
	"strings"
)

type queryType int

const (
	// Simulate the failure of an individual container.
	removeOneQuery = iota
	// Simulate the failure of the container's availability set.
	removeSetQuery
)

type query struct {
	form   queryType
	target string // Node to remove.
	str    string // Original query text.
}

type queryError struct {
	failer query
	reason error
}

func (err queryError) Error() string {
	return fmt.Sprintf("mutation %s failed: %s",
		err.failer.str, err.reason.Error())
}

type queryFunc func(graph Graph, invs []invariant, query query) error

var queryFormKeywords map[string]queryType
var queryFormImpls map[queryType]queryFunc

func init() {
	queryFormKeywords = map[string]queryType{
		"removeOne": removeOneQuery,
		"removeSet": removeOneQuery,
	}

	queryFormImpls = map[queryType]queryFunc{
		removeOneQuery: whatIfRemoveOne,
		removeSetQuery: whatIfRemoveSet,
	}
}

func ask(graph Graph, invs []invariant, path string) (query, invariant, error) {
	queries, err := parseQueries(graph, path)
	if err != nil {
		return query{}, invariant{}, err
	}

	return askQueries(graph, invs, queries)
}

func askQueries(graph Graph, invs []invariant, queries []query) (query,
	invariant, error) {

	for _, query := range queries {
		if err := queryFormImpls[query.form](graph, invs, query); err != nil {
			var inv invariant
			if invErr, ok := err.(invariantError); ok {
				inv = invErr.failer
			}
			return query, inv, queryError{
				failer: query,
				reason: err,
			}
		}
	}

	return query{}, invariant{}, nil
}

func parseQueryLine(graph Graph, line string) (query, error) {
	sp := strings.Split(line, " ")
	if _, ok := graph.Nodes[sp[1]]; !ok {
		return query{}, fmt.Errorf("malformed query (unknown label): %s", sp[1])
	}

	if form, ok := queryFormKeywords[sp[0]]; ok {
		return query{form: form, target: sp[1], str: line}, nil
	}
	return query{}, fmt.Errorf("could not parse query: %s", line)
}

func parseQueries(graph Graph, path string) ([]query, error) {
	var queries []query

	parse := func(line string) error {
		query, err := parseQueryLine(graph, line)
		if err != nil {
			return err
		}
		queries = append(queries, query)
		return nil
	}

	if err := forLineInFile(path, parse); err != nil {
		return queries, err
	}

	return queries, nil
}

func whatIfRemoveOne(graph Graph, invs []invariant, query query) error {
	graphCopy := graph.copyGraph()
	graphCopy.removeNode(query.target)

	return checkInvariants(graphCopy, invs)
}

func whatIfRemoveSet(graph Graph, invs []invariant, query query) error {
	graphCopy := graph.copyGraph()
	node := query.target
	avSet := graphCopy.findAvailabilitySet(node)
	if avSet == nil {
		return fmt.Errorf("could not find availability set: %s", node)
	}

	graphCopy.removeAvailabiltySet(avSet)

	return checkInvariants(graphCopy, invs)
}
