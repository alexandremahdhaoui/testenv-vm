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

package libvirt

import (
	"strings"
	"testing"
)

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		name        string
		cidr        string
		wantGateway string
		wantNetmask string
		wantStart   string
		wantEnd     string
		wantErr     bool
	}{
		{
			name:        "Valid /24 network",
			cidr:        "192.168.100.0/24",
			wantGateway: "192.168.100.1",
			wantNetmask: "255.255.255.0",
			wantStart:   "192.168.100.2",
			wantEnd:     "192.168.100.254",
			wantErr:     false,
		},
		{
			name:        "Valid /16 network",
			cidr:        "10.0.0.0/16",
			wantGateway: "10.0.0.1",
			wantNetmask: "255.255.0.0",
			wantStart:   "10.0.0.2",
			wantEnd:     "10.0.255.254",
			wantErr:     false,
		},
		{
			name:        "Valid /28 network",
			cidr:        "172.16.0.0/28",
			wantGateway: "172.16.0.1",
			wantNetmask: "255.255.255.240",
			wantStart:   "172.16.0.2",
			wantEnd:     "172.16.0.14",
			wantErr:     false,
		},
		{
			name:    "Invalid CIDR",
			cidr:    "invalid",
			wantErr: true,
		},
		{
			name:    "Missing mask",
			cidr:    "192.168.100.0",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway, netmask, dhcpStart, dhcpEnd, err := parseCIDR(tt.cidr)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if gateway != tt.wantGateway {
				t.Errorf("Gateway: got %s, want %s", gateway, tt.wantGateway)
			}
			if netmask != tt.wantNetmask {
				t.Errorf("Netmask: got %s, want %s", netmask, tt.wantNetmask)
			}
			if dhcpStart != tt.wantStart {
				t.Errorf("DHCP Start: got %s, want %s", dhcpStart, tt.wantStart)
			}
			if dhcpEnd != tt.wantEnd {
				t.Errorf("DHCP End: got %s, want %s", dhcpEnd, tt.wantEnd)
			}
		})
	}
}

func TestGenerateBridgeName(t *testing.T) {
	tests := []struct {
		networkName string
		wantPrefix  string
	}{
		{"test-net", "virbr-"},
		{"my-network", "virbr-"},
		{"prod", "virbr-"},
	}

	for _, tt := range tests {
		t.Run(tt.networkName, func(t *testing.T) {
			bridgeName := generateBridgeName(tt.networkName)

			if !strings.HasPrefix(bridgeName, tt.wantPrefix) {
				t.Errorf("Bridge name should start with %s, got %s", tt.wantPrefix, bridgeName)
			}

			// Bridge name should be max 15 chars for Linux interfaces
			if len(bridgeName) > 15 {
				t.Errorf("Bridge name too long: %d chars (max 15)", len(bridgeName))
			}
		})
	}
}

func TestGenerateBridgeName_Uniqueness(t *testing.T) {
	// Different network names should produce different bridge names (with high probability)
	name1 := generateBridgeName("network-a")
	name2 := generateBridgeName("network-b")

	// They should be different (hash-based)
	if name1 == name2 {
		t.Error("Different network names should produce different bridge names")
	}
}

func TestGenerateNATNetworkXML(t *testing.T) {
	config := NetworkConfig{
		Name:        "test-nat",
		BridgeName:  "virbr-test",
		Gateway:     "192.168.100.1",
		Netmask:     "255.255.255.0",
		DHCPEnabled: true,
		DHCPStart:   "192.168.100.2",
		DHCPEnd:     "192.168.100.254",
	}

	xml, err := generateNATNetworkXML(config)
	if err != nil {
		t.Fatalf("generateNATNetworkXML failed: %v", err)
	}

	// Verify XML contains expected elements
	checks := []string{
		"<name>test-nat</name>",
		"<bridge name='virbr-test'/>",
		"<forward mode='nat'>",
		"address='192.168.100.1'",
		"netmask='255.255.255.0'",
		"start='192.168.100.2'",
		"end='192.168.100.254'",
	}

	for _, check := range checks {
		if !strings.Contains(xml, check) {
			t.Errorf("NAT network XML should contain: %s", check)
		}
	}
}

