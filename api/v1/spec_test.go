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

package v1

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTestenvSpec_JSONRoundtrip(t *testing.T) {
	cleanupOnFailure := true
	tests := []struct {
		name string
		spec TestenvSpec
	}{
		{
			name: "minimal spec",
			spec: TestenvSpec{
				Providers: []ProviderConfig{
					{Name: "stub", Engine: "go://cmd/providers/stub"},
				},
			},
		},
		{
			name: "full spec",
			spec: TestenvSpec{
				StateDir:         "/tmp/state",
				ArtifactDir:      "/tmp/artifacts",
				CleanupOnFailure: &cleanupOnFailure,
				Providers: []ProviderConfig{
					{
						Name:    "stub",
						Engine:  "go://cmd/providers/stub",
						Default: true,
						Spec:    map[string]any{"key": "value"},
					},
				},
				DefaultProvider: "stub",
				Keys: []KeyResource{
					{
						Name:     "test-key",
						Provider: "stub",
						Spec: KeySpec{
							Type:      "ed25519",
							Bits:      0,
							Comment:   "test key",
							OutputDir: "/tmp/keys",
						},
						ProviderSpec: map[string]any{"custom": "config"},
					},
				},
				Networks: []NetworkResource{
					{
						Name:     "test-net",
						Kind:     "bridge",
						Provider: "stub",
						Spec: NetworkSpec{
							CIDR:    "192.168.100.0/24",
							Gateway: "192.168.100.1",
							MTU:     1500,
							DHCP: &DHCPSpec{
								Enabled:    true,
								RangeStart: "192.168.100.10",
								RangeEnd:   "192.168.100.100",
								LeaseTime:  "12h",
							},
							DNS: &DNSSpec{
								Enabled: true,
								Servers: []string{"8.8.8.8"},
							},
							TFTP: &TFTPSpec{
								Enabled:  true,
								Root:     "/var/tftp",
								BootFile: "undionly.kpxe",
							},
						},
					},
				},
				VMs: []VMResource{
					{
						Name:     "test-vm",
						Provider: "stub",
						Spec: VMSpec{
							Memory:  2048,
							VCPUs:   2,
							Network: "test-net",
							Disk: DiskSpec{
								BaseImage: "/images/ubuntu.qcow2",
								Size:      "20G",
							},
							Boot: BootSpec{
								Order:    []string{"hd", "network"},
								Firmware: "uefi",
							},
							CloudInit: &CloudInitSpec{
								Hostname: "test-vm",
								Users: []UserSpec{
									{
										Name:              "ubuntu",
										Sudo:              "ALL=(ALL) NOPASSWD:ALL",
										SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA..."},
									},
								},
								Packages: []string{"vim", "curl"},
							},
							Readiness: &ReadinessSpec{
								SSH: &SSHReadinessSpec{
									Enabled:    true,
									Timeout:    "5m",
									User:       "ubuntu",
									PrivateKey: "/tmp/keys/test-key",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.spec)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Unmarshal back
			var got TestenvSpec
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Compare
			if !reflect.DeepEqual(tt.spec, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.spec, got)
			}
		})
	}
}

func TestTestenvSpec_JSONFieldNames(t *testing.T) {
	cleanupOnFailure := false
	spec := TestenvSpec{
		StateDir:         "/state",
		ArtifactDir:      "/artifacts",
		CleanupOnFailure: &cleanupOnFailure,
		Providers: []ProviderConfig{
			{Name: "p1", Engine: "e1", Default: true},
		},
		DefaultProvider: "p1",
		Keys:            []KeyResource{{Name: "k1", Spec: KeySpec{Type: "rsa"}}},
		Networks:        []NetworkResource{{Name: "n1", Kind: "bridge", Spec: NetworkSpec{}}},
		VMs:             []VMResource{{Name: "v1", Spec: VMSpec{Memory: 1024, VCPUs: 1, Disk: DiskSpec{Size: "10G"}, Boot: BootSpec{Order: []string{"hd"}}}}},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	// Verify expected field names exist
	expectedFields := []string{"stateDir", "artifactDir", "cleanupOnFailure", "providers", "defaultProvider", "keys", "networks", "vms"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}
}

func TestProviderConfig_JSONRoundtrip(t *testing.T) {
	config := ProviderConfig{
		Name:    "test-provider",
		Engine:  "go://cmd/provider",
		Default: true,
		Spec:    map[string]any{"option1": "value1", "option2": float64(42)},
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got ProviderConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(config, got) {
		t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", config, got)
	}
}

func TestResourceRef_JSONRoundtrip(t *testing.T) {
	ref := ResourceRef{
		Kind:     "vm",
		Name:     "test-vm",
		Provider: "stub",
	}

	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got ResourceRef
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(ref, got) {
		t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", ref, got)
	}
}
