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
	"context"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/image"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

// newTestExecutor creates an Executor for testing with a temporary image cache.
func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	manager := provider.NewManager()
	store := state.NewStore(t.TempDir())
	imageMgr, err := image.NewCacheManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create image cache manager: %v", err)
	}
	return NewExecutor(manager, store, imageMgr)
}

func TestNewExecutor(t *testing.T) {
	manager := provider.NewManager()
	store := state.NewStore(t.TempDir())
	imageMgr, err := image.NewCacheManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create image cache manager: %v", err)
	}

	executor := NewExecutor(manager, store, imageMgr)

	if executor == nil {
		t.Fatal("NewExecutor returned nil")
	}
	if executor.manager == nil {
		t.Error("executor.manager is nil")
	}
	if executor.store == nil {
		t.Error("executor.store is nil")
	}
	if executor.imageMgr == nil {
		t.Error("executor.imageMgr is nil")
	}
}

func TestExecutor_ExecuteCreate_NilSpec(t *testing.T) {
	executor := newTestExecutor(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusCreating,
	}

	_, err := executor.ExecuteCreate(ctx, nil, nil, nil, envState, nil, nil)
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

func TestExecutor_ExecuteCreate_NilState(t *testing.T) {
	executor := newTestExecutor(t)

	ctx := context.Background()
	testenvSpec := &v1.Spec{}

	_, err := executor.ExecuteCreate(ctx, testenvSpec, nil, nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error for nil state")
	}
}

func TestExecutor_ExecuteCreate_EmptyPlan(t *testing.T) {
	executor := newTestExecutor(t)

	ctx := context.Background()
	testenvSpec := &v1.Spec{}
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusCreating,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}
	templateCtx := spec.NewTemplateContext()

	// Empty plan should succeed
	result, err := executor.ExecuteCreate(ctx, testenvSpec, [][]v1.ResourceRef{}, templateCtx, envState, nil, nil)
	if err != nil {
		t.Fatalf("ExecuteCreate() error = %v", err)
	}
	if !result.Success {
		t.Error("expected success for empty plan")
	}
}

func TestExecutor_ExecuteDelete_NilState(t *testing.T) {
	executor := newTestExecutor(t)

	ctx := context.Background()

	err := executor.ExecuteDelete(ctx, nil, nil)
	if err == nil {
		t.Error("expected error for nil state")
	}
}

func TestExecutor_ExecuteDelete_EmptyPlan(t *testing.T) {
	executor := newTestExecutor(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusDestroying,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: nil, // No plan
	}

	err := executor.ExecuteDelete(ctx, envState, nil)
	if err != nil {
		t.Errorf("ExecuteDelete() error = %v, expected nil for empty plan", err)
	}
}

func TestExecutor_ExecuteDelete_ReversePhaseOrder(t *testing.T) {
	// This test verifies that phases are reversed for deletion
	// Without mocking, we can only verify the structure is correct
	executor := newTestExecutor(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusDestroying,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: &v1.ExecutionPlan{
			Phases: []v1.Phase{
				{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
				{Resources: []v1.ResourceRef{{Kind: "network", Name: "net1"}}},
				{Resources: []v1.ResourceRef{{Kind: "vm", Name: "vm1"}}},
			},
		},
	}

	// Without a running provider, delete will fail for each resource
	// but the method should still process in reverse order
	// We test this by verifying no panic occurs
	err := executor.ExecuteDelete(ctx, envState, nil)
	// Error is expected since no providers are running
	// The important thing is it doesn't panic
	_ = err
}

func TestExecutor_findKeySpec(t *testing.T) {
	executor := newTestExecutor(t)

	testenvSpec := &v1.Spec{
		Keys: []v1.KeyResource{
			{Name: "key1", Spec: v1.KeySpec{Type: "ed25519"}},
			{Name: "key2", Spec: v1.KeySpec{Type: "rsa", Bits: 4096}},
		},
	}

	tests := []struct {
		name      string
		keyName   string
		expectErr bool
	}{
		{
			name:      "find existing key",
			keyName:   "key1",
			expectErr: false,
		},
		{
			name:      "find another existing key",
			keyName:   "key2",
			expectErr: false,
		},
		{
			name:      "key not found",
			keyName:   "nonexistent",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := executor.findKeySpec(testenvSpec, tt.keyName)
			if (err != nil) != tt.expectErr {
				t.Errorf("findKeySpec() error = %v, expectErr %v", err, tt.expectErr)
			}
			if !tt.expectErr && key == nil {
				t.Error("findKeySpec() returned nil for existing key")
			}
			if !tt.expectErr && key.Name != tt.keyName {
				t.Errorf("findKeySpec() returned wrong key: got %s, want %s", key.Name, tt.keyName)
			}
		})
	}
}

