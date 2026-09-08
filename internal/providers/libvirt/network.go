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

func (p *Provider) NetworkCreate(req *providerv1.NetworkCreateRequest) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.networks[req.Name]; exists {
		return providerv1.ErrorResult(providerv1.NewAlreadyExistsError("network", req.Name))
	}

	if existingNet, err := p.conn.NetworkLookupByName(req.Name); err == nil {
		_ = p.conn.NetworkDestroy(existingNet)
		_ = p.conn.NetworkUndefine(existingNet)
	}

	kind := req.Kind
	if kind == "" {
		kind = "nat"
	}

	switch kind {
	case "nat", "isolated", "bridge":
	default:
		return providerv1.ErrorResult(providerv1.NewInvalidSpecError("unsupported network kind: " + kind))
	}

	cidr := req.Spec.CIDR
	if cidr == "" {
		cidr = "192.168.100.0/24"
	}

	gateway, netmask, dhcpStart, dhcpEnd, err := parseCIDR(cidr)
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewInvalidSpecError("invalid CIDR: " + err.Error()))
	}
	gateway, err = resolveGateway(cidr, gateway, req.Spec.Gateway)
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewInvalidSpecError("invalid gateway: " + err.Error()))
	}

	bridgeName := generateBridgeName(req.Name)

	dhcpEnabled := true
	if req.Spec.DHCP != nil {
		dhcpEnabled = req.Spec.DHCP.Enabled
		if req.Spec.DHCP.RangeStart != "" {
			dhcpStart = req.Spec.DHCP.RangeStart
		}
		if req.Spec.DHCP.RangeEnd != "" {
			dhcpEnd = req.Spec.DHCP.RangeEnd
		}
	}

	config := NetworkConfig{
		Name:        req.Name,
		BridgeName:  bridgeName,
		Gateway:     gateway,
		Netmask:     netmask,
		DHCPEnabled: dhcpEnabled,
		DHCPStart:   dhcpStart,
		DHCPEnd:     dhcpEnd,
	}

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

	net, err := p.conn.NetworkDefineXML(networkXML)
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to define network: "+err.Error(), true))
	}

	if err := p.conn.NetworkCreate(net); err != nil {
		_ = p.conn.NetworkUndefine(net)
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to start network: "+err.Error(), true))
	}

	state := &providerv1.NetworkState{
		Name:          req.Name,
		Kind:          kind,
		Status:        "active",
		IP:            gateway,
		CIDR:          cidr,
		InterfaceName: bridgeName,
		UUID:          formatUUID(net.UUID),
	}

	p.networks[req.Name] = state
	return providerv1.SuccessResult(state)
}

func (p *Provider) NetworkGet(name string) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	network, exists := p.networks[name]
	if !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("network", name))
	}

	return providerv1.SuccessResult(network)
}

func (p *Provider) NetworkList(filter map[string]any) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	networks := make([]*providerv1.NetworkState, 0, len(p.networks))
	for _, network := range p.networks {
		networks = append(networks, network)
	}

	return providerv1.SuccessResult(networks)
}

func (p *Provider) NetworkDelete(name string) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, exists := p.networks[name]

	if exists {
		for _, vm := range p.vms {
			if vmUsesNetwork(vm, name) {
				return providerv1.ErrorResult(providerv1.NewResourceBusyError("network", name))
			}
		}
	}

	net, err := p.conn.NetworkLookupByName(name)
	if err == nil {
		_ = p.conn.NetworkDestroy(net)
		_ = p.conn.NetworkUndefine(net)
	}

	delete(p.networks, name)

	return providerv1.SuccessResult(nil)
}

func vmUsesNetwork(vm *providerv1.VMState, networkName string) bool {
	if vm == nil || vm.ProviderState == nil {
		return false
	}
	if nets, ok := vm.ProviderState["networks"]; ok {
		switch v := nets.(type) {
		case []string:
			for _, n := range v {
				if n == networkName {
					return true
				}
			}
		case []any:
			for _, n := range v {
				if s, ok := n.(string); ok && s == networkName {
					return true
				}
			}
		}
	}
	if netName, ok := vm.ProviderState["network"].(string); ok && netName == networkName {
		return true
	}
	return false
}

func formatUUID(uuid libvirt.UUID) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