func TestGenerateNATNetworkXML_DHCPDisabled(t *testing.T) {
	config := NetworkConfig{
		Name:        "no-dhcp",
		BridgeName:  "virbr-nodhcp",
		Gateway:     "192.168.100.1",
		Netmask:     "255.255.255.0",
		DHCPEnabled: false,
		DHCPStart:   "192.168.100.2",
		DHCPEnd:     "192.168.100.254",
	}

	xml, err := generateNATNetworkXML(config)
	if err != nil {
		t.Fatalf("generateNATNetworkXML failed: %v", err)
	}

	if strings.Contains(xml, "<dhcp>") {
		t.Error("NAT network XML should NOT contain <dhcp> when DHCPEnabled is false")
	}
	if !strings.Contains(xml, "<forward mode='nat'>") {
		t.Error("NAT network XML should still contain forward mode='nat'")
	}
	if !strings.Contains(xml, "address='192.168.100.1'") {
		t.Error("NAT network XML should still contain gateway IP")
	}
}

func TestGenerateNATNetworkXML_CustomDHCPRange(t *testing.T) {
	config := NetworkConfig{
		Name:        "custom-range",
		BridgeName:  "virbr-custom",
		Gateway:     "192.168.100.1",
		Netmask:     "255.255.255.0",
		DHCPEnabled: true,
		DHCPStart:   "192.168.100.50",
		DHCPEnd:     "192.168.100.200",
	}

	xml, err := generateNATNetworkXML(config)
	if err != nil {
		t.Fatalf("generateNATNetworkXML failed: %v", err)
	}

	if !strings.Contains(xml, "start='192.168.100.50'") {
		t.Error("NAT network XML should contain custom DHCP start")
	}
	if !strings.Contains(xml, "end='192.168.100.200'") {
		t.Error("NAT network XML should contain custom DHCP end")
	}
}

func TestGenerateIsolatedNetworkXML(t *testing.T) {
	config := NetworkConfig{
		Name:       "test-isolated",
		BridgeName: "virbr-iso",
		Gateway:    "10.0.0.1",
		Netmask:    "255.255.255.0",
		DHCPStart:  "10.0.0.2",
		DHCPEnd:    "10.0.0.254",
	}

	xml, err := generateIsolatedNetworkXML(config)
	if err != nil {
		t.Fatalf("generateIsolatedNetworkXML failed: %v", err)
	}

	// Should NOT have forward mode for isolated network
	if strings.Contains(xml, "<forward") {
		t.Error("Isolated network should not have forward element")
	}

	// Should have name and bridge
	if !strings.Contains(xml, "<name>test-isolated</name>") {
		t.Error("Isolated network XML should contain name")
	}
	if !strings.Contains(xml, "<bridge name='virbr-iso'/>") {
		t.Error("Isolated network XML should contain bridge")
	}
}

func TestGenerateBridgeNetworkXML(t *testing.T) {
	config := NetworkConfig{
		Name:       "test-bridge",
		BridgeName: "br0",
		Gateway:    "172.16.0.1",
		Netmask:    "255.255.0.0",
		DHCPStart:  "172.16.0.2",
		DHCPEnd:    "172.16.255.254",
	}

	xml, err := generateBridgeNetworkXML(config)
	if err != nil {
		t.Fatalf("generateBridgeNetworkXML failed: %v", err)
	}

	// Should have forward mode bridge
	if !strings.Contains(xml, "<forward mode='bridge'/>") {
		t.Error("Bridge network should have forward mode='bridge'")
	}

	// Should have name and bridge
	if !strings.Contains(xml, "<name>test-bridge</name>") {
		t.Error("Bridge network XML should contain name")
	}
}

func TestGenerateDomainXML(t *testing.T) {
	config := DomainConfig{
		Name:         "test-vm",
		MemoryMB:     2048,
		VCPU:         2,
		DiskPath:     "/var/lib/libvirt/images/test.qcow2",
		CloudInitISO: "/var/lib/libvirt/images/test-ci.iso",
		Networks:     []NetworkInterface{{Name: "default"}},
	}

	xml, err := generateDomainXML(config)
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}

	// Verify XML contains expected elements
	checks := []struct {
		check string
		desc  string
	}{
		{"<name>test-vm</name>", "domain name"},
		{"<memory unit='MiB'>2048</memory>", "memory"},
		{"<vcpu>2</vcpu>", "vcpu count"},
		{"<type arch='x86_64'>hvm</type>", "domain type"},
		{"/var/lib/libvirt/images/test.qcow2", "disk path"},
		{"/var/lib/libvirt/images/test-ci.iso", "cloud-init ISO path"},
		{"<source network='default'/>", "network source"},
		{"<model type='virtio'/>", "network model"},
	}

	for _, c := range checks {
		if !strings.Contains(xml, c.check) {
			t.Errorf("Domain XML should contain %s: %s", c.desc, c.check)
		}
	}
}

