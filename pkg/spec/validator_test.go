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
	"github.com/alexandremahdhaoui/testenv-vm/pkg/image"
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
		{
			name: "templated attachTo passes Phase 1 validation",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "parent-net", Kind: "bridge"},
					{
						Name: "child-net",
						Kind: "nat",
						Spec: v1.NetworkSpec{
							AttachTo: "{{ .Networks.parent-net.InterfaceName }}",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "templated attachTo with typo fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "parent-net", Kind: "bridge"},
					{
						Name: "child-net",
						Kind: "nat",
						Spec: v1.NetworkSpec{
							AttachTo: "{{ .Networks.typo-net.InterfaceName }}",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "template reference to non-existent network",
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

func TestIsTemplated(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty string returns false",
			input:    "",
			expected: false,
		},
		{
			name:     "literal value returns false",
			input:    "literal-value",
			expected: false,
		},
		{
			name:     "template expression returns true",
			input:    "{{ .Networks.foo.Name }}",
			expected: true,
		},
		{
			name:     "embedded template returns true",
			input:    "prefix-{{ .Keys.x.PublicKey }}-suffix",
			expected: true,
		},
		{
			name:     "single brace returns false",
			input:    "{ not a template }",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTemplated(tt.input)
			if result != tt.expected {
				t.Errorf("IsTemplated(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewTemplatedFields(t *testing.T) {
	tf := NewTemplatedFields()

	if tf == nil {
		t.Fatal("NewTemplatedFields() returned nil")
	}

	if tf.NetworkAttachTo == nil {
		t.Error("NetworkAttachTo map is nil")
	}

	if tf.VMNetwork == nil {
		t.Error("VMNetwork map is nil")
	}

	// Test that maps can store and retrieve values
	tf.NetworkAttachTo["test-net"] = true
	tf.VMNetwork["test-vm"] = true

	if !tf.NetworkAttachTo["test-net"] {
		t.Error("NetworkAttachTo map failed to store value")
	}

	if !tf.VMNetwork["test-vm"] {
		t.Error("VMNetwork map failed to store value")
	}
}

func TestValidateTemplateRefsExist(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1.TestenvSpec
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid template ref to existing network passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "parent-net", Kind: "bridge"},
					{
						Name: "child-net",
						Kind: "nat",
						Spec: v1.NetworkSpec{
							AttachTo: "{{ .Networks.parent-net.InterfaceName }}",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "template ref to non-existent network fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{
						Name: "child-net",
						Kind: "nat",
						Spec: v1.NetworkSpec{
							AttachTo: "{{ .Networks.typo-net.InterfaceName }}",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "template reference to non-existent network",
		},
		{
			name: "template ref to non-existent key fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				VMs: []v1.VMResource{
					{
						Name: "vm1",
						Spec: v1.VMSpec{
							Memory: 1024,
							VCPUs:  1,
							CloudInit: &v1.CloudInitSpec{
								Users: []v1.UserSpec{
									{
										Name:              "testuser",
										SSHAuthorizedKeys: []string{"{{ .Keys.nonexistent.PublicKey }}"},
									},
								},
							},
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "template reference to non-existent key",
		},
		{
			name: "valid template ref to existing key passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Keys: []v1.KeyResource{
					{Name: "my-key", Spec: v1.KeySpec{Type: "ed25519"}},
				},
				VMs: []v1.VMResource{
					{
						Name: "vm1",
						Spec: v1.VMSpec{
							Memory: 1024,
							VCPUs:  1,
							CloudInit: &v1.CloudInitSpec{
								Users: []v1.UserSpec{
									{
										Name:              "testuser",
										SSHAuthorizedKeys: []string{"{{ .Keys.my-key.PublicKey }}"},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Env references are skipped (no error for missing env vars)",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				VMs: []v1.VMResource{
					{
						Name: "vm1",
						Spec: v1.VMSpec{
							Memory: 1024,
							VCPUs:  1,
							Disk: v1.DiskSpec{
								BaseImage: "{{ .Env.BASE_IMAGE }}",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "template ref to non-existent image fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				VMs: []v1.VMResource{
					{
						Name: "vm1",
						Spec: v1.VMSpec{
							Memory: 1024,
							VCPUs:  1,
							Disk: v1.DiskSpec{
								BaseImage: "{{ .Images.nonexistent.Path }}",
							},
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "template reference to non-existent image",
		},
		{
			name: "valid template ref to existing image passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
				},
				VMs: []v1.VMResource{
					{
						Name: "vm1",
						Spec: v1.VMSpec{
							Memory: 1024,
							VCPUs:  1,
							Disk: v1.DiskSpec{
								BaseImage: "{{ .Images.ubuntu.Path }}",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "template ref to image alias passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "ubuntu-24-04", Spec: v1.ImageSpec{Source: "ubuntu:24.04", Alias: "ubuntu"}},
				},
				VMs: []v1.VMResource{
					{
						Name: "vm1",
						Spec: v1.VMSpec{
							Memory: 1024,
							VCPUs:  1,
							Disk: v1.DiskSpec{
								BaseImage: "{{ .Images.ubuntu.Path }}",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "spec with no template refs passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "net1", Kind: "bridge"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplateRefsExist(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTemplateRefsExist() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("validateTemplateRefsExist() error = %v, want error containing %q", err, tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateEarlyTemplatedFields(t *testing.T) {
	tests := []struct {
		name                    string
		spec                    *v1.TestenvSpec
		expectedNetworkAttachTo map[string]bool
		expectedVMNetwork       map[string]bool
	}{
		{
			name: "network with templated attachTo is marked",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "parent-net", Kind: "bridge"},
					{
						Name: "child-net",
						Kind: "nat",
						Spec: v1.NetworkSpec{
							AttachTo: "{{ .Networks.parent-net.InterfaceName }}",
						},
					},
				},
			},
			expectedNetworkAttachTo: map[string]bool{"child-net": true},
			expectedVMNetwork:       map[string]bool{},
		},
		{
			name: "network with literal attachTo is not marked",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "parent-net", Kind: "bridge"},
					{
						Name: "child-net",
						Kind: "nat",
						Spec: v1.NetworkSpec{
							AttachTo: "parent-net",
						},
					},
				},
			},
			expectedNetworkAttachTo: map[string]bool{},
			expectedVMNetwork:       map[string]bool{},
		},
		{
			name: "VM with templated network is marked",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "test-net", Kind: "bridge"},
				},
				VMs: []v1.VMResource{
					{
						Name: "test-vm",
						Spec: v1.VMSpec{
							Memory:  1024,
							VCPUs:   1,
							Network: "{{ .Networks.test-net.Name }}",
						},
					},
				},
			},
			expectedNetworkAttachTo: map[string]bool{},
			expectedVMNetwork:       map[string]bool{"test-vm": true},
		},
		{
			name: "multiple templated fields are all marked",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Networks: []v1.NetworkResource{
					{Name: "parent-net", Kind: "bridge"},
					{
						Name: "child-net-1",
						Kind: "nat",
						Spec: v1.NetworkSpec{
							AttachTo: "{{ .Networks.parent-net.InterfaceName }}",
						},
					},
					{
						Name: "child-net-2",
						Kind: "nat",
						Spec: v1.NetworkSpec{
							AttachTo: "{{ .Networks.parent-net.InterfaceName }}",
						},
					},
				},
				VMs: []v1.VMResource{
					{
						Name: "test-vm",
						Spec: v1.VMSpec{
							Memory:  1024,
							VCPUs:   1,
							Network: "{{ .Networks.parent-net.Name }}",
						},
					},
				},
			},
			expectedNetworkAttachTo: map[string]bool{"child-net-1": true, "child-net-2": true},
			expectedVMNetwork:       map[string]bool{"test-vm": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templatedFields, err := ValidateEarly(tt.spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if templatedFields == nil {
				t.Fatal("expected templatedFields to be non-nil")
			}

			// Check NetworkAttachTo
			for name, expected := range tt.expectedNetworkAttachTo {
				if templatedFields.NetworkAttachTo[name] != expected {
					t.Errorf("NetworkAttachTo[%q] = %v, want %v", name, templatedFields.NetworkAttachTo[name], expected)
				}
			}
			// Ensure no unexpected entries
			for name := range templatedFields.NetworkAttachTo {
				if _, ok := tt.expectedNetworkAttachTo[name]; !ok {
					t.Errorf("unexpected NetworkAttachTo[%q] = true", name)
				}
			}

			// Check VMNetwork
			for name, expected := range tt.expectedVMNetwork {
				if templatedFields.VMNetwork[name] != expected {
					t.Errorf("VMNetwork[%q] = %v, want %v", name, templatedFields.VMNetwork[name], expected)
				}
			}
			// Ensure no unexpected entries
			for name := range templatedFields.VMNetwork {
				if _, ok := tt.expectedVMNetwork[name]; !ok {
					t.Errorf("unexpected VMNetwork[%q] = true", name)
				}
			}
		})
	}
}

func TestValidateResourceRefsLate(t *testing.T) {
	fullSpec := &v1.TestenvSpec{
		Networks: []v1.NetworkResource{
			{Name: "net1", Kind: "bridge"},
			{Name: "net2", Kind: "nat"},
		},
	}

	tests := []struct {
		name            string
		resourceKind    string
		resourceName    string
		renderedSpec    interface{}
		templatedFields *TemplatedFields
		wantErr         bool
		errSubstr       string
	}{
		{
			name:         "non-templated network skips validation",
			resourceKind: "network",
			resourceName: "net1",
			renderedSpec: &v1.NetworkResource{
				Name: "net1",
				Spec: v1.NetworkSpec{AttachTo: "virbr0"},
			},
			templatedFields: NewTemplatedFields(),
			wantErr:         false,
		},
		{
			name:         "templated network attachTo accepts any non-empty value",
			resourceKind: "network",
			resourceName: "net1",
			renderedSpec: &v1.NetworkResource{
				Name: "net1",
				Spec: v1.NetworkSpec{AttachTo: "virbr0"},
			},
			templatedFields: &TemplatedFields{
				NetworkAttachTo: map[string]bool{"net1": true},
				VMNetwork:       make(map[string]bool),
			},
			wantErr: false,
		},
		{
			name:         "templated network attachTo rejects self-reference",
			resourceKind: "network",
			resourceName: "net1",
			renderedSpec: &v1.NetworkResource{
				Name: "net1",
				Spec: v1.NetworkSpec{AttachTo: "net1"},
			},
			templatedFields: &TemplatedFields{
				NetworkAttachTo: map[string]bool{"net1": true},
				VMNetwork:       make(map[string]bool),
			},
			wantErr:   true,
			errSubstr: "cannot reference itself",
		},
		{
			name:         "templated VM network accepts valid network name",
			resourceKind: "vm",
			resourceName: "vm1",
			renderedSpec: &v1.VMResource{
				Name: "vm1",
				Spec: v1.VMSpec{Network: "net1", Memory: 1024, VCPUs: 1},
			},
			templatedFields: &TemplatedFields{
				NetworkAttachTo: make(map[string]bool),
				VMNetwork:       map[string]bool{"vm1": true},
			},
			wantErr: false,
		},
		{
			name:         "templated VM network rejects invalid network name",
			resourceKind: "vm",
			resourceName: "vm1",
			renderedSpec: &v1.VMResource{
				Name: "vm1",
				Spec: v1.VMSpec{Network: "nonexistent", Memory: 1024, VCPUs: 1},
			},
			templatedFields: &TemplatedFields{
				NetworkAttachTo: make(map[string]bool),
				VMNetwork:       map[string]bool{"vm1": true},
			},
			wantErr:   true,
			errSubstr: "not a valid network name",
		},
		{
			name:         "nil templatedFields returns nil (no validation needed)",
			resourceKind: "network",
			resourceName: "net1",
			renderedSpec: &v1.NetworkResource{
				Name: "net1",
				Spec: v1.NetworkSpec{AttachTo: "virbr0"},
			},
			templatedFields: nil,
			wantErr:         false,
		},
		{
			name:         "empty attachTo passes for templated network",
			resourceKind: "network",
			resourceName: "net1",
			renderedSpec: &v1.NetworkResource{
				Name: "net1",
				Spec: v1.NetworkSpec{AttachTo: ""},
			},
			templatedFields: &TemplatedFields{
				NetworkAttachTo: map[string]bool{"net1": true},
				VMNetwork:       make(map[string]bool),
			},
			wantErr: false,
		},
		{
			name:         "empty network passes for templated VM",
			resourceKind: "vm",
			resourceName: "vm1",
			renderedSpec: &v1.VMResource{
				Name: "vm1",
				Spec: v1.VMSpec{Network: "", Memory: 1024, VCPUs: 1},
			},
			templatedFields: &TemplatedFields{
				NetworkAttachTo: make(map[string]bool),
				VMNetwork:       map[string]bool{"vm1": true},
			},
			wantErr: false,
		},
		{
			name:         "non-templated VM skips validation",
			resourceKind: "vm",
			resourceName: "vm1",
			renderedSpec: &v1.VMResource{
				Name: "vm1",
				Spec: v1.VMSpec{Network: "nonexistent", Memory: 1024, VCPUs: 1},
			},
			templatedFields: NewTemplatedFields(),
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResourceRefsLate(
				tt.resourceKind,
				tt.resourceName,
				tt.renderedSpec,
				fullSpec,
				tt.templatedFields,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateResourceRefsLate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ValidateResourceRefsLate() error = %v, want error containing %q", err, tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateImages(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1.TestenvSpec
		wantErr   bool
		errSubstr string
	}{
		{
			name: "empty images passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{},
			},
			wantErr: false,
		},
		{
			name: "valid well-known image passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
				},
			},
			wantErr: false,
		},
		{
			name: "valid HTTPS URL with SHA256 passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{
						Name: "custom",
						Spec: v1.ImageSpec{
							Source: "https://example.com/image.qcow2",
							SHA256: "abc123def456",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "image without name fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
				},
			},
			wantErr:   true,
			errSubstr: "name is required",
		},
		{
			name: "image without source fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "test", Spec: v1.ImageSpec{Source: ""}},
				},
			},
			wantErr:   true,
			errSubstr: "source is required",
		},
		{
			name: "HTTP URL fails (only HTTPS allowed)",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{
						Name: "test",
						Spec: v1.ImageSpec{
							Source: "http://example.com/image.qcow2",
							SHA256: "abc123",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "must be well-known reference or HTTPS URL",
		},
		{
			name: "custom URL without SHA256 fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{
						Name: "test",
						Spec: v1.ImageSpec{
							Source: "https://example.com/image.qcow2",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "custom URL requires sha256 checksum",
		},
		{
			name: "well-known image without SHA256 passes (checksum optional)",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{
						Name: "ubuntu",
						Spec: v1.ImageSpec{
							Source: "ubuntu:24.04",
							// SHA256 intentionally omitted
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate image name fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
					{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:22.04"}},
				},
			},
			wantErr:   true,
			errSubstr: "duplicate image name",
		},
		{
			name: "alias conflicting with name fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04"}},
					{Name: "debian", Spec: v1.ImageSpec{Source: "debian:12", Alias: "ubuntu"}},
				},
			},
			wantErr:   true,
			errSubstr: "alias \"ubuntu\" conflicts with another image name",
		},
		{
			name: "name conflicting with prior alias fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "first", Spec: v1.ImageSpec{Source: "ubuntu:24.04", Alias: "conflict"}},
					{Name: "conflict", Spec: v1.ImageSpec{Source: "debian:12"}},
				},
			},
			wantErr:   true,
			errSubstr: "name conflicts with alias from image",
		},
		{
			name: "duplicate alias fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04", Alias: "my-image"}},
					{Name: "debian", Spec: v1.ImageSpec{Source: "debian:12", Alias: "my-image"}},
				},
			},
			wantErr:   true,
			errSubstr: "alias \"my-image\" conflicts with alias from image",
		},
		{
			name: "valid alias passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04", Alias: "noble"}},
				},
			},
			wantErr: false,
		},
		{
			name: "unknown source (not well-known, not URL) fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "test", Spec: v1.ImageSpec{Source: "invalid:source"}},
				},
			},
			wantErr:   true,
			errSubstr: "must be well-known reference or HTTPS URL",
		},
		{
			name: "imageCacheDir with valid path passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				ImageCacheDir: "/tmp/images",
			},
			wantErr: false,
		},
		{
			name: "imageCacheDir with whitespace-only fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				ImageCacheDir: "   ",
			},
			wantErr:   true,
			errSubstr: "imageCacheDir cannot be whitespace-only",
		},
		{
			name: "multiple valid images pass",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "ubuntu", Spec: v1.ImageSpec{Source: "ubuntu:24.04", Alias: "noble"}},
					{Name: "debian", Spec: v1.ImageSpec{Source: "debian:12", Alias: "bookworm"}},
					{
						Name: "custom",
						Spec: v1.ImageSpec{
							Source: "https://example.com/image.qcow2",
							SHA256: "abc123",
						},
					},
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

func TestValidateImages_WithCustomRegistry(t *testing.T) {
	// Test with a custom registry to verify well-known image detection
	// Save original registry and restore after test
	t.Cleanup(func() {
		image.ResetRegistry()
	})

	// Set a test registry with a custom well-known image
	image.SetRegistry(map[string]image.WellKnownImage{
		"test:1.0": {
			Reference:   "test:1.0",
			URL:         "https://test.example.com/image.qcow2",
			SHA256:      "",
			Description: "Test image",
		},
	})

	tests := []struct {
		name      string
		spec      *v1.TestenvSpec
		wantErr   bool
		errSubstr string
	}{
		{
			name: "custom well-known image passes",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{Name: "test-img", Spec: v1.ImageSpec{Source: "test:1.0"}},
				},
			},
			wantErr: false,
		},
		{
			name: "original well-known image (ubuntu:24.04) now fails",
			spec: &v1.TestenvSpec{
				Providers: []v1.ProviderConfig{
					{Name: "provider1", Engine: "go://test"},
				},
				Images: []v1.ImageResource{
					{
						Name: "ubuntu",
						Spec: v1.ImageSpec{Source: "ubuntu:24.04"},
					},
				},
			},
			wantErr:   true,
			errSubstr: "must be well-known reference or HTTPS URL",
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
