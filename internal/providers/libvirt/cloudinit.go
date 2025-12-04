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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// CloudInitConfig holds configuration for cloud-init.
type CloudInitConfig struct {
	VMName          string
	Username        string
	SSHKeys         []string
	Packages        []string
	RunCMD          []string
	MatchedKeyNames []string // Names of provider keys that match SSH authorized keys
}

// generateMetaData generates the cloud-init meta-data file content.
func generateMetaData(vmName string) string {
	return fmt.Sprintf(`instance-id: %s
hostname: %s
local-hostname: %s
`, vmName, vmName, vmName)
}

// generateUserData generates the cloud-init user-data file content.
func generateUserData(config *CloudInitConfig) string {
	var sb strings.Builder

	sb.WriteString("#cloud-config\n\n")

	// User configuration
	username := config.Username
	if username == "" {
		username = "ubuntu"
	}

	sb.WriteString("users:\n")
	sb.WriteString(fmt.Sprintf("  - name: %s\n", username))
	sb.WriteString("    sudo: ['ALL=(ALL) NOPASSWD:ALL']\n")
	sb.WriteString("    shell: /bin/bash\n")

	if len(config.SSHKeys) > 0 {
		sb.WriteString("    ssh-authorized-keys:\n")
		for _, key := range config.SSHKeys {
			sb.WriteString(fmt.Sprintf("      - %s", strings.TrimSpace(key)))
			if !strings.HasSuffix(key, "\n") {
				sb.WriteString("\n")
			}
		}
	}

	// Packages
	if len(config.Packages) > 0 {
		sb.WriteString("\npackages:\n")
		for _, pkg := range config.Packages {
			sb.WriteString(fmt.Sprintf("  - %s\n", pkg))
		}
	}

	// Run commands (always include boot-finished marker)
	sb.WriteString("\nruncmd:\n")
	for _, cmd := range config.RunCMD {
		sb.WriteString(fmt.Sprintf("  - %s\n", cmd))
	}
	sb.WriteString("  - touch /var/lib/cloud/instance/boot-finished\n")

	return sb.String()
}

// generateNetworkConfig generates the cloud-init network-config file content.
// Uses netplan version 2 format with broad interface matching for reliability.
func generateNetworkConfig() string {
	// Match all ethernet interfaces and enable DHCP. This is more reliable
	// than matching on driver as it works regardless of interface naming.
	return `version: 2
ethernets:
  all-en:
    match:
      name: "en*"
    dhcp4: true
  all-eth:
    match:
      name: "eth*"
    dhcp4: true
`
}

// generateCloudInitISO generates a cloud-init ISO file.
func generateCloudInitISO(config *CloudInitConfig, outputPath, isoTool string) error {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "cidata-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Write meta-data
	metaDataPath := filepath.Join(tmpDir, "meta-data")
	if err := os.WriteFile(metaDataPath, []byte(generateMetaData(config.VMName)), 0644); err != nil {
		return fmt.Errorf("failed to write meta-data: %w", err)
	}

	// Write user-data
	userDataPath := filepath.Join(tmpDir, "user-data")
	if err := os.WriteFile(userDataPath, []byte(generateUserData(config)), 0644); err != nil {
		return fmt.Errorf("failed to write user-data: %w", err)
	}

	// Write network-config
	networkConfigPath := filepath.Join(tmpDir, "network-config")
	if err := os.WriteFile(networkConfigPath, []byte(generateNetworkConfig()), 0644); err != nil {
		return fmt.Errorf("failed to write network-config: %w", err)
	}

	// Generate ISO
	// Different tools have slightly different invocations
	var cmd *exec.Cmd
	if strings.Contains(isoTool, "xorriso") {
		cmd = exec.Command(isoTool,
			"-as", "genisoimage",
			"-output", outputPath,
			"-volid", "cidata",
			"-joliet", "-rock",
			metaDataPath, userDataPath, networkConfigPath)
	} else {
		cmd = exec.Command(isoTool,
			"-output", outputPath,
			"-volid", "cidata",
			"-joliet", "-rock",
			metaDataPath, userDataPath, networkConfigPath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate ISO: %w, output: %s", err, string(output))
	}

	return nil
}

// cloudInitConfigFromVMSpec extracts cloud-init configuration from a VM spec.
func cloudInitConfigFromVMSpec(vmName string, spec *providerv1.VMSpec, keys map[string]*providerv1.KeyState) *CloudInitConfig {
	config := &CloudInitConfig{
		VMName:          vmName,
		Username:        "ubuntu", // Default username
		MatchedKeyNames: make([]string, 0),
	}

	if spec.CloudInit != nil {
		// Extract username from first user if available
		if len(spec.CloudInit.Users) > 0 {
			config.Username = spec.CloudInit.Users[0].Name
			// Collect SSH keys from users
			for _, user := range spec.CloudInit.Users {
				config.SSHKeys = append(config.SSHKeys, user.SSHAuthorizedKeys...)
			}
		}

		config.Packages = spec.CloudInit.Packages
		config.RunCMD = spec.CloudInit.RunCommands
	}

	// Match SSH authorized keys against provider keys
	for _, sshKey := range config.SSHKeys {
		sshKeyTrimmed := strings.TrimSpace(sshKey)
		for keyName, keyState := range keys {
			if strings.TrimSpace(keyState.PublicKey) == sshKeyTrimmed {
				config.MatchedKeyNames = append(config.MatchedKeyNames, keyName)
				break // Each SSH key matches at most one provider key
			}
		}
	}

	return config
}
