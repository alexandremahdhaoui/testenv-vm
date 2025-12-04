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

package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
)

// mockCommandRunner is a mock implementation of CommandRunner for testing.
type mockCommandRunner struct {
	responses map[string]struct {
		output []byte
		err    error
	}
}

func newMockCommandRunner() *mockCommandRunner {
	return &mockCommandRunner{
		responses: make(map[string]struct {
			output []byte
			err    error
		}),
	}
}

func (m *mockCommandRunner) Run(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	if resp, ok := m.responses[key]; ok {
		return resp.output, resp.err
	}
	return nil, fmt.Errorf("no mock response for: %s", key)
}

func (m *mockCommandRunner) addResponse(name string, args []string, output []byte, err error) {
	key := name + " " + strings.Join(args, " ")
	m.responses[key] = struct {
		output []byte
		err    error
	}{output: output, err: err}
}

func TestLibvirtProvider_ImplementsClientProvider(t *testing.T) {
	// Compile-time check
	var _ client.ClientProvider = (*LibvirtProvider)(nil)
}

func TestNewLibvirtProvider_Defaults(t *testing.T) {
	p := NewLibvirtProvider()

	if p.connURI != "qemu:///session" {
		t.Errorf("expected connURI 'qemu:///session', got %q", p.connURI)
	}
	if p.defaultUser != "testuser" {
		t.Errorf("expected defaultUser 'testuser', got %q", p.defaultUser)
	}
	if p.defaultPort != "22" {
		t.Errorf("expected defaultPort '22', got %q", p.defaultPort)
	}
	if p.cmdRunner == nil {
		t.Error("expected cmdRunner to be non-nil")
	}
	if p.keyPath != "" {
		t.Errorf("expected empty keyPath, got %q", p.keyPath)
	}
}

func TestLibvirtProvider_WithConnectionURI(t *testing.T) {
	p := NewLibvirtProvider(WithConnectionURI("qemu:///system"))

	if p.connURI != "qemu:///system" {
		t.Errorf("expected connURI 'qemu:///system', got %q", p.connURI)
	}
}

func TestLibvirtProvider_WithKeyPath(t *testing.T) {
	p := NewLibvirtProvider(WithKeyPath("/path/to/key"))

	if p.keyPath != "/path/to/key" {
		t.Errorf("expected keyPath '/path/to/key', got %q", p.keyPath)
	}
}

func TestLibvirtProvider_WithUser(t *testing.T) {
	p := NewLibvirtProvider(WithUser("admin"))

	if p.defaultUser != "admin" {
		t.Errorf("expected defaultUser 'admin', got %q", p.defaultUser)
	}
}

func TestLibvirtProvider_WithPort(t *testing.T) {
	p := NewLibvirtProvider(WithPort("2222"))

	if p.defaultPort != "2222" {
		t.Errorf("expected defaultPort '2222', got %q", p.defaultPort)
	}
}

func TestLibvirtProvider_WithCommandRunner(t *testing.T) {
	mock := newMockCommandRunner()
	p := NewLibvirtProvider(WithCommandRunner(mock))

	if p.cmdRunner != mock {
		t.Error("expected cmdRunner to be the mock instance")
	}
}

func TestLibvirtProvider_GetVMInfo_ParsesDomifaddrOutput(t *testing.T) {
	// Create temporary key file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	keyData := []byte("test-private-key-content")
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	// Mock virsh domifaddr output
	domifaddrOutput := `Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
vnet0      52:54:00:11:22:33    ipv4         192.168.122.100/24
`

	mock := newMockCommandRunner()
	mock.addResponse("virsh", []string{"--connect", "qemu:///session", "domifaddr", "test-vm"},
		[]byte(domifaddrOutput), nil)

	p := NewLibvirtProvider(
		WithKeyPath(keyPath),
		WithCommandRunner(mock),
	)

	vmInfo, err := p.GetVMInfo("test-vm")
	if err != nil {
		t.Fatalf("GetVMInfo failed: %v", err)
	}

	if vmInfo.Host != "192.168.122.100" {
		t.Errorf("expected Host '192.168.122.100', got %q", vmInfo.Host)
	}
	if vmInfo.Port != "22" {
		t.Errorf("expected Port '22', got %q", vmInfo.Port)
	}
	if vmInfo.User != "testuser" {
		t.Errorf("expected User 'testuser', got %q", vmInfo.User)
	}
	if string(vmInfo.PrivateKey) != string(keyData) {
		t.Errorf("expected PrivateKey %q, got %q", keyData, vmInfo.PrivateKey)
	}
}

