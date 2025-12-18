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

package libvirt

import (
	"fmt"

	"github.com/digitalocean/go-libvirt"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// NetworkCreate creates a network via libvirt.
// This function is idempotent: if a network with the same name already exists
// in libvirt (e.g., from a previous failed run), it will be cleaned up first.
func (p *Provider) NetworkCreate(req *providerv1.NetworkCreateRequest) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.networks[req.Name]; exists {
		return providerv1.ErrorResult(providerv1.NewAlreadyExistsError("network", req.Name))
	}

	// Check if network already exists in libvirt (orphaned from previous run)
	// If so, clean it up to ensure idempotent behavior
	if existingNet, err := p.conn.NetworkLookupByName(req.Name); err == nil {
		// Network exists in libvirt but not in our state - clean it up
		_ = p.conn.NetworkDestroy(existingNet)
		_ = p.conn.NetworkUndefine(existingNet)
	}

	kind := req.Kind
	if kind == "" {
		kind = "nat"
	}

	// Validate kind
	switch kind {
	case "nat", "isolated", "bridge":
		// Valid kinds
	default:
		return providerv1.ErrorResult(providerv1.NewInvalidSpecError("unsupported network kind: " + kind))
	}

	// Get CIDR from spec or use default
	cidr := req.Spec.CIDR
	if cidr == "" {
		cidr = "192.168.100.0/24"
	}

	// Parse CIDR to get network config
	gateway, netmask, dhcpStart, dhcpEnd, err := parseCIDR(cidr)
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewInvalidSpecError("invalid CIDR: " + err.Error()))
	}

	// Generate bridge name
	bridgeName := generateBridgeName(req.Name)

	// Determine if DHCP should be enabled
	// Default to true for backward compatibility (if DHCP spec is nil)
	dhcpEnabled := true
	if req.Spec.DHCP != nil {
		dhcpEnabled = req.Spec.DHCP.Enabled
	}

	// Build network config
	config := NetworkConfig{
		Name:        req.Name,
		BridgeName:  bridgeName,
		Gateway:     gateway,
		Netmask:     netmask,
		DHCPEnabled: dhcpEnabled,
		DHCPStart:   dhcpStart,
		DHCPEnd:     dhcpEnd,
	}

	// Generate network XML based on kind
	var networkXML string
	switch kind {
	case "nat":
		networkXML, err = generateNATNetworkXML(config)
	case "isolated":
		networkXML, err = generateIsolatedNetworkXML(config)
	case "bridge":
		networkXML, err = generateBridgeNetworkXML(config)
	}
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to generate network XML: "+err.Error(), false))
	}

	// Define the network in libvirt
	net, err := p.conn.NetworkDefineXML(networkXML)
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to define network: "+err.Error(), true))
	}

	// Start the network
	if err := p.conn.NetworkCreate(net); err != nil {
		// Cleanup: undefine the network
		_ = p.conn.NetworkUndefine(net)
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to start network: "+err.Error(), true))
	}

	// Get network UUID
	uuid := formatUUID(net.UUID)

	state := &providerv1.NetworkState{
		Name:          req.Name,
		Kind:          kind,
		Status:        "active",
		IP:            gateway,
		CIDR:          cidr,
		InterfaceName: bridgeName,
		UUID:          uuid,
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

// NetworkList lists all networks.
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
// This function is idempotent: it will attempt to delete from libvirt even if
// the network is not in in-memory state (e.g., from a previous crashed run).
func (p *Provider) NetworkDelete(name string) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, exists := p.networks[name]

	// Check if any VM in our state is using this network
	// (only if we have state - if we don't, we can't know about VMs)
	if exists {
		for _, vm := range p.vms {
			if netName, ok := vm.ProviderState["network"].(string); ok && netName == name {
				return providerv1.ErrorResult(providerv1.NewResourceBusyError("network", name))
			}
		}
	}

	// Always try to get network from libvirt directly
	// This handles cases where network exists in libvirt but not in our state
	// (e.g., provider restarted, previous run crashed)
	net, err := p.conn.NetworkLookupByName(name)
	if err == nil {
		// Stop the network (ignore error if already stopped)
		_ = p.conn.NetworkDestroy(net)

		// Undefine the network (best-effort, don't fail if this errors)
		_ = p.conn.NetworkUndefine(net)
	}

	delete(p.networks, name)

	// Always return success for idempotent delete
	return providerv1.SuccessResult(nil)
}

// formatUUID formats a UUID byte array as a string.
func formatUUID(uuid libvirt.UUID) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
