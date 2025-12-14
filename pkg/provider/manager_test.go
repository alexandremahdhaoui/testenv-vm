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

// Package provider implements MCP client communication with provider processes.
package provider

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

// TestNewManager tests that NewManager creates a valid manager.
func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.providers == nil {
		t.Error("providers map is nil")
	}
}

// TestManagerGet tests Get method for different scenarios.
func TestManagerGet(t *testing.T) {
	t.Run("non-existent provider", func(t *testing.T) {
		m := NewManager()
		_, err := m.Get("non-existent")
		if err == nil {
			t.Error("expected error for non-existent provider")
		}
	})

	t.Run("stopped provider", func(t *testing.T) {
		m := NewManager()
		m.mu.Lock()
		m.providers["test"] = &ProviderInfo{
			Status: StatusStopped,
		}
		m.mu.Unlock()

		_, err := m.Get("test")
		if err == nil {
			t.Error("expected error for stopped provider")
		}
	})

	t.Run("failed provider", func(t *testing.T) {
		m := NewManager()
		m.mu.Lock()
		m.providers["test"] = &ProviderInfo{
			Status: StatusFailed,
		}
		m.mu.Unlock()

		_, err := m.Get("test")
		if err == nil {
			t.Error("expected error for failed provider")
		}
	})

	t.Run("running provider without client", func(t *testing.T) {
		m := NewManager()
		m.mu.Lock()
		m.providers["test"] = &ProviderInfo{
			Status: StatusRunning,
			Client: nil,
		}
		m.mu.Unlock()

		_, err := m.Get("test")
		if err == nil {
			t.Error("expected error for provider without client")
		}
	})
}

// TestManagerGetInfo tests GetInfo method.
func TestManagerGetInfo(t *testing.T) {
	m := NewManager()

	t.Run("non-existent provider", func(t *testing.T) {
		info, exists := m.GetInfo("non-existent")
		if exists {
			t.Error("expected exists=false for non-existent provider")
		}
		if info != nil {
			t.Error("expected nil info for non-existent provider")
		}
	})

	t.Run("existing provider", func(t *testing.T) {
		expectedInfo := &ProviderInfo{
			Status: StatusRunning,
			Config: v1.ProviderConfig{Name: "test"},
		}
		m.mu.Lock()
		m.providers["test"] = expectedInfo
		m.mu.Unlock()

		info, exists := m.GetInfo("test")
		if !exists {
			t.Error("expected exists=true for existing provider")
		}
		if info != expectedInfo {
			t.Error("info does not match expected")
		}
	})
}

// TestManagerList tests List method.
func TestManagerList(t *testing.T) {
	m := NewManager()

	// Empty list
	names := m.List()
	if len(names) != 0 {
		t.Errorf("expected empty list, got %d items", len(names))
	}

	// Add some providers
	m.mu.Lock()
	m.providers["provider1"] = &ProviderInfo{Status: StatusRunning}
	m.providers["provider2"] = &ProviderInfo{Status: StatusStopped}
	m.providers["provider3"] = &ProviderInfo{Status: StatusFailed}
	m.mu.Unlock()

	names = m.List()
	if len(names) != 3 {
		t.Errorf("expected 3 providers, got %d", len(names))
	}

	// Check that all names are present
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}
	for _, expected := range []string{"provider1", "provider2", "provider3"} {
		if !nameMap[expected] {
			t.Errorf("expected provider %s in list", expected)
		}
	}
}

// TestManagerStopNonExistent tests stopping a non-existent provider.
func TestManagerStopNonExistent(t *testing.T) {
	m := NewManager()
	err := m.Stop("non-existent")
	if err == nil {
		t.Error("expected error for non-existent provider")
	}
}

// TestManagerStopAlreadyStopped tests stopping an already stopped provider.
func TestManagerStopAlreadyStopped(t *testing.T) {
	m := NewManager()
	m.mu.Lock()
	m.providers["test"] = &ProviderInfo{
		Status: StatusStopped,
	}
	m.mu.Unlock()

	err := m.Stop("test")
	if err != nil {
		t.Errorf("expected no error for already stopped provider, got: %v", err)
	}
}

// TestManagerStopAll tests stopping all providers.
func TestManagerStopAll(t *testing.T) {
	m := NewManager()

	// Add some providers with different statuses
	m.mu.Lock()
	m.providers["provider1"] = &ProviderInfo{Status: StatusStopped}
	m.providers["provider2"] = &ProviderInfo{Status: StatusStopped}
	m.mu.Unlock()

	err := m.StopAll()
	if err != nil {
		t.Errorf("StopAll returned error: %v", err)
	}
}

