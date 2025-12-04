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

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

func TestGenerateMetaData(t *testing.T) {
	vmName := "test-vm"
	metaData := generateMetaData(vmName)

	// Verify instance-id
	if !strings.Contains(metaData, "instance-id: test-vm") {
		t.Error("meta-data should contain instance-id")
	}

	// Verify hostname
	if !strings.Contains(metaData, "hostname: test-vm") {
		t.Error("meta-data should contain hostname")
	}

	// Verify local-hostname
	if !strings.Contains(metaData, "local-hostname: test-vm") {
		t.Error("meta-data should contain local-hostname")
	}
}

func TestGenerateUserData_DefaultUser(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
	}

	userData := generateUserData(config)

	// Should have cloud-config header
	if !strings.HasPrefix(userData, "#cloud-config") {
		t.Error("user-data should start with #cloud-config")
	}

	// Should use default username ubuntu
	if !strings.Contains(userData, "name: ubuntu") {
		t.Error("user-data should use default username 'ubuntu'")
	}

	// Should have boot-finished marker
	if !strings.Contains(userData, "touch /var/lib/cloud/instance/boot-finished") {
		t.Error("user-data should include boot-finished marker")
	}
}

func TestGenerateUserData_WithUsername(t *testing.T) {
	config := &CloudInitConfig{
		VMName:   "test-vm",
		Username: "testuser",
	}

	userData := generateUserData(config)

	if !strings.Contains(userData, "name: testuser") {
		t.Error("user-data should use specified username")
	}
}

func TestGenerateUserData_WithSSHKeys(t *testing.T) {
	config := &CloudInitConfig{
		VMName:   "test-vm",
		Username: "testuser",
		SSHKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test@example.com",
			"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAATest test2@example.com",
		},
	}

	userData := generateUserData(config)

	// Should have ssh-authorized-keys section
	if !strings.Contains(userData, "ssh-authorized-keys:") {
		t.Error("user-data should contain ssh-authorized-keys section")
	}

	// Should contain both keys
	if !strings.Contains(userData, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest") {
		t.Error("user-data should contain ed25519 key")
	}
	if !strings.Contains(userData, "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAATest") {
		t.Error("user-data should contain rsa key")
	}
}

func TestGenerateUserData_WithPackages(t *testing.T) {
	config := &CloudInitConfig{
		VMName:   "test-vm",
		Packages: []string{"vim", "git", "curl"},
	}

	userData := generateUserData(config)

	// Should have packages section
	if !strings.Contains(userData, "packages:") {
		t.Error("user-data should contain packages section")
	}

	// Should contain all packages
	for _, pkg := range config.Packages {
		if !strings.Contains(userData, "- "+pkg) {
			t.Errorf("user-data should contain package %s", pkg)
		}
	}
}

func TestGenerateUserData_WithRunCMD(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
		RunCMD: []string{"echo hello", "apt-get update"},
	}

	userData := generateUserData(config)

	// Should have runcmd section
	if !strings.Contains(userData, "runcmd:") {
		t.Error("user-data should contain runcmd section")
	}

	// Should contain commands
	if !strings.Contains(userData, "echo hello") {
		t.Error("user-data should contain first command")
	}
	if !strings.Contains(userData, "apt-get update") {
		t.Error("user-data should contain second command")
	}
}

func TestGenerateNetworkConfig(t *testing.T) {
	networkConfig := generateNetworkConfig()

	// Should have version 2
	if !strings.Contains(networkConfig, "version: 2") {
		t.Error("network-config should have version 2")
	}

	// Should have ethernets section
	if !strings.Contains(networkConfig, "ethernets:") {
		t.Error("network-config should have ethernets section")
	}

	// Should enable dhcp4
	if !strings.Contains(networkConfig, "dhcp4: true") {
		t.Error("network-config should enable dhcp4")
	}

	// Should match common interface name patterns (en* and eth*)
	if !strings.Contains(networkConfig, `name: "en*"`) {
		t.Error("network-config should match en* interface names")
	}
	if !strings.Contains(networkConfig, `name: "eth*"`) {
		t.Error("network-config should match eth* interface names")
	}
}

