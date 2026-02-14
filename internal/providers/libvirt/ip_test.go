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
	"testing"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

func TestExtractMACFromDomainXML_ValidMAC(t *testing.T) {
	xml := `<domain type='kvm'>
		<devices>
			<interface type='network'>
				<mac address='52:54:00:12:34:56'/>
				<source network='default'/>
			</interface>
		</devices>
	</domain>`

	mac := extractMACFromDomainXML(xml)
	if mac != "52:54:00:12:34:56" {
		t.Errorf("Expected '52:54:00:12:34:56', got '%s'", mac)
	}
}

func TestExtractMACFromDomainXML_NoMACElement(t *testing.T) {
	xml := `<domain type='kvm'>
		<devices>
			<interface type='network'>
				<source network='default'/>
			</interface>
		</devices>
	</domain>`

	mac := extractMACFromDomainXML(xml)
	if mac != "" {
		t.Errorf("Expected empty string, got '%s'", mac)
	}
}

func TestExtractMACFromDomainXML_EmptyXML(t *testing.T) {
	mac := extractMACFromDomainXML("")
	if mac != "" {
		t.Errorf("Expected empty string for empty XML, got '%s'", mac)
	}
}

func TestExtractMACFromDomainXML_MalformedMAC(t *testing.T) {
	// MAC element exists but no closing quote
	xml := `<domain><devices><interface><mac address='52:54:00:12:34:56</interface></devices></domain>`

	mac := extractMACFromDomainXML(xml)
	if mac != "" {
		t.Errorf("Expected empty string for malformed MAC, got '%s'", mac)
	}
}

func TestExtractMACFromDomainXML_MultipleInterfaces(t *testing.T) {
	xml := `<domain type='kvm'>
		<devices>
			<interface type='network'>
				<mac address='52:54:00:aa:bb:cc'/>
				<source network='net1'/>
			</interface>
			<interface type='network'>
				<mac address='52:54:00:dd:ee:ff'/>
				<source network='net2'/>
			</interface>
		</devices>
	</domain>`

	// Should return the first MAC
	mac := extractMACFromDomainXML(xml)
	if mac != "52:54:00:aa:bb:cc" {
		t.Errorf("Expected first MAC '52:54:00:aa:bb:cc', got '%s'", mac)
	}
}

func TestExtractMACFromDomainXML_DifferentFormats(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		wantMAC string
	}{
		{
			name:    "Standard format",
			xml:     "<domain><devices><interface><mac address='AA:BB:CC:DD:EE:FF'/></interface></devices></domain>",
			wantMAC: "AA:BB:CC:DD:EE:FF",
		},
		{
			name:    "Lowercase",
			xml:     "<domain><devices><interface><mac address='aa:bb:cc:dd:ee:ff'/></interface></devices></domain>",
			wantMAC: "aa:bb:cc:dd:ee:ff",
		},
		{
			name:    "Mixed case",
			xml:     "<domain><devices><interface><mac address='Aa:Bb:Cc:Dd:Ee:Ff'/></interface></devices></domain>",
			wantMAC: "Aa:Bb:Cc:Dd:Ee:Ff",
		},
		{
			name:    "With extra whitespace",
			xml:     "<domain><devices><interface><mac address='52:54:00:11:22:33'  /></interface></devices></domain>",
			wantMAC: "52:54:00:11:22:33",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac := extractMACFromDomainXML(tt.xml)
			if mac != tt.wantMAC {
				t.Errorf("extractMACFromDomainXML() = '%s', want '%s'", mac, tt.wantMAC)
			}
		})
	}
}

func TestExtractMACFromDomainXML_NoDevices(t *testing.T) {
	xml := `<domain type='kvm'>
		<name>test</name>
		<memory>1024</memory>
	</domain>`

	mac := extractMACFromDomainXML(xml)
	if mac != "" {
		t.Errorf("Expected empty string for domain without devices, got '%s'", mac)
	}
}

func TestExtractMACFromDomainXML_PartialMACElement(t *testing.T) {
	// Has "<mac address='" but nothing after
	xml := `<domain><devices><interface><mac address='`

	mac := extractMACFromDomainXML(xml)
	if mac != "" {
		t.Errorf("Expected empty string for partial MAC element, got '%s'", mac)
	}
}

func TestExtractStaticIP_NilCloudInit(t *testing.T) {
	ip := extractStaticIP(nil)
	if ip != "" {
		t.Errorf("expected empty string for nil CloudInit, got %q", ip)
	}
}

func TestExtractStaticIP_NilNetworkConfig(t *testing.T) {
	ci := &providerv1.CloudInitSpec{}
	ip := extractStaticIP(ci)
	if ip != "" {
		t.Errorf("expected empty string for nil NetworkConfig, got %q", ip)
	}
}

func TestExtractStaticIP_EmptyEthernets(t *testing.T) {
	ci := &providerv1.CloudInitSpec{
		NetworkConfig: &providerv1.CloudInitNetworkConfig{},
	}
	ip := extractStaticIP(ci)
	if ip != "" {
		t.Errorf("expected empty string for empty ethernets, got %q", ip)
	}
}

func TestExtractStaticIP_CIDRNotation(t *testing.T) {
	ci := &providerv1.CloudInitSpec{
		NetworkConfig: &providerv1.CloudInitNetworkConfig{
			Ethernets: []providerv1.CloudInitEthernetConfig{
				{
					Name:      "ens2",
					Addresses: []string{"192.168.100.10/24"},
				},
			},
		},
	}
	ip := extractStaticIP(ci)
	if ip != "192.168.100.10" {
		t.Errorf("expected 192.168.100.10, got %q", ip)
	}
}

func TestExtractStaticIP_BareIP(t *testing.T) {
	ci := &providerv1.CloudInitSpec{
		NetworkConfig: &providerv1.CloudInitNetworkConfig{
			Ethernets: []providerv1.CloudInitEthernetConfig{
				{
					Name:      "ens2",
					Addresses: []string{"10.0.0.5"},
				},
			},
		},
	}
	ip := extractStaticIP(ci)
	if ip != "10.0.0.5" {
		t.Errorf("expected 10.0.0.5, got %q", ip)
	}
}

func TestExtractStaticIP_MultipleEthernets(t *testing.T) {
	ci := &providerv1.CloudInitSpec{
		NetworkConfig: &providerv1.CloudInitNetworkConfig{
			Ethernets: []providerv1.CloudInitEthernetConfig{
				{Name: "ens2", Addresses: []string{}},
				{Name: "ens3", Addresses: []string{"172.16.0.1/16"}},
			},
		},
	}
	ip := extractStaticIP(ci)
	if ip != "172.16.0.1" {
		t.Errorf("expected 172.16.0.1, got %q", ip)
	}
}