func TestGenerateDomainXML_SmallVM(t *testing.T) {
	config := DomainConfig{
		Name:         "small-vm",
		MemoryMB:     512,
		VCPU:         1,
		DiskPath:     "/tmp/small.qcow2",
		CloudInitISO: "/tmp/small-ci.iso",
		Networks:     []NetworkInterface{{Name: "isolated"}},
	}

	xml, err := generateDomainXML(config)
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}

	// Verify the smaller config values are present
	if !strings.Contains(xml, "<memory unit='MiB'>512</memory>") {
		t.Error("Domain XML should contain memory 512")
	}
	if !strings.Contains(xml, "<vcpu>1</vcpu>") {
		t.Error("Domain XML should contain vcpu 1")
	}
	if !strings.Contains(xml, "<source network='isolated'/>") {
		t.Error("Domain XML should contain network 'isolated'")
	}
}

func TestGenerateDomainXML_Defaults(t *testing.T) {
	// Test that defaults are applied when MemoryMB and VCPU are 0
	config := DomainConfig{
		Name:         "default-vm",
		MemoryMB:     0, // Should default to 2048
		VCPU:         0, // Should default to 2
		DiskPath:     "/tmp/default.qcow2",
		CloudInitISO: "/tmp/default-ci.iso",
		Networks:     []NetworkInterface{{Name: "default"}},
	}

	xml, err := generateDomainXML(config)
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}

	// Verify defaults were applied
	if !strings.Contains(xml, "<memory unit='MiB'>2048</memory>") {
		t.Error("Default memory should be 2048 MiB")
	}
	if !strings.Contains(xml, "<vcpu>2</vcpu>") {
		t.Error("Default VCPU should be 2")
	}
}

func TestGenerateDomainXML_CustomValues(t *testing.T) {
	config := DomainConfig{
		Name:         "custom-vm",
		MemoryMB:     4096,
		VCPU:         4,
		DiskPath:     "/var/lib/libvirt/images/custom.qcow2",
		CloudInitISO: "/var/lib/libvirt/images/custom-ci.iso",
		Networks:     []NetworkInterface{{Name: "custom-net"}},
	}

	xml, err := generateDomainXML(config)
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}

	if !strings.Contains(xml, "<memory unit='MiB'>4096</memory>") {
		t.Error("Memory should be 4096 MiB")
	}
	if !strings.Contains(xml, "<vcpu>4</vcpu>") {
		t.Error("VCPU should be 4")
	}
	if !strings.Contains(xml, "<source network='custom-net'/>") {
		t.Error("Network should be 'custom-net'")
	}
}

func TestExecuteTemplate_Success(t *testing.T) {
	tmpl := "Hello {{.Name}}!"
	data := struct{ Name string }{Name: "World"}

	result, err := executeTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("executeTemplate failed: %v", err)
	}

	if result != "Hello World!" {
		t.Errorf("Expected 'Hello World!', got '%s'", result)
	}
}

func TestExecuteTemplate_InvalidTemplate(t *testing.T) {
	// Invalid template syntax
	tmpl := "Hello {{.Name"
	data := struct{ Name string }{Name: "World"}

	_, err := executeTemplate(tmpl, data)
	if err == nil {
		t.Error("Expected error for invalid template")
	}
}

func TestExecuteTemplate_MissingField(t *testing.T) {
	tmpl := "Hello {{.Name}}! Your age is {{.Age}}"
	data := struct{ Name string }{Name: "World"} // Missing Age field

	_, err := executeTemplate(tmpl, data)
	if err == nil {
		t.Error("Expected error for missing field in template")
	}
}

func TestParseCIDR_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		cidr        string
		wantGateway string
		wantErr     bool
	}{
		{
			name:        "Class A /8",
			cidr:        "10.0.0.0/8",
			wantGateway: "10.0.0.1",
			wantErr:     false,
		},
		{
			name:        "Class C /30 (small subnet)",
			cidr:        "192.168.1.0/30",
			wantGateway: "192.168.1.1",
			wantErr:     false,
		},
		{
			name:    "Invalid CIDR with extra characters",
			cidr:    "192.168.1.0/24/extra",
			wantErr: true,
		},
		{
			name:    "Empty string",
			cidr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway, _, _, _, err := parseCIDR(tt.cidr)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if gateway != tt.wantGateway {
				t.Errorf("Gateway: got %s, want %s", gateway, tt.wantGateway)
			}
		})
	}
}

