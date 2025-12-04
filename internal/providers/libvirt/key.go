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
	"crypto/rsa"
	"encoding/pem"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// KeyCreate creates an SSH key and stores it in the state directory.
func (p *Provider) KeyCreate(req *providerv1.KeyCreateRequest) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.keys[req.Name]; exists {
		return providerv1.ErrorResult(providerv1.NewAlreadyExistsError("key", req.Name))
	}

	keyType := req.Spec.Type
	if keyType == "" {
		keyType = "ed25519"
	}

	var (
		privateKeyPEM []byte
		publicKey     ssh.PublicKey
		err           error
	)

	switch keyType {
	case "ed25519":
		privateKeyPEM, publicKey, err = generateED25519Key()
	case "rsa":
		bits := 4096
		if req.Spec.Bits > 0 {
			bits = req.Spec.Bits
		}
		privateKeyPEM, publicKey, err = generateRSAKey(bits)
	default:
		return providerv1.ErrorResult(providerv1.NewInvalidSpecError("unsupported key type: " + keyType))
	}

	if err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to generate key: "+err.Error(), true))
	}

	// Generate public key string in authorized_keys format
	publicKeyStr := string(ssh.MarshalAuthorizedKey(publicKey))

	// Calculate fingerprint
	fingerprint := ssh.FingerprintSHA256(publicKey)

	// Determine file paths
	privateKeyPath := filepath.Join(p.config.StateDir, "keys", req.Name)
	publicKeyPath := filepath.Join(p.config.StateDir, "keys", req.Name+".pub")

	// Write private key (mode 0600)
	if err := os.WriteFile(privateKeyPath, privateKeyPEM, 0600); err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to write private key: "+err.Error(), false))
	}

	// Write public key (mode 0644)
	if err := os.WriteFile(publicKeyPath, []byte(publicKeyStr), 0644); err != nil {
		// Cleanup private key on failure
		_ = os.Remove(privateKeyPath)
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to write public key: "+err.Error(), false))
	}

	state := &providerv1.KeyState{
		Name:           req.Name,
		Type:           keyType,
		PublicKey:      publicKeyStr,
		PublicKeyPath:  publicKeyPath,
		PrivateKeyPath: privateKeyPath,
		Fingerprint:    fingerprint,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	p.keys[req.Name] = state
	return providerv1.SuccessResult(state)
}

// KeyGet retrieves an SSH key by name.
func (p *Provider) KeyGet(name string) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key, exists := p.keys[name]
	if !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("key", name))
	}

	return providerv1.SuccessResult(key)
}

// KeyList lists all SSH keys.
func (p *Provider) KeyList(filter map[string]any) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	keys := make([]*providerv1.KeyState, 0, len(p.keys))
	for _, key := range p.keys {
		keys = append(keys, key)
	}

	return providerv1.SuccessResult(keys)
}

// KeyDelete deletes an SSH key by name.
// This function is idempotent: it will attempt to delete key files even if
// the key is not in in-memory state (e.g., from a previous crashed run).
func (p *Provider) KeyDelete(name string) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	key, exists := p.keys[name]

	// Check if any VM in our state is using this key
	// (only if we have state - if we don't, we can't know about VMs)
	if exists {
		for _, vm := range p.vms {
			if keys, ok := vm.ProviderState["keys"].([]string); ok {
				for _, k := range keys {
					if k == name {
						return providerv1.ErrorResult(providerv1.NewResourceBusyError("key", name))
					}
				}
			}
		}
	}

	// Delete key files from state if available
	if key != nil {
		_ = os.Remove(key.PrivateKeyPath)
		_ = os.Remove(key.PublicKeyPath)
	}

	// Also try to delete by convention if no state exists
	// This handles cases where state was lost but files remain
	if key == nil {
		privateKeyPath := filepath.Join(p.config.StateDir, "keys", name)
		publicKeyPath := filepath.Join(p.config.StateDir, "keys", name+".pub")
		_ = os.Remove(privateKeyPath)
		_ = os.Remove(publicKeyPath)
	}

	delete(p.keys, name)

	// Always return success for idempotent delete
	return providerv1.SuccessResult(nil)
}

// generateED25519Key generates an ED25519 key pair.
func generateED25519Key() ([]byte, ssh.PublicKey, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, nil, err
	}

	// Marshal private key to OpenSSH format
	pemBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, nil, err
	}

	return pem.EncodeToMemory(pemBlock), sshPubKey, nil
}

// generateRSAKey generates an RSA key pair with the specified bit size.
func generateRSAKey(bits int) ([]byte, ssh.PublicKey, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}

	sshPubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}

	// Marshal private key to OpenSSH format
	pemBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, nil, err
	}

	return pem.EncodeToMemory(pemBlock), sshPubKey, nil
}
