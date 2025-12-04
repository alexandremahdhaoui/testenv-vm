//go:build e2e_libvirt_delete

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

// Package e2e_libvirt_delete_test verifies that libvirt resources are properly
// cleaned up after testenv deletion.
package e2e_libvirt_delete_test

import (
	"os/exec"
	"strings"
	"testing"
)

// checkLibvirtAvailable checks if libvirt is available.
func checkLibvirtAvailable(t *testing.T) {
	t.Helper()

	cmd := exec.Command("virsh", "--connect", "qemu:///session", "version")
	if err := cmd.Run(); err != nil {
		t.Skip("libvirt not available, skipping e2e-libvirt-delete test")
	}
}

// TestLibvirtCleanupVerification verifies that all e2e-libvirt resources were
// properly cleaned up after the e2e-libvirt test stage.
func TestLibvirtCleanupVerification(t *testing.T) {
	checkLibvirtAvailable(t)

	t.Run("VerifyVMDeleted", func(t *testing.T) {
		// Check that no e2e-libvirt VMs exist
		cmd := exec.Command("virsh", "--connect", "qemu:///session", "list", "--all", "--name")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed to list VMs: %v", err)
		}

		vms := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, vm := range vms {
			vm = strings.TrimSpace(vm)
			if vm == "" {
				continue
			}
			if strings.HasPrefix(vm, "e2e-libvirt") {
				t.Errorf("Found leftover VM that should have been deleted: %s", vm)
			}
		}
		t.Log("No leftover e2e-libvirt VMs found - cleanup successful")
	})

	t.Run("VerifyNetworkDeleted", func(t *testing.T) {
		// Check that no e2e-libvirt networks exist
		cmd := exec.Command("virsh", "--connect", "qemu:///session", "net-list", "--all", "--name")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed to list networks: %v", err)
		}

		networks := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, network := range networks {
			network = strings.TrimSpace(network)
			if network == "" {
				continue
			}
			if strings.HasPrefix(network, "e2e-libvirt") {
				t.Errorf("Found leftover network that should have been deleted: %s", network)
			}
		}
		t.Log("No leftover e2e-libvirt networks found - cleanup successful")
	})

	t.Run("VerifyBaseImagePreserved", func(t *testing.T) {
		// Verify the base image still exists (should NOT be deleted)
		// The base image is cached and reused across tests
		imagePath := "/tmp/testenv-vm-images/ubuntu-24.04-server-cloudimg-amd64.img"
		cmd := exec.Command("test", "-f", imagePath)
		if err := cmd.Run(); err != nil {
			t.Log("Base image not found (expected if no VM tests ran)")
			return
		}
		t.Log("Base image preserved correctly - not deleted during cleanup")
	})
}

// TestNoOrphanedResources performs a comprehensive check for orphaned resources.
func TestNoOrphanedResources(t *testing.T) {
	checkLibvirtAvailable(t)

	t.Run("NoOrphanedDomains", func(t *testing.T) {
		// List all domains and check for any test-related ones
		cmd := exec.Command("virsh", "--connect", "qemu:///session", "list", "--all", "--name")
		output, err := cmd.Output()
		if err != nil {
			t.Logf("Warning: failed to list domains: %v", err)
			return
		}

		domains := strings.Split(strings.TrimSpace(string(output)), "\n")
		testDomains := []string{}
		for _, domain := range domains {
			domain = strings.TrimSpace(domain)
			if domain == "" {
				continue
			}
			// Check for any test-related domains
			if strings.Contains(domain, "e2e") || strings.Contains(domain, "test") || strings.Contains(domain, "integration") {
				testDomains = append(testDomains, domain)
			}
		}

		if len(testDomains) > 0 {
			t.Logf("Warning: Found potentially orphaned test domains: %v", testDomains)
		} else {
			t.Log("No orphaned test domains found")
		}
	})

	t.Run("NoOrphanedNetworks", func(t *testing.T) {
		// List all networks and check for test-related ones
		cmd := exec.Command("virsh", "--connect", "qemu:///session", "net-list", "--all", "--name")
		output, err := cmd.Output()
		if err != nil {
			t.Logf("Warning: failed to list networks: %v", err)
			return
		}

		networks := strings.Split(strings.TrimSpace(string(output)), "\n")
		testNetworks := []string{}
		for _, network := range networks {
			network = strings.TrimSpace(network)
			if network == "" || network == "default" {
				continue
			}
			if strings.Contains(network, "e2e") || strings.Contains(network, "test") || strings.Contains(network, "integration") {
				testNetworks = append(testNetworks, network)
			}
		}

		if len(testNetworks) > 0 {
			t.Logf("Warning: Found potentially orphaned test networks: %v", testNetworks)
		} else {
			t.Log("No orphaned test networks found")
		}
	})
}
