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
	"strings"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
)

// ArtifactProvider extracts VM connection info from TestEnvArtifact.
type ArtifactProvider struct {
	artifact    *v1.TestEnvArtifact
	defaultUser string // Default is "root" if WithDefaultUser not called
	defaultPort string // Default is "22"
}

// Compile-time check that ArtifactProvider implements client.ClientProvider
var _ client.ClientProvider = (*ArtifactProvider)(nil)

// ArtifactProviderOption configures ArtifactProvider.
type ArtifactProviderOption func(*ArtifactProvider)

// WithDefaultUser sets the default SSH user.
// If not called, defaults to "root".
func WithDefaultUser(user string) ArtifactProviderOption {
	return func(p *ArtifactProvider) {
		p.defaultUser = user
	}
}

// WithDefaultPort sets the default SSH port.
// If not called, defaults to "22".
func WithDefaultPort(port string) ArtifactProviderOption {
	return func(p *ArtifactProvider) {
		p.defaultPort = port
	}
}

// NewArtifactProvider creates a provider from a TestEnvArtifact.
// Default user is "root", default port is "22".
func NewArtifactProvider(artifact *v1.TestEnvArtifact, opts ...ArtifactProviderOption) *ArtifactProvider {
	p := &ArtifactProvider{
		artifact:    artifact,
		defaultUser: "root",
		defaultPort: "22",
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// GetVMInfo extracts VM connection info from the artifact.
// Steps:
// 1. Look up IP from artifact.Metadata["testenv-vm.vm.<vmName>.ip"]
// 2. Look up key path from artifact.Files["testenv-vm.key.<vmName>"] or first key
// 3. Read private key from file
// 4. Return VMInfo with user (default "root") and port (default "22")
func (p *ArtifactProvider) GetVMInfo(vmName string) (*client.VMInfo, error) {
	// Step 1: Look up IP from metadata
	ipKey := fmt.Sprintf("testenv-vm.vm.%s.ip", vmName)
	ip, ok := p.artifact.Metadata[ipKey]
	if !ok || ip == "" {
		return nil, fmt.Errorf("artifact provider: VM %q not found in metadata (key: %s)", vmName, ipKey)
	}

	// Step 2: Look up key path
	// First try VM-specific key
	keyFileKey := fmt.Sprintf("testenv-vm.key.%s", vmName)
	keyPath, ok := p.artifact.Files[keyFileKey]
	if !ok {
		// Fallback: use first key file that starts with "testenv-vm.key."
		for k, v := range p.artifact.Files {
			if strings.HasPrefix(k, "testenv-vm.key.") {
				keyPath = v
				break
			}
		}
	}

	if keyPath == "" {
		return nil, fmt.Errorf("artifact provider: no SSH key found in artifact.Files for VM %q", vmName)
	}

	// Step 3: Read private key from file
	keyContent, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("artifact provider: failed to read SSH key from %s: %w", keyPath, err)
	}

	// Step 4: Return VMInfo
	return &client.VMInfo{
		Host:       ip,
		Port:       p.defaultPort,
		User:       p.defaultUser,
		PrivateKey: keyContent,
	}, nil
}
