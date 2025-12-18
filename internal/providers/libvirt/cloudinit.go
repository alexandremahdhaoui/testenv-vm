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
	Hostname        string
	Users           []providerv1.UserSpec
	Packages        []string
	WriteFiles      []providerv1.WriteFileSpec
	Runcmd          []string
	NetworkConfig   *providerv1.CloudInitNetworkConfig
	MatchedKeyNames []string // Names of provider keys that match SSH authorized keys
}

// generateMetaData generates the cloud-init meta-data file content.
func generateMetaData(config *CloudInitConfig) string {
	hostname := config.Hostname
	if hostname == "" {
		hostname = config.VMName
	}
	return fmt.Sprintf(`instance-id: %s
hostname: %s
local-hostname: %s
`, config.VMName, hostname, hostname)
}

// generateUserData generates the cloud-init user-data file content.
func generateUserData(config *CloudInitConfig) string {
	var sb strings.Builder

	sb.WriteString("#cloud-config\n\n")

	// User configuration
	if len(config.Users) > 0 {
		sb.WriteString("users:\n")
		for _, user := range config.Users {
			sb.WriteString(fmt.Sprintf("  - name: %s\n", user.Name))
			// Sudo (default: ALL=(ALL) NOPASSWD:ALL)
			sudo := user.Sudo
			if sudo == "" {
				sudo = "ALL=(ALL) NOPASSWD:ALL"
			}
			sb.WriteString(fmt.Sprintf("    sudo: ['%s']\n", sudo))
			// Shell (default: /bin/bash)
			shell := user.Shell
			if shell == "" {
				shell = "/bin/bash"
			}
			sb.WriteString(fmt.Sprintf("    shell: %s\n", shell))
			// SSH authorized keys
			if len(user.SSHAuthorizedKeys) > 0 {
				sb.WriteString("    ssh-authorized-keys:\n")
				for _, key := range user.SSHAuthorizedKeys {
					sb.WriteString(fmt.Sprintf("      - %s\n", strings.TrimSpace(key)))
				}
			}
		}
	} else {
		// Default user if none specified
		sb.WriteString("users:\n")
		sb.WriteString("  - name: ubuntu\n")
		sb.WriteString("    sudo: ['ALL=(ALL) NOPASSWD:ALL']\n")
		sb.WriteString("    shell: /bin/bash\n")
	}

	// Packages
	if len(config.Packages) > 0 {
		sb.WriteString("\npackages:\n")
		for _, pkg := range config.Packages {
			sb.WriteString(fmt.Sprintf("  - %s\n", pkg))
		}
	}

	// Write files
	if len(config.WriteFiles) > 0 {
		sb.WriteString("\nwrite_files:\n")
		for _, wf := range config.WriteFiles {
			sb.WriteString(fmt.Sprintf("  - path: %s\n", wf.Path))
			sb.WriteString("    content: |\n")
			// Indent each line of content by 6 spaces
			for _, line := range strings.Split(wf.Content, "\n") {
				sb.WriteString(fmt.Sprintf("      %s\n", line))
			}
			if wf.Permissions != "" {
				sb.WriteString(fmt.Sprintf("    permissions: '%s'\n", wf.Permissions))
			}
		}
	}

	// Run commands (always include boot-finished marker)
	sb.WriteString("\nruncmd:\n")
	for _, cmd := range config.Runcmd {
		sb.WriteString(fmt.Sprintf("  - %s\n", cmd))
	}
	sb.WriteString("  - touch /var/lib/cloud/instance/boot-finished\n")

	return sb.String()
}