func TestExecutor_findNetworkSpec(t *testing.T) {
	executor := newTestExecutor(t)

	testenvSpec := &v1.Spec{
		Networks: []v1.NetworkResource{
			{Name: "net1", Kind: "bridge", Spec: v1.NetworkSpec{Cidr: "192.168.1.0/24"}},
			{Name: "net2", Kind: "dnsmasq", Spec: v1.NetworkSpec{Cidr: "192.168.2.0/24"}},
		},
	}

	tests := []struct {
		name        string
		networkName string
		expectErr   bool
	}{
		{
			name:        "find existing network",
			networkName: "net1",
			expectErr:   false,
		},
		{
			name:        "network not found",
			networkName: "nonexistent",
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			net, err := executor.findNetworkSpec(testenvSpec, tt.networkName)
			if (err != nil) != tt.expectErr {
				t.Errorf("findNetworkSpec() error = %v, expectErr %v", err, tt.expectErr)
			}
			if !tt.expectErr && net == nil {
				t.Error("findNetworkSpec() returned nil for existing network")
			}
			if !tt.expectErr && net.Name != tt.networkName {
				t.Errorf("findNetworkSpec() returned wrong network: got %s, want %s", net.Name, tt.networkName)
			}
		})
	}
}

func TestExecutor_findVMSpec(t *testing.T) {
	executor := newTestExecutor(t)

	testenvSpec := &v1.Spec{
		Vms: []v1.VMResource{
			{Name: "vm1", Spec: v1.VMSpec{Memory: 1024, Vcpus: 1}},
			{Name: "vm2", Spec: v1.VMSpec{Memory: 2048, Vcpus: 2}},
		},
	}

	tests := []struct {
		name      string
		vmName    string
		expectErr bool
	}{
		{
			name:      "find existing VM",
			vmName:    "vm1",
			expectErr: false,
		},
		{
			name:      "VM not found",
			vmName:    "nonexistent",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm, err := executor.findVMSpec(testenvSpec, tt.vmName)
			if (err != nil) != tt.expectErr {
				t.Errorf("findVMSpec() error = %v, expectErr %v", err, tt.expectErr)
			}
			if !tt.expectErr && vm == nil {
				t.Error("findVMSpec() returned nil for existing VM")
			}
			if !tt.expectErr && vm.Name != tt.vmName {
				t.Errorf("findVMSpec() returned wrong VM: got %s, want %s", vm.Name, tt.vmName)
			}
		})
	}
}

