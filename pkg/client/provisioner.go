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
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

// Compile-time check that RuntimeProvisioner implements ClientProvider.
var _ ClientProvider = (*RuntimeProvisioner)(nil)

// RuntimeProvisioner enables runtime VM creation during tests.
// It implements ClientProvider to provide VM info lookup, and additionally
// supports creating and deleting VMs at runtime.
//
// Thread safety: RuntimeProvisioner uses a RWMutex for thread safety during
// the TEST PHASE. Ownership of envState is transferred from the Orchestrator
// to RuntimeProvisioner after Create() returns.
type RuntimeProvisioner struct {
	manager     *provider.Manager
	store       *state.Store
	envState    *v1.EnvironmentState
	templateCtx *spec.TemplateContext
	defaultProv string
	mu          sync.RWMutex
}

// RuntimeProvisionerConfig contains configuration for NewRuntimeProvisioner.
type RuntimeProvisionerConfig struct {
	// Manager is the provider manager for MCP calls.
	Manager *provider.Manager
	// Store is the state store for persistence.
	Store *state.Store
	// EnvState is the shared environment state reference.
	EnvState *v1.EnvironmentState
	// TemplateCtx is the template context for rendering.
	TemplateCtx *spec.TemplateContext
	// Spec is the testenv specification for provider resolution.
	Spec *v1.TestenvSpec
}

// NewRuntimeProvisioner creates a new RuntimeProvisioner from the given configuration.
// It resolves the default provider from spec.DefaultProvider or the first provider
// marked as Default: true. Returns an error if no default provider can be determined.
func NewRuntimeProvisioner(cfg RuntimeProvisionerConfig) (*RuntimeProvisioner, error) {
	// Validate required fields
	if cfg.Manager == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: manager is required")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: store is required")
	}
	if cfg.EnvState == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: envState is required")
	}
	if cfg.TemplateCtx == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: templateCtx is required")
	}
	if cfg.Spec == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: spec is required")
	}

	// Resolve default provider
	defaultProv := cfg.Spec.DefaultProvider
	if defaultProv == "" {
		// Find first provider with Default: true
		for _, p := range cfg.Spec.Providers {
			if p.Default {
				defaultProv = p.Name
				break
			}
		}
	}

	if defaultProv == "" {
		return nil, fmt.Errorf("RuntimeProvisioner: no default provider found; set spec.DefaultProvider or mark a provider with Default: true")
	}

	return &RuntimeProvisioner{
		manager:     cfg.Manager,
		store:       cfg.Store,
		envState:    cfg.EnvState,
		templateCtx: cfg.TemplateCtx,
		defaultProv: defaultProv,
	}, nil
}

// GetVMInfo implements ClientProvider interface.
// It returns connection information for a VM by looking up state.
func (rp *RuntimeProvisioner) GetVMInfo(vmName string) (*VMInfo, error) {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	// Look up VM in envState.Resources.VMs
	if rp.envState.Resources.VMs == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q not found (no VMs in state)", vmName)
	}

	resourceState, exists := rp.envState.Resources.VMs[vmName]
	if !exists {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q not found", vmName)
	}

	// Check VM status
	if resourceState.Status != v1.StatusReady {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q is not ready (status: %s)", vmName, resourceState.Status)
	}

	// Extract connection info from resourceState.State
	state := resourceState.State
	if state == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q has no state data", vmName)
	}

	// Extract IP address
	ip, ok := state["ip"].(string)
	if !ok || ip == "" {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q missing ip in state", vmName)
	}

	// Extract SSH user (stored during CreateVM from cloud-init config)
	sshUser, ok := state["sshUser"].(string)
	if !ok || sshUser == "" {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q missing sshUser in state", vmName)
	}

	// Extract private key path (stored during CreateVM)
	privateKeyPath, ok := state["privateKeyPath"].(string)
	if !ok || privateKeyPath == "" {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q missing privateKeyPath in state", vmName)
	}

	// Read private key content from file
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q failed to read private key from %q: %w", vmName, privateKeyPath, err)
	}

	return &VMInfo{
		Host:       ip,
		Port:       "22",
		User:       sshUser,
		PrivateKey: privateKey,
	}, nil
}

// GetTemplateContext returns the template context for accessing resource data.
// This allows test code to inspect created resources, build template strings
// dynamically, and access key paths, network info, etc.
func (rp *RuntimeProvisioner) GetTemplateContext() *spec.TemplateContext {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	return rp.templateCtx
}

