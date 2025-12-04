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

// Package stub provides a stub provider that simulates real provider behavior
// for E2E testing without real infrastructure. This provider stores resources
// in memory and returns mock values.
package stub

import (
	"fmt"
	"sync"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// Provider is a stub provider that stores resources in memory for testing.
type Provider struct {
	mu       sync.RWMutex
	keys     map[string]*providerv1.KeyState
	networks map[string]*providerv1.NetworkState
	vms      map[string]*providerv1.VMState
}

// NewProvider creates a new stub provider with initialized maps.
func NewProvider() *Provider {
	return &Provider{
		keys:     make(map[string]*providerv1.KeyState),
		networks: make(map[string]*providerv1.NetworkState),
		vms:      make(map[string]*providerv1.VMState),
	}
}

// Capabilities returns the capabilities of the stub provider.
func (p *Provider) Capabilities() *providerv1.CapabilitiesResponse {
	return &providerv1.CapabilitiesResponse{
		ProviderName: "stub",
		Version:      "1.0.0",
		Resources: []providerv1.ResourceCapability{
			{Kind: "key", Operations: []string{"create", "get", "list", "delete"}},
			{Kind: "network", Operations: []string{"create", "get", "list", "delete"}},
			{Kind: "vm", Operations: []string{"create", "get", "list", "delete"}},
		},
	}
}

// KeyCreate creates a mock key and stores it in memory.
func (p *Provider) KeyCreate(req *providerv1.KeyCreateRequest) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.keys[req.Name]; exists {
		return providerv1.ErrorResult(providerv1.NewAlreadyExistsError("key", req.Name))
	}

	keyType := req.Spec.Type
	if keyType == "" {
		keyType = "ed25519"
	}

	state := &providerv1.KeyState{
		Name:           req.Name,
		Type:           keyType,
		PublicKey:      fmt.Sprintf("ssh-%s MOCK%s %s", keyType, req.Name, req.Spec.Comment),
		PublicKeyPath:  fmt.Sprintf("/tmp/stub-keys/%s.pub", req.Name),
		PrivateKeyPath: fmt.Sprintf("/tmp/stub-keys/%s", req.Name),
		Fingerprint:    fmt.Sprintf("SHA256:MOCK%s", req.Name),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	p.keys[req.Name] = state
	return providerv1.SuccessResult(state)
}

// KeyGet retrieves a key by name.
func (p *Provider) KeyGet(name string) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key, exists := p.keys[name]
	if !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("key", name))
	}

	return providerv1.SuccessResult(key)
}

// KeyList lists all keys, optionally filtered.
func (p *Provider) KeyList(filter map[string]any) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	keys := make([]*providerv1.KeyState, 0, len(p.keys))
	for _, key := range p.keys {
		keys = append(keys, key)
	}

	return providerv1.SuccessResult(keys)
}

// KeyDelete deletes a key by name.
func (p *Provider) KeyDelete(name string) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.keys[name]; !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("key", name))
	}

	delete(p.keys, name)
	return providerv1.SuccessResult(nil)
}

// NetworkCreate creates a mock network and stores it in memory.
func (p *Provider) NetworkCreate(req *providerv1.NetworkCreateRequest) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.networks[req.Name]; exists {
		return providerv1.ErrorResult(providerv1.NewAlreadyExistsError("network", req.Name))
	}

	kind := req.Kind
	if kind == "" {
		kind = "bridge"
	}

	state := &providerv1.NetworkState{
		Name:          req.Name,
		Kind:          kind,
		Status:        "ready",
		IP:            "192.168.100.1",
		CIDR:          req.Spec.CIDR,
		InterfaceName: fmt.Sprintf("stub-br-%s", req.Name),
		UUID:          fmt.Sprintf("stub-net-%s", req.Name),
	}

	// Use CIDR from spec if provided
	if state.CIDR == "" {
		state.CIDR = "192.168.100.0/24"
	}

	p.networks[req.Name] = state
	return providerv1.SuccessResult(state)
}

// NetworkGet retrieves a network by name.
func (p *Provider) NetworkGet(name string) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	network, exists := p.networks[name]
	if !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("network", name))
	}

	return providerv1.SuccessResult(network)
}

// NetworkList lists all networks, optionally filtered.
func (p *Provider) NetworkList(filter map[string]any) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	networks := make([]*providerv1.NetworkState, 0, len(p.networks))
	for _, network := range p.networks {
		networks = append(networks, network)
	}

	return providerv1.SuccessResult(networks)
}

// NetworkDelete deletes a network by name.
func (p *Provider) NetworkDelete(name string) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.networks[name]; !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("network", name))
	}

	delete(p.networks, name)
	return providerv1.SuccessResult(nil)
}

// VMCreate creates a mock VM and stores it in memory.
func (p *Provider) VMCreate(req *providerv1.VMCreateRequest) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.vms[req.Name]; exists {
		return providerv1.ErrorResult(providerv1.NewAlreadyExistsError("vm", req.Name))
	}

	state := &providerv1.VMState{
		Name:       req.Name,
		Status:     "running",
		IP:         "192.168.100.10",
		MAC:        "52:54:00:12:34:56",
		UUID:       fmt.Sprintf("stub-vm-%s", req.Name),
		SSHCommand: "ssh -i /tmp/key user@192.168.100.10",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	p.vms[req.Name] = state
	return providerv1.SuccessResult(state)
}

// VMGet retrieves a VM by name.
func (p *Provider) VMGet(name string) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	vm, exists := p.vms[name]
	if !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("vm", name))
	}

	return providerv1.SuccessResult(vm)
}

// VMList lists all VMs, optionally filtered.
func (p *Provider) VMList(filter map[string]any) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	vms := make([]*providerv1.VMState, 0, len(p.vms))
	for _, vm := range p.vms {
		vms = append(vms, vm)
	}

	return providerv1.SuccessResult(vms)
}

// VMDelete deletes a VM by name.
func (p *Provider) VMDelete(name string) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.vms[name]; !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("vm", name))
	}

	delete(p.vms, name)
	return providerv1.SuccessResult(nil)
}
