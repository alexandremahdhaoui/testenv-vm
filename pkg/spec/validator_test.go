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

// Package spec provides parsing, validation, and template rendering for
// testenv-vm specifications.
package spec

import (
	"strings"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1.TestenvSpec
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "nil spec returns error",
			spec:      nil,
			wantErr:   true,
			errSubstr: "spec cannot be nil",
		},
		{
			name: "valid minimal spec passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid full spec passes",
			spec: &v1.TestenvSpec{
				StateDir:    "/tmp/state",
				ArtifactDir: "/tmp/artifacts",
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test", Default: true},
				},
				DefaultProvider: "provider1",
				Keys: []v1.KeyResource{
					{Name: "key1", Spec: v1.KeySpec{Type: "ed25519"}},
				},
				Networks: []v1.NetworkResource{
					{Name: "net1", Kind: "bridge"},
				},
				VMs: []v1.VMResource{
					{
						Name: "vm1",
						Spec: v1.VMSpec{
							Memory:  1024,
							VCPUs:   2,
							Network: "net1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "defaultProvider not found fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				DefaultProvider: "nonexistent",
			},
			wantErr:   true,
			errSubstr: "defaultProvider \"nonexistent\" does not match any defined provider",
		},
		{
			name: "defaultProvider and Default:true mismatch fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test", Default: true},
					{Name: "provider2", Engine: "go://test2"},
				},
				DefaultProvider: "provider2",
			},
			wantErr:   true,
			errSubstr: "is marked as default, but defaultProvider is set to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateProviders(t *testing.T) {
	tests := []struct {
		name      string
		providers []v1.ProviderConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "empty providers fails",
			providers: []v1.ProviderConfig{},
			wantErr:   true,
			errSubstr: "at least one provider must be defined",
		},
		{
			name: "single provider passes",
			providers: []v1.ProviderConfig{
				{Name: "provider1", Engine: "go://test"},
			},
			wantErr: false,
		},
		{
			name: "multiple providers with one default passes",
			providers: []v1.ProviderConfig{
				{Name: "provider1", Engine: "go://test", Default: true},
				{Name: "provider2", Engine: "go://test2"},
			},
			wantErr: false,
		},
		{
			name: "multiple providers with no default fails",
			providers: []v1.ProviderConfig{
				{Name: "provider1", Engine: "go://test"},
				{Name: "provider2", Engine: "go://test2"},
			},
			wantErr:   true,
			errSubstr: "no default provider specified",
		},
		{
			name: "multiple providers with multiple defaults fails",
			providers: []v1.ProviderConfig{
				{Name: "provider1", Engine: "go://test", Default: true},
				{Name: "provider2", Engine: "go://test2", Default: true},
			},
			wantErr:   true,
			errSubstr: "multiple providers marked as default",
		},
		{
			name: "missing name fails",
			providers: []v1.ProviderConfig{
				{Name: "", Engine: "go://test"},
			},
			wantErr:   true,
			errSubstr: "name is required",
		},
		{
			name: "missing engine fails",
			providers: []v1.ProviderConfig{
				{Name: "provider1", Engine: ""},
			},
			wantErr:   true,
			errSubstr: "engine is required",
		},
		{
			name: "duplicate names fails",
			providers: []v1.ProviderConfig{
				{Name: "provider1", Engine: "go://test", Default: true},
				{Name: "provider1", Engine: "go://test2"},
			},
			wantErr:   true,
			errSubstr: "duplicate provider name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviders(tt.providers)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProviders() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ValidateProviders() error = %v, want error containing %q", err, tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateKeys(t *testing.T) {
	tests := []struct {
		name      string
		keys      []v1.KeyResource
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "empty keys passes",
			keys:    []v1.KeyResource{},
			wantErr: false,
		},
		{
			name: "valid key types pass - rsa",
			keys: []v1.KeyResource{
				{Name: "key1", Spec: v1.KeySpec{Type: "rsa"}},
			},
			wantErr: false,
		},
		{
			name: "valid key types pass - ed25519",
			keys: []v1.KeyResource{
				{Name: "key1", Spec: v1.KeySpec{Type: "ed25519"}},
			},
			wantErr: false,
		},
		{
			name: "valid key types pass - ecdsa",
			keys: []v1.KeyResource{
				{Name: "key1", Spec: v1.KeySpec{Type: "ecdsa"}},
			},
			wantErr: false,
		},
		{
			name: "valid key types pass - uppercase",
			keys: []v1.KeyResource{
				{Name: "key1", Spec: v1.KeySpec{Type: "RSA"}},
			},
			wantErr: false,
		},
		{
			name: "invalid type fails",
			keys: []v1.KeyResource{
				{Name: "key1", Spec: v1.KeySpec{Type: "dsa"}},
			},
			wantErr:   true,
			errSubstr: "invalid key type",
		},
		{
			name: "missing name fails",
			keys: []v1.KeyResource{
				{Name: "", Spec: v1.KeySpec{Type: "rsa"}},
			},
			wantErr:   true,
			errSubstr: "name is required",
		},
		{
			name: "missing type fails",
			keys: []v1.KeyResource{
				{Name: "key1", Spec: v1.KeySpec{Type: ""}},
			},
			wantErr:   true,
			errSubstr: "spec.type is required",
		},
		{
			name: "duplicate names fails",
			keys: []v1.KeyResource{
				{Name: "key1", Spec: v1.KeySpec{Type: "rsa"}},
				{Name: "key1", Spec: v1.KeySpec{Type: "ed25519"}},
			},
			wantErr:   true,
			errSubstr: "duplicate key name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKeys(tt.keys)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKeys() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ValidateKeys() error = %v, want error containing %q", err, tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateNetworks(t *testing.T) {
	tests := []struct {
		name      string
		networks  []v1.NetworkResource
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "empty networks passes",
			networks: []v1.NetworkResource{},
			wantErr:  false,
		},
		{
			name: "valid network passes",
			networks: []v1.NetworkResource{
				{Name: "net1", Kind: "bridge"},
			},
			wantErr: false,
		},
		{
			name: "missing name fails",
			networks: []v1.NetworkResource{
				{Name: "", Kind: "bridge"},
			},
			wantErr:   true,
			errSubstr: "name is required",
		},
		{
			name: "missing kind fails",
			networks: []v1.NetworkResource{
				{Name: "net1", Kind: ""},
			},
			wantErr:   true,
			errSubstr: "kind is required",
		},
		{
			name: "duplicate names fails",
			networks: []v1.NetworkResource{
				{Name: "net1", Kind: "bridge"},
				{Name: "net1", Kind: "libvirt"},
			},
			wantErr:   true,
			errSubstr: "duplicate network name",
		},
		{
			name: "DHCP without CIDR fails",
			networks: []v1.NetworkResource{
				{
					Name: "net1",
					Kind: "bridge",
					Spec: v1.NetworkSpec{
						DHCP: &v1.DHCPSpec{
							Enabled:    true,
							RangeStart: "192.168.1.100",
							RangeEnd:   "192.168.1.200",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "cidr is required when DHCP is enabled",
		},
		{
			name: "DHCP with CIDR passes",
			networks: []v1.NetworkResource{
				{
					Name: "net1",
					Kind: "bridge",
					Spec: v1.NetworkSpec{
						CIDR: "192.168.1.0/24",
						DHCP: &v1.DHCPSpec{
							Enabled:    true,
							RangeStart: "192.168.1.100",
							RangeEnd:   "192.168.1.200",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "DHCP disabled without CIDR passes",
			networks: []v1.NetworkResource{
				{
					Name: "net1",
					Kind: "bridge",
					Spec: v1.NetworkSpec{
						DHCP: &v1.DHCPSpec{
							Enabled: false,
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNetworks(tt.networks)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNetworks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ValidateNetworks() error = %v, want error containing %q", err, tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateVMs(t *testing.T) {
	tests := []struct {
		name      string
		vms       []v1.VMResource
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "empty VMs passes",
			vms:     []v1.VMResource{},
			wantErr: false,
		},
		{
			name: "valid VM passes",
			vms: []v1.VMResource{
				{
					Name: "vm1",
					Spec: v1.VMSpec{
						Memory: 1024,
						VCPUs:  2,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name fails",
			vms: []v1.VMResource{
				{
					Name: "",
					Spec: v1.VMSpec{
						Memory: 1024,
						VCPUs:  2,
					},
				},
			},
			wantErr:   true,
			errSubstr: "name is required",
		},
		{
			name: "zero memory fails",
			vms: []v1.VMResource{
				{
					Name: "vm1",
					Spec: v1.VMSpec{
						Memory: 0,
						VCPUs:  2,
					},
				},
			},
			wantErr:   true,
			errSubstr: "memory must be a positive value",
		},
		{
			name: "negative memory fails",
			vms: []v1.VMResource{
				{
					Name: "vm1",
					Spec: v1.VMSpec{
						Memory: -1024,
						VCPUs:  2,
					},
				},
			},
			wantErr:   true,
			errSubstr: "memory must be a positive value",
		},
		{
			name: "zero vcpus fails",
			vms: []v1.VMResource{
				{
					Name: "vm1",
					Spec: v1.VMSpec{
						Memory: 1024,
						VCPUs:  0,
					},
				},
			},
			wantErr:   true,
			errSubstr: "vcpus must be a positive value",
		},
		{
			name: "negative vcpus fails",
			vms: []v1.VMResource{
				{
					Name: "vm1",
					Spec: v1.VMSpec{
						Memory: 1024,
						VCPUs:  -1,
					},
				},
			},
			wantErr:   true,
			errSubstr: "vcpus must be a positive value",
		},
		{
			name: "duplicate names fails",
			vms: []v1.VMResource{
				{
					Name: "vm1",
					Spec: v1.VMSpec{
						Memory: 1024,
						VCPUs:  2,
					},
				},
				{
					Name: "vm1",
					Spec: v1.VMSpec{
						Memory: 2048,
						VCPUs:  4,
					},
				},
			},
			wantErr:   true,
			errSubstr: "duplicate vm name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVMs(tt.vms)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVMs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ValidateVMs() error = %v, want error containing %q", err, tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateCrossReferences(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1.TestenvSpec
		wantErr   bool
		errSubstr string
	}{
		{
			name: "provider reference not found fails - key",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Keys: []v1.KeyResource{
					{Name: "key1", Provider: "nonexistent", Spec: v1.KeySpec{Type: "rsa"}},
				},
			},
			wantErr:   true,
			errSubstr: "provider \"nonexistent\" not found",
		},
		{
			name: "provider reference not found fails - network",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "net1", Kind: "bridge", Provider: "nonexistent"},
				},
			},
			wantErr:   true,
			errSubstr: "provider \"nonexistent\" not found",
		},
		{
			name: "provider reference not found fails - vm",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				VMs: []v1.VMResource{
					{Name: "vm1", Provider: "nonexistent", Spec: v1.VMSpec{Memory: 1024, VCPUs: 2}},
				},
			},
			wantErr:   true,
			errSubstr: "provider \"nonexistent\" not found",
		},
		{
			name: "network AttachTo not found fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{
						Name: "net1",
						Kind: "libvirt",
						Spec: v1.NetworkSpec{
							AttachTo: "nonexistent",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "attachTo references non-existent network",
		},
		{
			name: "network self-reference fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{
						Name: "net1",
						Kind: "libvirt",
						Spec: v1.NetworkSpec{
							AttachTo: "net1",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "attachTo cannot reference itself",
		},
		{
			name: "VM network not found fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				VMs: []v1.VMResource{
					{
						Name: "vm1",
						Spec: v1.VMSpec{
							Memory:  1024,
							VCPUs:   2,
							Network: "nonexistent",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "network \"nonexistent\" not found",
		},
		{
			name: "valid cross-references pass",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test", Default: true},
					{Name: "provider2", Engine: "go://test2"},
				},
				Keys: []v1.KeyResource{
					{Name: "key1", Provider: "provider1", Spec: v1.KeySpec{Type: "rsa"}},
				},
				Networks: []v1.NetworkResource{
					{Name: "net1", Kind: "bridge", Provider: "provider1"},
					{Name: "net2", Kind: "libvirt", Provider: "provider2", Spec: v1.NetworkSpec{AttachTo: "net1"}},
				},
				VMs: []v1.VMResource{
					{
						Name:     "vm1",
						Provider: "provider1",
						Spec: v1.VMSpec{
							Memory:  1024,
							VCPUs:   2,
							Network: "net1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid AttachTo between networks passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "bridge1", Kind: "bridge"},
					{Name: "libvirt1", Kind: "libvirt", Spec: v1.NetworkSpec{AttachTo: "bridge1"}},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errSubstr)
				}
			}
		})
	}
}
