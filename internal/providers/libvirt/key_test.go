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
	"os"
	"strings"
	"testing"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"golang.org/x/crypto/ssh"
)

// testProvider creates a Provider with a temp directory for testing key operations.
func testProvider(t *testing.T) (*Provider, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "libvirt-key-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create state directories
	if err := createStateDirs(tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create state dirs: %v", err)
	}

	config := ProviderConfig{
		StateDir: tmpDir,
	}

	p := NewProviderWithConfig(config, nil)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return p, cleanup
}

func TestGenerateED25519Key(t *testing.T) {
	privateKeyPEM, publicKey, err := generateED25519Key()
	if err != nil {
		t.Fatalf("generateED25519Key failed: %v", err)
	}

	// Verify private key is in PEM format
	if !strings.HasPrefix(string(privateKeyPEM), "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Error("Private key should be in PEM format with OPENSSH header")
	}
	if !strings.HasSuffix(strings.TrimSpace(string(privateKeyPEM)), "-----END OPENSSH PRIVATE KEY-----") {
		t.Error("Private key should end with PEM footer")
	}

	// Verify public key is not nil
	if publicKey == nil {
		t.Error("Public key should not be nil")
	}

	// Verify public key type
	if publicKey.Type() != "ssh-ed25519" {
		t.Errorf("Expected key type ssh-ed25519, got %s", publicKey.Type())
	}

	// Verify private key can be parsed
	_, err = ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		t.Errorf("Failed to parse private key: %v", err)
	}
}

func TestGenerateRSAKey(t *testing.T) {
	// Test with default 4096 bits
	privateKeyPEM, publicKey, err := generateRSAKey(4096)
	if err != nil {
		t.Fatalf("generateRSAKey failed: %v", err)
	}

	// Verify private key is in PEM format
	if !strings.HasPrefix(string(privateKeyPEM), "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Error("Private key should be in PEM format with OPENSSH header")
	}
	if !strings.HasSuffix(strings.TrimSpace(string(privateKeyPEM)), "-----END OPENSSH PRIVATE KEY-----") {
		t.Error("Private key should end with PEM footer")
	}

	// Verify public key is not nil
	if publicKey == nil {
		t.Error("Public key should not be nil")
	}

	// Verify public key type
	if publicKey.Type() != "ssh-rsa" {
		t.Errorf("Expected key type ssh-rsa, got %s", publicKey.Type())
	}

	// Verify private key can be parsed
	_, err = ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		t.Errorf("Failed to parse private key: %v", err)
	}
}

func TestGenerateRSAKey_DifferentBitSizes(t *testing.T) {
	testCases := []int{2048, 4096}

	for _, bits := range testCases {
		t.Run(string(rune(bits)), func(t *testing.T) {
			privateKeyPEM, publicKey, err := generateRSAKey(bits)
			if err != nil {
				t.Fatalf("generateRSAKey(%d) failed: %v", bits, err)
			}

			if publicKey == nil {
				t.Error("Public key should not be nil")
			}

			// Verify private key can be parsed
			_, err = ssh.ParsePrivateKey(privateKeyPEM)
			if err != nil {
				t.Errorf("Failed to parse private key: %v", err)
			}
		})
	}
}

func TestGenerateED25519Key_UniqueKeys(t *testing.T) {
	// Generate two keys and verify they're different
	pem1, pub1, err := generateED25519Key()
	if err != nil {
		t.Fatalf("First generateED25519Key failed: %v", err)
	}

	pem2, pub2, err := generateED25519Key()
	if err != nil {
		t.Fatalf("Second generateED25519Key failed: %v", err)
	}

	// Keys should be different
	if string(pem1) == string(pem2) {
		t.Error("Generated private keys should be unique")
	}

	// Public keys should be different
	pubStr1 := string(ssh.MarshalAuthorizedKey(pub1))
	pubStr2 := string(ssh.MarshalAuthorizedKey(pub2))
	if pubStr1 == pubStr2 {
		t.Error("Generated public keys should be unique")
	}
}

// --- Key CRUD Tests ---

