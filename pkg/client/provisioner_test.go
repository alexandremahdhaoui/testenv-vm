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

package client

import (
	"context"
	"os"
	"strings"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

// --- Mock Manager ---

type mockManager struct {
	providers map[string]*provider.ProviderInfo
}

func newMockManager() *mockManager {
	return &mockManager{
		providers: make(map[string]*provider.ProviderInfo),
	}
}

func (m *mockManager) addProvider(name string) {
	m.providers[name] = &provider.ProviderInfo{
		Config: v1.ProviderConfig{Name: name},
		Status: provider.StatusRunning,
	}
}

// --- Test Fixtures ---

func newTestEnvState(id string) *v1.EnvironmentState {
	return &v1.EnvironmentState{
		ID: id,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}
}

func newTestSpec(defaultProvider string, providers ...v1.ProviderConfig) *v1.Spec {
	return &v1.Spec{
		DefaultProvider: defaultProvider,
		Providers:       providers,
	}
}

// --- NewRuntimeProvisioner Tests ---

func TestNewRuntimeProvisioner_ResolvesDefaultProviderFromSpec(t *testing.T) {
	mgr := newMockManager()
	mgr.addProvider("libvirt")
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("libvirt", v1.ProviderConfig{Name: "libvirt"})

	rp, err := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rp.defaultProv != "libvirt" {
		t.Errorf("expected defaultProv 'libvirt', got %q", rp.defaultProv)
	}
}

func TestNewRuntimeProvisioner_FallsBackToDefaultTrueProvider(t *testing.T) {
	mgr := newMockManager()
	mgr.addProvider("stub")
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	tmplCtx := spec.NewTemplateContext()
	// No DefaultProvider set, but stub has Default: true
	testSpec := newTestSpec("", v1.ProviderConfig{Name: "stub", Default: true})

	rp, err := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rp.defaultProv != "stub" {
		t.Errorf("expected defaultProv 'stub', got %q", rp.defaultProv)
	}
}

func TestNewRuntimeProvisioner_ErrorsWhenNoDefaultProvider(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	tmplCtx := spec.NewTemplateContext()
	// No DefaultProvider and no provider with Default: true
	testSpec := newTestSpec("", v1.ProviderConfig{Name: "stub", Default: false})

	_, err := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})
	if err == nil {
		t.Fatal("expected error for no default provider")
	}
	if !strings.Contains(err.Error(), "no default provider found") {
		t.Errorf("expected error about no default provider, got: %v", err)
	}
}

// --- GetVMInfo Tests ---

func TestGetVMInfo_ReturnsErrorWhenVMNotFound(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	_, err := rp.GetVMInfo("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent VM")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestGetVMInfo_ReturnsErrorWhenVMNotReady(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	envState.Resources.VMs["test-vm"] = &v1.ResourceState{
		Status: v1.StatusCreating, // Not ready
	}
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	_, err := rp.GetVMInfo("test-vm")
	if err == nil {
		t.Fatal("expected error for VM not ready")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("expected 'not ready' error, got: %v", err)
	}
}

func TestGetVMInfo_ReturnsErrorWhenMissingIP(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	envState.Resources.VMs["test-vm"] = &v1.ResourceState{
		Status: v1.StatusReady,
		State: map[string]any{
			// Missing "ip"
			"sshUser":        "testuser",
			"privateKeyPath": "/tmp/key",
		},
	}
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	_, err := rp.GetVMInfo("test-vm")
	if err == nil {
		t.Fatal("expected error for missing IP")
	}
	if !strings.Contains(err.Error(), "missing ip") {
		t.Errorf("expected 'missing ip' error, got: %v", err)
	}
}

func TestGetVMInfo_ReturnsVMInfoSuccessfully(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := tmpDir + "/test-key"
	// Create a fake key file
	if err := writeTestFile(keyPath, "-----BEGIN OPENSSH PRIVATE KEY-----\nfakekey\n-----END OPENSSH PRIVATE KEY-----"); err != nil {
		t.Fatalf("failed to create test key: %v", err)
	}

	store := state.NewStore(tmpDir)
	envState := newTestEnvState("test-1")
	envState.Resources.VMs["test-vm"] = &v1.ResourceState{
		Status: v1.StatusReady,
		State: map[string]any{
			"ip":             "192.168.1.100",
			"sshUser":        "testuser",
			"privateKeyPath": keyPath,
		},
	}
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	vmInfo, err := rp.GetVMInfo("test-vm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vmInfo.Host != "192.168.1.100" {
		t.Errorf("expected Host '192.168.1.100', got %q", vmInfo.Host)
	}
	if vmInfo.User != "testuser" {
		t.Errorf("expected User 'testuser', got %q", vmInfo.User)
	}
	if vmInfo.Port != "22" {
		t.Errorf("expected Port '22', got %q", vmInfo.Port)
	}
}

// --- validateRuntimeVM Tests ---

func TestValidateRuntimeVM_RejectsInvalidMemory(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	envState.Resources.Networks["test-net"] = &v1.ResourceState{Status: v1.StatusReady}
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	err := rp.validateRuntimeVM("test-vm", v1.VMSpec{
		Memory:  0, // Invalid
		Vcpus:   1,
		Network: "test-net",
		Disk:    v1.DiskSpec{Size: "10G"},
	}, "")

	if err == nil {
		t.Fatal("expected error for invalid memory")
	}
	if !strings.Contains(err.Error(), "memory must be greater than 0") {
		t.Errorf("expected memory validation error, got: %v", err)
	}
}

func TestValidateRuntimeVM_RejectsInvalidDiskSize(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	envState.Resources.Networks["test-net"] = &v1.ResourceState{Status: v1.StatusReady}
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	err := rp.validateRuntimeVM("test-vm", v1.VMSpec{
		Memory:  512,
		Vcpus:   1,
		Network: "test-net",
		Disk:    v1.DiskSpec{Size: "invalid"},
	}, "")

	if err == nil {
		t.Fatal("expected error for invalid disk size")
	}
	if !strings.Contains(err.Error(), "disk.size") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected disk size validation error, got: %v", err)
	}
}

