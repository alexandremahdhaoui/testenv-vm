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

package providerv1

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestVMCreateRequest_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		req  VMCreateRequest
	}{
		{
			name: "minimal request",
			req: VMCreateRequest{
				Name: "test-vm",
				Spec: VMSpec{
					Memory:  2048,
					VCPUs:   2,
					Network: "test-net",
					Disk:    DiskSpec{Size: "20G"},
					Boot:    BootSpec{Order: []string{"hd"}},
				},
			},
		},
		{
			name: "full request",
			req: VMCreateRequest{
				Name: "full-vm",
				Spec: VMSpec{
					Memory:       4096,
					VCPUs:        4,
					Architecture: "x86_64",
					MachineType:  "pc-q35-8.0",
					CPU: &CPUSpec{
						Mode:    "host-passthrough",
						Cores:   2,
						Sockets: 2,
					},
					Disk: DiskSpec{
						BaseImage: "/images/ubuntu.qcow2",
						Size:      "50G",
						Bus:       "virtio",
						Cache:     "none",
					},
					Network: "prod-net",
					CloudInit: &CloudInitSpec{
						Hostname: "full-vm",
						Users: []UserSpec{
							{
								Name:              "admin",
								Sudo:              "ALL=(ALL) NOPASSWD:ALL",
								Shell:             "/bin/bash",
								SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA..."},
							},
						},
						Packages:    []string{"vim", "curl", "htop"},
						WriteFiles:  []WriteFileSpec{{Path: "/etc/motd", Content: "Welcome!", Permissions: "0644"}},
						Runcmd: []string{"apt-get update"},
					},
					Boot: BootSpec{
						Order:      []string{"hd", "network"},
						Firmware:   "uefi",
						SecureBoot: true,
					},
					Console: &ConsoleSpec{
						Serial:  true,
						VNC:     true,
						VNCPort: 5900,
					},
					MemoryBacking: &MemoryBackingSpec{
						Source: "memfd",
						Access: "shared",
					},
					VirtioFS: []VirtioFSSpec{
						{Tag: "workspace", HostPath: "/home/user/project", Queue: 1024},
					},
					GuestAgent: true,
					Readiness: &ReadinessSpec{
						SSH: &SSHReadinessSpec{Enabled: true, Timeout: "5m", User: "admin"},
						TCP: &TCPReadinessSpec{Port: 22, Timeout: "2m"},
					},
				},
				ProviderSpec: map[string]any{"custom_option": "value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got VMCreateRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.req, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.req, got)
			}
		})
	}
}

func TestVMState_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		state VMState
	}{
		{
			name: "minimal state",
			state: VMState{
				Name:   "test-vm",
				Status: "running",
			},
		},
		{
			name: "full state",
			state: VMState{
				Name:          "full-vm",
				Status:        "running",
				IP:            "192.168.100.10",
				MAC:           "52:54:00:12:34:56",
				UUID:          "abc123-def456",
				ConsoleOutput: "/var/log/vm/console.log",
				SSHCommand:    "ssh -i /keys/vm admin@192.168.100.10",
				VNCAddress:    "localhost:5900",
				SerialDevice:  "/dev/pts/1",
				DomainXML:     "<domain>...</domain>",
				QMPSocket:     "/var/run/qmp/vm.sock",
				CreatedAt:     "2024-01-01T00:00:00Z",
				ProviderState: map[string]any{"libvirt_id": float64(123)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.state)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got VMState
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.state, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.state, got)
			}
		})
	}
}

func TestNetworkCreateRequest_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		req  NetworkCreateRequest
	}{
		{
			name: "minimal request",
			req: NetworkCreateRequest{
				Name: "test-net",
				Kind: "bridge",
				Spec: NetworkSpec{
					CIDR: "192.168.100.0/24",
				},
			},
		},
		{
			name: "full request",
			req: NetworkCreateRequest{
				Name: "full-net",
				Kind: "libvirt",
				Spec: NetworkSpec{
					CIDR:     "10.0.0.0/16",
					Gateway:  "10.0.0.1",
					AttachTo: "external-bridge",
					MTU:      9000,
					DHCP: &DHCPSpec{
						Enabled:    true,
						RangeStart: "10.0.1.1",
						RangeEnd:   "10.0.1.254",
						LeaseTime:  "24h",
						Router:     "10.0.0.1",
						DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						Domain:     "test.local",
						NextServer: "10.0.0.1",
						StaticLeases: []StaticLease{
							{MAC: "52:54:00:00:00:01", IP: "10.0.0.10", Hostname: "server1"},
						},
					},
					DNS: &DNSSpec{
						Enabled: true,
						Servers: []string{"8.8.8.8"},
						Hosts: []DNSHost{
							{Hostname: "server1.test.local", IP: "10.0.0.10"},
						},
						Domain: "test.local",
					},
					TFTP: &TFTPSpec{
						Enabled:         true,
						Root:            "/var/tftp",
						BootFile:        "undionly.kpxe",
						BootFileEFI:     "ipxe.efi",
						DHCPBootOptions: map[string]string{"option43": "value"},
					},
					IPv6: &IPv6Spec{
						CIDR:    "fd00::/64",
						Gateway: "fd00::1",
						DHCP6:   true,
						SLAAC:   true,
					},
				},
				ProviderSpec: map[string]any{"vlan_id": float64(100)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got NetworkCreateRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.req, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.req, got)
			}
		})
	}
}