func TestKeyCreate_ED25519(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	req := &providerv1.KeyCreateRequest{
		Name: "test-key",
		Spec: providerv1.KeySpec{
			Type: "ed25519",
		},
	}

	result := p.KeyCreate(req)
	if !result.Success {
		t.Fatalf("KeyCreate failed: %v", result.Error)
	}

	// Verify the key state
	keyState, ok := result.Resource.(*providerv1.KeyState)
	if !ok {
		t.Fatal("Expected KeyState in result data")
	}

	if keyState.Name != "test-key" {
		t.Errorf("Expected name 'test-key', got %s", keyState.Name)
	}

	if keyState.Type != "ed25519" {
		t.Errorf("Expected type 'ed25519', got %s", keyState.Type)
	}

	if keyState.PublicKey == "" {
		t.Error("PublicKey should not be empty")
	}

	if !strings.HasPrefix(keyState.PublicKey, "ssh-ed25519") {
		t.Error("PublicKey should start with 'ssh-ed25519'")
	}

	if keyState.Fingerprint == "" {
		t.Error("Fingerprint should not be empty")
	}

	if !strings.HasPrefix(keyState.Fingerprint, "SHA256:") {
		t.Error("Fingerprint should start with 'SHA256:'")
	}

	// Verify files were created
	if _, err := os.Stat(keyState.PrivateKeyPath); err != nil {
		t.Errorf("Private key file should exist: %v", err)
	}

	if _, err := os.Stat(keyState.PublicKeyPath); err != nil {
		t.Errorf("Public key file should exist: %v", err)
	}

	// Verify private key has correct permissions (0600)
	info, _ := os.Stat(keyState.PrivateKeyPath)
	if info.Mode().Perm() != 0600 {
		t.Errorf("Private key should have 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestKeyCreate_RSA(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	req := &providerv1.KeyCreateRequest{
		Name: "test-rsa-key",
		Spec: providerv1.KeySpec{
			Type: "rsa",
			Bits: 2048,
		},
	}

	result := p.KeyCreate(req)
	if !result.Success {
		t.Fatalf("KeyCreate failed: %v", result.Error)
	}

	keyState, ok := result.Resource.(*providerv1.KeyState)
	if !ok {
		t.Fatal("Expected KeyState in result Resource")
	}

	if keyState.Type != "rsa" {
		t.Errorf("Expected type 'rsa', got %s", keyState.Type)
	}

	if !strings.HasPrefix(keyState.PublicKey, "ssh-rsa") {
		t.Error("PublicKey should start with 'ssh-rsa'")
	}
}

func TestKeyCreate_DefaultType(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	req := &providerv1.KeyCreateRequest{
		Name: "default-key",
		Spec: providerv1.KeySpec{}, // No type specified
	}

	result := p.KeyCreate(req)
	if !result.Success {
		t.Fatalf("KeyCreate failed: %v", result.Error)
	}

	keyState := result.Resource.(*providerv1.KeyState)
	if keyState.Type != "ed25519" {
		t.Errorf("Default type should be 'ed25519', got %s", keyState.Type)
	}
}

func TestKeyCreate_UnsupportedType(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	req := &providerv1.KeyCreateRequest{
		Name: "unsupported-key",
		Spec: providerv1.KeySpec{
			Type: "dsa", // Not supported
		},
	}

	result := p.KeyCreate(req)
	if result.Success {
		t.Fatal("Expected KeyCreate to fail for unsupported type")
	}

	if result.Error == nil {
		t.Fatal("Expected error in result")
	}

	if result.Error.Code != providerv1.ErrCodeInvalidSpec {
		t.Errorf("Expected ErrCodeInvalidSpec, got %s", result.Error.Code)
	}
}

func TestKeyCreate_AlreadyExists(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	req := &providerv1.KeyCreateRequest{
		Name: "duplicate-key",
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}

	// First create should succeed
	result := p.KeyCreate(req)
	if !result.Success {
		t.Fatalf("First KeyCreate failed: %v", result.Error)
	}

	// Second create with same name should fail
	result = p.KeyCreate(req)
	if result.Success {
		t.Fatal("Expected second KeyCreate to fail")
	}

	if result.Error.Code != providerv1.ErrCodeAlreadyExists {
		t.Errorf("Expected ErrCodeAlreadyExists, got %s", result.Error.Code)
	}
}

func TestKeyGet(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	// Create a key first
	createReq := &providerv1.KeyCreateRequest{
		Name: "get-test-key",
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}
	createResult := p.KeyCreate(createReq)
	if !createResult.Success {
		t.Fatalf("KeyCreate failed: %v", createResult.Error)
	}

	// Get the key
	result := p.KeyGet("get-test-key")
	if !result.Success {
		t.Fatalf("KeyGet failed: %v", result.Error)
	}

	keyState := result.Resource.(*providerv1.KeyState)
	if keyState.Name != "get-test-key" {
		t.Errorf("Expected name 'get-test-key', got %s", keyState.Name)
	}
}

func TestKeyGet_NotFound(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	result := p.KeyGet("nonexistent-key")
	if result.Success {
		t.Fatal("Expected KeyGet to fail for nonexistent key")
	}

	if result.Error.Code != providerv1.ErrCodeNotFound {
		t.Errorf("Expected ErrCodeNotFound, got %s", result.Error.Code)
	}
}

func TestKeyList(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	// Create multiple keys
	keys := []string{"key1", "key2", "key3"}
	for _, name := range keys {
		req := &providerv1.KeyCreateRequest{
			Name: name,
			Spec: providerv1.KeySpec{Type: "ed25519"},
		}
		result := p.KeyCreate(req)
		if !result.Success {
			t.Fatalf("KeyCreate failed for %s: %v", name, result.Error)
		}
	}

	// List keys
	result := p.KeyList(nil)
	if !result.Success {
		t.Fatalf("KeyList failed: %v", result.Error)
	}

	keyList := result.Resource.([]*providerv1.KeyState)
	if len(keyList) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keyList))
	}

	// Verify all keys are in the list
	foundKeys := make(map[string]bool)
	for _, key := range keyList {
		foundKeys[key.Name] = true
	}
	for _, name := range keys {
		if !foundKeys[name] {
			t.Errorf("Key %s not found in list", name)
		}
	}
}