func TestLibvirtProvider_GetVMInfo_FallbackToNetDHCPLeases(t *testing.T) {
	// Create temporary key file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	keyData := []byte("test-private-key-content")
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	// Mock virsh domifaddr output (empty, no IP)
	domifaddrOutput := `Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
`

	// Mock virsh net-dhcp-leases output
	dhcpLeasesOutput := `Expiry Time           MAC address         Protocol   IP address          Hostname        Client ID or DUID
----------------------------------------------------------------------------------------------------------------------------------------
2025-12-05 10:00:00   52:54:00:11:22:33   ipv4       192.168.122.50/24   test-vm         -
`

	mock := newMockCommandRunner()
	mock.addResponse("virsh", []string{"--connect", "qemu:///session", "domifaddr", "test-vm"},
		[]byte(domifaddrOutput), nil)
	mock.addResponse("virsh", []string{"--connect", "qemu:///session", "net-dhcp-leases", "default"},
		[]byte(dhcpLeasesOutput), nil)

	p := NewLibvirtProvider(
		WithKeyPath(keyPath),
		WithCommandRunner(mock),
	)

	vmInfo, err := p.GetVMInfo("test-vm")
	if err != nil {
		t.Fatalf("GetVMInfo failed: %v", err)
	}

	if vmInfo.Host != "192.168.122.50" {
		t.Errorf("expected Host '192.168.122.50', got %q", vmInfo.Host)
	}
}

func TestLibvirtProvider_GetVMInfo_ErrorOnUnknownVM(t *testing.T) {
	// Create temporary key file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(keyPath, []byte("key"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	// Mock empty outputs (no IP found)
	mock := newMockCommandRunner()
	mock.addResponse("virsh", []string{"--connect", "qemu:///session", "domifaddr", "unknown-vm"},
		[]byte(""), nil)
	mock.addResponse("virsh", []string{"--connect", "qemu:///session", "net-dhcp-leases", "default"},
		[]byte(""), nil)

	p := NewLibvirtProvider(
		WithKeyPath(keyPath),
		WithCommandRunner(mock),
	)

	_, err := p.GetVMInfo("unknown-vm")
	if err == nil {
		t.Fatal("expected error for unknown VM, got nil")
	}
	if !strings.Contains(err.Error(), "no IP address found") {
		t.Errorf("expected error to contain 'no IP address found', got %q", err.Error())
	}
}

func TestLibvirtProvider_GetVMInfo_ErrorOnMissingKeyPath(t *testing.T) {
	// Mock successful virsh output
	domifaddrOutput := `Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
vnet0      52:54:00:11:22:33    ipv4         192.168.122.100/24
`

	mock := newMockCommandRunner()
	mock.addResponse("virsh", []string{"--connect", "qemu:///session", "domifaddr", "test-vm"},
		[]byte(domifaddrOutput), nil)

	// Create provider without keyPath
	p := NewLibvirtProvider(WithCommandRunner(mock))

	_, err := p.GetVMInfo("test-vm")
	if err == nil {
		t.Fatal("expected error for missing keyPath, got nil")
	}
	if !strings.Contains(err.Error(), "keyPath not configured") {
		t.Errorf("expected error to contain 'keyPath not configured', got %q", err.Error())
	}
}

