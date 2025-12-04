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
	"os/exec"
	"regexp"
	"strings"

	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
)

// CommandRunner abstracts exec.Command for testing.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

// realCommandRunner is the default implementation using os/exec.
type realCommandRunner struct{}

func (r *realCommandRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// LibvirtProvider queries libvirt for VM connection info.
type LibvirtProvider struct {
	connURI     string        // e.g., "qemu:///session" or "qemu:///system"
	keyPath     string        // Path to SSH private key
	defaultUser string        // Default "testuser"
	defaultPort string        // Default "22"
	cmdRunner   CommandRunner // For testing
}

// Compile-time check that LibvirtProvider implements client.ClientProvider
var _ client.ClientProvider = (*LibvirtProvider)(nil)

// LibvirtProviderOption configures the LibvirtProvider.
type LibvirtProviderOption func(*LibvirtProvider)

// WithConnectionURI sets the libvirt connection URI.
func WithConnectionURI(uri string) LibvirtProviderOption {
	return func(p *LibvirtProvider) {
		p.connURI = uri
	}
}

// WithKeyPath sets the path to the SSH private key.
func WithKeyPath(path string) LibvirtProviderOption {
	return func(p *LibvirtProvider) {
		p.keyPath = path
	}
}

// WithUser sets the default SSH user.
func WithUser(user string) LibvirtProviderOption {
	return func(p *LibvirtProvider) {
		p.defaultUser = user
	}
}

// WithPort sets the default SSH port.
func WithPort(port string) LibvirtProviderOption {
	return func(p *LibvirtProvider) {
		p.defaultPort = port
	}
}

// WithCommandRunner sets a custom CommandRunner for testing.
func WithCommandRunner(r CommandRunner) LibvirtProviderOption {
	return func(p *LibvirtProvider) {
		p.cmdRunner = r
	}
}

// NewLibvirtProvider creates a new libvirt provider.
// Defaults: connURI="qemu:///session", user="testuser", port="22"
func NewLibvirtProvider(opts ...LibvirtProviderOption) *LibvirtProvider {
	p := &LibvirtProvider{
		connURI:     "qemu:///session",
		defaultUser: "testuser",
		defaultPort: "22",
		cmdRunner:   &realCommandRunner{},
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// GetVMInfo queries libvirt for VM IP and returns connection info.
// Steps:
// 1. Run: virsh --connect <connURI> domifaddr <vmName>
// 2. Parse IP from output
// 3. Fallback: virsh net-dhcp-leases default (for NAT networks)
// 4. Read private key from keyPath
// 5. Return VMInfo
func (p *LibvirtProvider) GetVMInfo(vmName string) (*client.VMInfo, error) {
	// Try domifaddr first
	output, _ := p.cmdRunner.Run("virsh", "--connect", p.connURI, "domifaddr", vmName)
	ip := parseIPFromVirshOutput(string(output))

	// Fallback to net-dhcp-leases if domifaddr failed or returned no IP
	if ip == "" {
		output, err := p.cmdRunner.Run("virsh", "--connect", p.connURI, "net-dhcp-leases", "default")
		if err != nil {
			return nil, fmt.Errorf("libvirt: failed to get VM IP for %s: %w", vmName, err)
		}
		ip = parseIPFromDHCPLeases(string(output), vmName)
	}

	if ip == "" {
		return nil, fmt.Errorf("libvirt: no IP address found for VM %s", vmName)
	}

	// Read private key
	if p.keyPath == "" {
		return nil, fmt.Errorf("libvirt: keyPath not configured")
	}

	keyData, err := os.ReadFile(p.keyPath)
	if err != nil {
		return nil, fmt.Errorf("libvirt: failed to read private key from %s: %w", p.keyPath, err)
	}

	return &client.VMInfo{
		Host:       ip,
		Port:       p.defaultPort,
		User:       p.defaultUser,
		PrivateKey: keyData,
	}, nil
}

// parseIPFromVirshOutput extracts IP from virsh domifaddr output.
// Example format:
// Name       MAC address          Protocol     Address
// -------------------------------------------------------------------------------
// vnet0      52:54:00:xx:xx:xx    ipv4         192.168.122.100/24
func parseIPFromVirshOutput(output string) string {
	// Match IP address with CIDR notation
	re := regexp.MustCompile(`ipv4\s+(\d+\.\d+\.\d+\.\d+)/\d+`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseIPFromDHCPLeases extracts IP from virsh net-dhcp-leases output for a specific VM.
// Example format:
// Expiry Time           MAC address         Protocol   IP address          Hostname        Client ID or DUID
// ----------------------------------------------------------------------------------------------------------------------------------------
// 2025-12-05 10:00:00   52:54:00:xx:xx:xx   ipv4       192.168.122.100/24  test-vm         -
func parseIPFromDHCPLeases(output, vmName string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Look for lines containing the VM name
		if !strings.Contains(line, vmName) {
			continue
		}

		// Match IP address with CIDR notation
		re := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)/\d+`)
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 2 {
			return matches[1]
		}
	}
	return ""
}
