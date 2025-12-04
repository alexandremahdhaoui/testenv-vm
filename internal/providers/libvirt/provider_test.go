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
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNewProviderWithConfig(t *testing.T) {
	config := ProviderConfig{
		URI:         "qemu:///session",
		StateDir:    "/tmp/test-state",
		ISOTool:     "/usr/bin/genisoimage",
		QemuImgPath: "/usr/bin/qemu-img",
	}

	// NewProviderWithConfig should work with nil connection for testing
	p := NewProviderWithConfig(config, nil)

	if p == nil {
		t.Fatal("Expected non-nil provider")
	}

	if p.config.URI != config.URI {
		t.Errorf("Expected URI %s, got %s", config.URI, p.config.URI)
	}

	if p.config.StateDir != config.StateDir {
		t.Errorf("Expected StateDir %s, got %s", config.StateDir, p.config.StateDir)
	}

	if p.keys == nil {
		t.Error("Expected non-nil keys map")
	}

	if p.networks == nil {
		t.Error("Expected non-nil networks map")
	}

	if p.vms == nil {
		t.Error("Expected non-nil vms map")
	}
}

func TestProviderConfig(t *testing.T) {
	config := ProviderConfig{
		URI:         "qemu:///system",
		StateDir:    "/var/lib/testenv-vm",
		ISOTool:     "/usr/bin/mkisofs",
		QemuImgPath: "/usr/bin/qemu-img",
	}

	p := NewProviderWithConfig(config, nil)
	retrievedConfig := p.Config()

	if retrievedConfig.URI != config.URI {
		t.Errorf("Expected URI %s, got %s", config.URI, retrievedConfig.URI)
	}

	if retrievedConfig.StateDir != config.StateDir {
		t.Errorf("Expected StateDir %s, got %s", config.StateDir, retrievedConfig.StateDir)
	}

	if retrievedConfig.ISOTool != config.ISOTool {
		t.Errorf("Expected ISOTool %s, got %s", config.ISOTool, retrievedConfig.ISOTool)
	}

	if retrievedConfig.QemuImgPath != config.QemuImgPath {
		t.Errorf("Expected QemuImgPath %s, got %s", config.QemuImgPath, retrievedConfig.QemuImgPath)
	}
}

func TestProviderClose_NilConnection(t *testing.T) {
	p := NewProviderWithConfig(ProviderConfig{}, nil)

	// Close should not error with nil connection
	err := p.Close()
	if err != nil {
		t.Errorf("Close with nil connection should not error: %v", err)
	}
}

func TestCreateStateDirs(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "libvirt-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateDir := filepath.Join(tmpDir, "state")

	err = createStateDirs(stateDir)
	if err != nil {
		t.Fatalf("createStateDirs failed: %v", err)
	}

	// Verify directories were created
	expectedDirs := []string{
		filepath.Join(stateDir, "keys"),
		filepath.Join(stateDir, "disks"),
		filepath.Join(stateDir, "cloudinit"),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Expected directory %s to exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("Expected %s to be a directory", dir)
		}
	}
}

func TestCreateStateDirs_Idempotent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "libvirt-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateDir := filepath.Join(tmpDir, "state")

	// Call twice - should not error
	err = createStateDirs(stateDir)
	if err != nil {
		t.Fatalf("First createStateDirs failed: %v", err)
	}

	err = createStateDirs(stateDir)
	if err != nil {
		t.Fatalf("Second createStateDirs should be idempotent: %v", err)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("TESTENV_VM_LIBVIRT_URI")
	os.Unsetenv("TESTENV_VM_STATE_DIR")

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Default URI depends on whether running as root
	if os.Getuid() == 0 {
		if config.URI != "qemu:///system" {
			t.Errorf("Expected default URI 'qemu:///system' for root, got %s", config.URI)
		}
		if config.StateDir != "/var/lib/testenv-vm" {
			t.Errorf("Expected default StateDir '/var/lib/testenv-vm' for root, got %s", config.StateDir)
		}
	} else {
		if config.URI != "qemu:///session" {
			t.Errorf("Expected default URI 'qemu:///session' for non-root, got %s", config.URI)
		}
		// For session mode, state dir is in /tmp to avoid home directory traversal issues
		expectedStateDir := filepath.Join(os.TempDir(), fmt.Sprintf("testenv-vm-%d", os.Getuid()))
		if config.StateDir != expectedStateDir {
			t.Errorf("Expected default StateDir '%s' for non-root, got %s", expectedStateDir, config.StateDir)
		}
	}
}

func TestLoadConfig_SessionURI(t *testing.T) {
	os.Setenv("TESTENV_VM_LIBVIRT_URI", "qemu:///session")
	os.Unsetenv("TESTENV_VM_STATE_DIR")
	defer os.Unsetenv("TESTENV_VM_LIBVIRT_URI")

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if config.URI != "qemu:///session" {
		t.Errorf("Expected URI 'qemu:///session', got %s", config.URI)
	}

	// For session, state dir should be in /tmp to avoid home directory traversal issues
	expectedStateDir := filepath.Join(os.TempDir(), fmt.Sprintf("testenv-vm-%d", os.Getuid()))
	if config.StateDir != expectedStateDir {
		t.Errorf("Expected StateDir %s, got %s", expectedStateDir, config.StateDir)
	}
}

func TestLoadConfig_CustomEnvVars(t *testing.T) {
	os.Setenv("TESTENV_VM_LIBVIRT_URI", "qemu+ssh://user@host/system")
	os.Setenv("TESTENV_VM_STATE_DIR", "/custom/state/dir")
	defer os.Unsetenv("TESTENV_VM_LIBVIRT_URI")
	defer os.Unsetenv("TESTENV_VM_STATE_DIR")

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if config.URI != "qemu+ssh://user@host/system" {
		t.Errorf("Expected custom URI, got %s", config.URI)
	}

	if config.StateDir != "/custom/state/dir" {
		t.Errorf("Expected custom StateDir, got %s", config.StateDir)
	}
}