func TestExecutor_updateResourceState(t *testing.T) {
	executor := newTestExecutor(t)

	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusCreating,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}

	tests := []struct {
		name     string
		ref      v1.ResourceRef
		provider string
		status   string
		data     map[string]any
		errMsg   string
	}{
		{
			name:     "update key state",
			ref:      v1.ResourceRef{Kind: "key", Name: "key1"},
			provider: "test-provider",
			status:   v1.StatusReady,
			data:     map[string]any{"publicKey": "ssh-ed25519 AAAA..."},
			errMsg:   "",
		},
		{
			name:     "update network state",
			ref:      v1.ResourceRef{Kind: "network", Name: "net1"},
			provider: "test-provider",
			status:   v1.StatusReady,
			data:     map[string]any{"ip": "192.168.1.1"},
			errMsg:   "",
		},
		{
			name:     "update vm state",
			ref:      v1.ResourceRef{Kind: "vm", Name: "vm1"},
			provider: "test-provider",
			status:   v1.StatusReady,
			data:     map[string]any{"ip": "192.168.1.10"},
			errMsg:   "",
		},
		{
			name:     "update with error",
			ref:      v1.ResourceRef{Kind: "vm", Name: "vm2"},
			provider: "test-provider",
			status:   v1.StatusFailed,
			data:     nil,
			errMsg:   "failed to create VM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor.updateResourceState(envState, tt.ref, tt.provider, tt.status, tt.data, tt.errMsg)

			var resourceState *v1.ResourceState
			switch tt.ref.Kind {
			case "key":
				resourceState = envState.Resources.Keys[tt.ref.Name]
			case "network":
				resourceState = envState.Resources.Networks[tt.ref.Name]
			case "vm":
				resourceState = envState.Resources.VMs[tt.ref.Name]
			}

			if resourceState == nil {
				t.Fatal("resource state not updated")
			}
			if resourceState.Provider != tt.provider {
				t.Errorf("provider = %s, want %s", resourceState.Provider, tt.provider)
			}
			if resourceState.Status != tt.status {
				t.Errorf("status = %s, want %s", resourceState.Status, tt.status)
			}
			if resourceState.Error != tt.errMsg {
				t.Errorf("error = %s, want %s", resourceState.Error, tt.errMsg)
			}
		})
	}
}

func TestExecutor_getResourceState(t *testing.T) {
	executor := newTestExecutor(t)

	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusReady,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"key1": {Provider: "test", Status: v1.StatusReady},
			},
			Networks: map[string]*v1.ResourceState{
				"net1": {Provider: "test", Status: v1.StatusReady},
			},
			VMs: map[string]*v1.ResourceState{
				"vm1": {Provider: "test", Status: v1.StatusReady},
			},
		},
	}

	tests := []struct {
		name     string
		ref      v1.ResourceRef
		expected bool
	}{
		{
			name:     "get existing key",
			ref:      v1.ResourceRef{Kind: "key", Name: "key1"},
			expected: true,
		},
		{
			name:     "get existing network",
			ref:      v1.ResourceRef{Kind: "network", Name: "net1"},
			expected: true,
		},
		{
			name:     "get existing vm",
			ref:      v1.ResourceRef{Kind: "vm", Name: "vm1"},
			expected: true,
		},
		{
			name:     "get nonexistent key",
			ref:      v1.ResourceRef{Kind: "key", Name: "nonexistent"},
			expected: false,
		},
		{
			name:     "get unknown kind",
			ref:      v1.ResourceRef{Kind: "unknown", Name: "test"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := executor.getResourceState(envState, tt.ref)
			if (state != nil) != tt.expected {
				t.Errorf("getResourceState() returned %v, expected found=%v", state, tt.expected)
			}
		})
	}
}

func TestExecutor_getResourceState_NilMaps(t *testing.T) {
	executor := newTestExecutor(t)

	envState := &v1.EnvironmentState{
		ID:        "test-1",
		Status:    v1.StatusReady,
		Resources: v1.ResourceMap{}, // All maps are nil
	}

	// Should not panic when maps are nil
	keyState := executor.getResourceState(envState, v1.ResourceRef{Kind: "key", Name: "key1"})
	if keyState != nil {
		t.Error("expected nil for nil Keys map")
	}

	netState := executor.getResourceState(envState, v1.ResourceRef{Kind: "network", Name: "net1"})
	if netState != nil {
		t.Error("expected nil for nil Networks map")
	}

	vmState := executor.getResourceState(envState, v1.ResourceRef{Kind: "vm", Name: "vm1"})
	if vmState != nil {
		t.Error("expected nil for nil VMs map")
	}
}

