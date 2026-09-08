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

func TestResolveGatewayHonorsADeclaredGatewayInsideTheCIDR(t *testing.T) {
	tests := []struct {
		name     string
		declared string
		want     string
		wantErr  string
	}{
		{name: "no declared gateway keeps the default", declared: "", want: "192.168.1.1"},
		{name: "a declared gateway inside the cidr wins", declared: "192.168.1.254", want: "192.168.1.254"},
		{name: "a declared gateway outside the cidr is refused", declared: "10.0.0.1", wantErr: "outside"},
		{name: "a declared gateway that is not an address is refused", declared: "router", wantErr: "not an IP address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveGateway("192.168.1.0/24", "192.168.1.1", tt.declared)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveGateway() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveGateway() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveGateway() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateNATNetworkXMLPlacesTheHostAtTheDeclaredGateway(t *testing.T) {
	xml, err := generateNATNetworkXML(NetworkConfig{
		Name:        "lan",
		BridgeName:  "virbr-lan",
		Gateway:     "192.168.1.254",
		Netmask:     "255.255.255.0",
		DHCPEnabled: false,
	})
	if err != nil {
		t.Fatalf("generateNATNetworkXML failed: %v", err)
	}
	if !strings.Contains(xml, "<ip address='192.168.1.254' netmask='255.255.255.0'>") {
		t.Errorf("network XML should place the host at 192.168.1.254")
	}
	if strings.Contains(xml, "<dhcp>") {
		t.Errorf("network XML should carry no dhcp block")
	}
}
