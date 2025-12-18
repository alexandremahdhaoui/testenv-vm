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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"text/template"
)

// NetworkConfig holds configuration for generating network XML.
type NetworkConfig struct {
	Name        string
	BridgeName  string
	Gateway     string
	Netmask     string
	DHCPEnabled bool
	DHCPStart   string
	DHCPEnd     string
}

// DomainConfig holds configuration for generating domain XML.
type DomainConfig struct {
	Name         string
	MemoryMB     int
	VCPU         int
	DiskPath     string
	CloudInitISO string
	NetworkName  string
}

// generateBridgeName generates a unique bridge name from the network name.
// The bridge name is limited to 15 characters (Linux limit).
func generateBridgeName(networkName string) string {
	hash := sha256.Sum256([]byte(networkName))
	hashStr := hex.EncodeToString(hash[:])
	// virbr- (6) + 8 chars = 14, within 15 char limit
	return "virbr-" + hashStr[:8]
}

// parseCIDR parses a CIDR string and returns gateway, netmask, and DHCP range.
func parseCIDR(cidr string) (gateway, netmask, dhcpStart, dhcpEnd string, err error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid CIDR: %w", err)
	}

	// Use the network address, not the provided IP
	networkIP := ipNet.IP

	// Calculate gateway (first usable IP: x.x.x.1)
	gatewayIP := make(net.IP, len(networkIP))
	copy(gatewayIP, networkIP)
	gatewayIP[len(gatewayIP)-1] = 1
	gateway = gatewayIP.String()

	// Convert mask to dotted decimal
	mask := ipNet.Mask
	netmask = fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])

	// Calculate DHCP range
	// Start: x.x.x.2
	dhcpStartIP := make(net.IP, len(networkIP))
	copy(dhcpStartIP, networkIP)
	dhcpStartIP[len(dhcpStartIP)-1] = 2
	dhcpStart = dhcpStartIP.String()

	// End: broadcast - 1 (e.g., x.x.x.254 for /24)
	// Calculate broadcast address
	broadcast := make(net.IP, len(networkIP))
	for i := range broadcast {
		broadcast[i] = networkIP[i] | ^mask[i]
	}
	// End is broadcast - 1
	dhcpEndIP := make(net.IP, len(broadcast))
	copy(dhcpEndIP, broadcast)
	dhcpEndIP[len(dhcpEndIP)-1]--
	dhcpEnd = dhcpEndIP.String()

	// Validate that we didn't mess up (the original IP should be in the network)
	if !ipNet.Contains(ip) {
		return "", "", "", "", fmt.Errorf("IP %s not in network %s", ip, ipNet)
	}

	return gateway, netmask, dhcpStart, dhcpEnd, nil
}

// Network XML templates
const natNetworkTemplate = `<network>
    <name>{{.Name}}</name>
    <bridge name='{{.BridgeName}}'/>
    <forward mode='nat'>
        <nat>
            <port start='1024' end='65535'/>
        </nat>
    </forward>
    <ip address='{{.Gateway}}' netmask='{{.Netmask}}'>
{{- if .DHCPEnabled}}
        <dhcp>
            <range start='{{.DHCPStart}}' end='{{.DHCPEnd}}'/>
        </dhcp>
{{- end}}
    </ip>
</network>`

const isolatedNetworkTemplate = `<network>
    <name>{{.Name}}</name>
    <bridge name='{{.BridgeName}}'/>
    <ip address='{{.Gateway}}' netmask='{{.Netmask}}'>
{{- if .DHCPEnabled}}
        <dhcp>
            <range start='{{.DHCPStart}}' end='{{.DHCPEnd}}'/>
        </dhcp>
{{- end}}
    </ip>
</network>`

const bridgeNetworkTemplate = `<network>
    <name>{{.Name}}</name>
    <forward mode='bridge'/>
    <bridge name='{{.BridgeName}}'/>
</network>`

// generateNATNetworkXML generates XML for a NAT network.
func generateNATNetworkXML(config NetworkConfig) (string, error) {
	return executeTemplate(natNetworkTemplate, config)
}

// generateIsolatedNetworkXML generates XML for an isolated network.
func generateIsolatedNetworkXML(config NetworkConfig) (string, error) {
	return executeTemplate(isolatedNetworkTemplate, config)
}

// generateBridgeNetworkXML generates XML for a bridge network.
func generateBridgeNetworkXML(config NetworkConfig) (string, error) {
	return executeTemplate(bridgeNetworkTemplate, config)
}

// Domain XML template
const domainTemplate = `<domain type='kvm'>
    <name>{{.Name}}</name>
    <memory unit='MiB'>{{.MemoryMB}}</memory>
    <vcpu>{{.VCPU}}</vcpu>
    <os>
        <type arch='x86_64'>hvm</type>
        <boot dev='hd'/>
    </os>
    <features>
        <acpi/>
        <apic/>
    </features>
    <cpu mode='host-passthrough'/>
    <devices>
        <!-- Main disk -->
        <disk type='file' device='disk'>
            <driver name='qemu' type='qcow2'/>
            <source file='{{.DiskPath}}'/>
            <target dev='vda' bus='virtio'/>
        </disk>
{{if .CloudInitISO}}
        <!-- Cloud-init ISO -->
        <disk type='file' device='cdrom'>
            <driver name='qemu' type='raw'/>
            <source file='{{.CloudInitISO}}'/>
            <target dev='sda' bus='sata'/>
            <readonly/>
        </disk>
{{end}}
        <!-- Network interface -->
        <interface type='network'>
            <source network='{{.NetworkName}}'/>
            <model type='virtio'/>
        </interface>

        <!-- Serial console -->
        <serial type='pty'>
            <target port='0'/>
        </serial>
        <console type='pty'>
            <target type='serial' port='0'/>
        </console>
    </devices>
</domain>`

// generateDomainXML generates XML for a domain (VM).
func generateDomainXML(config DomainConfig) (string, error) {
	// Apply defaults
	if config.MemoryMB == 0 {
		config.MemoryMB = 2048
	}
	if config.VCPU == 0 {
		config.VCPU = 2
	}

	return executeTemplate(domainTemplate, config)
}

// executeTemplate executes a template with the given data.
func executeTemplate(tmpl string, data interface{}) (string, error) {
	t, err := template.New("xml").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
