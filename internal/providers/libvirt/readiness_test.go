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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"golang.org/x/crypto/ssh"
)

// generateTestKey creates a temporary ed25519 SSH key for testing and returns its path.
func generateTestKey(t *testing.T) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
	return keyPath
}

func TestWaitForReadiness_NilSpec(t *testing.T) {
	err := waitForReadiness(nil, "192.168.1.1")
	if err != nil {
		t.Errorf("expected nil error for nil spec, got: %v", err)
	}
}

func TestWaitForReadiness_NoIP(t *testing.T) {
	spec := &providerv1.ReadinessSpec{
		SSH: &providerv1.SSHReadinessSpec{
			Enabled: true,
			Timeout: "1s",
			User:    "ubuntu",
		},
	}
	err := waitForReadiness(spec, "")
	if err == nil {
		t.Fatal("expected error for empty IP")
	}
	if !strings.Contains(err.Message, "no IP address") {
		t.Errorf("expected 'no IP address' in error, got: %s", err.Message)
	}
}

func TestWaitForReadiness_SSHDisabled(t *testing.T) {
	spec := &providerv1.ReadinessSpec{
		SSH: &providerv1.SSHReadinessSpec{
			Enabled: false,
		},
	}
	err := waitForReadiness(spec, "192.168.1.1")
	if err != nil {
		t.Errorf("expected nil error when SSH disabled, got: %v", err)
	}
}

func TestWaitForReadiness_CloudInitWithoutSSH(t *testing.T) {
	spec := &providerv1.ReadinessSpec{
		CloudInit: &providerv1.CloudInitReadinessSpec{
			Enabled: true,
			Timeout: "1s",
		},
	}
	err := waitForReadiness(spec, "192.168.1.1")
	if err == nil {
		t.Fatal("expected error for cloud-init without SSH")
	}
	if err.Code != "INVALID_SPEC" {
		t.Errorf("expected INVALID_SPEC error code, got: %s", err.Code)
	}
}

func TestBuildSSHClientConfig_MissingPrivateKey(t *testing.T) {
	spec := &providerv1.SSHReadinessSpec{
		Enabled: true,
		Timeout: "1s",
		User:    "ubuntu",
	}
	_, _, err := buildSSHClientConfig(spec)
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
	if err.Code != "INVALID_SPEC" {
		t.Errorf("expected INVALID_SPEC error code, got: %s", err.Code)
	}
}

func TestBuildSSHClientConfig_MissingUser(t *testing.T) {
	spec := &providerv1.SSHReadinessSpec{
		Enabled:    true,
		Timeout:    "1s",
		PrivateKey: "/some/key",
	}
	_, _, err := buildSSHClientConfig(spec)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
	if err.Code != "INVALID_SPEC" {
		t.Errorf("expected INVALID_SPEC error code, got: %s", err.Code)
	}
}

func TestBuildSSHClientConfig_NonexistentKey(t *testing.T) {
	spec := &providerv1.SSHReadinessSpec{
		Enabled:    true,
		Timeout:    "1s",
		User:       "ubuntu",
		PrivateKey: "/nonexistent/path/key",
	}
	_, _, err := buildSSHClientConfig(spec)
	if err == nil {
		t.Fatal("expected error for nonexistent key file")
	}
	if err.Code != "PROVIDER_ERROR" {
		t.Errorf("expected PROVIDER_ERROR error code, got: %s", err.Code)
	}
}

func TestBuildSSHClientConfig_InvalidKeyContent(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "bad_key")
	if err := os.WriteFile(keyPath, []byte("not a valid key"), 0600); err != nil {
		t.Fatalf("failed to write bad key: %v", err)
	}

	spec := &providerv1.SSHReadinessSpec{
		Enabled:    true,
		Timeout:    "1s",
		User:       "ubuntu",
		PrivateKey: keyPath,
	}
	_, _, err := buildSSHClientConfig(spec)
	if err == nil {
		t.Fatal("expected error for invalid key content")
	}
	if err.Code != "PROVIDER_ERROR" {
		t.Errorf("expected PROVIDER_ERROR error code, got: %s", err.Code)
	}
}

func TestBuildSSHClientConfig_ValidKey(t *testing.T) {
	keyPath := generateTestKey(t)

	spec := &providerv1.SSHReadinessSpec{
		Enabled:    true,
		Timeout:    "5m",
		User:       "ubuntu",
		PrivateKey: keyPath,
	}
	config, fingerprint, err := buildSSHClientConfig(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.User != "ubuntu" {
		t.Errorf("expected user 'ubuntu', got %q", config.User)
	}
	if fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
}

func TestWaitForSSH_InvalidTimeout(t *testing.T) {
	spec := &providerv1.SSHReadinessSpec{
		Enabled:    true,
		Timeout:    "not-a-duration",
		User:       "ubuntu",
		PrivateKey: "/some/key",
	}
	// Pass nil sshConfig since we expect the timeout parsing error before it's used.
	err := waitForSSH(nil, spec, "192.168.1.1")
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
	if err.Code != "INVALID_SPEC" {
		t.Errorf("expected INVALID_SPEC error code, got: %s", err.Code)
	}
}

func TestWaitForSSH_Timeout(t *testing.T) {
	keyPath := generateTestKey(t)

	spec := &providerv1.SSHReadinessSpec{
		Enabled:    true,
		Timeout:    "1s",
		User:       "ubuntu",
		PrivateKey: keyPath,
	}

	sshConfig, _, opErr := buildSSHClientConfig(spec)
	if opErr != nil {
		t.Fatalf("failed to build SSH config: %v", opErr)
	}

	// Connect to a non-routable address to trigger timeout quickly
	err := waitForSSH(sshConfig, spec, "192.0.2.1")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Message, "SSH readiness timeout") {
		t.Errorf("expected SSH readiness timeout message, got: %s", err.Message)
	}
	if !err.Retryable {
		t.Error("expected retryable error")
	}
}

func TestWaitForCloudInit_InvalidTimeout(t *testing.T) {
	sshSpec := &providerv1.SSHReadinessSpec{
		Enabled:    true,
		Timeout:    "5m",
		User:       "ubuntu",
		PrivateKey: "/some/key",
	}
	ciSpec := &providerv1.CloudInitReadinessSpec{
		Enabled: true,
		Timeout: "bad",
	}
	// Pass nil sshConfig since we expect the timeout parsing error before it's used.
	err := waitForCloudInit(nil, "", ciSpec, sshSpec, "192.168.1.1")
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
	if err.Code != "INVALID_SPEC" {
		t.Errorf("expected INVALID_SPEC error code, got: %s", err.Code)
	}
}
