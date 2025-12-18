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
	config := &CloudInitConfig{VMName: "test-vm"}
	metaData := generateMetaData(config)

	// Verify instance-id
	if !strings.Contains(metaData, "instance-id: test-vm") {
		t.Error("meta-data should contain instance-id")
	}

	// Verify hostname (fallback to VMName when Hostname is empty)
	if !strings.Contains(metaData, "hostname: test-vm") {
		t.Error("meta-data should contain hostname (fallback to VMName)")
	}

	// Verify local-hostname
	if !strings.Contains(metaData, "local-hostname: test-vm") {
		t.Error("meta-data should contain local-hostname")
	}
}

func TestGenerateMetaData_WithCustomHostname(t *testing.T) {
	config := &CloudInitConfig{
		VMName:   "test-vm",
		Hostname: "custom-host",
	}
	metaData := generateMetaData(config)

	// instance-id should still use VMName
	if !strings.Contains(metaData, "instance-id: test-vm") {
		t.Error("meta-data should use VMName for instance-id")
	}
	// hostname should use custom value
	if !strings.Contains(metaData, "hostname: custom-host") {
		t.Error("meta-data should use custom hostname")
	}
	if !strings.Contains(metaData, "local-hostname: custom-host") {
		t.Error("meta-data should use custom hostname for local-hostname")
	}
	// Should NOT contain VMName in hostname fields
	if strings.Contains(metaData, "hostname: test-vm") {
		t.Error("meta-data should not use VMName when custom hostname is set")
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
		VMName: "test-vm",
		Users: []providerv1.UserSpec{
			{Name: "testuser"},
		},
	}

	userData := generateUserData(config)

	if !strings.Contains(userData, "name: testuser") {
		t.Error("user-data should use specified username")
	}
}

func TestGenerateUserData_WithSSHKeys(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
		Users: []providerv1.UserSpec{
			{
				Name: "testuser",
				SSHAuthorizedKeys: []string{
					"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test@example.com",
					"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAATest test2@example.com",
				},
			},
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

func TestGenerateUserData_WithRuncmd(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
		Runcmd: []string{"echo hello", "apt-get update"},
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
	networkConfig := generateNetworkConfig(nil)

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

	// No default username - defaults are applied in generateUserData
	if len(config.Users) != 0 {
		t.Errorf("Expected empty Users, got %v", config.Users)
	}

	if len(config.MatchedKeyNames) != 0 {
		t.Errorf("Expected empty MatchedKeyNames, got %v", config.MatchedKeyNames)
	}
}

func TestCloudInitConfigFromVMSpec_WithCloudInit(t *testing.T) {
	vmName := "test-vm"
	spec := &providerv1.VMSpec{
		CloudInit: &providerv1.CloudInitSpec{
			Hostname: "custom-hostname",
			Users: []providerv1.UserSpec{
				{
					Name: "customuser",
					SSHAuthorizedKeys: []string{
						"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test@example.com",
					},
				},
			},
			Packages: []string{"vim", "git"},
			WriteFiles: []providerv1.WriteFileSpec{
				{Path: "/etc/test", Content: "hello", Permissions: "0644"},
			},
			Runcmd: []string{"echo hello"},
		},
	}

	config := cloudInitConfigFromVMSpec(vmName, spec, nil)

	if config.Hostname != "custom-hostname" {
		t.Errorf("Expected hostname 'custom-hostname', got %s", config.Hostname)
	}

	if len(config.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(config.Users))
	}

	if config.Users[0].Name != "customuser" {
		t.Errorf("Expected username 'customuser', got %s", config.Users[0].Name)
	}

	if len(config.Users[0].SSHAuthorizedKeys) != 1 {
		t.Errorf("Expected 1 SSH key, got %d", len(config.Users[0].SSHAuthorizedKeys))
	}

	if len(config.Packages) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(config.Packages))
	}

	if len(config.WriteFiles) != 1 {
		t.Errorf("Expected 1 WriteFile, got %d", len(config.WriteFiles))
	}

	if len(config.Runcmd) != 1 {
		t.Errorf("Expected 1 run command, got %d", len(config.Runcmd))
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

func TestCloudInitConfigFromVMSpec_MultipleUsers(t *testing.T) {
	vmName := "test-vm"
	spec := &providerv1.VMSpec{
		CloudInit: &providerv1.CloudInitSpec{
			Users: []providerv1.UserSpec{
				{Name: "user1", SSHAuthorizedKeys: []string{"key1"}},
				{Name: "user2", SSHAuthorizedKeys: []string{"key2", "key3"}},
			},
		},
	}
	config := cloudInitConfigFromVMSpec(vmName, spec, nil)
	if len(config.Users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(config.Users))
	}
	// Verify users are not flattened
	if config.Users[0].Name != "user1" || config.Users[1].Name != "user2" {
		t.Error("Users should preserve individual names")
	}
	if len(config.Users[0].SSHAuthorizedKeys) != 1 || len(config.Users[1].SSHAuthorizedKeys) != 2 {
		t.Error("Users should preserve individual SSH keys")
	}
}