// TestManagerConcurrentAccess tests concurrent access to manager.
func TestManagerConcurrentAccess(t *testing.T) {
	m := NewManager()

	// Add a provider
	m.mu.Lock()
	m.providers["test"] = &ProviderInfo{
		Status: StatusRunning,
		Client: nil,
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.GetInfo("test")
			_ = m.List()
		}()
	}

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.mu.Lock()
			m.providers["test"].Status = StatusRunning
			m.mu.Unlock()
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}
}

// TestResolveEngine tests the engine resolution logic.
func TestResolveEngine(t *testing.T) {
	t.Run("binary path not found", func(t *testing.T) {
		_, err := resolveEngine("/nonexistent/binary")
		if err == nil {
			t.Error("expected error for non-existent binary")
		}
	})

	t.Run("go:// internal path without local enabled returns error", func(t *testing.T) {
		// Ensure FORGE_RUN_LOCAL_ENABLED is not set
		os.Unsetenv(EnvRunLocalEnabled)

		// Internal path should fail without FORGE_RUN_LOCAL_ENABLED
		_, err := resolveEngine("go://cmd/nonexistent/provider")
		if err == nil {
			t.Error("expected error for internal go:// without local enabled")
		}
	})

	t.Run("go:// external path without local enabled succeeds", func(t *testing.T) {
		// Ensure FORGE_RUN_LOCAL_ENABLED is not set
		os.Unsetenv(EnvRunLocalEnabled)

		// External path should work without FORGE_RUN_LOCAL_ENABLED
		cmd, err := resolveEngine("go://github.com/user/repo/cmd/tool@v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error for external go://: %v", err)
		}
		if cmd == nil {
			t.Fatal("expected non-nil command for external module")
		}
	})

	t.Run("go:// prefix with local enabled", func(t *testing.T) {
		os.Setenv(EnvRunLocalEnabled, "true")
		defer os.Unsetenv(EnvRunLocalEnabled)

		cmd, err := resolveEngine("go://cmd/providers/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the command is "go run"
		if cmd.Path != "go" && !containsPath(cmd.Path, "go") {
			// Path might be absolute
			if !isGoCommand(cmd) {
				t.Errorf("expected go command, got: %s", cmd.Path)
			}
		}

		// Verify args contain "run" and the package path
		argsStr := ""
		for _, arg := range cmd.Args {
			argsStr += arg + " "
		}
		if !containsSubstring(argsStr, "run") {
			t.Errorf("expected 'run' in args: %v", cmd.Args)
		}
	})

	t.Run("existing binary", func(t *testing.T) {
		// Use a binary that exists on most systems
		cmd, err := resolveEngine("echo")
		if err != nil {
			t.Fatalf("unexpected error for 'echo': %v", err)
		}
		if cmd == nil {
			t.Fatal("expected non-nil command")
		}
	})
}