func TestGenerateNATNetworkXML_Complete(t *testing.T) {
	config := NetworkConfig{
		Name:        "complete-nat",
		BridgeName:  "virbr-complete",
		Gateway:     "10.10.10.1",
		Netmask:     "255.255.255.0",
		DHCPEnabled: true,
		DHCPStart:   "10.10.10.10",
		DHCPEnd:     "10.10.10.200",
	}

	xml, err := generateNATNetworkXML(config)
	if err != nil {
		t.Fatalf("generateNATNetworkXML failed: %v", err)
	}

	// Verify all config values are in the XML
	if !strings.Contains(xml, "<name>complete-nat</name>") {
		t.Error("XML should contain network name")
	}
	if !strings.Contains(xml, "<bridge name='virbr-complete'/>") {
		t.Error("XML should contain bridge name")
	}
	if !strings.Contains(xml, "address='10.10.10.1'") {
		t.Error("XML should contain gateway IP")
	}
	if !strings.Contains(xml, "start='10.10.10.10'") {
		t.Error("XML should contain DHCP start")
	}
	if !strings.Contains(xml, "end='10.10.10.200'") {
		t.Error("XML should contain DHCP end")
	}
	if !strings.Contains(xml, "<port start='1024' end='65535'/>") {
		t.Error("XML should contain NAT port range")
	}
}

func TestGenerateIsolatedNetworkXML_Complete(t *testing.T) {
	config := NetworkConfig{
		Name:        "isolated-complete",
		BridgeName:  "virbr-iso",
		Gateway:     "172.16.0.1",
		Netmask:     "255.255.0.0",
		DHCPEnabled: true,
		DHCPStart:   "172.16.0.10",
		DHCPEnd:     "172.16.255.200",
	}

	xml, err := generateIsolatedNetworkXML(config)
	if err != nil {
		t.Fatalf("generateIsolatedNetworkXML failed: %v", err)
	}

	// Verify it's isolated (no forward element)
	if strings.Contains(xml, "<forward") {
		t.Error("Isolated network should not have forward element")
	}

	// Verify DHCP is still included
	if !strings.Contains(xml, "<dhcp>") {
		t.Error("Isolated network should have DHCP")
	}
}

func TestGenerateBridgeNetworkXML_NoIP(t *testing.T) {
	config := NetworkConfig{
		Name:       "bridge-net",
		BridgeName: "br0",
		Gateway:    "192.168.0.1",
		Netmask:    "255.255.255.0",
		DHCPStart:  "192.168.0.10",
		DHCPEnd:    "192.168.0.200",
	}

	xml, err := generateBridgeNetworkXML(config)
	if err != nil {
		t.Fatalf("generateBridgeNetworkXML failed: %v", err)
	}

	// Bridge mode doesn't include IP configuration in the template
	if strings.Contains(xml, "<dhcp>") {
		t.Error("Bridge network should not have DHCP in template")
	}
}

func TestExtractMACFromDomainXML(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		wantMAC string
	}{
		{
			name: "Valid MAC address",
			xml: `<domain type='kvm'>
				<devices>
					<interface type='network'>
						<mac address='52:54:00:12:34:56'/>
						<source network='default'/>
					</interface>
				</devices>
			</domain>`,
			wantMAC: "52:54:00:12:34:56",
		},
		{
			name:    "No MAC address",
			xml:     "<domain><devices></devices></domain>",
			wantMAC: "",
		},
		{
			name:    "Empty XML",
			xml:     "",
			wantMAC: "",
		},
		{
			name: "Multiple interfaces - returns first",
			xml: `<domain>
				<devices>
					<interface type='network'>
						<mac address='52:54:00:aa:bb:cc'/>
					</interface>
					<interface type='network'>
						<mac address='52:54:00:dd:ee:ff'/>
					</interface>
				</devices>
			</domain>`,
			wantMAC: "52:54:00:aa:bb:cc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac := extractMACFromDomainXML(tt.xml)
			if mac != tt.wantMAC {
				t.Errorf("extractMACFromDomainXML: got %q, want %q", mac, tt.wantMAC)
			}
		})
	}
}

