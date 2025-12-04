//go:build integration

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

// Package libvirt provides integration tests for the libvirt provider.
// These tests require a running libvirt daemon and associated tools.
//
// Run with: go test -tags=integration ./internal/providers/libvirt/...
//
// Prerequisites:
// - libvirt daemon running (libvirtd)
// - qemu-img installed
// - genisoimage, mkisofs, or xorriso installed
// - User must have permission to connect to libvirt (e.g., in libvirt group)
// - wget for downloading cloud images (auto-downloaded if not cached)
//
// Optional:
// - Set TESTENV_VM_LIBVIRT_URI to customize the connection (default: qemu:///session)
// - Set TESTENV_VM_IMAGE_CACHE_DIR to customize image cache location (default: /tmp/testenv-vm-images)
package libvirt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

const (
	// Ubuntu 24.04 LTS cloud image
	defaultImageName = "ubuntu-24.04-server-cloudimg-amd64.img"
	defaultImageURL  = "https://cloud-images.ubuntu.com/releases/noble/release/" + defaultImageName
)

// integrationTestProvider creates a provider configured for integration testing.
// It skips the test if libvirt or required tools are not available.
func integrationTestProvider(t *testing.T) (*Provider, func()) {
	t.Helper()

	// Check if libvirt tools are available
	if !checkLibvirtAvailable(t) {
		t.Skip("libvirt not available, skipping integration test")
	}

	// Check for qemu-img
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available, skipping integration test")
	}

	// Check for ISO tool
	isoToolAvailable := false
	for _, tool := range []string{"genisoimage", "mkisofs", "xorriso"} {
		if _, err := exec.LookPath(tool); err == nil {
			isoToolAvailable = true
			break
		}
	}
	if !isoToolAvailable {
		t.Skip("no ISO generation tool available (genisoimage/mkisofs/xorriso), skipping integration test")
	}

	// Create a temporary state directory
	tmpDir, err := os.MkdirTemp("", "libvirt-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}

	// Prepare directory with libvirt-accessible permissions
	prepareLibvirtDir(t, tmpDir)

	// Also prepare subdirectories that will be created by the provider
	disksDir := filepath.Join(tmpDir, "disks")
	if err := os.MkdirAll(disksDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create disks directory: %v", err)
	}
	prepareLibvirtDir(t, disksDir)

	cloudinitDir := filepath.Join(tmpDir, "cloudinit")
	if err := os.MkdirAll(cloudinitDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create cloudinit directory: %v", err)
	}
	prepareLibvirtDir(t, cloudinitDir)

	// Set up environment for session-based testing
	// Use qemu:///session for non-root testing
	uri := os.Getenv("TESTENV_VM_LIBVIRT_URI")
	if uri == "" {
		uri = "qemu:///session"
	}

	os.Setenv("TESTENV_VM_LIBVIRT_URI", uri)
	os.Setenv("TESTENV_VM_STATE_DIR", tmpDir)

	// Create the provider
	provider, err := NewProvider()
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Skipf("failed to create provider (libvirt may not be running): %v", err)
	}

	cleanup := func() {
		provider.Close()
		os.RemoveAll(tmpDir)
	}

	return provider, cleanup
}