func TestNetworkState_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		state NetworkState
	}{
		{
			name: "minimal state",
			state: NetworkState{
				Name:   "test-net",
				Kind:   "bridge",
				Status: "ready",
			},
		},
		{
			name: "full state",
			state: NetworkState{
				Name:          "full-net",
				Kind:          "libvirt",
				Status:        "ready",
				IP:            "192.168.100.1",
				CIDR:          "192.168.100.0/24",
				InterfaceName: "virbr1",
				UUID:          "net-uuid-123",
				PID:           12345,
				ProviderState: map[string]any{"dnsmasq_pid": float64(12346)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.state)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got NetworkState
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.state, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.state, got)
			}
		})
	}
}

func TestKeyCreateRequest_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		req  KeyCreateRequest
	}{
		{
			name: "minimal request",
			req: KeyCreateRequest{
				Name: "test-key",
				Spec: KeySpec{
					Type: "ed25519",
				},
			},
		},
		{
			name: "full request",
			req: KeyCreateRequest{
				Name: "full-key",
				Spec: KeySpec{
					Type:      "rsa",
					Bits:      4096,
					Comment:   "test@example.com",
					OutputDir: "/tmp/keys",
				},
				ProviderSpec: map[string]any{"aws_region": "us-west-2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got KeyCreateRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.req, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.req, got)
			}
		})
	}
}

func TestKeyState_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		state KeyState
	}{
		{
			name: "minimal state",
			state: KeyState{
				Name:           "test-key",
				Type:           "ed25519",
				PublicKey:      "ssh-ed25519 AAAA...",
				PublicKeyPath:  "/keys/test-key.pub",
				PrivateKeyPath: "/keys/test-key",
				Fingerprint:    "SHA256:abcd1234",
			},
		},
		{
			name: "full state",
			state: KeyState{
				Name:           "full-key",
				Type:           "rsa",
				PublicKey:      "ssh-rsa AAAA...",
				PublicKeyPath:  "/keys/full-key.pub",
				PrivateKeyPath: "/keys/full-key",
				Fingerprint:    "SHA256:efgh5678",
				AWSKeyPairID:   "key-0123456789abcdef",
				CreatedAt:      "2024-01-01T00:00:00Z",
				ProviderState:  map[string]any{"aws_arn": "arn:aws:..."},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.state)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got KeyState
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.state, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.state, got)
			}
		})
	}
}

func TestVMState_JSONFieldNames(t *testing.T) {
	state := VMState{
		Name:          "test",
		Status:        "running",
		IP:            "1.2.3.4",
		MAC:           "aa:bb:cc:dd:ee:ff",
		UUID:          "uuid",
		ConsoleOutput: "/log",
		SSHCommand:    "ssh",
		VNCAddress:    "vnc",
		SerialDevice:  "/dev/pts/0",
		DomainXML:     "<xml/>",
		QMPSocket:     "/sock",
		CreatedAt:     "2024-01-01",
		ProviderState: map[string]any{"k": "v"},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{
		"name", "status", "ip", "mac", "uuid", "consoleOutput",
		"sshCommand", "vncAddress", "serialDevice", "domainXML",
		"qmpSocket", "createdAt", "providerState",
	}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}
}

func TestNetworkState_JSONFieldNames(t *testing.T) {
	state := NetworkState{
		Name:          "test",
		Kind:          "bridge",
		Status:        "ready",
		IP:            "1.2.3.4",
		CIDR:          "1.2.3.0/24",
		InterfaceName: "br0",
		UUID:          "uuid",
		PID:           123,
		ProviderState: map[string]any{"k": "v"},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{
		"name", "kind", "status", "ip", "cidr", "interfaceName",
		"uuid", "pid", "providerState",
	}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}
}

func TestKeyState_JSONFieldNames(t *testing.T) {
	state := KeyState{
		Name:           "test",
		Type:           "rsa",
		PublicKey:      "pub",
		PublicKeyPath:  "/pub",
		PrivateKeyPath: "/priv",
		Fingerprint:    "fp",
		AWSKeyPairID:   "aws",
		CreatedAt:      "2024-01-01",
		ProviderState:  map[string]any{"k": "v"},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{
		"name", "type", "publicKey", "publicKeyPath", "privateKeyPath",
		"fingerprint", "awsKeyPairId", "createdAt", "providerState",
	}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}
}