func TestValidateRuntimeVM_RejectsMissingNetwork(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	// No networks in state
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	err := rp.validateRuntimeVM("test-vm", v1.VMSpec{
		Memory:  512,
		Vcpus:   1,
		Network: "nonexistent-net",
		Disk:    v1.DiskSpec{Size: "10G"},
	}, "")

	if err == nil {
		t.Fatal("expected error for missing network")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected network not found error, got: %v", err)
	}
}

func TestValidateRuntimeVM_RejectsNetworkNotReady(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	envState.Resources.Networks["test-net"] = &v1.ResourceState{Status: v1.StatusFailed}
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	err := rp.validateRuntimeVM("test-vm", v1.VMSpec{
		Memory:  512,
		Vcpus:   1,
		Network: "test-net",
		Disk:    v1.DiskSpec{Size: "10G"},
	}, "")

	if err == nil {
		t.Fatal("expected error for network not ready")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("expected network not ready error, got: %v", err)
	}
}

// --- CreateVM Tests ---

func TestCreateVM_RejectsDuplicateVMName(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	envState.Resources.VMs["existing-vm"] = &v1.ResourceState{Status: v1.StatusReady}
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	_, err := rp.CreateVM(context.Background(), "existing-vm", v1.VMSpec{})
	if err == nil {
		t.Fatal("expected error for duplicate VM name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestCreateVM_CleansUpOnValidationFailure(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	// Invalid spec (memory = 0)
	_, err := rp.CreateVM(context.Background(), "test-vm", v1.VMSpec{
		Memory:  0,
		Vcpus:   1,
		Network: "test-net",
		Disk:    v1.DiskSpec{Size: "10G"},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	// Verify placeholder was cleaned up
	if _, exists := envState.Resources.VMs["test-vm"]; exists {
		t.Error("placeholder should have been cleaned up after validation failure")
	}
}

// --- DeleteVM Tests ---

func TestDeleteVM_ReturnsErrorForNonexistentVM(t *testing.T) {
	store := state.NewStore(t.TempDir())
	envState := newTestEnvState("test-1")
	tmplCtx := spec.NewTemplateContext()
	testSpec := newTestSpec("stub", v1.ProviderConfig{Name: "stub", Default: true})

	rp, _ := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     &provider.Manager{},
		Store:       store,
		EnvState:    envState,
		TemplateCtx: tmplCtx,
		Spec:        testSpec,
	})

	err := rp.DeleteVM(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent VM")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// --- convertVMSpec Tests ---

func TestConvertVMSpec_ConvertsCloudInitCorrectly(t *testing.T) {
	input := v1.VMSpec{
		Memory:  1024,
		Vcpus:   2,
		Network: "test-net",
		Disk: v1.DiskSpec{
			BaseImage: "/path/to/image.qcow2",
			Size:      "20G",
		},
		CloudInit: v1.CloudInitSpec{
			Hostname: "my-hostname",
			Packages: []string{"curl", "vim"},
			Users: []v1.UserSpec{
				{
					Name: "admin",
					Sudo: "ALL=(ALL) NOPASSWD:ALL",
					SshAuthorizedKeys: []string{
						"ssh-ed25519 AAAA...",
					},
				},
				{
					Name: "regular",
				},
			},
		},
	}

	result := convertVMSpec(input)

	if result.Memory != 1024 {
		t.Errorf("expected Memory 1024, got %d", result.Memory)
	}
	if result.VCPUs != 2 {
		t.Errorf("expected VCPUs 2, got %d", result.VCPUs)
	}
	if result.Network != "test-net" {
		t.Errorf("expected Network 'test-net', got %q", result.Network)
	}
	if result.Disk.BaseImage != "/path/to/image.qcow2" {
		t.Errorf("expected BaseImage, got %q", result.Disk.BaseImage)
	}
	if result.CloudInit == nil {
		t.Fatal("expected CloudInit to be set")
	}
	if result.CloudInit.Hostname != "my-hostname" {
		t.Errorf("expected Hostname 'my-hostname', got %q", result.CloudInit.Hostname)
	}
	if len(result.CloudInit.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(result.CloudInit.Users))
	}
	if result.CloudInit.Users[0].Name != "admin" {
		t.Errorf("expected first user 'admin', got %q", result.CloudInit.Users[0].Name)
	}
	if result.CloudInit.Users[0].Sudo != "ALL=(ALL) NOPASSWD:ALL" {
		t.Errorf("expected sudo config, got %q", result.CloudInit.Users[0].Sudo)
	}
	if len(result.CloudInit.Users[0].SSHAuthorizedKeys) != 1 {
		t.Errorf("expected 1 SSH key, got %d", len(result.CloudInit.Users[0].SSHAuthorizedKeys))
	}
}

func TestConvertVMSpec_HandlesNilCloudInit(t *testing.T) {
	input := v1.VMSpec{
		Memory:    512,
		Vcpus:     1,
		Network:   "test-net",
		Disk:      v1.DiskSpec{Size: "10G"},
		CloudInit: v1.CloudInitSpec{},
	}

	result := convertVMSpec(input)

	if result.CloudInit != nil {
		t.Error("expected CloudInit to be nil")
	}
}

// --- Helper Functions ---

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0600)
}
