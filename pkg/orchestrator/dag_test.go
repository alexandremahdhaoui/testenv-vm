//go:build unit

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
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func TestNewDAG(t *testing.T) {
	dag := NewDAG()
	if dag == nil {
		t.Fatal("NewDAG returned nil")
	}
	if dag.nodes == nil {
		t.Error("nodes map is nil")
	}
	if dag.edges == nil {
		t.Error("edges map is nil")
	}
	if dag.NodeCount() != 0 {
		t.Errorf("expected 0 nodes, got %d", dag.NodeCount())
	}
	if dag.EdgeCount() != 0 {
		t.Errorf("expected 0 edges, got %d", dag.EdgeCount())
	}
}

func TestNodeKey(t *testing.T) {
	tests := []struct {
		name     string
		ref      v1.ResourceRef
		expected string
	}{
		{
			name:     "key resource",
			ref:      v1.ResourceRef{Kind: "key", Name: "ssh-key"},
			expected: "key:ssh-key",
		},
		{
			name:     "network resource",
			ref:      v1.ResourceRef{Kind: "network", Name: "test-net"},
			expected: "network:test-net",
		},
		{
			name:     "vm resource",
			ref:      v1.ResourceRef{Kind: "vm", Name: "test-vm"},
			expected: "vm:test-vm",
		},
		{
			name:     "with provider (ignored in key)",
			ref:      v1.ResourceRef{Kind: "vm", Name: "test-vm", Provider: "libvirt"},
			expected: "vm:test-vm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nodeKey(tt.ref)
			if result != tt.expected {
				t.Errorf("nodeKey() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDAG_AddNode(t *testing.T) {
	dag := NewDAG()

	ref := v1.ResourceRef{Kind: "key", Name: "test-key"}
	dag.AddNode(ref)

	if dag.NodeCount() != 1 {
		t.Errorf("expected 1 node, got %d", dag.NodeCount())
	}

	node := dag.GetNode(ref)
	if node == nil {
		t.Fatal("GetNode returned nil for added node")
	}
	if node.Ref.Kind != "key" || node.Ref.Name != "test-key" {
		t.Errorf("node ref mismatch: got %+v", node.Ref)
	}

	// Adding same node again should be no-op
	dag.AddNode(ref)
	if dag.NodeCount() != 1 {
		t.Errorf("adding duplicate node changed count: got %d", dag.NodeCount())
	}
}

func TestDAG_AddEdge(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*DAG)
		from      v1.ResourceRef
		to        v1.ResourceRef
		expectErr bool
	}{
		{
			name: "valid edge between existing nodes",
			setup: func(dag *DAG) {
				dag.AddNode(v1.ResourceRef{Kind: "key", Name: "key1"})
				dag.AddNode(v1.ResourceRef{Kind: "vm", Name: "vm1"})
			},
			from:      v1.ResourceRef{Kind: "vm", Name: "vm1"},
			to:        v1.ResourceRef{Kind: "key", Name: "key1"},
			expectErr: false,
		},
		{
			name: "error when source node does not exist",
			setup: func(dag *DAG) {
				dag.AddNode(v1.ResourceRef{Kind: "key", Name: "key1"})
			},
			from:      v1.ResourceRef{Kind: "vm", Name: "vm1"},
			to:        v1.ResourceRef{Kind: "key", Name: "key1"},
			expectErr: true,
		},
		{
			name: "error when target node does not exist",
			setup: func(dag *DAG) {
				dag.AddNode(v1.ResourceRef{Kind: "vm", Name: "vm1"})
			},
			from:      v1.ResourceRef{Kind: "vm", Name: "vm1"},
			to:        v1.ResourceRef{Kind: "key", Name: "key1"},
			expectErr: true,
		},
		{
			name: "duplicate edge is no-op",
			setup: func(dag *DAG) {
				dag.AddNode(v1.ResourceRef{Kind: "key", Name: "key1"})
				dag.AddNode(v1.ResourceRef{Kind: "vm", Name: "vm1"})
				// Add edge first time
				dag.AddEdge(
					v1.ResourceRef{Kind: "vm", Name: "vm1"},
					v1.ResourceRef{Kind: "key", Name: "key1"},
				)
			},
			from:      v1.ResourceRef{Kind: "vm", Name: "vm1"},
			to:        v1.ResourceRef{Kind: "key", Name: "key1"},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag := NewDAG()
			tt.setup(dag)

			err := dag.AddEdge(tt.from, tt.to)
			if (err != nil) != tt.expectErr {
				t.Errorf("AddEdge() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestDAG_TopologicalSort_EmptyDAG(t *testing.T) {
	dag := NewDAG()

	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}
	if phases != nil {
		t.Errorf("expected nil phases for empty DAG, got %v", phases)
	}
}

func TestDAG_TopologicalSort_SingleNode(t *testing.T) {
	dag := NewDAG()
	ref := v1.ResourceRef{Kind: "key", Name: "key1"}
	dag.AddNode(ref)

	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}
	if len(phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(phases))
	}
	if len(phases[0]) != 1 {
		t.Fatalf("expected 1 resource in phase, got %d", len(phases[0]))
	}
	if phases[0][0].Kind != "key" || phases[0][0].Name != "key1" {
		t.Errorf("unexpected resource: %+v", phases[0][0])
	}
}

func TestDAG_TopologicalSort_ParallelResources(t *testing.T) {
	dag := NewDAG()

	// Add three independent nodes (no dependencies)
	dag.AddNode(v1.ResourceRef{Kind: "key", Name: "key1"})
	dag.AddNode(v1.ResourceRef{Kind: "key", Name: "key2"})
	dag.AddNode(v1.ResourceRef{Kind: "key", Name: "key3"})

	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	// All should be in the same phase (parallel execution)
	if len(phases) != 1 {
		t.Fatalf("expected 1 phase for independent resources, got %d", len(phases))
	}
	if len(phases[0]) != 3 {
		t.Fatalf("expected 3 resources in phase, got %d", len(phases[0]))
	}
}

func TestDAG_TopologicalSort_SimpleDependencyChain(t *testing.T) {
	dag := NewDAG()

	// Create: key -> network -> vm
	keyRef := v1.ResourceRef{Kind: "key", Name: "key1"}
	netRef := v1.ResourceRef{Kind: "network", Name: "net1"}
	vmRef := v1.ResourceRef{Kind: "vm", Name: "vm1"}

	dag.AddNode(keyRef)
	dag.AddNode(netRef)
	dag.AddNode(vmRef)

	// vm depends on network
	if err := dag.AddEdge(vmRef, netRef); err != nil {
		t.Fatalf("AddEdge(vm, net) error = %v", err)
	}
	// network depends on key
	if err := dag.AddEdge(netRef, keyRef); err != nil {
		t.Fatalf("AddEdge(net, key) error = %v", err)
	}

	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	// Should have 3 phases: key, network, vm
	if len(phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(phases))
	}

	// Phase 0: key (no dependencies)
	if len(phases[0]) != 1 || phases[0][0].Kind != "key" {
		t.Errorf("phase 0: expected key, got %+v", phases[0])
	}

	// Phase 1: network (depends on key)
	if len(phases[1]) != 1 || phases[1][0].Kind != "network" {
		t.Errorf("phase 1: expected network, got %+v", phases[1])
	}

	// Phase 2: vm (depends on network)
	if len(phases[2]) != 1 || phases[2][0].Kind != "vm" {
		t.Errorf("phase 2: expected vm, got %+v", phases[2])
	}
}

func TestDAG_TopologicalSort_ComplexDependencies(t *testing.T) {
	dag := NewDAG()

	// Create complex graph:
	// key1, key2 (no deps) -> parallel in phase 1
	// network (depends on key1) -> phase 2
	// vm1 (depends on network, key2) -> phase 3
	// vm2 (depends on network) -> phase 3 (parallel with vm1)

	key1 := v1.ResourceRef{Kind: "key", Name: "key1"}
	key2 := v1.ResourceRef{Kind: "key", Name: "key2"}
	net := v1.ResourceRef{Kind: "network", Name: "net1"}
	vm1 := v1.ResourceRef{Kind: "vm", Name: "vm1"}
	vm2 := v1.ResourceRef{Kind: "vm", Name: "vm2"}

	dag.AddNode(key1)
	dag.AddNode(key2)
	dag.AddNode(net)
	dag.AddNode(vm1)
	dag.AddNode(vm2)

	// network depends on key1
	dag.AddEdge(net, key1)
	// vm1 depends on network and key2
	dag.AddEdge(vm1, net)
	dag.AddEdge(vm1, key2)
	// vm2 depends on network
	dag.AddEdge(vm2, net)

	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	// Verify phases
	if len(phases) < 3 {
		t.Fatalf("expected at least 3 phases, got %d", len(phases))
	}

	// First phase should contain keys (no dependencies)
	phase0Resources := make(map[string]bool)
	for _, ref := range phases[0] {
		phase0Resources[ref.Kind+":"+ref.Name] = true
	}
	if !phase0Resources["key:key1"] || !phase0Resources["key:key2"] {
		t.Errorf("phase 0 should contain both keys, got %v", phases[0])
	}

	// VMs should be in last phase together (both depend on network)
	lastPhase := phases[len(phases)-1]
	lastPhaseResources := make(map[string]bool)
	for _, ref := range lastPhase {
		lastPhaseResources[ref.Kind+":"+ref.Name] = true
	}
	if !lastPhaseResources["vm:vm1"] || !lastPhaseResources["vm:vm2"] {
		t.Errorf("last phase should contain both VMs, got %v", lastPhase)
	}
}

func TestDAG_HasCycle_NoCycle(t *testing.T) {
	dag := NewDAG()

	key := v1.ResourceRef{Kind: "key", Name: "key1"}
	net := v1.ResourceRef{Kind: "network", Name: "net1"}
	vm := v1.ResourceRef{Kind: "vm", Name: "vm1"}

	dag.AddNode(key)
	dag.AddNode(net)
	dag.AddNode(vm)

	dag.AddEdge(vm, net)
	dag.AddEdge(net, key)

	if dag.HasCycle() {
		t.Error("HasCycle() returned true for acyclic graph")
	}
}

func TestDAG_HasCycle_WithCycle(t *testing.T) {
	dag := NewDAG()

	a := v1.ResourceRef{Kind: "network", Name: "a"}
	b := v1.ResourceRef{Kind: "network", Name: "b"}
	c := v1.ResourceRef{Kind: "network", Name: "c"}

	dag.AddNode(a)
	dag.AddNode(b)
	dag.AddNode(c)

	// Create cycle: a -> b -> c -> a
	dag.AddEdge(a, b)
	dag.AddEdge(b, c)
	dag.AddEdge(c, a)

	if !dag.HasCycle() {
		t.Error("HasCycle() returned false for cyclic graph")
	}
}

func TestDAG_HasCycle_EmptyDAG(t *testing.T) {
	dag := NewDAG()
	if dag.HasCycle() {
		t.Error("HasCycle() returned true for empty DAG")
	}
}

func TestDAG_DependsOn(t *testing.T) {
	dag := NewDAG()

	key := v1.ResourceRef{Kind: "key", Name: "key1"}
	net := v1.ResourceRef{Kind: "network", Name: "net1"}
	vm := v1.ResourceRef{Kind: "vm", Name: "vm1"}

	dag.AddNode(key)
	dag.AddNode(net)
	dag.AddNode(vm)

	// vm -> net -> key
	dag.AddEdge(vm, net)
	dag.AddEdge(net, key)

	tests := []struct {
		name     string
		from     v1.ResourceRef
		to       v1.ResourceRef
		expected bool
	}{
		{
			name:     "direct dependency",
			from:     vm,
			to:       net,
			expected: true,
		},
		{
			name:     "transitive dependency",
			from:     vm,
			to:       key,
			expected: true,
		},
		{
			name:     "no dependency",
			from:     key,
			to:       vm,
			expected: false,
		},
		{
			name:     "self dependency",
			from:     vm,
			to:       vm,
			expected: false,
		},
		{
			name:     "non-existent node",
			from:     v1.ResourceRef{Kind: "vm", Name: "nonexistent"},
			to:       key,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dag.DependsOn(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("DependsOn() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDAG_GetNode(t *testing.T) {
	dag := NewDAG()

	ref := v1.ResourceRef{Kind: "key", Name: "key1"}
	dag.AddNode(ref)

	// Get existing node
	node := dag.GetNode(ref)
	if node == nil {
		t.Error("GetNode() returned nil for existing node")
	}

	// Get non-existent node
	nonExistent := dag.GetNode(v1.ResourceRef{Kind: "vm", Name: "nonexistent"})
	if nonExistent != nil {
		t.Error("GetNode() should return nil for non-existent node")
	}
}

func TestDAG_Nodes(t *testing.T) {
	dag := NewDAG()

	dag.AddNode(v1.ResourceRef{Kind: "key", Name: "key1"})
	dag.AddNode(v1.ResourceRef{Kind: "network", Name: "net1"})
	dag.AddNode(v1.ResourceRef{Kind: "vm", Name: "vm1"})

	nodes := dag.Nodes()
	if len(nodes) != 3 {
		t.Errorf("Nodes() returned %d nodes, expected 3", len(nodes))
	}
}

func TestBuildDAG_EmptySpec(t *testing.T) {
	spec := &v1.Spec{}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}
	if dag.NodeCount() != 0 {
		t.Errorf("expected 0 nodes for empty spec, got %d", dag.NodeCount())
	}
}

func TestBuildDAG_SimpleSpec(t *testing.T) {
	spec := &v1.Spec{
		Keys: []v1.KeyResource{
			{Name: "ssh-key", Spec: v1.KeySpec{Type: "ed25519"}},
		},
		Networks: []v1.NetworkResource{
			{Name: "test-net", Kind: "bridge", Spec: v1.NetworkSpec{Cidr: "192.168.100.1/24"}},
		},
		Vms: []v1.VMResource{
			{
				Name: "test-vm",
				Spec: v1.VMSpec{
					Memory:  1024,
					Vcpus:   1,
					Network: "test-net",
				},
			},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// Should have 3 nodes
	if dag.NodeCount() != 3 {
		t.Errorf("expected 3 nodes, got %d", dag.NodeCount())
	}

	// VM should depend on network (via spec.Network field)
	vmRef := v1.ResourceRef{Kind: "vm", Name: "test-vm"}
	netRef := v1.ResourceRef{Kind: "network", Name: "test-net"}
	if !dag.DependsOn(vmRef, netRef) {
		t.Error("VM should depend on network")
	}

	// Verify topological sort works
	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}
	if len(phases) < 2 {
		t.Errorf("expected at least 2 phases, got %d", len(phases))
	}
}

func TestBuildDAG_WithTemplateDependencies(t *testing.T) {
	spec := &v1.Spec{
		Keys: []v1.KeyResource{
			{Name: "ssh-key", Spec: v1.KeySpec{Type: "ed25519"}},
		},
		Networks: []v1.NetworkResource{
			{Name: "test-net", Kind: "bridge", Spec: v1.NetworkSpec{Cidr: "192.168.100.1/24"}},
		},
		Vms: []v1.VMResource{
			{
				Name: "test-vm",
				Spec: v1.VMSpec{
					Memory:  1024,
					Vcpus:   1,
					Network: "test-net",
					CloudInit: v1.CloudInitSpec{
						Users: []v1.UserSpec{
							{
								Name: "test",
								// Template reference to key
								SshAuthorizedKeys: []string{"{{ .Keys.ssh-key.PublicKey }}"},
							},
						},
					},
					Readiness: v1.ReadinessSpec{
						Ssh: v1.SSHReadinessSpec{
							Enabled:    true,
							Timeout:    "5m",
							User:       "test",
							PrivateKey: "{{ .Keys.ssh-key.PrivateKeyPath }}",
						},
					},
				},
			},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// VM should depend on both network and key
	vmRef := v1.ResourceRef{Kind: "vm", Name: "test-vm"}
	netRef := v1.ResourceRef{Kind: "network", Name: "test-net"}
	keyRef := v1.ResourceRef{Kind: "key", Name: "ssh-key"}

	if !dag.DependsOn(vmRef, netRef) {
		t.Error("VM should depend on network")
	}
	if !dag.DependsOn(vmRef, keyRef) {
		t.Error("VM should depend on key (via template)")
	}
}

func TestBuildDAG_NetworkAttachTo(t *testing.T) {
	spec := &v1.Spec{
		Networks: []v1.NetworkResource{
			{Name: "base-net", Kind: "bridge", Spec: v1.NetworkSpec{Cidr: "192.168.100.1/24"}},
			{Name: "overlay-net", Kind: "dnsmasq", Spec: v1.NetworkSpec{AttachTo: "base-net"}},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// overlay-net should depend on base-net
	overlayRef := v1.ResourceRef{Kind: "network", Name: "overlay-net"}
	baseRef := v1.ResourceRef{Kind: "network", Name: "base-net"}

	if !dag.DependsOn(overlayRef, baseRef) {
		t.Error("overlay-net should depend on base-net via attachTo")
	}
}

func TestBuildDAG_CycleDetection(t *testing.T) {
	// This test creates a scenario where cycle detection should fail
	// However, since template references are parsed from string fields,
	// we need to create a cycle through template references

	// Note: In practice, cycles are unlikely because:
	// - Keys don't typically depend on anything
	// - Networks might depend on other networks (attachTo)
	// - VMs depend on networks and keys

	// We'll test the HasCycle detection directly since BuildDAG
	// already checks for cycles
	dag := NewDAG()

	a := v1.ResourceRef{Kind: "network", Name: "net-a"}
	b := v1.ResourceRef{Kind: "network", Name: "net-b"}

	dag.AddNode(a)
	dag.AddNode(b)

	// Create cycle
	dag.AddEdge(a, b)
	dag.AddEdge(b, a)

	if !dag.HasCycle() {
		t.Error("HasCycle() should detect cycle")
	}

	// Topological sort should fail on cycle
	_, err := dag.TopologicalSort()
	if err == nil {
		t.Error("TopologicalSort() should return error on cycle")
	}
}

func TestDAG_EdgeCount(t *testing.T) {
	dag := NewDAG()

	// Empty DAG should have 0 edges
	if dag.EdgeCount() != 0 {
		t.Errorf("expected 0 edges, got %d", dag.EdgeCount())
	}

	// Add nodes
	a := v1.ResourceRef{Kind: "key", Name: "a"}
	b := v1.ResourceRef{Kind: "key", Name: "b"}
	c := v1.ResourceRef{Kind: "key", Name: "c"}

	dag.AddNode(a)
	dag.AddNode(b)
	dag.AddNode(c)

	// Still 0 edges
	if dag.EdgeCount() != 0 {
		t.Errorf("expected 0 edges after adding nodes, got %d", dag.EdgeCount())
	}

	// Add edges
	dag.AddEdge(b, a)
	dag.AddEdge(c, a)
	dag.AddEdge(c, b)

	if dag.EdgeCount() != 3 {
		t.Errorf("expected 3 edges, got %d", dag.EdgeCount())
	}
}

func TestBuildDAG_VMWithTemplateNetwork(t *testing.T) {
	// Test that network references containing templates are handled
	spec := &v1.Spec{
		Networks: []v1.NetworkResource{
			{Name: "base-net", Kind: "bridge"},
		},
		Vms: []v1.VMResource{
			{
				Name: "test-vm",
				Spec: v1.VMSpec{
					// Template reference to network
					Network: "{{ .Networks.base-net.Name }}",
				},
			},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// VM should depend on network via template reference
	vmRef := v1.ResourceRef{Kind: "vm", Name: "test-vm"}
	netRef := v1.ResourceRef{Kind: "network", Name: "base-net"}

	if !dag.DependsOn(vmRef, netRef) {
		t.Error("VM should depend on network via template reference")
	}
}

func TestBuildDAG_NetworkWithTemplateAttachTo(t *testing.T) {
	// Test that attachTo containing templates is handled
	spec := &v1.Spec{
		Networks: []v1.NetworkResource{
			{Name: "base-net", Kind: "bridge"},
			{
				Name: "overlay-net",
				Kind: "dnsmasq",
				Spec: v1.NetworkSpec{
					// Template reference to attachTo
					AttachTo: "{{ .Networks.base-net.InterfaceName }}",
				},
			},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// overlay should depend on base via template reference
	overlayRef := v1.ResourceRef{Kind: "network", Name: "overlay-net"}
	baseRef := v1.ResourceRef{Kind: "network", Name: "base-net"}

	if !dag.DependsOn(overlayRef, baseRef) {
		t.Error("overlay should depend on base via template reference")
	}
}

func TestBuildDAG_KeyWithNoTemplates(t *testing.T) {
	// Keys typically don't have template dependencies
	spec := &v1.Spec{
		Keys: []v1.KeyResource{
			{Name: "key1", Spec: v1.KeySpec{Type: "ed25519"}},
			{Name: "key2", Spec: v1.KeySpec{Type: "rsa", Bits: 4096}},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// Should have 2 nodes and 0 edges
	if dag.NodeCount() != 2 {
		t.Errorf("expected 2 nodes, got %d", dag.NodeCount())
	}
	if dag.EdgeCount() != 0 {
		t.Errorf("expected 0 edges, got %d", dag.EdgeCount())
	}
}

func TestDAG_HasCycle_SelfLoop(t *testing.T) {
	dag := NewDAG()

	a := v1.ResourceRef{Kind: "network", Name: "a"}
	dag.AddNode(a)

	// Self loop
	dag.AddEdge(a, a)

	if !dag.HasCycle() {
		t.Error("HasCycle() should detect self loop")
	}
}

func TestDAG_HasCycle_LongChain(t *testing.T) {
	dag := NewDAG()

	// Create a long chain: a -> b -> c -> d -> e
	refs := make([]v1.ResourceRef, 5)
	for i := 0; i < 5; i++ {
		refs[i] = v1.ResourceRef{Kind: "key", Name: string(rune('a' + i))}
		dag.AddNode(refs[i])
	}

	for i := 0; i < 4; i++ {
		dag.AddEdge(refs[i], refs[i+1])
	}

	// No cycle
	if dag.HasCycle() {
		t.Error("HasCycle() should not detect cycle in chain")
	}

	// Add edge to create cycle: e -> a
	dag.AddEdge(refs[4], refs[0])

	if !dag.HasCycle() {
		t.Error("HasCycle() should detect cycle after adding closing edge")
	}
}

func TestBuildDAG_WithImages(t *testing.T) {
	// Test that spec with images, keys, and VMs includes images in phase 0
	spec := &v1.Spec{
		Images: []v1.ImageResource{
			{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
		},
		Keys: []v1.KeyResource{
			{Name: "ssh-key", Spec: v1.KeySpec{Type: "ed25519"}},
		},
		Vms: []v1.VMResource{
			{
				Name: "test-vm",
				Spec: v1.VMSpec{
					Memory: 1024,
					Vcpus:  1,
				},
			},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// Should have 3 nodes: 1 image + 1 key + 1 VM
	if dag.NodeCount() != 3 {
		t.Errorf("expected 3 nodes, got %d", dag.NodeCount())
	}

	// Verify image node exists
	imageRef := v1.ResourceRef{Kind: "image", Name: "ubuntu"}
	imageNode := dag.GetNode(imageRef)
	if imageNode == nil {
		t.Error("image node should exist")
	}

	// Verify topological sort places images in phase 0
	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	// Find which phase contains the image
	imageInPhase0 := false
	for _, ref := range phases[0] {
		if ref.Kind == "image" && ref.Name == "ubuntu" {
			imageInPhase0 = true
			break
		}
	}
	if !imageInPhase0 {
		t.Error("image should be in phase 0 (no dependencies)")
	}
}

func TestBuildDAG_VMDependsOnImage(t *testing.T) {
	// Test that VM with image template reference depends on the image
	spec := &v1.Spec{
		Images: []v1.ImageResource{
			{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
		},
		Vms: []v1.VMResource{
			{
				Name: "test-vm",
				Spec: v1.VMSpec{
					Memory: 1024,
					Vcpus:  1,
					Disk: v1.DiskSpec{
						BaseImage: "{{ .Images.ubuntu.Path }}",
					},
				},
			},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// VM should depend on image
	vmRef := v1.ResourceRef{Kind: "vm", Name: "test-vm"}
	imageRef := v1.ResourceRef{Kind: "image", Name: "ubuntu"}

	if !dag.DependsOn(vmRef, imageRef) {
		t.Error("VM should depend on image via template reference")
	}

	// Verify topological sort places image before VM
	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	imagePhase := -1
	vmPhase := -1
	for i, phase := range phases {
		for _, ref := range phase {
			if ref.Kind == "image" && ref.Name == "ubuntu" {
				imagePhase = i
			}
			if ref.Kind == "vm" && ref.Name == "test-vm" {
				vmPhase = i
			}
		}
	}

	if imagePhase == -1 {
		t.Fatal("image not found in any phase")
	}
	if vmPhase == -1 {
		t.Fatal("VM not found in any phase")
	}
	if imagePhase >= vmPhase {
		t.Errorf("image (phase %d) should be before VM (phase %d)", imagePhase, vmPhase)
	}
}

func TestBuildDAG_ImageNoDependencies(t *testing.T) {
	// Test that images have no dependencies
	spec := &v1.Spec{
		Images: []v1.ImageResource{
			{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
		},
		Keys: []v1.KeyResource{
			{Name: "ssh-key", Spec: v1.KeySpec{Type: "ed25519"}},
		},
		Networks: []v1.NetworkResource{
			{Name: "test-net", Kind: "bridge"},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// Image should have no dependencies
	imageRef := v1.ResourceRef{Kind: "image", Name: "ubuntu"}
	imageNode := dag.GetNode(imageRef)
	if imageNode == nil {
		t.Fatal("image node should exist")
	}
	if len(imageNode.Dependencies) != 0 {
		t.Errorf("image should have no dependencies, got %d", len(imageNode.Dependencies))
	}

	// Image should not depend on keys or networks
	keyRef := v1.ResourceRef{Kind: "key", Name: "ssh-key"}
	netRef := v1.ResourceRef{Kind: "network", Name: "test-net"}

	if dag.DependsOn(imageRef, keyRef) {
		t.Error("image should not depend on key")
	}
	if dag.DependsOn(imageRef, netRef) {
		t.Error("image should not depend on network")
	}
}

func TestBuildDAG_MultipleImages(t *testing.T) {
	// Test that multiple images are all in the same phase (parallel)
	spec := &v1.Spec{
		Images: []v1.ImageResource{
			{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
			{Name: "debian", Spec: v1.ImageSpec{Source: "debian:12"}},
			{Name: "fedora", Spec: v1.ImageSpec{Source: "fedora:40"}},
		},
	}

	dag, err := BuildDAG(spec)
	if err != nil {
		t.Fatalf("BuildDAG() error = %v", err)
	}

	// Should have 3 image nodes
	if dag.NodeCount() != 3 {
		t.Errorf("expected 3 nodes, got %d", dag.NodeCount())
	}

	// Should have 0 edges (no dependencies between images)
	if dag.EdgeCount() != 0 {
		t.Errorf("expected 0 edges, got %d", dag.EdgeCount())
	}

	// All images should be in the same phase (phase 0)
	phases, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	// Should have exactly 1 phase with all 3 images
	if len(phases) != 1 {
		t.Errorf("expected 1 phase for independent images, got %d", len(phases))
	}

	if len(phases[0]) != 3 {
		t.Errorf("expected 3 images in phase 0, got %d", len(phases[0]))
	}

	// Verify all are images
	imageCount := 0
	for _, ref := range phases[0] {
		if ref.Kind == "image" {
			imageCount++
		}
	}
	if imageCount != 3 {
		t.Errorf("expected 3 images in phase 0, got %d", imageCount)
	}
}
