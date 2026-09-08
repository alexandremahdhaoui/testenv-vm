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

type NetworkConfig struct {
	Name        string
	BridgeName  string
	Gateway     string
	Netmask     string
	DHCPEnabled bool
	DHCPStart   string
	DHCPEnd     string
}

type NetworkInterface struct {
	Name           string
	HasNetworkBoot bool
}

type DomainConfig struct {
	Name         string
	MemoryMB     int
	VCPU         int
	DiskPath     string
	CloudInitISO string
	CdromPath    string
	Networks     []NetworkInterface
	BootOrder    []string
	Firmware     string
}

func generateBridgeName(networkName string) string {
	hash := sha256.Sum256([]byte(networkName))
	hashStr := hex.EncodeToString(hash[:])
	return "virbr-" + hashStr[:8]
}

func parseCIDR(cidr string) (gateway, netmask, dhcpStart, dhcpEnd string, err error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid CIDR: %w", err)
	}

	networkIP := ipNet.IP

	gatewayIP := make(net.IP, len(networkIP))
	copy(gatewayIP, networkIP)
	gatewayIP[len(gatewayIP)-1] = 1
	gateway = gatewayIP.String()

	mask := ipNet.Mask
	netmask = fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])

	dhcpStartIP := make(net.IP, len(networkIP))
	copy(dhcpStartIP, networkIP)
	dhcpStartIP[len(dhcpStartIP)-1] = 2
	dhcpStart = dhcpStartIP.String()

	broadcast := make(net.IP, len(networkIP))
	for i := range broadcast {
		broadcast[i] = networkIP[i] | ^mask[i]
	}
	dhcpEndIP := make(net.IP, len(broadcast))
	copy(dhcpEndIP, broadcast)
	dhcpEndIP[len(dhcpEndIP)-1]--
	dhcpEnd = dhcpEndIP.String()

	if !ipNet.Contains(ip) {
		return "", "", "", "", fmt.Errorf("IP %s not in network %s", ip, ipNet)
	}

	return gateway, netmask, dhcpStart, dhcpEnd, nil
}

func resolveGateway(cidr, defaultGateway, declaredGateway string) (string, error) {
	if declaredGateway == "" {
		return defaultGateway, nil
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("parsing cidr %s: %w", cidr, err)
	}
	declared := net.ParseIP(declaredGateway)
	if declared == nil {
		return "", fmt.Errorf("gateway %s is not an IP address", declaredGateway)
	}
	if !ipNet.Contains(declared) {
		return "", fmt.Errorf("gateway %s is outside %s", declaredGateway, cidr)
	}
	return declaredGateway, nil
}

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

func generateNATNetworkXML(config NetworkConfig) (string, error) {
	return executeTemplate(natNetworkTemplate, config)
}

func generateIsolatedNetworkXML(config NetworkConfig) (string, error) {
	return executeTemplate(isolatedNetworkTemplate, config)
}

func generateBridgeNetworkXML(config NetworkConfig) (string, error) {
	return executeTemplate(bridgeNetworkTemplate, config)
}

const domainTemplate = `<domain type='kvm'>
    <name>{{.Name}}</name>
    <memory unit='MiB'>{{.MemoryMB}}</memory>
    <vcpu>{{.VCPU}}</vcpu>
    <os>
        <type arch='x86_64'>hvm</type>
{{- range .BootOrder}}
        <boot dev='{{.}}'/>
{{- end}}
{{- if not .BootOrder}}
        <boot dev='hd'/>
{{- end}}
    </os>
    <features>
        <acpi/>
        <apic/>
    </features>
    <cpu mode='host-passthrough'/>
    <devices>
        <disk type='file' device='disk'>
            <driver name='qemu' type='qcow2'/>
            <source file='{{.DiskPath}}'/>
            <target dev='vda' bus='virtio'/>
        </disk>
{{if .CdromPath}}
        <disk type='file' device='cdrom'>
            <driver name='qemu' type='raw'/>
            <source file='{{.CdromPath}}'/>
            <target dev='sda' bus='sata'/>
            <readonly/>
        </disk>
{{end}}
{{if .CloudInitISO}}
        <disk type='file' device='cdrom'>
            <driver name='qemu' type='raw'/>
            <source file='{{.CloudInitISO}}'/>
            <target dev='{{if .CdromPath}}sdb{{else}}sda{{end}}' bus='sata'/>
            <readonly/>
        </disk>
{{end}}
{{- range .Networks}}
        <interface type='network'>
            <source network='{{.Name}}'/>
            <model type='virtio'/>
{{- if .HasNetworkBoot}}
            <rom bar='on'/>
{{- end}}
        </interface>
{{- end}}
        <serial type='pty'>
            <target port='0'/>
        </serial>
        <console type='pty'>
            <target type='serial' port='0'/>
        </console>
    </devices>
</domain>`

func generateDomainXML(config DomainConfig) (string, error) {
	if config.MemoryMB == 0 {
		config.MemoryMB = 2048
	}
	if config.VCPU == 0 {
		config.VCPU = 2
	}

	return executeTemplate(domainTemplate, config)
}

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