func TestExecutor_updateTemplateContext(t *testing.T) {
	executor := newTestExecutor(t)

	templateCtx := spec.NewTemplateContext()

	// Update with key data
	executor.updateTemplateContext(
		templateCtx,
		v1.ResourceRef{Kind: "key", Name: "ssh-key"},
		map[string]any{
			"publicKey":      "ssh-ed25519 AAAA...",
			"privateKeyPath": "/tmp/key",
			"publicKeyPath":  "/tmp/key.pub",
			"fingerprint":    "SHA256:xxx",
		},
	)

	if templateCtx.Keys["ssh-key"].PublicKey != "ssh-ed25519 AAAA..." {
		t.Error("key template data not updated correctly")
	}

	// Update with network data
	executor.updateTemplateContext(
		templateCtx,
		v1.ResourceRef{Kind: "network", Name: "test-net"},
		map[string]any{
			"name":          "test-net",
			"ip":            "192.168.1.1",
			"cidr":          "192.168.1.0/24",
			"interfaceName": "br-test",
			"uuid":          "uuid-123",
		},
	)

	if templateCtx.Networks["test-net"].IP != "192.168.1.1" {
		t.Error("network template data not updated correctly")
	}

	// Update with VM data
	executor.updateTemplateContext(
		templateCtx,
		v1.ResourceRef{Kind: "vm", Name: "test-vm"},
		map[string]any{
			"name":       "test-vm",
			"ip":         "192.168.1.10",
			"mac":        "52:54:00:00:00:01",
			"sshCommand": "ssh user@192.168.1.10",
		},
	)

	if templateCtx.VMs["test-vm"].IP != "192.168.1.10" {
		t.Error("vm template data not updated correctly")
	}
}

func TestExecutor_updateTemplateContext_NilContext(t *testing.T) {
	executor := newTestExecutor(t)

	// Should not panic with nil context
	executor.updateTemplateContext(nil, v1.ResourceRef{Kind: "key", Name: "key1"}, map[string]any{})
}

func TestExecutor_updateTemplateContext_NilData(t *testing.T) {
	executor := newTestExecutor(t)

	templateCtx := spec.NewTemplateContext()

	// Should not panic with nil data
	executor.updateTemplateContext(templateCtx, v1.ResourceRef{Kind: "key", Name: "key1"}, nil)
}