func TestExtractAllMACsFromDomainXML(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		wantMACs []string
	}{
		{
			name:     "No interfaces",
			xml:      "<domain><devices></devices></domain>",
			wantMACs: nil,
		},
		{
			name: "Single interface",
			xml: `<domain><devices>
				<interface type='network'>
					<mac address='52:54:00:aa:bb:cc'/>
				</interface>
			</devices></domain>`,
			wantMACs: []string{"52:54:00:aa:bb:cc"},
		},
		{
			name: "Two interfaces",
			xml: `<domain><devices>
				<interface type='network'>
					<mac address='52:54:00:aa:bb:cc'/>
				</interface>
				<interface type='network'>
					<mac address='52:54:00:dd:ee:ff'/>
				</interface>
			</devices></domain>`,
			wantMACs: []string{"52:54:00:aa:bb:cc", "52:54:00:dd:ee:ff"},
		},
		{
			name: "Three interfaces",
			xml: `<domain><devices>
				<interface type='network'>
					<mac address='52:54:00:11:22:33'/>
				</interface>
				<interface type='network'>
					<mac address='52:54:00:44:55:66'/>
				</interface>
				<interface type='network'>
					<mac address='52:54:00:77:88:99'/>
				</interface>
			</devices></domain>`,
			wantMACs: []string{"52:54:00:11:22:33", "52:54:00:44:55:66", "52:54:00:77:88:99"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			macs := extractAllMACsFromDomainXML(tt.xml)
			if len(macs) != len(tt.wantMACs) {
				t.Fatalf("extractAllMACsFromDomainXML: got %d MACs, want %d", len(macs), len(tt.wantMACs))
			}
			for i, mac := range macs {
				if mac != tt.wantMACs[i] {
					t.Errorf("extractAllMACsFromDomainXML[%d]: got %q, want %q", i, mac, tt.wantMACs[i])
				}
			}
		})
	}
}

func TestGenerateDomainXML_MultipleNICs(t *testing.T) {
	tests := []struct {
		name           string
		networks       []NetworkInterface
		wantCount      int
		wantNetworks   []string
		wantPXECount   int // number of NICs with rom bar='on'
	}{
		{
			name:         "Single NIC",
			networks:     []NetworkInterface{{Name: "mgmt"}},
			wantCount:    1,
			wantNetworks: []string{"mgmt"},
			wantPXECount: 0,
		},
		{
			name: "Two NICs",
			networks: []NetworkInterface{
				{Name: "mgmt"},
				{Name: "data"},
			},
			wantCount:    2,
			wantNetworks: []string{"mgmt", "data"},
			wantPXECount: 0,
		},
		{
			name: "Three NICs",
			networks: []NetworkInterface{
				{Name: "mgmt"},
				{Name: "frontend"},
				{Name: "backend"},
			},
			wantCount:    3,
			wantNetworks: []string{"mgmt", "frontend", "backend"},
			wantPXECount: 0,
		},
		{
			name: "Two NICs with PXE on first",
			networks: []NetworkInterface{
				{Name: "pxe-net", HasNetworkBoot: true},
				{Name: "data"},
			},
			wantCount:    2,
			wantNetworks: []string{"pxe-net", "data"},
			wantPXECount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DomainConfig{
				Name:         "multi-nic-vm",
				MemoryMB:     1024,
				VCPU:         2,
				DiskPath:     "/tmp/test.qcow2",
				CloudInitISO: "/tmp/test-ci.iso",
				Networks:     tt.networks,
			}

			xml, err := generateDomainXML(config)
			if err != nil {
				t.Fatalf("generateDomainXML failed: %v", err)
			}

			// Count interface blocks
			interfaceCount := strings.Count(xml, "<interface type='network'>")
			if interfaceCount != tt.wantCount {
				t.Errorf("Expected %d interface blocks, got %d\nXML:\n%s", tt.wantCount, interfaceCount, xml)
			}

			// Verify each network name appears
			for _, netName := range tt.wantNetworks {
				expected := "<source network='" + netName + "'/>"
				if !strings.Contains(xml, expected) {
					t.Errorf("XML should contain %s\nXML:\n%s", expected, xml)
				}
			}

			// Count PXE ROM entries
			pxeCount := strings.Count(xml, "<rom bar='on'/>")
			if pxeCount != tt.wantPXECount {
				t.Errorf("Expected %d PXE ROM entries, got %d\nXML:\n%s", tt.wantPXECount, pxeCount, xml)
			}
		})
	}
}

func TestGenerateDomainXML_NoNetworks(t *testing.T) {
	config := DomainConfig{
		Name:         "no-net-vm",
		MemoryMB:     1024,
		VCPU:         1,
		DiskPath:     "/tmp/test.qcow2",
		CloudInitISO: "/tmp/test-ci.iso",
		Networks:     nil,
	}

	xml, err := generateDomainXML(config)
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}

	// Should produce no interface blocks
	if strings.Contains(xml, "<interface") {
		t.Errorf("Domain XML with no networks should not contain interface blocks\nXML:\n%s", xml)
	}
}