// renderVMSpec creates a deep copy and renders templates in a VM spec.
// Uses JSON marshal/unmarshal for deep copy to avoid shared pointer issues.
func (rp *RuntimeProvisioner) renderVMSpec(vmSpec v1.VMSpec) (v1.VMSpec, error) {
	// Deep copy via JSON
	data, err := json.Marshal(vmSpec)
	if err != nil {
		return v1.VMSpec{}, fmt.Errorf("failed to marshal VM spec: %w", err)
	}
	var copy v1.VMSpec
	if err := json.Unmarshal(data, &copy); err != nil {
		return v1.VMSpec{}, fmt.Errorf("failed to unmarshal VM spec: %w", err)
	}

	// Render templates
	if err := spec.RenderSpec(&copy, rp.templateCtx); err != nil {
		return v1.VMSpec{}, fmt.Errorf("failed to render templates: %w", err)
	}

	return copy, nil
}

// diskSizePattern validates disk size format like "10G", "100M", "1T".
var diskSizePattern = regexp.MustCompile(`^[1-9][0-9]*[GMTK]$`)

// validateRuntimeVM validates a runtime VM spec after template rendering.
// It verifies referenced resources exist in the current environment state.
func (rp *RuntimeProvisioner) validateRuntimeVM(name string, vmSpec v1.VMSpec, providerName string) error {
	// Validate required fields
	if vmSpec.Memory <= 0 {
		return fmt.Errorf("VM %q: memory must be greater than 0", name)
	}
	if vmSpec.VCPUs <= 0 {
		return fmt.Errorf("VM %q: vcpus must be greater than 0", name)
	}
	if vmSpec.Disk.Size == "" {
		return fmt.Errorf("VM %q: disk.size is required", name)
	}
	if !diskSizePattern.MatchString(vmSpec.Disk.Size) {
		return fmt.Errorf("VM %q: disk.size %q is invalid (expected format like '10G', '100M')", name, vmSpec.Disk.Size)
	}
	if vmSpec.Network == "" {
		return fmt.Errorf("VM %q: network is required", name)
	}

	// Verify the referenced network exists in state
	if rp.envState.Resources.Networks == nil {
		return fmt.Errorf("VM %q: network %q not found (no networks in state)", name, vmSpec.Network)
	}
	networkState, exists := rp.envState.Resources.Networks[vmSpec.Network]
	if !exists {
		return fmt.Errorf("VM %q: network %q not found in state", name, vmSpec.Network)
	}
	if networkState.Status != v1.StatusReady {
		return fmt.Errorf("VM %q: network %q is not ready (status: %s)", name, vmSpec.Network, networkState.Status)
	}

	// Verify provider exists or use default
	resolvedProvider := providerName
	if resolvedProvider == "" {
		resolvedProvider = rp.defaultProv
	}
	if _, exists := rp.manager.GetInfo(resolvedProvider); !exists {
		return fmt.Errorf("VM %q: provider %q not found", name, resolvedProvider)
	}

	return nil
}

// convertVMSpec converts v1.VMSpec to providerv1.VMSpec.
// This is similar to executor.convertVMSpec but operates on v1.VMSpec directly.
func convertVMSpec(spec v1.VMSpec) providerv1.VMSpec {
	result := providerv1.VMSpec{
		Memory:  spec.Memory,
		VCPUs:   spec.VCPUs,
		Network: spec.Network,
		Disk: providerv1.DiskSpec{
			BaseImage: spec.Disk.BaseImage,
			Size:      spec.Disk.Size,
		},
		Boot: providerv1.BootSpec{
			Order:    spec.Boot.Order,
			Firmware: spec.Boot.Firmware,
		},
	}

	if spec.CloudInit != nil {
		result.CloudInit = &providerv1.CloudInitSpec{
			Hostname: spec.CloudInit.Hostname,
			Packages: spec.CloudInit.Packages,
		}
		for _, u := range spec.CloudInit.Users {
			result.CloudInit.Users = append(result.CloudInit.Users, providerv1.UserSpec{
				Name:              u.Name,
				Sudo:              u.Sudo,
				SSHAuthorizedKeys: u.SSHAuthorizedKeys,
			})
		}
	}

	if spec.Readiness != nil && spec.Readiness.SSH != nil {
		result.Readiness = &providerv1.ReadinessSpec{
			SSH: &providerv1.SSHReadinessSpec{
				Enabled:    spec.Readiness.SSH.Enabled,
				Timeout:    spec.Readiness.SSH.Timeout,
				User:       spec.Readiness.SSH.User,
				PrivateKey: spec.Readiness.SSH.PrivateKey,
			},
		}
	}

	return result
}