func TestKeyList_Empty(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	result := p.KeyList(nil)
	if !result.Success {
		t.Fatalf("KeyList failed: %v", result.Error)
	}

	keyList := result.Resource.([]*providerv1.KeyState)
	if len(keyList) != 0 {
		t.Errorf("Expected 0 keys, got %d", len(keyList))
	}
}

func TestKeyDelete(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	// Create a key first
	createReq := &providerv1.KeyCreateRequest{
		Name: "delete-test-key",
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}
	createResult := p.KeyCreate(createReq)
	if !createResult.Success {
		t.Fatalf("KeyCreate failed: %v", createResult.Error)
	}

	keyState := createResult.Resource.(*providerv1.KeyState)
	privateKeyPath := keyState.PrivateKeyPath
	publicKeyPath := keyState.PublicKeyPath

	// Verify files exist before delete
	if _, err := os.Stat(privateKeyPath); err != nil {
		t.Fatalf("Private key file should exist before delete")
	}

	// Delete the key
	result := p.KeyDelete("delete-test-key")
	if !result.Success {
		t.Fatalf("KeyDelete failed: %v", result.Error)
	}

	// Verify key is no longer retrievable
	getResult := p.KeyGet("delete-test-key")
	if getResult.Success {
		t.Error("Key should not be retrievable after delete")
	}

	// Verify files were deleted
	if _, err := os.Stat(privateKeyPath); !os.IsNotExist(err) {
		t.Error("Private key file should be deleted")
	}
	if _, err := os.Stat(publicKeyPath); !os.IsNotExist(err) {
		t.Error("Public key file should be deleted")
	}
}

func TestKeyDelete_Idempotent(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	// Delete should be idempotent - return success even for nonexistent keys
	result := p.KeyDelete("nonexistent-key")
	if !result.Success {
		t.Fatalf("Expected KeyDelete to succeed for nonexistent key (idempotent), got error: %v", result.Error)
	}
}

func TestKeyDelete_InUseByVM(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	// Create a key
	createReq := &providerv1.KeyCreateRequest{
		Name: "in-use-key",
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}
	result := p.KeyCreate(createReq)
	if !result.Success {
		t.Fatalf("KeyCreate failed: %v", result.Error)
	}

	// Manually add a VM that references the key
	p.vms["test-vm"] = &providerv1.VMState{
		Name: "test-vm",
		ProviderState: map[string]any{
			"keys": []string{"in-use-key"},
		},
	}

	// Try to delete the key - should fail
	deleteResult := p.KeyDelete("in-use-key")
	if deleteResult.Success {
		t.Fatal("Expected KeyDelete to fail when key is in use by VM")
	}

	if deleteResult.Error.Code != providerv1.ErrCodeResourceBusy {
		t.Errorf("Expected ErrCodeResourceBusy, got %s", deleteResult.Error.Code)
	}
}

func TestKeyCreate_PrivateKeyParseable(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	req := &providerv1.KeyCreateRequest{
		Name: "parseable-key",
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}

	result := p.KeyCreate(req)
	if !result.Success {
		t.Fatalf("KeyCreate failed: %v", result.Error)
	}

	keyState := result.Resource.(*providerv1.KeyState)

	// Read the private key file
	privateKeyPEM, err := os.ReadFile(keyState.PrivateKeyPath)
	if err != nil {
		t.Fatalf("Failed to read private key file: %v", err)
	}

	// Verify it can be parsed
	_, err = ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		t.Errorf("Private key should be parseable: %v", err)
	}
}

func TestKeyCreate_RSADefaultBits(t *testing.T) {
	p, cleanup := testProvider(t)
	defer cleanup()

	req := &providerv1.KeyCreateRequest{
		Name: "rsa-default-bits",
		Spec: providerv1.KeySpec{
			Type: "rsa",
			// Bits not specified, should default to 4096
		},
	}

	result := p.KeyCreate(req)
	if !result.Success {
		t.Fatalf("KeyCreate failed: %v", result.Error)
	}

	// The key should have been created successfully
	// We can't easily verify the bit size from the public key, but we can verify it's valid
	keyState := result.Resource.(*providerv1.KeyState)
	if !strings.HasPrefix(keyState.PublicKey, "ssh-rsa") {
		t.Error("RSA key public key should start with 'ssh-rsa'")
	}
}
