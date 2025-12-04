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

// Package mcp provides the MCP server for testenv-vm.
package mcp

import (
	"path/filepath"
	"testing"

	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
)

func createTestOrchestrator(t *testing.T) *orchestrator.Orchestrator {
	t.Helper()
	tmpDir := t.TempDir()
	orch, err := orchestrator.NewOrchestrator(orchestrator.Config{
		StateDir:         filepath.Join(tmpDir, "state"),
		CleanupOnFailure: true,
	})
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	return orch
}

func TestNewServer_ValidOrchestrator(t *testing.T) {
	orch := createTestOrchestrator(t)
	defer orch.Close()

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v, want nil", err)
	}
	if server == nil {
		t.Fatal("NewServer() returned nil server")
	}
	if server.server == nil {
		t.Error("server.server is nil")
	}
	if server.orch == nil {
		t.Error("server.orch is nil")
	}
	if server.name != "testenv-vm" {
		t.Errorf("server.name = %q, want %q", server.name, "testenv-vm")
	}
}

func TestNewServer_NilOrchestrator(t *testing.T) {
	server, err := NewServer(nil, "testenv-vm", "1.0.0")
	if err == nil {
		t.Fatal("NewServer(nil) error = nil, want error")
	}
	if server != nil {
		t.Error("NewServer(nil) returned non-nil server")
	}
	expectedErr := "orchestrator cannot be nil"
	if err.Error() != expectedErr {
		t.Errorf("NewServer(nil) error = %q, want %q", err.Error(), expectedErr)
	}
}

func TestServer_ToolsRegistered(t *testing.T) {
	orch := createTestOrchestrator(t)
	defer orch.Close()

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// The MCP SDK does not expose a way to list registered tools directly,
	// but we can verify that the server was created successfully which means
	// the AddTool calls completed without error.
	// The tools are registered during NewServer:
	// - "create" tool
	// - "delete" tool
	if server.server == nil {
		t.Error("server.server is nil - tools would not be registered")
	}

	// Verify the server has the orchestrator reference needed for handlers
	if server.orch != orch {
		t.Error("server.orch does not match the provided orchestrator")
	}
}

func TestNewServer_DifferentVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{
			name:    "semantic version",
			version: "1.2.3",
		},
		{
			name:    "git commit hash",
			version: "abc123def",
		},
		{
			name:    "development version",
			version: "dev",
		},
		{
			name:    "empty version",
			version: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch := createTestOrchestrator(t)
			defer orch.Close()

			server, err := NewServer(orch, "testenv-vm", tt.version)
			if err != nil {
				t.Errorf("NewServer() error = %v", err)
			}
			if server == nil {
				t.Error("NewServer() returned nil")
			}
		})
	}
}

func TestNewServer_DifferentNames(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
	}{
		{
			name:       "standard name",
			serverName: "testenv-vm",
		},
		{
			name:       "custom name",
			serverName: "my-custom-testenv",
		},
		{
			name:       "empty name",
			serverName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch := createTestOrchestrator(t)
			defer orch.Close()

			server, err := NewServer(orch, tt.serverName, "1.0.0")
			if err != nil {
				t.Errorf("NewServer() error = %v", err)
			}
			if server == nil {
				t.Error("NewServer() returned nil")
			}
			if server.name != tt.serverName {
				t.Errorf("server.name = %q, want %q", server.name, tt.serverName)
			}
		})
	}
}