// CreateVM creates a new VM at runtime and returns a Client for it.
// The returned Client can be used to interact with the VM.
// This method uses a lock-release-lock pattern to avoid holding mutex during network I/O.
func (rp *RuntimeProvisioner) CreateVM(ctx context.Context, name string, vmSpec v1.VMSpec) (*Client, error) {
	// Phase 1: Pre-flight checks (under lock)
	rp.mu.Lock()
	// Check if VM name already exists
	if rp.envState.Resources.VMs == nil {
		rp.envState.Resources.VMs = make(map[string]*v1.ResourceState)
	}
	if _, exists := rp.envState.Resources.VMs[name]; exists {
		rp.mu.Unlock()
		return nil, fmt.Errorf("CreateVM: VM %q already exists", name)
	}
	// Reserve the VM name by creating a placeholder entry with status "creating"
	rp.envState.Resources.VMs[name] = &v1.ResourceState{
		Provider:  rp.defaultProv,
		Status:    v1.StatusCreating,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	rp.mu.Unlock()

	// Phase 2: Template rendering and validation (no lock - CPU only)
	renderedSpec, err := rp.renderVMSpec(vmSpec)
	if err != nil {
		// Clean up placeholder on error
		rp.mu.Lock()
		delete(rp.envState.Resources.VMs, name)
		rp.mu.Unlock()
		return nil, fmt.Errorf("CreateVM: %w", err)
	}

	// Validate rendered spec
	if err := rp.validateRuntimeVM(name, renderedSpec, ""); err != nil {
		// Clean up placeholder on error
		rp.mu.Lock()
		delete(rp.envState.Resources.VMs, name)
		rp.mu.Unlock()
		return nil, fmt.Errorf("CreateVM: %w", err)
	}

	// Phase 3: Provider call (no lock - network I/O)
	request := &providerv1.VMCreateRequest{
		Name:         name,
		Spec:         convertVMSpec(renderedSpec),
		ProviderSpec: nil, // Runtime VMs don't support ProviderSpec
	}

	result, err := rp.manager.Call(rp.defaultProv, "vm_create", request)

	// Phase 4: State update (under lock)
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if err != nil {
		// Update state to failed
		rp.envState.Resources.VMs[name].Status = v1.StatusFailed
		rp.envState.Resources.VMs[name].Error = err.Error()
		rp.envState.Resources.VMs[name].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if saveErr := rp.store.Save(rp.envState); saveErr != nil {
			return nil, fmt.Errorf("CreateVM: provider call failed: %v; failed to save state: %w", err, saveErr)
		}
		return nil, fmt.Errorf("CreateVM: provider call failed: %w", err)
	}

	if !result.Success {
		errMsg := "unknown error"
		if result.Error != nil {
			errMsg = result.Error.Message
		}
		rp.envState.Resources.VMs[name].Status = v1.StatusFailed
		rp.envState.Resources.VMs[name].Error = errMsg
		rp.envState.Resources.VMs[name].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if saveErr := rp.store.Save(rp.envState); saveErr != nil {
			return nil, fmt.Errorf("CreateVM: provider returned error: %s; failed to save state: %w", errMsg, saveErr)
		}
		return nil, fmt.Errorf("CreateVM: provider returned error: %s", errMsg)
	}

	// Convert resource state to map
	resourceState, err := convertResourceToMap(result.Resource)
	if err != nil {
		rp.envState.Resources.VMs[name].Status = v1.StatusFailed
		rp.envState.Resources.VMs[name].Error = err.Error()
		rp.envState.Resources.VMs[name].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = rp.store.Save(rp.envState)
		return nil, fmt.Errorf("CreateVM: failed to convert resource state: %w", err)
	}

	// Extract SSH user from cloud-init config (or "root" if not specified)
	sshUser := "root"
	if renderedSpec.CloudInit != nil && len(renderedSpec.CloudInit.Users) > 0 {
		if renderedSpec.CloudInit.Users[0].Name != "" {
			sshUser = renderedSpec.CloudInit.Users[0].Name
		}
	}

	// Find private key path from templateCtx.Keys
	privateKeyPath := ""
	if renderedSpec.CloudInit != nil && len(renderedSpec.CloudInit.Users) > 0 {
		// Look for the first key that matches an SSH authorized key
		for keyName, keyData := range rp.templateCtx.Keys {
			for _, authKey := range renderedSpec.CloudInit.Users[0].SSHAuthorizedKeys {
				if authKey == keyData.PublicKey {
					privateKeyPath = keyData.PrivateKeyPath
					_ = keyName // suppress unused variable warning
					break
				}
			}
			if privateKeyPath != "" {
				break
			}
		}
	}

	// Store sshUser and privateKeyPath in resourceState for GetVMInfo to use
	if resourceState == nil {
		resourceState = make(map[string]any)
	}
	resourceState["sshUser"] = sshUser
	resourceState["privateKeyPath"] = privateKeyPath

	// Update state with success
	now := time.Now().UTC().Format(time.RFC3339)
	rp.envState.Resources.VMs[name] = &v1.ResourceState{
		Provider:  rp.defaultProv,
		Status:    v1.StatusReady,
		State:     resourceState,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Append new phase to ExecutionPlan.Phases with single VM ResourceRef
	if rp.envState.ExecutionPlan == nil {
		rp.envState.ExecutionPlan = &v1.ExecutionPlan{
			Phases: []v1.Phase{},
		}
	}
	rp.envState.ExecutionPlan.Phases = append(rp.envState.ExecutionPlan.Phases, v1.Phase{
		Resources: []v1.ResourceRef{{
			Kind:     "vm",
			Name:     name,
			Provider: rp.defaultProv,
		}},
	})

	// Update templateCtx.VMs with VMTemplateData
	if rp.templateCtx.VMs == nil {
		rp.templateCtx.VMs = make(map[string]spec.VMTemplateData)
	}
	ip := getString(resourceState, "ip")
	mac := getString(resourceState, "mac")
	rp.templateCtx.VMs[name] = spec.VMTemplateData{
		Name:       name,
		IP:         ip,
		MAC:        mac,
		SSHCommand: getString(resourceState, "sshCommand"),
	}

	// Persist state
	rp.envState.UpdatedAt = now
	if err := rp.store.Save(rp.envState); err != nil {
		return nil, fmt.Errorf("CreateVM: failed to save state: %w", err)
	}

	// Create and return new Client for the VM with provisioner attached
	// so the returned Client can also create VMs.
	client, err := NewClient(rp, name, WithProvisioner(rp))
	if err != nil {
		return nil, fmt.Errorf("CreateVM: failed to create client: %w", err)
	}

	return client, nil
}

// convertResourceToMap converts a resource to a map[string]any.
func convertResourceToMap(resource any) (map[string]any, error) {
	if resource == nil {
		return nil, nil
	}

	// If it's already a map, return it
	if m, ok := resource.(map[string]any); ok {
		return m, nil
	}

	// Otherwise, convert via JSON
	data, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// getString safely extracts a string from a map.
func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// DeleteVM deletes a runtime-created VM.
// This is optional - VMs are automatically cleaned up when the environment is deleted.
func (rp *RuntimeProvisioner) DeleteVM(ctx context.Context, name string) error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	// Look up VM in envState.Resources.VMs
	if rp.envState.Resources.VMs == nil {
		return fmt.Errorf("DeleteVM: VM %q not found (no VMs in state)", name)
	}

	resourceState, exists := rp.envState.Resources.VMs[name]
	if !exists {
		return fmt.Errorf("DeleteVM: VM %q not found", name)
	}

	// Get provider name from ResourceState.Provider
	providerName := resourceState.Provider
	if providerName == "" {
		providerName = rp.defaultProv
	}

	// Build DeleteRequest
	request := &providerv1.DeleteRequest{
		Name: name,
	}

	// Call provider to delete VM (best effort - continue on error)
	_, err := rp.manager.Call(providerName, "vm_delete", request)
	// Error will be returned after state is updated - we continue to mark as destroyed

	// Update state to destroyed
	now := time.Now().UTC().Format(time.RFC3339)
	resourceState.Status = v1.StatusDestroyed
	resourceState.UpdatedAt = now

	// Persist state
	rp.envState.UpdatedAt = now
	if saveErr := rp.store.Save(rp.envState); saveErr != nil {
		if err != nil {
			return fmt.Errorf("DeleteVM: provider call failed: %v; failed to save state: %w", err, saveErr)
		}
		return fmt.Errorf("DeleteVM: failed to save state: %w", saveErr)
	}

	// Return original error if provider call failed
	if err != nil {
		return fmt.Errorf("DeleteVM: provider call failed (state updated to destroyed): %w", err)
	}

	return nil
}
