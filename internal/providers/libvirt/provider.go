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

// Package libvirt provides a libvirt-based provider that manages VMs, networks,
// and SSH keys through the libvirt API. This provider uses qemu-img for disk
// management and cloud-init for VM configuration.
package libvirt

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/digitalocean/go-libvirt"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// ProviderConfig holds configuration for the libvirt provider.
type ProviderConfig struct {
	// URI is the libvirt connection URI (e.g., "qemu:///system", "qemu:///session")
	URI string
	// StateDir is the directory where provider artifacts are stored (keys, disks, ISOs)
	StateDir string
	// ISOTool is the path to the ISO generation tool (genisoimage, mkisofs, or xorriso)
	ISOTool string
	// QemuImgPath is the path to qemu-img binary
	QemuImgPath string
}

// Provider is a libvirt-based provider that manages VMs, networks, and SSH keys.
type Provider struct {
	config   ProviderConfig
	conn     *libvirt.Libvirt
	mu       sync.RWMutex
	keys     map[string]*providerv1.KeyState
	networks map[string]*providerv1.NetworkState
	vms      map[string]*providerv1.VMState
}

// NewProvider creates a new libvirt provider with the given configuration.
// It reads configuration from environment variables:
//   - TESTENV_VM_LIBVIRT_URI: libvirt connection URI (default: qemu:///system)
//   - TESTENV_VM_STATE_DIR: state directory (default: /var/lib/testenv-vm or ~/.testenv-vm)
//
// It checks for required dependencies (genisoimage/mkisofs/xorriso, qemu-img)
// and creates the necessary state directories.
func NewProvider() (*Provider, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Check for ISO generation tool
	isoTool, err := findISOTool()
	if err != nil {
		return nil, err
	}
	config.ISOTool = isoTool

	// Check for qemu-img
	qemuImgPath, err := exec.LookPath("qemu-img")
	if err != nil {
		return nil, fmt.Errorf("disk image creation requires qemu-img: %w", err)
	}
	config.QemuImgPath = qemuImgPath

	// Create state directories
	if err := createStateDirs(config.StateDir); err != nil {
		return nil, fmt.Errorf("failed to create state directories: %w", err)
	}

	// Connect to libvirt
	conn, err := connectLibvirt(config.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to libvirt: %w", err)
	}

	return &Provider{
		config:   config,
		conn:     conn,
		keys:     make(map[string]*providerv1.KeyState),
		networks: make(map[string]*providerv1.NetworkState),
		vms:      make(map[string]*providerv1.VMState),
	}, nil
}

// NewProviderWithConfig creates a provider with explicit configuration.
// This is useful for testing with custom settings.
func NewProviderWithConfig(config ProviderConfig, conn *libvirt.Libvirt) *Provider {
	return &Provider{
		config:   config,
		conn:     conn,
		keys:     make(map[string]*providerv1.KeyState),
		networks: make(map[string]*providerv1.NetworkState),
		vms:      make(map[string]*providerv1.VMState),
	}
}

// Close closes the libvirt connection.
func (p *Provider) Close() error {
	if p.conn != nil {
		return p.conn.Disconnect()
	}
	return nil
}

// loadConfig loads provider configuration from environment variables.
func loadConfig() (ProviderConfig, error) {
	uri := os.Getenv("TESTENV_VM_LIBVIRT_URI")
	if uri == "" {
		// Default to session mode for non-root users
		// Use qemu:///system only when running as root
		if os.Getuid() == 0 {
			uri = "qemu:///system"
		} else {
			uri = "qemu:///session"
		}
	}

	stateDir := os.Getenv("TESTENV_VM_STATE_DIR")
	if stateDir == "" {
		// Default state directory depends on connection type
		if strings.Contains(uri, "session") {
			// Use /tmp-based directory for session mode to avoid home directory
			// traversal permission issues with libvirt. The libvirt daemon runs
			// as a different user and needs execute permission on all parent
			// directories to access disk files. Using /tmp ensures accessibility.
			stateDir = filepath.Join(os.TempDir(), fmt.Sprintf("testenv-vm-%d", os.Getuid()))
		} else {
			stateDir = "/var/lib/testenv-vm"
		}
	}

	return ProviderConfig{
		URI:      uri,
		StateDir: stateDir,
	}, nil
}

// findISOTool finds an available ISO generation tool.
func findISOTool() (string, error) {
	tools := []string{"genisoimage", "mkisofs", "xorriso"}
	for _, tool := range tools {
		path, err := exec.LookPath(tool)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("cloud-init ISO generation requires genisoimage, mkisofs, or xorriso")
}

// createStateDirs creates the required state directories with proper permissions
// for libvirt access.
func createStateDirs(stateDir string) error {
	dirs := []string{
		stateDir,
		filepath.Join(stateDir, "keys"),
		filepath.Join(stateDir, "disks"),
		filepath.Join(stateDir, "cloudinit"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		// Ensure permissions are set correctly
		if err := os.Chmod(dir, 0755); err != nil {
			return fmt.Errorf("failed to chmod directory %s: %w", dir, err)
		}
	}

	// Set ACLs for libvirt groups to allow access to disk files
	setLibvirtACLs(stateDir)
	setLibvirtACLs(filepath.Join(stateDir, "disks"))
	setLibvirtACLs(filepath.Join(stateDir, "cloudinit"))

	return nil
}

// setLibvirtACLs sets ACLs on a directory for libvirt groups.
// This allows the libvirt daemon to access disk files created by the provider.
func setLibvirtACLs(dir string) {
	setfaclPath, err := exec.LookPath("setfacl")
	if err != nil {
		return // setfacl not available, skip ACL setup
	}

	libvirtGroups := []string{"libvirt", "libvirt-qemu", "kvm", "qemu"}
	for _, group := range libvirtGroups {
		// Check if group exists
		checkCmd := exec.Command("getent", "group", group)
		if err := checkCmd.Run(); err != nil {
			continue // Group doesn't exist
		}

		// Set ACL for this group
		aclCmd := exec.Command(setfaclPath, "-m", "g:"+group+":rwx", dir)
		_ = aclCmd.Run()

		// Set default ACL so new files inherit permissions
		defaultAclCmd := exec.Command(setfaclPath, "-d", "-m", "g:"+group+":rwx", dir)
		_ = defaultAclCmd.Run()
	}
}

// connectLibvirt establishes a connection to the libvirt daemon.
func connectLibvirt(uri string) (*libvirt.Libvirt, error) {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid libvirt URI: %w", err)
	}

	conn, err := libvirt.ConnectToURI(parsedURI)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", uri, err)
	}

	return conn, nil
}

// Config returns the provider configuration.
func (p *Provider) Config() ProviderConfig {
	return p.config
}