func TestLibvirtProvider_GetVMInfo_ErrorOnUnreadableKeyFile(t *testing.T) {
	// Mock successful virsh output
	domifaddrOutput := `Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
vnet0      52:54:00:11:22:33    ipv4         192.168.122.100/24
`

	mock := newMockCommandRunner()
	mock.addResponse("virsh", []string{"--connect", "qemu:///session", "domifaddr", "test-vm"},
		[]byte(domifaddrOutput), nil)

	// Use a non-existent key file path
	p := NewLibvirtProvider(
		WithKeyPath("/nonexistent/key"),
		WithCommandRunner(mock),
	)

	_, err := p.GetVMInfo("test-vm")
	if err == nil {
		t.Fatal("expected error for unreadable key file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read private key") {
		t.Errorf("expected error to contain 'failed to read private key', got %q", err.Error())
	}
}

func TestParseIPFromVirshOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name: "standard domifaddr output",
			output: `Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
vnet0      52:54:00:11:22:33    ipv4         192.168.122.100/24
`,
			expected: "192.168.122.100",
		},
		{
			name: "multiple interfaces",
			output: `Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
vnet0      52:54:00:11:22:33    ipv4         192.168.122.100/24
vnet1      52:54:00:44:55:66    ipv4         10.0.0.50/16
`,
			expected: "192.168.122.100",
		},
		{
			name:     "no IP",
			output:   `Name       MAC address          Protocol     Address`,
			expected: "",
		},
		{
			name:     "empty output",
			output:   "",
			expected: "",
		},
		{
			name: "different CIDR",
			output: `Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
vnet0      52:54:00:11:22:33    ipv4         10.20.30.40/16
`,
			expected: "10.20.30.40",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIPFromVirshOutput(tt.output)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseIPFromDHCPLeases(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		vmName   string
		expected string
	}{
		{
			name: "standard dhcp leases output",
			output: `Expiry Time           MAC address         Protocol   IP address          Hostname        Client ID or DUID
----------------------------------------------------------------------------------------------------------------------------------------
2025-12-05 10:00:00   52:54:00:11:22:33   ipv4       192.168.122.50/24   test-vm         -
`,
			vmName:   "test-vm",
			expected: "192.168.122.50",
		},
		{
			name: "multiple leases",
			output: `Expiry Time           MAC address         Protocol   IP address          Hostname        Client ID or DUID
----------------------------------------------------------------------------------------------------------------------------------------
2025-12-05 10:00:00   52:54:00:11:22:33   ipv4       192.168.122.50/24   test-vm         -
2025-12-05 10:05:00   52:54:00:44:55:66   ipv4       192.168.122.51/24   other-vm        -
`,
			vmName:   "test-vm",
			expected: "192.168.122.50",
		},
		{
			name: "VM not found",
			output: `Expiry Time           MAC address         Protocol   IP address          Hostname        Client ID or DUID
----------------------------------------------------------------------------------------------------------------------------------------
2025-12-05 10:00:00   52:54:00:11:22:33   ipv4       192.168.122.50/24   other-vm        -
`,
			vmName:   "test-vm",
			expected: "",
		},
		{
			name:     "empty output",
			output:   "",
			vmName:   "test-vm",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIPFromDHCPLeases(tt.output, tt.vmName)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestLibvirtProvider_GetVMInfo_CustomUserAndPort(t *testing.T) {
	// Create temporary key file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	keyData := []byte("test-private-key-content")
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	// Mock virsh domifaddr output
	domifaddrOutput := `Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
vnet0      52:54:00:11:22:33    ipv4         192.168.122.100/24
`

	mock := newMockCommandRunner()
	mock.addResponse("virsh", []string{"--connect", "qemu:///session", "domifaddr", "test-vm"},
		[]byte(domifaddrOutput), nil)

	p := NewLibvirtProvider(
		WithKeyPath(keyPath),
		WithUser("admin"),
		WithPort("2222"),
		WithCommandRunner(mock),
	)

	vmInfo, err := p.GetVMInfo("test-vm")
	if err != nil {
		t.Fatalf("GetVMInfo failed: %v", err)
	}

	if vmInfo.User != "admin" {
		t.Errorf("expected User 'admin', got %q", vmInfo.User)
	}
	if vmInfo.Port != "2222" {
		t.Errorf("expected Port '2222', got %q", vmInfo.Port)
	}
}
