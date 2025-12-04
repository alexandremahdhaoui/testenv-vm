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

package provider

import (
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
)

// TestArtifactProviderInterface verifies compile-time interface implementation
func TestArtifactProviderInterface(t *testing.T) {
	var _ client.ClientProvider = (*ArtifactProvider)(nil)
}

// TestNewArtifactProviderDefaults verifies default user and port
func TestNewArtifactProviderDefaults(t *testing.T) {
	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{},
		Files:    map[string]string{},
	}

	p := NewArtifactProvider(artifact)

	if p.defaultUser != "root" {
		t.Errorf("expected default user 'root', got %q", p.defaultUser)
	}
	if p.defaultPort != "22" {
		t.Errorf("expected default port '22', got %q", p.defaultPort)
	}
}

// TestWithDefaultUser sets custom user
func TestWithDefaultUser(t *testing.T) {
	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{},
		Files:    map[string]string{},
	}

	p := NewArtifactProvider(artifact, WithDefaultUser("testuser"))

	if p.defaultUser != "testuser" {
		t.Errorf("expected user 'testuser', got %q", p.defaultUser)
	}
}

// TestWithDefaultPort sets custom port
func TestWithDefaultPort(t *testing.T) {
	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{},
		Files:    map[string]string{},
	}

	p := NewArtifactProvider(artifact, WithDefaultPort("2222"))

	if p.defaultPort != "2222" {
		t.Errorf("expected port '2222', got %q", p.defaultPort)
	}
}

// TestGetVMInfoExtractsIP verifies IP extraction from metadata
func TestGetVMInfoExtractsIP(t *testing.T) {
	// Create temp key file
	tmpDir, err := os.MkdirTemp("", "artifact-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, "test-key")
	keyContent := []byte("test-private-key-content")
	if err := os.WriteFile(keyPath, keyContent, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{
			"testenv-vm.vm.test-vm.ip": "192.168.100.10",
		},
		Files: map[string]string{
			"testenv-vm.key.test-key": keyPath,
		},
	}

	p := NewArtifactProvider(artifact)
	vmInfo, err := p.GetVMInfo("test-vm")
	if err != nil {
		t.Fatalf("GetVMInfo failed: %v", err)
	}

	if vmInfo.Host != "192.168.100.10" {
		t.Errorf("expected Host '192.168.100.10', got %q", vmInfo.Host)
	}
	if vmInfo.Port != "22" {
		t.Errorf("expected Port '22', got %q", vmInfo.Port)
	}
	if vmInfo.User != "root" {
		t.Errorf("expected User 'root', got %q", vmInfo.User)
	}
	if string(vmInfo.PrivateKey) != string(keyContent) {
		t.Errorf("expected PrivateKey %q, got %q", string(keyContent), string(vmInfo.PrivateKey))
	}
}

// TestGetVMInfoReadsKeyFromFiles verifies key file reading
func TestGetVMInfoReadsKeyFromFiles(t *testing.T) {
	// Create temp key file
	tmpDir, err := os.MkdirTemp("", "artifact-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, "test-key")
	keyContent := []byte("-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----")
	if err := os.WriteFile(keyPath, keyContent, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{
			"testenv-vm.vm.my-vm.ip": "10.0.0.5",
		},
		Files: map[string]string{
			"testenv-vm.key.my-key": keyPath,
		},
	}

	p := NewArtifactProvider(artifact, WithDefaultUser("ubuntu"), WithDefaultPort("2222"))
	vmInfo, err := p.GetVMInfo("my-vm")
	if err != nil {
		t.Fatalf("GetVMInfo failed: %v", err)
	}

	if vmInfo.User != "ubuntu" {
		t.Errorf("expected User 'ubuntu', got %q", vmInfo.User)
	}
	if vmInfo.Port != "2222" {
		t.Errorf("expected Port '2222', got %q", vmInfo.Port)
	}
	if string(vmInfo.PrivateKey) != string(keyContent) {
		t.Errorf("PrivateKey content mismatch")
	}
}

// TestGetVMInfoErrorMissingVM returns error for missing VM IP
func TestGetVMInfoErrorMissingVM(t *testing.T) {
	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{},
		Files:    map[string]string{},
	}

	p := NewArtifactProvider(artifact)
	_, err := p.GetVMInfo("nonexistent-vm")
	if err == nil {
		t.Fatal("expected error for missing VM, got nil")
	}
	// Verify error message contains helpful info
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

// TestGetVMInfoErrorMissingKeyFile returns error for missing key file
func TestGetVMInfoErrorMissingKeyFile(t *testing.T) {
	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{
			"testenv-vm.vm.test-vm.ip": "192.168.100.10",
		},
		Files: map[string]string{},
	}

	p := NewArtifactProvider(artifact)
	_, err := p.GetVMInfo("test-vm")
	if err == nil {
		t.Fatal("expected error for missing key file, got nil")
	}
}

// TestGetVMInfoFallbackToFirstKey verifies fallback to first available key
func TestGetVMInfoFallbackToFirstKey(t *testing.T) {
	// Create temp key file
	tmpDir, err := os.MkdirTemp("", "artifact-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, "default-key")
	keyContent := []byte("default-key-content")
	if err := os.WriteFile(keyPath, keyContent, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{
			"testenv-vm.vm.test-vm.ip": "192.168.100.10",
		},
		Files: map[string]string{
			"testenv-vm.key.default": keyPath,
		},
	}

	p := NewArtifactProvider(artifact)
	vmInfo, err := p.GetVMInfo("test-vm")
	if err != nil {
		t.Fatalf("GetVMInfo failed: %v", err)
	}

	if string(vmInfo.PrivateKey) != string(keyContent) {
		t.Error("expected to use fallback key")
	}
}

// TestGetVMInfoErrorUnreadableKeyFile returns error when key file can't be read
func TestGetVMInfoErrorUnreadableKeyFile(t *testing.T) {
	artifact := &v1.TestEnvArtifact{
		Metadata: map[string]string{
			"testenv-vm.vm.test-vm.ip": "192.168.100.10",
		},
		Files: map[string]string{
			"testenv-vm.key.test-key": "/nonexistent/path/to/key",
		},
	}

	p := NewArtifactProvider(artifact)
	_, err := p.GetVMInfo("test-vm")
	if err == nil {
		t.Fatal("expected error for unreadable key file, got nil")
	}
}