func TestCloudInitConfigFromVMSpec_Defaults(t *testing.T) {
	vmName := "test-vm"
	spec := &providerv1.VMSpec{}

	config := cloudInitConfigFromVMSpec(vmName, spec, nil)

	if config.VMName != vmName {
		t.Errorf("Expected VMName %s, got %s", vmName, config.VMName)
	}

	if config.Username != "ubuntu" {
		t.Errorf("Expected default username 'ubuntu', got %s", config.Username)
	}

	if len(config.MatchedKeyNames) != 0 {
		t.Errorf("Expected empty MatchedKeyNames, got %v", config.MatchedKeyNames)
	}
}

func TestCloudInitConfigFromVMSpec_WithCloudInit(t *testing.T) {
	vmName := "test-vm"
	spec := &providerv1.VMSpec{
		CloudInit: &providerv1.CloudInitSpec{
			Users: []providerv1.UserSpec{
				{
					Name: "customuser",
					SSHAuthorizedKeys: []string{
						"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test@example.com",
					},
				},
			},
			Packages:    []string{"vim", "git"},
			RunCommands: []string{"echo hello"},
		},
	}

	config := cloudInitConfigFromVMSpec(vmName, spec, nil)

	if config.Username != "customuser" {
		t.Errorf("Expected username 'customuser', got %s", config.Username)
	}

	if len(config.SSHKeys) != 1 {
		t.Errorf("Expected 1 SSH key, got %d", len(config.SSHKeys))
	}

	if len(config.Packages) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(config.Packages))
	}

	if len(config.RunCMD) != 1 {
		t.Errorf("Expected 1 run command, got %d", len(config.RunCMD))
	}
}

func TestCloudInitConfigFromVMSpec_MatchesProviderKeys(t *testing.T) {
	vmName := "test-vm"
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test@example.com\n"

	spec := &providerv1.VMSpec{
		CloudInit: &providerv1.CloudInitSpec{
			Users: []providerv1.UserSpec{
				{
					Name: "testuser",
					SSHAuthorizedKeys: []string{
						"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test@example.com",
					},
				},
			},
		},
	}

	keys := map[string]*providerv1.KeyState{
		"my-key": {
			Name:      "my-key",
			PublicKey: publicKey,
		},
		"other-key": {
			Name:      "other-key",
			PublicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAOther other@example.com\n",
		},
	}

	config := cloudInitConfigFromVMSpec(vmName, spec, keys)

	if len(config.MatchedKeyNames) != 1 {
		t.Errorf("Expected 1 matched key, got %d", len(config.MatchedKeyNames))
	}

	if len(config.MatchedKeyNames) > 0 && config.MatchedKeyNames[0] != "my-key" {
		t.Errorf("Expected matched key 'my-key', got %s", config.MatchedKeyNames[0])
	}
}

func TestCloudInitConfigFromVMSpec_NoMatchingKeys(t *testing.T) {
	vmName := "test-vm"
	spec := &providerv1.VMSpec{
		CloudInit: &providerv1.CloudInitSpec{
			Users: []providerv1.UserSpec{
				{
					Name: "testuser",
					SSHAuthorizedKeys: []string{
						"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIUnmatched unmatched@example.com",
					},
				},
			},
		},
	}

	keys := map[string]*providerv1.KeyState{
		"my-key": {
			Name:      "my-key",
			PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDifferent different@example.com\n",
		},
	}

	config := cloudInitConfigFromVMSpec(vmName, spec, keys)

	if len(config.MatchedKeyNames) != 0 {
		t.Errorf("Expected 0 matched keys, got %d", len(config.MatchedKeyNames))
	}
}