func TestCloudInitConfigFromVMSpec_Hostname(t *testing.T) {
	spec := &providerv1.VMSpec{
		CloudInit: &providerv1.CloudInitSpec{
			Hostname: "custom-host",
		},
	}
	config := cloudInitConfigFromVMSpec("vm-name", spec, nil)
	if config.Hostname != "custom-host" {
		t.Errorf("Expected hostname 'custom-host', got %q", config.Hostname)
	}
}

func TestCloudInitConfigFromVMSpec_WriteFiles(t *testing.T) {
	spec := &providerv1.VMSpec{
		CloudInit: &providerv1.CloudInitSpec{
			WriteFiles: []providerv1.WriteFileSpec{
				{Path: "/etc/test", Content: "hello", Permissions: "0644"},
			},
		},
	}
	config := cloudInitConfigFromVMSpec("vm-name", spec, nil)
	if len(config.WriteFiles) != 1 {
		t.Errorf("Expected 1 WriteFile, got %d", len(config.WriteFiles))
	}
	if config.WriteFiles[0].Path != "/etc/test" {
		t.Errorf("Expected path '/etc/test', got %q", config.WriteFiles[0].Path)
	}
}

func TestGenerateUserData_MultipleUsers(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
		Users: []providerv1.UserSpec{
			{
				Name:              "admin",
				Sudo:              "ALL=(ALL) NOPASSWD:ALL",
				Shell:             "/bin/zsh",
				SSHAuthorizedKeys: []string{"ssh-ed25519 AAA1 admin@test"},
			},
			{
				Name:              "developer",
				Shell:             "/bin/bash",
				SSHAuthorizedKeys: []string{"ssh-ed25519 AAA2 dev@test"},
			},
		},
	}
	userData := generateUserData(config)

	// Both users should appear
	if !strings.Contains(userData, "name: admin") {
		t.Error("user-data should contain admin user")
	}
	if !strings.Contains(userData, "name: developer") {
		t.Error("user-data should contain developer user")
	}
	// Shell settings should be preserved
	if !strings.Contains(userData, "shell: /bin/zsh") {
		t.Error("admin user should have zsh shell")
	}
	if !strings.Contains(userData, "shell: /bin/bash") {
		t.Error("developer user should have bash shell")
	}
	// Each user's keys should appear
	if !strings.Contains(userData, "ssh-ed25519 AAA1") {
		t.Error("admin's SSH key should appear")
	}
	if !strings.Contains(userData, "ssh-ed25519 AAA2") {
		t.Error("developer's SSH key should appear")
	}
}

func TestGenerateUserData_CustomSudo(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
		Users: []providerv1.UserSpec{
			{Name: "limited", Sudo: "ALL=(ALL) ALL"},
		},
	}
	userData := generateUserData(config)
	if !strings.Contains(userData, "sudo: ['ALL=(ALL) ALL']") {
		t.Error("user-data should use custom sudo rule")
	}
}

func TestGenerateUserData_WithWriteFiles(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
		WriteFiles: []providerv1.WriteFileSpec{
			{
				Path:        "/etc/motd",
				Content:     "Welcome to the VM!",
				Permissions: "0644",
			},
		},
	}
	userData := generateUserData(config)

	if !strings.Contains(userData, "write_files:") {
		t.Error("user-data should contain write_files section")
	}
	if !strings.Contains(userData, "path: /etc/motd") {
		t.Error("user-data should contain file path")
	}
	if !strings.Contains(userData, "Welcome to the VM!") {
		t.Error("user-data should contain file content")
	}
	if !strings.Contains(userData, "permissions: '0644'") {
		t.Error("user-data should contain permissions")
	}
}

func TestGenerateUserData_WriteFilesMultiline(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
		WriteFiles: []providerv1.WriteFileSpec{
			{
				Path:    "/usr/local/bin/test.sh",
				Content: "#!/bin/bash\necho \"Hello\"\nexit 0",
			},
		},
	}
	userData := generateUserData(config)

	if !strings.Contains(userData, "content: |") {
		t.Error("user-data should use literal block style for content")
	}
	if !strings.Contains(userData, "#!/bin/bash") {
		t.Error("user-data should contain first line of script")
	}
	if !strings.Contains(userData, "echo \"Hello\"") {
		t.Error("user-data should contain middle line of script")
	}
}

func TestGenerateUserData_WriteFilesNoPermissions(t *testing.T) {
	config := &CloudInitConfig{
		VMName: "test-vm",
		WriteFiles: []providerv1.WriteFileSpec{
			{Path: "/tmp/test", Content: "data"},
		},
	}
	userData := generateUserData(config)

	if !strings.Contains(userData, "path: /tmp/test") {
		t.Error("user-data should contain file path")
	}
	// Should not have permissions line when not specified
	lines := strings.Split(userData, "\n")
	for i, line := range lines {
		if strings.Contains(line, "path: /tmp/test") {
			// Check next few lines don't have permissions
			for j := i + 1; j < len(lines) && j < i+4; j++ {
				if strings.Contains(lines[j], "permissions:") {
					t.Error("user-data should not include permissions when not specified")
				}
				if strings.HasPrefix(strings.TrimSpace(lines[j]), "- ") || strings.HasPrefix(strings.TrimSpace(lines[j]), "runcmd") {
					break // Next section started
				}
			}
			break
		}
	}
}
