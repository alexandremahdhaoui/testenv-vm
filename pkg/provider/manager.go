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

// Package provider implements MCP client communication with provider processes.
package provider

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// Provider status constants.
const (
	StatusRunning = "running"
	StatusStopped = "stopped"
	StatusFailed  = "failed"
)

// Environment variable to enable local Go package execution.
const EnvRunLocalEnabled = "FORGE_RUN_LOCAL_ENABLED"

// ProviderInfo holds information about an active provider.
type ProviderInfo struct {
	// Config is the provider configuration.
	Config v1.ProviderConfig
	// Client is the MCP client for communicating with the provider.
	Client *Client
	// Capabilities describes what the provider supports.
	Capabilities *providerv1.CapabilitiesResponse
	// Status is the current provider status: running, stopped, failed.
	Status string
}

// Manager manages provider lifecycle and communication.
type Manager struct {
	providers map[string]*ProviderInfo
	mu        sync.RWMutex
}

// NewManager creates a new provider manager.
func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]*ProviderInfo),
	}
}

// Start starts a provider process based on its configuration.
// It resolves the engine path, starts the process, and fetches capabilities.
func (m *Manager) Start(config v1.ProviderConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if provider is already running
	if info, exists := m.providers[config.Name]; exists {
		if info.Status == StatusRunning {
			return fmt.Errorf("provider %q is already running", config.Name)
		}
	}

	log.Printf("Starting provider %q with engine %q", config.Name, config.Engine)

	// Resolve engine to command
	cmd, err := resolveEngine(config.Engine)
	if err != nil {
		m.providers[config.Name] = &ProviderInfo{
			Config: config,
			Status: StatusFailed,
		}
		return fmt.Errorf("failed to resolve engine for provider %q: %w", config.Name, err)
	}

	// Create MCP client (this starts the process and performs handshake)
	client, err := NewClient(cmd)
	if err != nil {
		m.providers[config.Name] = &ProviderInfo{
			Config: config,
			Status: StatusFailed,
		}
		return fmt.Errorf("failed to start provider %q: %w", config.Name, err)
	}

	// Fetch provider capabilities
	capabilities, err := client.Capabilities()
	if err != nil {
		// Close the client on failure
		_ = client.Close()
		m.providers[config.Name] = &ProviderInfo{
			Config: config,
			Status: StatusFailed,
		}
		return fmt.Errorf("failed to fetch capabilities for provider %q: %w", config.Name, err)
	}

	// Store provider info
	m.providers[config.Name] = &ProviderInfo{
		Config:       config,
		Client:       client,
		Capabilities: capabilities,
		Status:       StatusRunning,
	}

	log.Printf("Provider %q started successfully (version: %s)", config.Name, capabilities.Version)
	return nil
}

// Stop stops a provider process by name.
func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.providers[name]
	if !exists {
		return fmt.Errorf("provider %q not found", name)
	}

	if info.Status != StatusRunning {
		return nil // Already stopped
	}

	log.Printf("Stopping provider %q", name)

	if info.Client != nil {
		if err := info.Client.Close(); err != nil {
			info.Status = StatusFailed
			return fmt.Errorf("failed to stop provider %q: %w", name, err)
		}
	}

	info.Status = StatusStopped
	log.Printf("Provider %q stopped", name)
	return nil
}

// StopAll stops all running providers.
func (m *Manager) StopAll() error {
	m.mu.Lock()
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	m.mu.Unlock()

	var errs []error
	for _, name := range names {
		if err := m.Stop(name); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop some providers: %v", errs)
	}
	return nil
}

// Get returns the client for a provider by name.
func (m *Manager) Get(name string) (*Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %q not found", name)
	}

	if info.Status != StatusRunning {
		return nil, fmt.Errorf("provider %q is not running (status: %s)", name, info.Status)
	}

	if info.Client == nil {
		return nil, fmt.Errorf("provider %q has no client", name)
	}

	return info.Client, nil
}

// Call invokes a tool on a provider and returns the result.
func (m *Manager) Call(provider, tool string, input interface{}) (*providerv1.OperationResult, error) {
	client, err := m.Get(provider)
	if err != nil {
		return nil, err
	}

	return client.Call(tool, input)
}

// GetInfo returns the ProviderInfo for a provider by name.
func (m *Manager) GetInfo(name string) (*ProviderInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.providers[name]
	return info, exists
}

// List returns the names of all registered providers.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// resolveEngine resolves an engine specification to an exec.Cmd.
// Supported formats:
//   - go://path/to/package - Go package path (requires FORGE_RUN_LOCAL_ENABLED=true or pre-built binary)
//   - /path/to/binary - Absolute path to binary
//   - ./relative/path - Relative path to binary
func resolveEngine(engine string) (*exec.Cmd, error) {
	if strings.HasPrefix(engine, "go://") {
		return resolveGoEngine(engine)
	}

	// Treat as binary path
	return resolveBinaryEngine(engine)
}

// resolveGoEngine resolves a go:// engine specification.
func resolveGoEngine(engine string) (*exec.Cmd, error) {
	// Extract package path from go://path/to/package
	pkgPath := strings.TrimPrefix(engine, "go://")

	// Check if local execution is enabled
	if os.Getenv(EnvRunLocalEnabled) == "true" {
		// Use "go run" to execute the package
		cmd := exec.Command("go", "run", "./"+pkgPath, "--mcp")
		return cmd, nil
	}

	// Look for pre-built binary
	// Convention: binary name is the last component of the package path
	parts := strings.Split(pkgPath, "/")
	binaryName := parts[len(parts)-1]

	// Check common binary locations
	binaryPaths := []string{
		"./build/bin/" + binaryName,
		"./" + binaryName,
		binaryName, // Will be looked up in PATH
	}

	for _, binPath := range binaryPaths {
		if _, err := exec.LookPath(binPath); err == nil {
			cmd := exec.Command(binPath, "--mcp")
			return cmd, nil
		}
		// Also check if file exists directly (for relative paths)
		if _, err := os.Stat(binPath); err == nil {
			cmd := exec.Command(binPath, "--mcp")
			return cmd, nil
		}
	}

	return nil, fmt.Errorf("go engine %q: set %s=true to use 'go run' or ensure binary %q is built", engine, EnvRunLocalEnabled, binaryName)
}

// resolveBinaryEngine resolves a binary path engine specification.
func resolveBinaryEngine(engine string) (*exec.Cmd, error) {
	// Check if binary exists
	if _, err := exec.LookPath(engine); err != nil {
		// Try direct file check for relative paths
		if _, err := os.Stat(engine); err != nil {
			return nil, fmt.Errorf("binary %q not found: %w", engine, err)
		}
	}

	cmd := exec.Command(engine, "--mcp")
	return cmd, nil
}