func TestExecutor_convertResourceToMap(t *testing.T) {
	executor := newTestExecutor(t)

	tests := []struct {
		name     string
		resource any
		wantNil  bool
		wantErr  bool
	}{
		{
			name:     "nil resource",
			resource: nil,
			wantNil:  true,
			wantErr:  false,
		},
		{
			name:     "already a map",
			resource: map[string]any{"key": "value"},
			wantNil:  false,
			wantErr:  false,
		},
		{
			name: "struct resource",
			resource: struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			}{Name: "test", Status: "ready"},
			wantNil: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.convertResourceToMap(tt.resource)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertResourceToMap() error = %v, wantErr %v", err, tt.wantErr)
			}
			if (result == nil) != tt.wantNil {
				t.Errorf("convertResourceToMap() result = %v, wantNil %v", result, tt.wantNil)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected string
	}{
		{
			name:     "existing string key",
			m:        map[string]any{"name": "test"},
			key:      "name",
			expected: "test",
		},
		{
			name:     "nonexistent key",
			m:        map[string]any{"name": "test"},
			key:      "other",
			expected: "",
		},
		{
			name:     "non-string value",
			m:        map[string]any{"count": 42},
			key:      "count",
			expected: "",
		},
		{
			name:     "empty map",
			m:        map[string]any{},
			key:      "name",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getString(tt.m, tt.key)
			if result != tt.expected {
				t.Errorf("getString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExecutor_renderKeySpec(t *testing.T) {
	executor := newTestExecutor(t)

	templateCtx := spec.NewTemplateContext()
	templateCtx.Env["OUTPUT_DIR"] = "/tmp/keys"

	original := &v1.KeyResource{
		Name:     "test-key",
		Provider: "test",
		Spec: v1.KeySpec{
			Type:      "ed25519",
			Comment:   "test key",
			OutputDir: "{{ .Env.OUTPUT_DIR }}",
		},
	}

	rendered, err := executor.renderKeySpec(original, templateCtx)
	if err != nil {
		t.Fatalf("renderKeySpec() error = %v", err)
	}

	// Verify original is unchanged (deep copy)
	if original.Spec.OutputDir != "{{ .Env.OUTPUT_DIR }}" {
		t.Error("original spec was modified")
	}

	// Verify rendered value
	if rendered.Spec.OutputDir != "/tmp/keys" {
		t.Errorf("rendered OutputDir = %q, want /tmp/keys", rendered.Spec.OutputDir)
	}
}

func TestExecutor_renderNetworkSpec(t *testing.T) {
	executor := newTestExecutor(t)

	templateCtx := spec.NewTemplateContext()
	templateCtx.Networks["base"] = spec.NetworkTemplateData{
		InterfaceName: "br-base",
	}

	original := &v1.NetworkResource{
		Name:     "overlay",
		Kind:     "dnsmasq",
		Provider: "test",
		Spec: v1.NetworkSpec{
			AttachTo: "{{ .Networks.base.InterfaceName }}",
			Cidr:     "192.168.1.0/24",
		},
	}

	rendered, err := executor.renderNetworkSpec(original, templateCtx)
	if err != nil {
		t.Fatalf("renderNetworkSpec() error = %v", err)
	}

	// Verify original is unchanged
	if original.Spec.AttachTo != "{{ .Networks.base.InterfaceName }}" {
		t.Error("original spec was modified")
	}

	// Verify rendered value
	if rendered.Spec.AttachTo != "br-base" {
		t.Errorf("rendered AttachTo = %q, want br-base", rendered.Spec.AttachTo)
	}
}

func TestExecutor_renderVMSpec(t *testing.T) {
	executor := newTestExecutor(t)

	templateCtx := spec.NewTemplateContext()
	templateCtx.Keys["ssh"] = spec.KeyTemplateData{
		PublicKey:      "ssh-ed25519 AAAA...",
		PrivateKeyPath: "/tmp/ssh.key",
	}

	original := &v1.VMResource{
		Name:     "test-vm",
		Provider: "test",
		Spec: v1.VMSpec{
			Memory:  1024,
			Vcpus:   1,
			Network: "test-net",
			CloudInit: v1.CloudInitSpec{
				Users: []v1.UserSpec{
					{
						Name:              "test",
						SshAuthorizedKeys: []string{"{{ .Keys.ssh.PublicKey }}"},
					},
				},
			},
			Readiness: v1.ReadinessSpec{
				Ssh: v1.SSHReadinessSpec{
					Enabled:    true,
					Timeout:    "5m",
					PrivateKey: "{{ .Keys.ssh.PrivateKeyPath }}",
				},
			},
		},
	}

	rendered, err := executor.renderVMSpec(original, templateCtx)
	if err != nil {
		t.Fatalf("renderVMSpec() error = %v", err)
	}

	// Verify original is unchanged
	if original.Spec.CloudInit.Users[0].SshAuthorizedKeys[0] != "{{ .Keys.ssh.PublicKey }}" {
		t.Error("original spec was modified")
	}

	// Verify rendered values
	if rendered.Spec.CloudInit.Users[0].SshAuthorizedKeys[0] != "ssh-ed25519 AAAA..." {
		t.Errorf("rendered SSHAuthorizedKeys = %q, want ssh-ed25519 AAAA...",
			rendered.Spec.CloudInit.Users[0].SshAuthorizedKeys[0])
	}
	if rendered.Spec.Readiness.Ssh.PrivateKey != "/tmp/ssh.key" {
		t.Errorf("rendered PrivateKey = %q, want /tmp/ssh.key",
			rendered.Spec.Readiness.Ssh.PrivateKey)
	}
}

func TestExecutor_convertNetworkSpec(t *testing.T) {
	executor := newTestExecutor(t)

	spec := v1.NetworkSpec{
		Cidr:     "192.168.1.0/24",
		Gateway:  "192.168.1.1",
		AttachTo: "br0",
		Mtu:      1500,
		Dhcp: &v1.DHCPSpec{
			Enabled:    true,
			RangeStart: "192.168.1.100",
			RangeEnd:   "192.168.1.200",
			LeaseTime:  "12h",
			Router:     "192.168.1.1",
			DnsServers: []string{"8.8.8.8"},
		},
		Dns: &v1.DNSSpec{
			Enabled: true,
			Servers: []string{"8.8.8.8"},
		},
		Tftp: &v1.TFTPSpec{
			Enabled:  true,
			Root:     "/tftpboot",
			BootFile: "pxelinux.0",
		},
	}

	result := executor.convertNetworkSpec(spec)

	if result.CIDR != "192.168.1.0/24" {
		t.Errorf("CIDR = %s, want 192.168.1.0/24", result.CIDR)
	}
	if result.Gateway != "192.168.1.1" {
		t.Errorf("Gateway = %s, want 192.168.1.1", result.Gateway)
	}
	if result.DHCP == nil {
		t.Fatal("DHCP is nil")
	}
	if !result.DHCP.Enabled {
		t.Error("DHCP.Enabled should be true")
	}
	if result.DHCP.Router != "192.168.1.1" {
		t.Errorf("DHCP.Router = %s, want 192.168.1.1", result.DHCP.Router)
	}
	if len(result.DHCP.DNSServers) != 1 || result.DHCP.DNSServers[0] != "8.8.8.8" {
		t.Errorf("DHCP.DNSServers = %v, want [8.8.8.8]", result.DHCP.DNSServers)
	}
	if result.DNS == nil {
		t.Fatal("DNS is nil")
	}
	if result.TFTP == nil {
		t.Fatal("TFTP is nil")
	}
}

func TestExecutor_convertNetworkSpec_NilSubspecs(t *testing.T) {
	executor := newTestExecutor(t)

	spec := v1.NetworkSpec{
		Cidr: "192.168.1.0/24",
		// All sub-specs are nil (pointer types)
	}

	result := executor.convertNetworkSpec(spec)

	if result.DHCP != nil {
		t.Error("DHCP should be nil when Dhcp pointer is nil")
	}
	if result.DNS != nil {
		t.Error("DNS should be nil when Dns pointer is nil")
	}
	if result.TFTP != nil {
		t.Error("TFTP should be nil when Tftp pointer is nil")
	}
}

func TestExecutor_convertNetworkSpec_ExplicitDisable(t *testing.T) {
	executor := newTestExecutor(t)

	spec := v1.NetworkSpec{
		Cidr: "192.168.1.0/24",
		Dhcp: &v1.DHCPSpec{
			Enabled: false,
		},
	}

	result := executor.convertNetworkSpec(spec)

	if result.DHCP == nil {
		t.Fatal("DHCP should not be nil when explicitly set")
	}
	if result.DHCP.Enabled {
		t.Error("DHCP.Enabled should be false when explicitly disabled")
	}
}

func TestExecutor_convertVMSpec(t *testing.T) {
	executor := newTestExecutor(t)

	vmSpec := v1.VMSpec{
		Memory:  2048,
		Vcpus:   2,
		Network: "test-net",
		Disk: v1.DiskSpec{
			BaseImage: "/images/ubuntu.qcow2",
			Size:      "20G",
		},
		Boot: v1.BootSpec{
			Order:    []string{"hd", "network"},
			Firmware: "uefi",
		},
		CloudInit: v1.CloudInitSpec{
			Hostname: "test-vm",
			Packages: []string{"nginx", "curl"},
			Users: []v1.UserSpec{
				{
					Name: "admin",
					Sudo: "ALL=(ALL) NOPASSWD:ALL",
					SshAuthorizedKeys: []string{"ssh-ed25519 AAAA..."},
				},
			},
		},
		Readiness: v1.ReadinessSpec{
			Ssh: v1.SSHReadinessSpec{
				Enabled: true,
				Timeout: "5m",
				User:    "admin",
			},
		},
	}

	result := executor.convertVMSpec(vmSpec)

	if result.Memory != 2048 {
		t.Errorf("Memory = %d, want 2048", result.Memory)
	}
	if result.VCPUs != 2 {
		t.Errorf("VCPUs = %d, want 2", result.VCPUs)
	}
	if result.Network != "test-net" {
		t.Errorf("Network = %s, want test-net", result.Network)
	}
	if result.Disk.BaseImage != "/images/ubuntu.qcow2" {
		t.Errorf("Disk.BaseImage = %s, want /images/ubuntu.qcow2", result.Disk.BaseImage)
	}
	if result.Boot.Firmware != "uefi" {
		t.Errorf("Boot.Firmware = %s, want uefi", result.Boot.Firmware)
	}
	if result.CloudInit == nil {
		t.Fatal("CloudInit is nil")
	}
	if result.CloudInit.Hostname != "test-vm" {
		t.Errorf("CloudInit.Hostname = %s, want test-vm", result.CloudInit.Hostname)
	}
	if len(result.CloudInit.Users) != 1 {
		t.Fatalf("CloudInit.Users len = %d, want 1", len(result.CloudInit.Users))
	}
	if result.CloudInit.Users[0].Name != "admin" {
		t.Errorf("CloudInit.Users[0].Name = %s, want admin", result.CloudInit.Users[0].Name)
	}
	if result.Readiness == nil || result.Readiness.SSH == nil {
		t.Fatal("Readiness or Readiness.SSH is nil")
	}
}

func TestExecutor_convertVMSpec_NilSubspecs(t *testing.T) {
	executor := newTestExecutor(t)

	vmSpec := v1.VMSpec{
		Memory:  1024,
		Vcpus:   1,
		Network: "test-net",
		Disk:    v1.DiskSpec{Size: "10G"},
		Boot:    v1.BootSpec{Order: []string{"hd"}},
		// CloudInit and Readiness are nil
	}

	result := executor.convertVMSpec(vmSpec)

	if result.CloudInit != nil {
		t.Error("CloudInit should be nil")
	}
	if result.Readiness != nil {
		t.Error("Readiness should be nil")
	}
}

func TestExecutor_ExecuteCreate_SkipsEmptyPhases(t *testing.T) {
	stateDir := t.TempDir()
	manager := provider.NewManager()
	store := state.NewStore(stateDir)
	imageMgr, err := image.NewCacheManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create image cache manager: %v", err)
	}
	executor := NewExecutor(manager, store, imageMgr)

	ctx := context.Background()
	testenvSpec := &v1.Spec{}
	envState := &v1.EnvironmentState{
		ID:     "test-skip-empty",
		Status: v1.StatusCreating,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}
	templateCtx := spec.NewTemplateContext()

	// Plan with empty phases (no resources)
	plan := [][]v1.ResourceRef{
		{}, // Empty phase
		{}, // Another empty phase
	}

	// Save initial state
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	result, err := executor.ExecuteCreate(ctx, testenvSpec, plan, templateCtx, envState, nil, nil)
	if err != nil {
		t.Fatalf("ExecuteCreate() error = %v", err)
	}
	if !result.Success {
		t.Error("expected success for plan with empty phases")
	}
}

func TestExecutor_updateResourceState_InitializesMaps(t *testing.T) {
	executor := newTestExecutor(t)

	// Start with nil maps
	envState := &v1.EnvironmentState{
		ID:        "test-1",
		Status:    v1.StatusCreating,
		Resources: v1.ResourceMap{}, // All maps are nil initially
	}

	// Update should initialize the maps
	executor.updateResourceState(
		envState,
		v1.ResourceRef{Kind: "key", Name: "key1"},
		"provider1",
		v1.StatusReady,
		map[string]any{"publicKey": "test"},
		"",
	)

	if envState.Resources.Keys == nil {
		t.Error("Keys map should be initialized")
	}
	if envState.Resources.Keys["key1"] == nil {
		t.Error("key1 should be in Keys map")
	}

	executor.updateResourceState(
		envState,
		v1.ResourceRef{Kind: "network", Name: "net1"},
		"provider1",
		v1.StatusReady,
		map[string]any{"ip": "192.168.1.1"},
		"",
	)

	if envState.Resources.Networks == nil {
		t.Error("Networks map should be initialized")
	}

	executor.updateResourceState(
		envState,
		v1.ResourceRef{Kind: "vm", Name: "vm1"},
		"provider1",
		v1.StatusReady,
		map[string]any{"ip": "192.168.1.10"},
		"",
	)

	if envState.Resources.VMs == nil {
		t.Error("VMs map should be initialized")
	}
}

func TestExecutor_updateTemplateContext_AllKinds(t *testing.T) {
	executor := newTestExecutor(t)

	// Start with nil maps in template context
	templateCtx := &spec.TemplateContext{}

	// Update key - should initialize Keys map
	executor.updateTemplateContext(
		templateCtx,
		v1.ResourceRef{Kind: "key", Name: "key1"},
		map[string]any{
			"publicKey":      "ssh-key-content",
			"privateKeyPath": "/path/to/key",
		},
	)

	if templateCtx.Keys == nil {
		t.Error("Keys map should be initialized")
	}
	if templateCtx.Keys["key1"].PublicKey != "ssh-key-content" {
		t.Error("key data not set correctly")
	}

	// Update network - should initialize Networks map
	executor.updateTemplateContext(
		templateCtx,
		v1.ResourceRef{Kind: "network", Name: "net1"},
		map[string]any{
			"ip":   "192.168.1.1",
			"cidr": "192.168.1.0/24",
		},
	)

	if templateCtx.Networks == nil {
		t.Error("Networks map should be initialized")
	}

	// Update VM - should initialize VMs map
	executor.updateTemplateContext(
		templateCtx,
		v1.ResourceRef{Kind: "vm", Name: "vm1"},
		map[string]any{
			"ip":  "192.168.1.10",
			"mac": "52:54:00:00:00:01",
		},
	)

	if templateCtx.VMs == nil {
		t.Error("VMs map should be initialized")
	}
}

func TestExecutor_updateResourceState_UnknownKind(t *testing.T) {
	executor := newTestExecutor(t)

	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusCreating,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}

	// Update with unknown kind - should not panic
	executor.updateResourceState(
		envState,
		v1.ResourceRef{Kind: "unknown", Name: "test"},
		"provider1",
		v1.StatusReady,
		map[string]any{},
		"",
	)

	// Verify nothing was added to the maps
	if len(envState.Resources.Keys) != 0 {
		t.Error("unexpected key added")
	}
	if len(envState.Resources.Networks) != 0 {
		t.Error("unexpected network added")
	}
	if len(envState.Resources.VMs) != 0 {
		t.Error("unexpected vm added")
	}
}

func TestExecutor_updateTemplateContext_UnknownKind(t *testing.T) {
	executor := newTestExecutor(t)

	templateCtx := spec.NewTemplateContext()

	// Update with unknown kind - should not panic
	executor.updateTemplateContext(
		templateCtx,
		v1.ResourceRef{Kind: "unknown", Name: "test"},
		map[string]any{"data": "value"},
	)

	// Verify nothing was added
	if len(templateCtx.Keys) != 0 {
		t.Error("unexpected key data added")
	}
	if len(templateCtx.Networks) != 0 {
		t.Error("unexpected network data added")
	}
	if len(templateCtx.VMs) != 0 {
		t.Error("unexpected vm data added")
	}
}