// Helper functions
func containsPath(path, substr string) bool {
	return len(path) >= len(substr) && path[len(path)-len(substr):] == substr
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func isGoCommand(cmd *exec.Cmd) bool {
	// Check if the command is "go" either directly or as an absolute path
	if len(cmd.Args) > 0 {
		lastPart := cmd.Args[0]
		// Get just the binary name
		for i := len(lastPart) - 1; i >= 0; i-- {
			if lastPart[i] == '/' {
				lastPart = lastPart[i+1:]
				break
			}
		}
		return lastPart == "go"
	}
	return false
}

// TestManagerStartWithInvalidEngine tests starting a provider with invalid engine.
func TestManagerStartWithInvalidEngine(t *testing.T) {
	m := NewManager()

	config := v1.ProviderConfig{
		Name:   "test-provider",
		Engine: "/nonexistent/binary/path",
	}

	err := m.Start(config)
	if err == nil {
		t.Error("expected error for invalid engine path")
	}

	// Verify provider is marked as failed
	info, exists := m.GetInfo("test-provider")
	if !exists {
		t.Error("expected provider info to exist even on failure")
	}
	if info != nil && info.Status != StatusFailed {
		t.Errorf("expected status=failed, got %s", info.Status)
	}
}

// TestManagerStartDuplicateProvider tests starting a provider that's already running.
func TestManagerStartDuplicateProvider(t *testing.T) {
	m := NewManager()

	// First, manually add a running provider
	m.mu.Lock()
	m.providers["test-provider"] = &ProviderInfo{
		Config: v1.ProviderConfig{Name: "test-provider"},
		Status: StatusRunning,
	}
	m.mu.Unlock()

	config := v1.ProviderConfig{
		Name:   "test-provider",
		Engine: "echo", // Use a simple command
	}

	err := m.Start(config)
	if err == nil {
		t.Error("expected error for duplicate running provider")
	}
}

// TestResolveGoEngine tests the Go engine resolution specifically.
func TestResolveGoEngine(t *testing.T) {
	t.Run("external module with version", func(t *testing.T) {
		// External modules should work regardless of FORGE_RUN_LOCAL_ENABLED
		os.Unsetenv(EnvRunLocalEnabled)

		cmd, err := resolveGoEngine("go://github.com/user/repo/cmd/tool@v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedArgs := []string{"go", "run", "github.com/user/repo/cmd/tool@v1.0.0", "--mcp"}
		if !argsEqual(cmd.Args, expectedArgs) {
			t.Errorf("args = %v, want %v", cmd.Args, expectedArgs)
		}
	})

	t.Run("external module without version defaults to latest", func(t *testing.T) {
		os.Unsetenv(EnvRunLocalEnabled)

		cmd, err := resolveGoEngine("go://github.com/user/repo/cmd/tool")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedArgs := []string{"go", "run", "github.com/user/repo/cmd/tool@latest", "--mcp"}
		if !argsEqual(cmd.Args, expectedArgs) {
			t.Errorf("args = %v, want %v", cmd.Args, expectedArgs)
		}
	})

	t.Run("external module with FORGE_RUN_LOCAL_ENABLED still uses full path", func(t *testing.T) {
		os.Setenv(EnvRunLocalEnabled, "true")
		defer os.Unsetenv(EnvRunLocalEnabled)

		cmd, err := resolveGoEngine("go://github.com/user/repo/cmd/tool@v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// External modules should NOT have ./ prefix
		expectedArgs := []string{"go", "run", "github.com/user/repo/cmd/tool@v1.0.0", "--mcp"}
		if !argsEqual(cmd.Args, expectedArgs) {
			t.Errorf("args = %v, want %v", cmd.Args, expectedArgs)
		}
	})

	t.Run("internal module with FORGE_RUN_LOCAL_ENABLED", func(t *testing.T) {
		os.Setenv(EnvRunLocalEnabled, "true")
		defer os.Unsetenv(EnvRunLocalEnabled)

		cmd, err := resolveGoEngine("go://cmd/providers/stub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedArgs := []string{"go", "run", "./cmd/providers/stub", "--mcp"}
		if !argsEqual(cmd.Args, expectedArgs) {
			t.Errorf("args = %v, want %v", cmd.Args, expectedArgs)
		}
	})

	t.Run("internal module without FORGE_RUN_LOCAL_ENABLED returns error", func(t *testing.T) {
		os.Unsetenv(EnvRunLocalEnabled)

		_, err := resolveGoEngine("go://cmd/providers/stub")
		if err == nil {
			t.Error("expected error for internal module without FORGE_RUN_LOCAL_ENABLED")
		}
		// Error message should be helpful
		if !strings.Contains(err.Error(), EnvRunLocalEnabled) {
			t.Errorf("error should mention %s: %v", EnvRunLocalEnabled, err)
		}
	})

	t.Run("short name without FORGE_RUN_LOCAL_ENABLED returns error", func(t *testing.T) {
		os.Unsetenv(EnvRunLocalEnabled)

		_, err := resolveGoEngine("go://tool-name")
		if err == nil {
			t.Error("expected error for short name without FORGE_RUN_LOCAL_ENABLED")
		}
	})

	t.Run("short name with FORGE_RUN_LOCAL_ENABLED", func(t *testing.T) {
		os.Setenv(EnvRunLocalEnabled, "true")
		defer os.Unsetenv(EnvRunLocalEnabled)

		cmd, err := resolveGoEngine("go://tool-name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedArgs := []string{"go", "run", "./tool-name", "--mcp"}
		if !argsEqual(cmd.Args, expectedArgs) {
			t.Errorf("args = %v, want %v", cmd.Args, expectedArgs)
		}
	})

	// IMPORTANT FIX (Issue 1.2): Test path normalization to avoid ././path
	t.Run("internal module with existing ./ prefix", func(t *testing.T) {
		os.Setenv(EnvRunLocalEnabled, "true")
		defer os.Unsetenv(EnvRunLocalEnabled)

		cmd, err := resolveGoEngine("go://./cmd/providers/stub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should NOT produce ././cmd/providers/stub
		expectedArgs := []string{"go", "run", "./cmd/providers/stub", "--mcp"}
		if !argsEqual(cmd.Args, expectedArgs) {
			t.Errorf("args = %v, want %v (should not have double ./)", cmd.Args, expectedArgs)
		}
	})

	t.Run("internal module with existing ../ prefix", func(t *testing.T) {
		os.Setenv(EnvRunLocalEnabled, "true")
		defer os.Unsetenv(EnvRunLocalEnabled)

		cmd, err := resolveGoEngine("go://../other/cmd/tool")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should preserve ../ prefix without adding ./
		expectedArgs := []string{"go", "run", "../other/cmd/tool", "--mcp"}
		if !argsEqual(cmd.Args, expectedArgs) {
			t.Errorf("args = %v, want %v (should not add ./ to ../)", cmd.Args, expectedArgs)
		}
	})

	// IMPORTANT FIX (Issue 1.3): Test version on internal modules errors
	t.Run("internal module with version returns error", func(t *testing.T) {
		os.Setenv(EnvRunLocalEnabled, "true")
		defer os.Unsetenv(EnvRunLocalEnabled)

		_, err := resolveGoEngine("go://cmd/providers/stub@v1.0.0")
		if err == nil {
			t.Error("expected error for internal module with version specifier")
		}
		// Error message should mention the version
		if !strings.Contains(err.Error(), "@v1.0.0") {
			t.Errorf("error should mention the version specifier: %v", err)
		}
	})

	t.Run("local path with version returns error", func(t *testing.T) {
		os.Setenv(EnvRunLocalEnabled, "true")
		defer os.Unsetenv(EnvRunLocalEnabled)

		_, err := resolveGoEngine("go://./cmd/providers/stub@v1.0.0")
		if err == nil {
			t.Error("expected error for local path with version specifier")
		}
	})
}

// argsEqual compares two string slices for equality.
func argsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestResolveBinaryEngine tests the binary engine resolution.
func TestResolveBinaryEngine(t *testing.T) {
	t.Run("absolute path not found", func(t *testing.T) {
		_, err := resolveBinaryEngine("/this/path/does/not/exist")
		if err == nil {
			t.Error("expected error for non-existent path")
		}
	})

	t.Run("relative path not found", func(t *testing.T) {
		_, err := resolveBinaryEngine("./this/does/not/exist")
		if err == nil {
			t.Error("expected error for non-existent relative path")
		}
	})
}

// TestStripVersion tests the stripVersion helper function.
func TestStripVersion(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedPath    string
		expectedVersion string
	}{
		{
			name:            "external module with version",
			input:           "github.com/user/repo/cmd/tool@v1.2.3",
			expectedPath:    "github.com/user/repo/cmd/tool",
			expectedVersion: "@v1.2.3",
		},
		{
			name:            "external module without version",
			input:           "github.com/user/repo/cmd/tool",
			expectedPath:    "github.com/user/repo/cmd/tool",
			expectedVersion: "",
		},
		{
			name:            "internal path without version",
			input:           "cmd/providers/stub",
			expectedPath:    "cmd/providers/stub",
			expectedVersion: "",
		},
		{
			name:            "version with latest",
			input:           "github.com/user/repo@latest",
			expectedPath:    "github.com/user/repo",
			expectedVersion: "@latest",
		},
		{
			name:            "version with dirty suffix",
			input:           "github.com/user/repo@v1.0.0-dirty",
			expectedPath:    "github.com/user/repo",
			expectedVersion: "@v1.0.0-dirty",
		},
		{
			name:            "empty string",
			input:           "",
			expectedPath:    "",
			expectedVersion: "",
		},
		// IMPORTANT FIX (Issue 4.2): Test malformed inputs
		{
			name:            "double @ symbol (malformed)",
			input:           "github.com/user/repo@v1.0.0@v2.0.0",
			expectedPath:    "github.com/user/repo",
			expectedVersion: "@v1.0.0@v2.0.0", // Takes everything after first @
		},
		{
			name:            "@ at start (malformed)",
			input:           "@v1.0.0",
			expectedPath:    "",
			expectedVersion: "@v1.0.0",
		},
		{
			name:            "only @ symbol",
			input:           "@",
			expectedPath:    "",
			expectedVersion: "@",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, version := stripVersion(tt.input)
			if path != tt.expectedPath {
				t.Errorf("path: got %q, want %q", path, tt.expectedPath)
			}
			if version != tt.expectedVersion {
				t.Errorf("version: got %q, want %q", version, tt.expectedVersion)
			}
		})
	}
}

// TestIsExternalModule tests the isExternalModule helper function.
func TestIsExternalModule(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "github.com path",
			input:    "github.com/user/repo/cmd/tool",
			expected: true,
		},
		{
			name:     "gitlab.com path",
			input:    "gitlab.com/user/repo/cmd/tool",
			expected: true,
		},
		{
			name:     "custom domain path",
			input:    "my.company.com/project/cmd/tool",
			expected: true,
		},
		{
			name:     "internal cmd path",
			input:    "cmd/providers/stub",
			expected: false,
		},
		{
			name:     "internal pkg path",
			input:    "pkg/provider/manager",
			expected: false,
		},
		{
			name:     "local path with ./",
			input:    "./cmd/providers/stub",
			expected: false,
		},
		{
			name:     "local path with ../",
			input:    "../other/cmd/tool",
			expected: false,
		},
		{
			name:     "short name without slash",
			input:    "tool-name",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "golang.org path",
			input:    "golang.org/x/tools/cmd/stringer",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isExternalModule(tt.input)
			if result != tt.expected {
				t.Errorf("isExternalModule(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