// generateNetworkConfig generates the cloud-init network-config file content.
// Uses netplan version 2 format with broad interface matching for reliability.
// If a custom network config is provided, it will be used instead of the default DHCP config.
func generateNetworkConfig(config *providerv1.CloudInitNetworkConfig) string {
	// If no custom config provided, use default DHCP on all ethernet interfaces
	if config == nil || len(config.Ethernets) == 0 {
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

	// Generate custom network config
	var sb strings.Builder
	sb.WriteString("version: 2\n")
	sb.WriteString("ethernets:\n")

	for i, eth := range config.Ethernets {
		// Use interface name or generate a unique identifier
		ifaceName := eth.Name
		if ifaceName == "" {
			ifaceName = fmt.Sprintf("eth%d", i)
		}

		// Check if name contains wildcards
		hasWildcard := strings.Contains(ifaceName, "*")

		if hasWildcard {
			// Use match syntax for wildcard patterns
			sb.WriteString(fmt.Sprintf("  %s:\n", sanitizeInterfaceName(ifaceName)))
			sb.WriteString("    match:\n")
			sb.WriteString(fmt.Sprintf("      name: \"%s\"\n", ifaceName))
		} else {
			// Direct interface name
			sb.WriteString(fmt.Sprintf("  %s:\n", ifaceName))
		}

		// DHCP or static
		if eth.DHCP4 != nil && *eth.DHCP4 {
			sb.WriteString("    dhcp4: true\n")
		} else if len(eth.Addresses) > 0 {
			sb.WriteString("    dhcp4: false\n")
			sb.WriteString("    addresses:\n")
			for _, addr := range eth.Addresses {
				sb.WriteString(fmt.Sprintf("      - %s\n", addr))
			}

			// Routes/gateway
			if eth.Gateway4 != "" {
				sb.WriteString("    routes:\n")
				sb.WriteString("      - to: default\n")
				sb.WriteString(fmt.Sprintf("        via: %s\n", eth.Gateway4))
			}

			// Nameservers
			if eth.Nameservers != nil && len(eth.Nameservers.Addresses) > 0 {
				sb.WriteString("    nameservers:\n")
				sb.WriteString("      addresses:\n")
				for _, ns := range eth.Nameservers.Addresses {
					sb.WriteString(fmt.Sprintf("        - %s\n", ns))
				}
			}
		} else {
			// Default to DHCP if no addresses specified
			sb.WriteString("    dhcp4: true\n")
		}
	}

	return sb.String()
}

// sanitizeInterfaceName creates a valid netplan key from an interface pattern
func sanitizeInterfaceName(name string) string {
	// Replace wildcards with descriptive text
	result := strings.ReplaceAll(name, "*", "all")
	// Replace other invalid chars
	result = strings.ReplaceAll(result, "-", "_")
	return result
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
	if err := os.WriteFile(metaDataPath, []byte(generateMetaData(config)), 0644); err != nil {
		return fmt.Errorf("failed to write meta-data: %w", err)
	}

	// Write user-data
	userDataPath := filepath.Join(tmpDir, "user-data")
	if err := os.WriteFile(userDataPath, []byte(generateUserData(config)), 0644); err != nil {
		return fmt.Errorf("failed to write user-data: %w", err)
	}

	// Write network-config
	networkConfigPath := filepath.Join(tmpDir, "network-config")
	if err := os.WriteFile(networkConfigPath, []byte(generateNetworkConfig(config.NetworkConfig)), 0644); err != nil {
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
		MatchedKeyNames: make([]string, 0),
	}

	if spec.CloudInit != nil {
		config.Hostname = spec.CloudInit.Hostname
		config.Users = spec.CloudInit.Users
		config.Packages = spec.CloudInit.Packages
		config.WriteFiles = spec.CloudInit.WriteFiles
		config.Runcmd = spec.CloudInit.Runcmd
		config.NetworkConfig = spec.CloudInit.NetworkConfig
	}

	// Match SSH authorized keys against provider keys
	for _, user := range config.Users {
		for _, sshKey := range user.SSHAuthorizedKeys {
			sshKeyTrimmed := strings.TrimSpace(sshKey)
			for keyName, keyState := range keys {
				if strings.TrimSpace(keyState.PublicKey) == sshKeyTrimmed {
					config.MatchedKeyNames = append(config.MatchedKeyNames, keyName)
					break // Each SSH key matches at most one provider key
				}
			}
		}
	}

	return config
}
