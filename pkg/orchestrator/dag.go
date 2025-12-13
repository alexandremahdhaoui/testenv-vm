// Copyright 2025 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package orchestrator provides resource orchestration and execution.
package orchestrator

import (
	"fmt"
	"sync"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
)

// DAG represents a directed acyclic graph of resource dependencies.
// It is used to determine the correct order of resource creation.
type DAG struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	edges map[string][]string // from -> to dependencies (from depends on to)
}

// Node represents a resource in the dependency graph.
type Node struct {
	// Ref is the resource reference (kind, name, provider).
	Ref v1.ResourceRef
	// Dependencies are resources this node depends on (must be created first).
	Dependencies []v1.ResourceRef
	// Dependents are resources that depend on this node.
	Dependents []v1.ResourceRef
}

// nodeKey generates a consistent key for a resource reference.
// Format: {kind}:{name}
func nodeKey(ref v1.ResourceRef) string {
	return ref.Kind + ":" + ref.Name
}

// NewDAG creates a new empty DAG.
func NewDAG() *DAG {
	return &DAG{
		nodes: make(map[string]*Node),
		edges: make(map[string][]string),
	}
}

// BuildDAG constructs a dependency graph from a TestenvSpec.
// It scans all resources for template references and builds edges accordingly.
func BuildDAG(testenvSpec *v1.TestenvSpec) (*DAG, error) {
	dag := NewDAG()

	// Add image nodes first (Phase 0 - no dependencies)
	// Images are downloaded before any other resources.
	for _, image := range testenvSpec.Images {
		ref := v1.ResourceRef{
			Kind: "image",
			Name: image.Name,
		}
		dag.AddNode(ref)
	}

	// Add all resources as nodes
	for _, key := range testenvSpec.Keys {
		ref := v1.ResourceRef{
			Kind:     "key",
			Name:     key.Name,
			Provider: key.Provider,
		}
		dag.AddNode(ref)
	}

	for _, network := range testenvSpec.Networks {
		ref := v1.ResourceRef{
			Kind:     "network",
			Name:     network.Name,
			Provider: network.Provider,
		}
		dag.AddNode(ref)
	}

	for _, vm := range testenvSpec.VMs {
		ref := v1.ResourceRef{
			Kind:     "vm",
			Name:     vm.Name,
			Provider: vm.Provider,
		}
		dag.AddNode(ref)
	}

	// Scan resources for template dependencies and build edges
	// Keys typically have no dependencies
	for _, key := range testenvSpec.Keys {
		fromRef := v1.ResourceRef{Kind: "key", Name: key.Name, Provider: key.Provider}
		deps := spec.ExtractTemplateRefs(key)
		for _, dep := range deps {
			if err := dag.AddEdge(fromRef, dep); err != nil {
				return nil, fmt.Errorf("failed to add edge from key %q: %w", key.Name, err)
			}
		}
	}

	// Networks may depend on other networks (attachTo) or keys
	for _, network := range testenvSpec.Networks {
		fromRef := v1.ResourceRef{Kind: "network", Name: network.Name, Provider: network.Provider}
		deps := spec.ExtractTemplateRefs(network)
		for _, dep := range deps {
			if err := dag.AddEdge(fromRef, dep); err != nil {
				return nil, fmt.Errorf("failed to add edge from network %q: %w", network.Name, err)
			}
		}

		// Check for attachTo field (explicit network dependency)
		if network.Spec.AttachTo != "" {
			// Check if attachTo contains a template
			attachToRefs := spec.ExtractTemplateRefs(struct {
				AttachTo string
			}{AttachTo: network.Spec.AttachTo})

			if len(attachToRefs) > 0 {
				// Template reference already extracted above
				continue
			}
			// attachTo is a literal network name
			attachToRef := v1.ResourceRef{Kind: "network", Name: network.Spec.AttachTo}
			if err := dag.AddEdge(fromRef, attachToRef); err != nil {
				return nil, fmt.Errorf("failed to add attachTo edge from network %q: %w", network.Name, err)
			}
		}
	}

	// VMs may depend on networks, keys, and other VMs
	for _, vm := range testenvSpec.VMs {
		fromRef := v1.ResourceRef{Kind: "vm", Name: vm.Name, Provider: vm.Provider}
		deps := spec.ExtractTemplateRefs(vm)
		for _, dep := range deps {
			if err := dag.AddEdge(fromRef, dep); err != nil {
				return nil, fmt.Errorf("failed to add edge from vm %q: %w", vm.Name, err)
			}
		}

		// Check for network field (explicit network dependency)
		if vm.Spec.Network != "" {
			// Check if network contains a template
			networkRefs := spec.ExtractTemplateRefs(struct {
				Network string
			}{Network: vm.Spec.Network})

			if len(networkRefs) > 0 {
				// Template reference already extracted above
				continue
			}
			// network is a literal network name
			networkRef := v1.ResourceRef{Kind: "network", Name: vm.Spec.Network}
			if err := dag.AddEdge(fromRef, networkRef); err != nil {
				return nil, fmt.Errorf("failed to add network edge from vm %q: %w", vm.Name, err)
			}
		}
	}

	// Check for cycles
	if dag.HasCycle() {
		return nil, fmt.Errorf("circular dependency detected in resource graph")
	}

	return dag, nil
}