// checkLibvirtAvailable checks if libvirt daemon is accessible.
func checkLibvirtAvailable(t *testing.T) bool {
	t.Helper()

	// Try to run virsh to check if libvirt is available
	cmd := exec.Command("virsh", "--connect", "qemu:///session", "version")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// prepareLibvirtDir prepares a directory with permissions accessible by libvirt.
// This is necessary because libvirt runs as a separate user and needs to access disk files.
func prepareLibvirtDir(t *testing.T, dir string) {
	t.Helper()

	// Set directory permissions to 0755 (world readable/executable)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Logf("Warning: failed to chmod directory %s: %v", dir, err)
	}

	// Try to set permissions on parent directories as well
	// Libvirt needs +x permission on all ancestor directories to traverse
	currentDir := dir
	for {
		if err := os.Chmod(currentDir, 0o755); err != nil {
			t.Logf("Warning: failed to chmod directory %s: %v", currentDir, err)
		}

		// Stop at /tmp or root
		if currentDir == "/tmp" || currentDir == "/" {
			break
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}

	// Try to set ACLs for libvirt groups (optional, requires setfacl)
	libvirtGroups := []string{"libvirt", "libvirt-qemu", "kvm", "qemu"}
	setfaclPath, err := exec.LookPath("setfacl")
	if err != nil {
		return // setfacl not available, skip ACL setup
	}

	for _, group := range libvirtGroups {
		// Check if group exists
		checkCmd := exec.Command("getent", "group", group)
		if err := checkCmd.Run(); err != nil {
			continue // Group doesn't exist
		}

		// Set ACL for this group
		aclCmd := exec.Command(setfaclPath, "-m", "g:"+group+":rwx", dir)
		if err := aclCmd.Run(); err != nil {
			t.Logf("Warning: failed to set ACL for group %s: %v", group, err)
			continue
		}

		// Also set default ACL so new files inherit permissions
		defaultAclCmd := exec.Command(setfaclPath, "-d", "-m", "g:"+group+":rwx", dir)
		if err := defaultAclCmd.Run(); err != nil {
			t.Logf("Warning: failed to set default ACL for group %s: %v", group, err)
		}

		t.Logf("Set ACL on %s for group %s", dir, group)
	}
}

// ensureBaseImage ensures the base cloud image is available, downloading it if necessary.
// Returns the path to the base image.
func ensureBaseImage(t *testing.T) string {
	t.Helper()

	// Determine cache directory
	cacheDir := os.Getenv("TESTENV_VM_IMAGE_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "/tmp/testenv-vm-images"
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("failed to create image cache directory: %v", err)
	}

	// Prepare with libvirt-accessible permissions
	prepareLibvirtDir(t, cacheDir)

	imagePath := filepath.Join(cacheDir, defaultImageName)

	// Check if image already exists
	if _, err := os.Stat(imagePath); err == nil {
		t.Logf("Using cached base image: %s", imagePath)
		return imagePath
	}

	// Check if wget is available
	if _, err := exec.LookPath("wget"); err != nil {
		t.Fatalf("wget not found, cannot download base image")
	}

	// Download the image
	t.Logf("Downloading base image from %s to %s (this may take a few minutes)...", defaultImageURL, imagePath)

	// Use wget with progress indicator
	cmd := exec.Command("wget",
		"--progress=dot:giga",
		"-O", imagePath,
		defaultImageURL,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Clean up partial download
		os.Remove(imagePath)
		t.Fatalf("failed to download base image: %v", err)
	}

	t.Logf("Successfully downloaded base image: %s", imagePath)
	return imagePath
}

// TestIntegration_KeyLifecycle tests the full key lifecycle with libvirt.
func TestIntegration_KeyLifecycle(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	// Test creating an ED25519 key
	t.Run("CreateED25519Key", func(t *testing.T) {
		req := &providerv1.KeyCreateRequest{
			Name: "integration-test-ed25519",
			Spec: providerv1.KeySpec{
				Type: "ed25519",
			},
		}

		result := provider.KeyCreate(req)
		if !result.Success {
			t.Fatalf("KeyCreate failed: %v", result.Error)
		}

		keyState := result.Resource.(*providerv1.KeyState)

		// Verify key properties
		if keyState.Name != "integration-test-ed25519" {
			t.Errorf("expected name 'integration-test-ed25519', got %s", keyState.Name)
		}
		if keyState.Type != "ed25519" {
			t.Errorf("expected type 'ed25519', got %s", keyState.Type)
		}
		if !strings.HasPrefix(keyState.PublicKey, "ssh-ed25519") {
			t.Errorf("expected public key to start with 'ssh-ed25519', got %s", keyState.PublicKey[:20])
		}
		if !strings.HasPrefix(keyState.Fingerprint, "SHA256:") {
			t.Errorf("expected fingerprint to start with 'SHA256:', got %s", keyState.Fingerprint)
		}

		// Verify files exist
		if _, err := os.Stat(keyState.PrivateKeyPath); err != nil {
			t.Errorf("private key file should exist: %v", err)
		}
		if _, err := os.Stat(keyState.PublicKeyPath); err != nil {
			t.Errorf("public key file should exist: %v", err)
		}
	})

	// Test creating an RSA key
	t.Run("CreateRSAKey", func(t *testing.T) {
		req := &providerv1.KeyCreateRequest{
			Name: "integration-test-rsa",
			Spec: providerv1.KeySpec{
				Type: "rsa",
				Bits: 2048,
			},
		}

		result := provider.KeyCreate(req)
		if !result.Success {
			t.Fatalf("KeyCreate failed: %v", result.Error)
		}

		keyState := result.Resource.(*providerv1.KeyState)
		if keyState.Type != "rsa" {
			t.Errorf("expected type 'rsa', got %s", keyState.Type)
		}
		if !strings.HasPrefix(keyState.PublicKey, "ssh-rsa") {
			t.Errorf("expected public key to start with 'ssh-rsa'")
		}
	})

	// Test listing keys
	t.Run("ListKeys", func(t *testing.T) {
		result := provider.KeyList(nil)
		if !result.Success {
			t.Fatalf("KeyList failed: %v", result.Error)
		}

		keyList := result.Resource.([]*providerv1.KeyState)
		if len(keyList) != 2 {
			t.Errorf("expected 2 keys, got %d", len(keyList))
		}
	})

	// Test getting a specific key
	t.Run("GetKey", func(t *testing.T) {
		result := provider.KeyGet("integration-test-ed25519")
		if !result.Success {
			t.Fatalf("KeyGet failed: %v", result.Error)
		}

		keyState := result.Resource.(*providerv1.KeyState)
		if keyState.Name != "integration-test-ed25519" {
			t.Errorf("expected name 'integration-test-ed25519', got %s", keyState.Name)
		}
	})

	// Test deleting keys
	t.Run("DeleteKeys", func(t *testing.T) {
		// Delete ED25519 key
		result := provider.KeyDelete("integration-test-ed25519")
		if !result.Success {
			t.Fatalf("KeyDelete failed: %v", result.Error)
		}

		// Verify it's gone
		getResult := provider.KeyGet("integration-test-ed25519")
		if getResult.Success {
			t.Error("key should be deleted")
		}

		// Delete RSA key
		result = provider.KeyDelete("integration-test-rsa")
		if !result.Success {
			t.Fatalf("KeyDelete failed: %v", result.Error)
		}

		// Verify list is empty
		listResult := provider.KeyList(nil)
		if !listResult.Success {
			t.Fatalf("KeyList failed: %v", listResult.Error)
		}
		keyList := listResult.Resource.([]*providerv1.KeyState)
		if len(keyList) != 0 {
			t.Errorf("expected 0 keys after deletion, got %d", len(keyList))
		}
	})
}

// TestIntegration_NetworkLifecycle tests the full network lifecycle with libvirt.
func TestIntegration_NetworkLifecycle(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	networkName := "integration-test-net"

	// Cleanup any existing network with same name (from failed previous runs)
	_ = provider.NetworkDelete(networkName)

	// Test creating a NAT network
	t.Run("CreateNATNetwork", func(t *testing.T) {
		req := &providerv1.NetworkCreateRequest{
			Name: networkName,
			Kind: "nat",
			Spec: providerv1.NetworkSpec{
				CIDR: "192.168.222.0/24",
			},
		}

		result := provider.NetworkCreate(req)
		if !result.Success {
			t.Fatalf("NetworkCreate failed: %v", result.Error)
		}

		networkState := result.Resource.(*providerv1.NetworkState)

		// Verify network properties
		if networkState.Name != networkName {
			t.Errorf("expected name '%s', got %s", networkName, networkState.Name)
		}
		if networkState.Kind != "nat" {
			t.Errorf("expected kind 'nat', got %s", networkState.Kind)
		}
		if networkState.Status != "active" {
			t.Errorf("expected status 'active', got %s", networkState.Status)
		}
		if networkState.IP == "" {
			t.Error("expected network to have an IP address")
		}
		if networkState.CIDR != "192.168.222.0/24" {
			t.Errorf("expected CIDR '192.168.222.0/24', got %s", networkState.CIDR)
		}
		if networkState.UUID == "" {
			t.Error("expected network to have a UUID")
		}
	})

	// Test getting the network
	t.Run("GetNetwork", func(t *testing.T) {
		result := provider.NetworkGet(networkName)
		if !result.Success {
			t.Fatalf("NetworkGet failed: %v", result.Error)
		}

		networkState := result.Resource.(*providerv1.NetworkState)
		if networkState.Name != networkName {
			t.Errorf("expected name '%s', got %s", networkName, networkState.Name)
		}
	})

	// Test listing networks
	t.Run("ListNetworks", func(t *testing.T) {
		result := provider.NetworkList(nil)
		if !result.Success {
			t.Fatalf("NetworkList failed: %v", result.Error)
		}

		networkList := result.Resource.([]*providerv1.NetworkState)
		found := false
		for _, net := range networkList {
			if net.Name == networkName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("network '%s' not found in list", networkName)
		}
	})

	// Test deleting the network
	t.Run("DeleteNetwork", func(t *testing.T) {
		result := provider.NetworkDelete(networkName)
		if !result.Success {
			t.Fatalf("NetworkDelete failed: %v", result.Error)
		}

		// Verify it's gone
		getResult := provider.NetworkGet(networkName)
		if getResult.Success {
			t.Error("network should be deleted")
		}
	})
}

// TestIntegration_IsolatedNetwork tests creating an isolated network.
func TestIntegration_IsolatedNetwork(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	networkName := "integration-test-isolated"

	// Cleanup any existing network
	_ = provider.NetworkDelete(networkName)

	// Create isolated network
	req := &providerv1.NetworkCreateRequest{
		Name: networkName,
		Kind: "isolated",
		Spec: providerv1.NetworkSpec{
			CIDR: "10.99.0.0/24",
		},
	}

	result := provider.NetworkCreate(req)
	if !result.Success {
		t.Fatalf("NetworkCreate failed: %v", result.Error)
	}

	networkState := result.Resource.(*providerv1.NetworkState)

	if networkState.Kind != "isolated" {
		t.Errorf("expected kind 'isolated', got %s", networkState.Kind)
	}

	// Cleanup
	provider.NetworkDelete(networkName)
}

// TestIntegration_VMLifecycle tests the full VM lifecycle with libvirt.
// The base image is automatically downloaded if not cached.
func TestIntegration_VMLifecycle(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	// Ensure base image is available (downloads if needed)
	baseImage := ensureBaseImage(t)

	vmName := "integration-test-vm"
	networkName := "integration-test-vm-net"
	keyName := "integration-test-vm-key"

	// Cleanup any existing resources
	_ = provider.VMDelete(vmName)
	_ = provider.NetworkDelete(networkName)
	_ = provider.KeyDelete(keyName)

	// Create a key first
	t.Run("CreateKey", func(t *testing.T) {
		req := &providerv1.KeyCreateRequest{
			Name: keyName,
			Spec: providerv1.KeySpec{
				Type: "ed25519",
			},
		}
		result := provider.KeyCreate(req)
		if !result.Success {
			t.Fatalf("KeyCreate failed: %v", result.Error)
		}
	})

	// Create a network
	t.Run("CreateNetwork", func(t *testing.T) {
		req := &providerv1.NetworkCreateRequest{
			Name: networkName,
			Kind: "nat",
			Spec: providerv1.NetworkSpec{
				CIDR: "192.168.223.0/24",
			},
		}
		result := provider.NetworkCreate(req)
		if !result.Success {
			t.Fatalf("NetworkCreate failed: %v", result.Error)
		}
	})

	// Get the key's public key for cloud-init
	keyResult := provider.KeyGet(keyName)
	if !keyResult.Success {
		t.Fatalf("KeyGet failed: %v", keyResult.Error)
	}
	keyState := keyResult.Resource.(*providerv1.KeyState)

	// Create the VM
	t.Run("CreateVM", func(t *testing.T) {
		req := &providerv1.VMCreateRequest{
			Name: vmName,
			Spec: providerv1.VMSpec{
				Memory:  512,
				VCPUs:   1,
				Network: networkName,
				Disk: providerv1.DiskSpec{
					BaseImage: baseImage,
					Size:      "5G",
				},
				CloudInit: &providerv1.CloudInitSpec{
					Hostname: vmName,
					Users: []providerv1.UserSpec{
						{
							Name: "testuser",
							Sudo: "ALL=(ALL) NOPASSWD:ALL",
							SSHAuthorizedKeys: []string{
								strings.TrimSpace(keyState.PublicKey),
							},
						},
					},
				},
			},
		}

		result := provider.VMCreate(req)
		if !result.Success {
			t.Fatalf("VMCreate failed: %v", result.Error)
		}

		vmState := result.Resource.(*providerv1.VMState)

		// Verify VM properties
		if vmState.Name != vmName {
			t.Errorf("expected name '%s', got %s", vmName, vmState.Name)
		}
		if vmState.Status != "running" {
			t.Errorf("expected status 'running', got %s", vmState.Status)
		}
		if vmState.UUID == "" {
			t.Error("expected VM to have a UUID")
		}
		if vmState.MAC == "" {
			t.Error("expected VM to have a MAC address")
		}

		t.Logf("VM created: name=%s, uuid=%s, mac=%s, ip=%s",
			vmState.Name, vmState.UUID, vmState.MAC, vmState.IP)

		if vmState.SSHCommand != "" {
			t.Logf("SSH command: %s", vmState.SSHCommand)
		}
	})

	// Test getting the VM
	t.Run("GetVM", func(t *testing.T) {
		result := provider.VMGet(vmName)
		if !result.Success {
			t.Fatalf("VMGet failed: %v", result.Error)
		}

		vmState := result.Resource.(*providerv1.VMState)
		if vmState.Name != vmName {
			t.Errorf("expected name '%s', got %s", vmName, vmState.Name)
		}
	})

	// Test listing VMs
	t.Run("ListVMs", func(t *testing.T) {
		result := provider.VMList(nil)
		if !result.Success {
			t.Fatalf("VMList failed: %v", result.Error)
		}

		vmList := result.Resource.([]*providerv1.VMState)
		found := false
		for _, vm := range vmList {
			if vm.Name == vmName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("VM '%s' not found in list", vmName)
		}
	})

	// Wait a bit for the VM to fully start (optional)
	time.Sleep(2 * time.Second)

	// Test deleting the VM
	t.Run("DeleteVM", func(t *testing.T) {
		result := provider.VMDelete(vmName)
		if !result.Success {
			t.Fatalf("VMDelete failed: %v", result.Error)
		}

		// Verify it's gone
		getResult := provider.VMGet(vmName)
		if getResult.Success {
			t.Error("VM should be deleted")
		}
	})

	// Cleanup network and key
	t.Run("Cleanup", func(t *testing.T) {
		provider.NetworkDelete(networkName)
		provider.KeyDelete(keyName)
	})
}

// TestIntegration_NetworkDeletionBlockedByVM tests that a network cannot be deleted
// while a VM is using it.
func TestIntegration_NetworkDeletionBlockedByVM(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	// Ensure base image is available (downloads if needed)
	baseImage := ensureBaseImage(t)

	vmName := "integration-block-test-vm"
	networkName := "integration-block-test-net"

	// Cleanup any existing resources
	_ = provider.VMDelete(vmName)
	_ = provider.NetworkDelete(networkName)

	// Create network
	netReq := &providerv1.NetworkCreateRequest{
		Name: networkName,
		Kind: "nat",
		Spec: providerv1.NetworkSpec{
			CIDR: "192.168.224.0/24",
		},
	}
	netResult := provider.NetworkCreate(netReq)
	if !netResult.Success {
		t.Fatalf("NetworkCreate failed: %v", netResult.Error)
	}

	// Create VM using the network
	vmReq := &providerv1.VMCreateRequest{
		Name: vmName,
		Spec: providerv1.VMSpec{
			Memory:  256,
			VCPUs:   1,
			Network: networkName,
			Disk: providerv1.DiskSpec{
				BaseImage: baseImage,
				Size:      "2G",
			},
		},
	}
	vmResult := provider.VMCreate(vmReq)
	if !vmResult.Success {
		provider.NetworkDelete(networkName)
		t.Fatalf("VMCreate failed: %v", vmResult.Error)
	}

	// Try to delete the network - should fail
	deleteResult := provider.NetworkDelete(networkName)
	if deleteResult.Success {
		t.Error("expected network deletion to fail while VM is using it")
	}

	if deleteResult.Error == nil || deleteResult.Error.Code != providerv1.ErrCodeResourceBusy {
		t.Errorf("expected ErrCodeResourceBusy, got: %v", deleteResult.Error)
	}

	// Cleanup
	provider.VMDelete(vmName)
	provider.NetworkDelete(networkName)
}

// TestIntegration_KeyDeletionBlockedByVM tests that a key cannot be deleted
// while a VM is using it.
func TestIntegration_KeyDeletionBlockedByVM(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	// Ensure base image is available (downloads if needed)
	baseImage := ensureBaseImage(t)

	vmName := "integration-key-block-vm"
	networkName := "integration-key-block-net"
	keyName := "integration-key-block-key"

	// Cleanup any existing resources
	_ = provider.VMDelete(vmName)
	_ = provider.NetworkDelete(networkName)
	_ = provider.KeyDelete(keyName)

	// Create key
	keyReq := &providerv1.KeyCreateRequest{
		Name: keyName,
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}
	keyResult := provider.KeyCreate(keyReq)
	if !keyResult.Success {
		t.Fatalf("KeyCreate failed: %v", keyResult.Error)
	}
	keyState := keyResult.Resource.(*providerv1.KeyState)

	// Create network
	netReq := &providerv1.NetworkCreateRequest{
		Name: networkName,
		Kind: "nat",
		Spec: providerv1.NetworkSpec{
			CIDR: "192.168.225.0/24",
		},
	}
	netResult := provider.NetworkCreate(netReq)
	if !netResult.Success {
		provider.KeyDelete(keyName)
		t.Fatalf("NetworkCreate failed: %v", netResult.Error)
	}

	// Create VM using the key
	vmReq := &providerv1.VMCreateRequest{
		Name: vmName,
		Spec: providerv1.VMSpec{
			Memory:  256,
			VCPUs:   1,
			Network: networkName,
			Disk: providerv1.DiskSpec{
				BaseImage: baseImage,
				Size:      "2G",
			},
			CloudInit: &providerv1.CloudInitSpec{
				Users: []providerv1.UserSpec{
					{
						Name: "testuser",
						SSHAuthorizedKeys: []string{
							strings.TrimSpace(keyState.PublicKey),
						},
					},
				},
			},
		},
	}
	vmResult := provider.VMCreate(vmReq)
	if !vmResult.Success {
		provider.NetworkDelete(networkName)
		provider.KeyDelete(keyName)
		t.Fatalf("VMCreate failed: %v", vmResult.Error)
	}

	// Try to delete the key - should fail
	deleteResult := provider.KeyDelete(keyName)
	if deleteResult.Success {
		t.Error("expected key deletion to fail while VM is using it")
	}

	if deleteResult.Error == nil || deleteResult.Error.Code != providerv1.ErrCodeResourceBusy {
		t.Errorf("expected ErrCodeResourceBusy, got: %v", deleteResult.Error)
	}

	// Cleanup
	provider.VMDelete(vmName)
	provider.NetworkDelete(networkName)
	provider.KeyDelete(keyName)
}

// TestIntegration_ProviderCapabilities tests that capabilities are reported correctly.
func TestIntegration_ProviderCapabilities(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	caps := provider.Capabilities()

	if caps.ProviderName != "libvirt" {
		t.Errorf("expected provider name 'libvirt', got %s", caps.ProviderName)
	}

	if caps.Version == "" {
		t.Error("expected version to be set")
	}

	// Verify all resource types are supported
	resourceKinds := make(map[string]bool)
	for _, res := range caps.Resources {
		resourceKinds[res.Kind] = true
	}

	expectedKinds := []string{"key", "network", "vm"}
	for _, kind := range expectedKinds {
		if !resourceKinds[kind] {
			t.Errorf("expected resource kind '%s' to be supported", kind)
		}
	}
}
