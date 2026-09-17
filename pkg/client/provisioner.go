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

var _ ClientProvider = (*RuntimeProvisioner)(nil)

type RuntimeProvisioner struct {
	manager     *provider.Manager
	store       *state.Store
	envState    *v1.EnvironmentState
	templateCtx *spec.TemplateContext
	defaultProv string
	mu          sync.RWMutex
}

type RuntimeProvisionerConfig struct {
	Manager     *provider.Manager
	Store       *state.Store
	EnvState    *v1.EnvironmentState
	TemplateCtx *spec.TemplateContext
	Spec        *v1.Spec
}

func NewRuntimeProvisioner(cfg RuntimeProvisionerConfig) (*RuntimeProvisioner, error) {
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

	defaultProv := cfg.Spec.DefaultProvider
	if defaultProv == "" {
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

func (rp *RuntimeProvisioner) GetVMInfo(vmName string) (*VMInfo, error) {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	if rp.envState.Resources.VMs == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q not found (no VMs in state)", vmName)
	}

	resourceState, exists := rp.envState.Resources.VMs[vmName]
	if !exists {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q not found", vmName)
	}

	if resourceState.Status != v1.StatusReady {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q is not ready (status: %s)", vmName, resourceState.Status)
	}

	state := resourceState.State
	if state == nil {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q has no state data", vmName)
	}

	ip, ok := state["ip"].(string)
	if !ok || ip == "" {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q missing ip in state", vmName)
	}

	sshUser, ok := state["sshUser"].(string)
	if !ok || sshUser == "" {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q missing sshUser in state", vmName)
	}

	privateKeyPath, ok := state["privateKeyPath"].(string)
	if !ok || privateKeyPath == "" {
		return nil, fmt.Errorf("RuntimeProvisioner: VM %q missing privateKeyPath in state", vmName)
	}

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

func (rp *RuntimeProvisioner) GetTemplateContext() *spec.TemplateContext {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	return rp.templateCtx
}

func (rp *RuntimeProvisioner) renderVMSpec(vmSpec v1.VMSpec) (v1.VMSpec, error) {
	data, err := json.Marshal(vmSpec)
	if err != nil {
		return v1.VMSpec{}, fmt.Errorf("failed to marshal VM spec: %w", err)
	}
	var copy v1.VMSpec
	if err := json.Unmarshal(data, &copy); err != nil {
		return v1.VMSpec{}, fmt.Errorf("failed to unmarshal VM spec: %w", err)
	}

	if err := spec.RenderSpec(&copy, rp.templateCtx); err != nil {
		return v1.VMSpec{}, fmt.Errorf("failed to render templates: %w", err)
	}

	return copy, nil
}

var diskSizePattern = regexp.MustCompile(`^[1-9][0-9]*[GMTK]$`)

func (rp *RuntimeProvisioner) validateRuntimeVM(name string, vmSpec v1.VMSpec, providerName string) error {
	if vmSpec.Memory <= 0 {
		return fmt.Errorf("VM %q: memory must be greater than 0", name)
	}
	if vmSpec.Vcpus <= 0 {
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

	resolvedProvider := providerName
	if resolvedProvider == "" {
		resolvedProvider = rp.defaultProv
	}
	if _, exists := rp.manager.GetInfo(resolvedProvider); !exists {
		return fmt.Errorf("VM %q: provider %q not found", name, resolvedProvider)
	}

	return nil
}

func convertVMSpec(spec v1.VMSpec) providerv1.VMSpec {
	result := providerv1.VMSpec{
		Memory:  spec.Memory,
		VCPUs:   spec.Vcpus,
		Network: spec.Network,
		Disk: providerv1.DiskSpec{
			BaseImage: spec.Disk.BaseImage,
			Size:      spec.Disk.Size,
			Bus:       spec.Disk.Bus,
			WWN:       spec.Disk.Wwn,
		},
		Boot: providerv1.BootSpec{
			Order:    spec.Boot.Order,
			Firmware: spec.Boot.Firmware,
		},
	}

	if spec.CloudInit.Hostname != "" || len(spec.CloudInit.Users) > 0 || len(spec.CloudInit.Packages) > 0 {
		result.CloudInit = &providerv1.CloudInitSpec{
			Hostname: spec.CloudInit.Hostname,
			Packages: spec.CloudInit.Packages,
		}
		for _, u := range spec.CloudInit.Users {
			result.CloudInit.Users = append(result.CloudInit.Users, providerv1.UserSpec{
				Name:              u.Name,
				Sudo:              u.Sudo,
				SSHAuthorizedKeys: u.SshAuthorizedKeys,
			})
		}
	}

	if spec.Readiness.Ssh.Enabled {
		result.Readiness = &providerv1.ReadinessSpec{
			SSH: &providerv1.SSHReadinessSpec{
				Enabled:    spec.Readiness.Ssh.Enabled,
				Timeout:    spec.Readiness.Ssh.Timeout,
				User:       spec.Readiness.Ssh.User,
				PrivateKey: spec.Readiness.Ssh.PrivateKey,
			},
		}
	}

	return result
}

func (rp *RuntimeProvisioner) CreateVM(ctx context.Context, name string, vmSpec v1.VMSpec) (*Client, error) {
	rp.mu.Lock()
	if rp.envState.Resources.VMs == nil {
		rp.envState.Resources.VMs = make(map[string]*v1.ResourceState)
	}
	if _, exists := rp.envState.Resources.VMs[name]; exists {
		rp.mu.Unlock()
		return nil, fmt.Errorf("CreateVM: VM %q already exists", name)
	}
	rp.envState.Resources.VMs[name] = &v1.ResourceState{
		Provider:  rp.defaultProv,
		Status:    v1.StatusCreating,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	rp.mu.Unlock()

	renderedSpec, err := rp.renderVMSpec(vmSpec)
	if err != nil {
		rp.mu.Lock()
		delete(rp.envState.Resources.VMs, name)
		rp.mu.Unlock()
		return nil, fmt.Errorf("CreateVM: %w", err)
	}

	if err := rp.validateRuntimeVM(name, renderedSpec, ""); err != nil {
		rp.mu.Lock()
		delete(rp.envState.Resources.VMs, name)
		rp.mu.Unlock()
		return nil, fmt.Errorf("CreateVM: %w", err)
	}

	request := &providerv1.VMCreateRequest{
		Name:         name,
		Spec:         convertVMSpec(renderedSpec),
		ProviderSpec: nil,
	}

	result, err := rp.manager.Call(rp.defaultProv, "vm_create", request)

	rp.mu.Lock()
	defer rp.mu.Unlock()

	if err != nil {
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

	resourceState, err := convertResourceToMap(result.Resource)
	if err != nil {
		rp.envState.Resources.VMs[name].Status = v1.StatusFailed
		rp.envState.Resources.VMs[name].Error = err.Error()
		rp.envState.Resources.VMs[name].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = rp.store.Save(rp.envState)
		return nil, fmt.Errorf("CreateVM: failed to convert resource state: %w", err)
	}

	sshUser := "root"
	if len(renderedSpec.CloudInit.Users) > 0 {
		if renderedSpec.CloudInit.Users[0].Name != "" {
			sshUser = renderedSpec.CloudInit.Users[0].Name
		}
	}

	privateKeyPath := ""
	if len(renderedSpec.CloudInit.Users) > 0 {
		for _, keyData := range rp.templateCtx.Keys {
			for _, authKey := range renderedSpec.CloudInit.Users[0].SshAuthorizedKeys {
				if authKey == keyData.PublicKey {
					privateKeyPath = keyData.PrivateKeyPath
					break
				}
			}
			if privateKeyPath != "" {
				break
			}
		}
	}

	if resourceState == nil {
		resourceState = make(map[string]any)
	}
	resourceState["sshUser"] = sshUser
	resourceState["privateKeyPath"] = privateKeyPath

	now := time.Now().UTC().Format(time.RFC3339)
	rp.envState.Resources.VMs[name] = &v1.ResourceState{
		Provider:  rp.defaultProv,
		Status:    v1.StatusReady,
		State:     resourceState,
		CreatedAt: now,
		UpdatedAt: now,
	}

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

	rp.envState.UpdatedAt = now
	if err := rp.store.Save(rp.envState); err != nil {
		return nil, fmt.Errorf("CreateVM: failed to save state: %w", err)
	}

	client, err := NewClient(rp, name, WithProvisioner(rp))
	if err != nil {
		return nil, fmt.Errorf("CreateVM: failed to create client: %w", err)
	}

	return client, nil
}

func convertResourceToMap(resource any) (map[string]any, error) {
	if resource == nil {
		return nil, nil
	}

	if m, ok := resource.(map[string]any); ok {
		return m, nil
	}

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

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (rp *RuntimeProvisioner) DeleteVM(ctx context.Context, name string) error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if rp.envState.Resources.VMs == nil {
		return fmt.Errorf("DeleteVM: VM %q not found (no VMs in state)", name)
	}

	resourceState, exists := rp.envState.Resources.VMs[name]
	if !exists {
		return fmt.Errorf("DeleteVM: VM %q not found", name)
	}

	providerName := resourceState.Provider
	if providerName == "" {
		providerName = rp.defaultProv
	}

	request := &providerv1.DeleteRequest{
		Name: name,
	}

	_, err := rp.manager.Call(providerName, "vm_delete", request)

	now := time.Now().UTC().Format(time.RFC3339)
	resourceState.Status = v1.StatusDestroyed
	resourceState.UpdatedAt = now

	rp.envState.UpdatedAt = now
	if saveErr := rp.store.Save(rp.envState); saveErr != nil {
		if err != nil {
			return fmt.Errorf("DeleteVM: provider call failed: %v; failed to save state: %w", err, saveErr)
		}
		return fmt.Errorf("DeleteVM: failed to save state: %w", saveErr)
	}

	if err != nil {
		return fmt.Errorf("DeleteVM: provider call failed (state updated to destroyed): %w", err)
	}

	return nil
}