// AddNode adds a resource node to the graph.
// If the node already exists, this is a no-op.
func (d *DAG) AddNode(ref v1.ResourceRef) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := nodeKey(ref)
	if _, exists := d.nodes[key]; exists {
		return
	}

	d.nodes[key] = &Node{
		Ref:          ref,
		Dependencies: []v1.ResourceRef{},
		Dependents:   []v1.ResourceRef{},
	}
}

// AddEdge adds a dependency edge from -> to, meaning "from" depends on "to".
// The "to" resource must be created before "from".
// Returns an error if either node does not exist.
func (d *DAG) AddEdge(from, to v1.ResourceRef) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	fromKey := nodeKey(from)
	toKey := nodeKey(to)

	fromNode, fromExists := d.nodes[fromKey]
	toNode, toExists := d.nodes[toKey]

	if !fromExists {
		return fmt.Errorf("source node %q does not exist", fromKey)
	}
	if !toExists {
		return fmt.Errorf("target node %q does not exist", toKey)
	}

	// Check if edge already exists
	for _, existing := range d.edges[fromKey] {
		if existing == toKey {
			return nil // Edge already exists
		}
	}

	// Add the edge
	d.edges[fromKey] = append(d.edges[fromKey], toKey)

	// Update node relationships
	fromNode.Dependencies = append(fromNode.Dependencies, to)
	toNode.Dependents = append(toNode.Dependents, from)

	return nil
}

// TopologicalSort returns resources grouped into phases for parallel execution.
// Resources in the same phase have no dependencies on each other and can run in parallel.
// Uses Kahn's algorithm.
func (d *DAG) TopologicalSort() ([][]v1.ResourceRef, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.nodes) == 0 {
		return nil, nil
	}

	// Calculate in-degree for each node (number of dependencies)
	inDegree := make(map[string]int)
	for key := range d.nodes {
		inDegree[key] = len(d.edges[key])
	}

	// Find all nodes with in-degree 0 (no dependencies)
	var phases [][]v1.ResourceRef
	remaining := len(d.nodes)

	for remaining > 0 {
		// Collect all nodes with in-degree 0
		var currentPhase []v1.ResourceRef
		for key, degree := range inDegree {
			if degree == 0 {
				currentPhase = append(currentPhase, d.nodes[key].Ref)
			}
		}

		if len(currentPhase) == 0 {
			// No nodes with in-degree 0 but still have remaining nodes = cycle
			return nil, fmt.Errorf("circular dependency detected during topological sort")
		}

		phases = append(phases, currentPhase)

		// Remove processed nodes and update in-degrees
		for _, ref := range currentPhase {
			key := nodeKey(ref)
			delete(inDegree, key)
			remaining--

			// Reduce in-degree of all nodes that depend on this one
			for depKey := range inDegree {
				for _, edge := range d.edges[depKey] {
					if edge == key {
						inDegree[depKey]--
					}
				}
			}
		}
	}

	return phases, nil
}

// HasCycle detects if the graph contains a circular dependency.
// Uses DFS-based cycle detection.
func (d *DAG) HasCycle() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.nodes) == 0 {
		return false
	}

	// States: 0 = unvisited, 1 = visiting (in current path), 2 = visited
	state := make(map[string]int)

	var hasCycle bool
	var dfs func(key string) bool

	dfs = func(key string) bool {
		if state[key] == 1 {
			// Currently visiting this node = cycle found
			return true
		}
		if state[key] == 2 {
			// Already fully processed
			return false
		}

		state[key] = 1 // Mark as visiting

		// Visit all dependencies (nodes this one depends on)
		for _, depKey := range d.edges[key] {
			if dfs(depKey) {
				return true
			}
		}

		state[key] = 2 // Mark as visited
		return false
	}

	for key := range d.nodes {
		if state[key] == 0 {
			if dfs(key) {
				hasCycle = true
				break
			}
		}
	}

	return hasCycle
}

// DependsOn checks if the "from" resource depends on the "to" resource,
// either directly or transitively.
func (d *DAG) DependsOn(from, to v1.ResourceRef) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	fromKey := nodeKey(from)
	toKey := nodeKey(to)

	if _, exists := d.nodes[fromKey]; !exists {
		return false
	}
	if _, exists := d.nodes[toKey]; !exists {
		return false
	}

	// BFS to find if there's a path from "from" to "to"
	visited := make(map[string]bool)
	queue := []string{fromKey}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		// Check all dependencies of current node
		for _, depKey := range d.edges[current] {
			if depKey == toKey {
				return true
			}
			if !visited[depKey] {
				queue = append(queue, depKey)
			}
		}
	}

	return false
}

// GetNode returns the node for a given resource reference.
// Returns nil if the node does not exist.
func (d *DAG) GetNode(ref v1.ResourceRef) *Node {
	d.mu.RLock()
	defer d.mu.RUnlock()

	key := nodeKey(ref)
	return d.nodes[key]
}

// Nodes returns all nodes in the DAG.
func (d *DAG) Nodes() []*Node {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes := make([]*Node, 0, len(d.nodes))
	for _, node := range d.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// NodeCount returns the number of nodes in the DAG.
func (d *DAG) NodeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.nodes)
}

// EdgeCount returns the number of edges in the DAG.
func (d *DAG) EdgeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	count := 0
	for _, edges := range d.edges {
		count += len(edges)
	}
	return count
}
