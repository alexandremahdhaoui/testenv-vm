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

// Package stub provides a stub provider that simulates real provider behavior
// for E2E testing without real infrastructure.
package stub

import (
	"sync"
	"testing"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// ----------------------------------------------------------------------------
// NewProvider tests
// ----------------------------------------------------------------------------

func TestNewProvider(t *testing.T) {
	p := NewProvider()

	if p == nil {
		t.Fatal("NewProvider returned nil")
	}
	if p.keys == nil {
		t.Error("keys map is nil")
	}
	if p.networks == nil {
		t.Error("networks map is nil")
	}
	if p.vms == nil {
		t.Error("vms map is nil")
	}
}

// ----------------------------------------------------------------------------
// Capabilities tests
// ----------------------------------------------------------------------------

func TestCapabilities(t *testing.T) {
	p := NewProvider()
	caps := p.Capabilities()

	if caps == nil {
		t.Fatal("Capabilities returned nil")
	}
	if caps.ProviderName != "stub" {
		t.Errorf("expected ProviderName 'stub', got %q", caps.ProviderName)
	}
	if caps.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", caps.Version)
	}
	if len(caps.Resources) != 3 {
		t.Errorf("expected 3 resource capabilities, got %d", len(caps.Resources))
	}

	// Verify each resource capability
	expectedResources := map[string][]string{
		"key":     {"create", "get", "list", "delete"},
		"network": {"create", "get", "list", "delete"},
		"vm":      {"create", "get", "list", "delete"},
	}

	for _, rc := range caps.Resources {
		expectedOps, ok := expectedResources[rc.Kind]
		if !ok {
			t.Errorf("unexpected resource kind: %s", rc.Kind)
			continue
		}
		if len(rc.Operations) != len(expectedOps) {
			t.Errorf("resource %s: expected %d operations, got %d", rc.Kind, len(expectedOps), len(rc.Operations))
		}
		for i, op := range rc.Operations {
			if op != expectedOps[i] {
				t.Errorf("resource %s: expected operation %q at index %d, got %q", rc.Kind, expectedOps[i], i, op)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Key operation tests
// ----------------------------------------------------------------------------

func TestKeyCreate_Success(t *testing.T) {
	p := NewProvider()
	req := &providerv1.KeyCreateRequest{
		Name: "test-key",
		Spec: providerv1.KeySpec{
			Type:    "ed25519",
			Comment: "test comment",
		},
	}

	result := p.KeyCreate(req)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got: %v", result.Error)
	}

	keyState, ok := result.Resource.(*providerv1.KeyState)
	if !ok {
		t.Fatalf("expected *KeyState, got %T", result.Resource)
	}
	if keyState.Name != "test-key" {
		t.Errorf("expected name 'test-key', got %q", keyState.Name)
	}
	if keyState.Type != "ed25519" {
		t.Errorf("expected type 'ed25519', got %q", keyState.Type)
	}
	if keyState.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
	if keyState.PublicKey == "" {
		t.Error("expected non-empty public key")
	}
	if keyState.PublicKeyPath == "" {
		t.Error("expected non-empty public key path")
	}
	if keyState.PrivateKeyPath == "" {
		t.Error("expected non-empty private key path")
	}
	if keyState.CreatedAt == "" {
		t.Error("expected non-empty created at timestamp")
	}
}

func TestKeyCreate_DefaultType(t *testing.T) {
	p := NewProvider()
	req := &providerv1.KeyCreateRequest{
		Name: "test-key",
		Spec: providerv1.KeySpec{
			Type: "", // Empty type should default to ed25519
		},
	}

	result := p.KeyCreate(req)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	keyState := result.Resource.(*providerv1.KeyState)
	if keyState.Type != "ed25519" {
		t.Errorf("expected default type 'ed25519', got %q", keyState.Type)
	}
}

func TestKeyCreate_AlreadyExists(t *testing.T) {
	p := NewProvider()
	req := &providerv1.KeyCreateRequest{
		Name: "test-key",
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}

	// First creation should succeed
	result := p.KeyCreate(req)
	if !result.Success {
		t.Fatalf("first creation failed: %v", result.Error)
	}

	// Second creation should fail
	result = p.KeyCreate(req)

	if result.Success {
		t.Fatal("expected failure for duplicate key")
	}
	if result.Error == nil {
		t.Fatal("expected error for duplicate key")
	}
	if result.Error.Code != providerv1.ErrCodeAlreadyExists {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeAlreadyExists, result.Error.Code)
	}
}

func TestKeyGet_Success(t *testing.T) {
	p := NewProvider()

	// Create a key first
	createReq := &providerv1.KeyCreateRequest{
		Name: "test-key",
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}
	p.KeyCreate(createReq)

	// Get the key
	result := p.KeyGet("test-key")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	keyState, ok := result.Resource.(*providerv1.KeyState)
	if !ok {
		t.Fatalf("expected *KeyState, got %T", result.Resource)
	}
	if keyState.Name != "test-key" {
		t.Errorf("expected name 'test-key', got %q", keyState.Name)
	}
}

func TestKeyGet_NotFound(t *testing.T) {
	p := NewProvider()

	result := p.KeyGet("nonexistent-key")

	if result.Success {
		t.Fatal("expected failure for nonexistent key")
	}
	if result.Error == nil {
		t.Fatal("expected error for nonexistent key")
	}
	if result.Error.Code != providerv1.ErrCodeNotFound {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeNotFound, result.Error.Code)
	}
}

func TestKeyList_Empty(t *testing.T) {
	p := NewProvider()

	result := p.KeyList(nil)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	keys, ok := result.Resource.([]*providerv1.KeyState)
	if !ok {
		t.Fatalf("expected []*KeyState, got %T", result.Resource)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty list, got %d keys", len(keys))
	}
}

func TestKeyList_WithKeys(t *testing.T) {
	p := NewProvider()

	// Create some keys
	for _, name := range []string{"key1", "key2", "key3"} {
		p.KeyCreate(&providerv1.KeyCreateRequest{
			Name: name,
			Spec: providerv1.KeySpec{Type: "ed25519"},
		})
	}

	result := p.KeyList(nil)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	keys, ok := result.Resource.([]*providerv1.KeyState)
	if !ok {
		t.Fatalf("expected []*KeyState, got %T", result.Resource)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestKeyDelete_Success(t *testing.T) {
	p := NewProvider()

	// Create a key first
	p.KeyCreate(&providerv1.KeyCreateRequest{
		Name: "test-key",
		Spec: providerv1.KeySpec{Type: "ed25519"},
	})

	// Delete the key
	result := p.KeyDelete("test-key")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	// Verify key is deleted
	getResult := p.KeyGet("test-key")
	if getResult.Success {
		t.Error("expected key to be deleted")
	}
}

func TestKeyDelete_NotFound(t *testing.T) {
	p := NewProvider()

	result := p.KeyDelete("nonexistent-key")

	if result.Success {
		t.Fatal("expected failure for nonexistent key")
	}
	if result.Error == nil {
		t.Fatal("expected error for nonexistent key")
	}
	if result.Error.Code != providerv1.ErrCodeNotFound {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeNotFound, result.Error.Code)
	}
}

// ----------------------------------------------------------------------------
// Network operation tests
// ----------------------------------------------------------------------------

func TestNetworkCreate_Success(t *testing.T) {
	p := NewProvider()
	req := &providerv1.NetworkCreateRequest{
		Name: "test-network",
		Kind: "bridge",
		Spec: providerv1.NetworkSpec{
			CIDR: "10.0.0.0/24",
		},
	}

	result := p.NetworkCreate(req)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got: %v", result.Error)
	}

	netState, ok := result.Resource.(*providerv1.NetworkState)
	if !ok {
		t.Fatalf("expected *NetworkState, got %T", result.Resource)
	}
	if netState.Name != "test-network" {
		t.Errorf("expected name 'test-network', got %q", netState.Name)
	}
	if netState.Kind != "bridge" {
		t.Errorf("expected kind 'bridge', got %q", netState.Kind)
	}
	if netState.CIDR != "10.0.0.0/24" {
		t.Errorf("expected CIDR '10.0.0.0/24', got %q", netState.CIDR)
	}
	if netState.Status != "ready" {
		t.Errorf("expected status 'ready', got %q", netState.Status)
	}
	if netState.IP == "" {
		t.Error("expected non-empty IP")
	}
	if netState.InterfaceName == "" {
		t.Error("expected non-empty interface name")
	}
	if netState.UUID == "" {
		t.Error("expected non-empty UUID")
	}
}

func TestNetworkCreate_DefaultKind(t *testing.T) {
	p := NewProvider()
	req := &providerv1.NetworkCreateRequest{
		Name: "test-network",
		Kind: "", // Empty kind should default to bridge
		Spec: providerv1.NetworkSpec{},
	}

	result := p.NetworkCreate(req)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	netState := result.Resource.(*providerv1.NetworkState)
	if netState.Kind != "bridge" {
		t.Errorf("expected default kind 'bridge', got %q", netState.Kind)
	}
}

func TestNetworkCreate_DefaultCIDR(t *testing.T) {
	p := NewProvider()
	req := &providerv1.NetworkCreateRequest{
		Name: "test-network",
		Kind: "bridge",
		Spec: providerv1.NetworkSpec{
			CIDR: "", // Empty CIDR should get default
		},
	}

	result := p.NetworkCreate(req)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	netState := result.Resource.(*providerv1.NetworkState)
	if netState.CIDR != "192.168.100.0/24" {
		t.Errorf("expected default CIDR '192.168.100.0/24', got %q", netState.CIDR)
	}
}

func TestNetworkCreate_AlreadyExists(t *testing.T) {
	p := NewProvider()
	req := &providerv1.NetworkCreateRequest{
		Name: "test-network",
		Kind: "bridge",
		Spec: providerv1.NetworkSpec{},
	}

	// First creation should succeed
	result := p.NetworkCreate(req)
	if !result.Success {
		t.Fatalf("first creation failed: %v", result.Error)
	}

	// Second creation should fail
	result = p.NetworkCreate(req)

	if result.Success {
		t.Fatal("expected failure for duplicate network")
	}
	if result.Error == nil {
		t.Fatal("expected error for duplicate network")
	}
	if result.Error.Code != providerv1.ErrCodeAlreadyExists {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeAlreadyExists, result.Error.Code)
	}
}

func TestNetworkGet_Success(t *testing.T) {
	p := NewProvider()

	// Create a network first
	p.NetworkCreate(&providerv1.NetworkCreateRequest{
		Name: "test-network",
		Kind: "bridge",
		Spec: providerv1.NetworkSpec{},
	})

	// Get the network
	result := p.NetworkGet("test-network")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	netState, ok := result.Resource.(*providerv1.NetworkState)
	if !ok {
		t.Fatalf("expected *NetworkState, got %T", result.Resource)
	}
	if netState.Name != "test-network" {
		t.Errorf("expected name 'test-network', got %q", netState.Name)
	}
}

func TestNetworkGet_NotFound(t *testing.T) {
	p := NewProvider()

	result := p.NetworkGet("nonexistent-network")

	if result.Success {
		t.Fatal("expected failure for nonexistent network")
	}
	if result.Error == nil {
		t.Fatal("expected error for nonexistent network")
	}
	if result.Error.Code != providerv1.ErrCodeNotFound {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeNotFound, result.Error.Code)
	}
}

func TestNetworkList_Empty(t *testing.T) {
	p := NewProvider()

	result := p.NetworkList(nil)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	networks, ok := result.Resource.([]*providerv1.NetworkState)
	if !ok {
		t.Fatalf("expected []*NetworkState, got %T", result.Resource)
	}
	if len(networks) != 0 {
		t.Errorf("expected empty list, got %d networks", len(networks))
	}
}

func TestNetworkList_WithNetworks(t *testing.T) {
	p := NewProvider()

	// Create some networks
	for _, name := range []string{"net1", "net2", "net3"} {
		p.NetworkCreate(&providerv1.NetworkCreateRequest{
			Name: name,
			Kind: "bridge",
			Spec: providerv1.NetworkSpec{},
		})
	}

	result := p.NetworkList(nil)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	networks, ok := result.Resource.([]*providerv1.NetworkState)
	if !ok {
		t.Fatalf("expected []*NetworkState, got %T", result.Resource)
	}
	if len(networks) != 3 {
		t.Errorf("expected 3 networks, got %d", len(networks))
	}
}

func TestNetworkDelete_Success(t *testing.T) {
	p := NewProvider()

	// Create a network first
	p.NetworkCreate(&providerv1.NetworkCreateRequest{
		Name: "test-network",
		Kind: "bridge",
		Spec: providerv1.NetworkSpec{},
	})

	// Delete the network
	result := p.NetworkDelete("test-network")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	// Verify network is deleted
	getResult := p.NetworkGet("test-network")
	if getResult.Success {
		t.Error("expected network to be deleted")
	}
}

func TestNetworkDelete_NotFound(t *testing.T) {
	p := NewProvider()

	result := p.NetworkDelete("nonexistent-network")

	if result.Success {
		t.Fatal("expected failure for nonexistent network")
	}
	if result.Error == nil {
		t.Fatal("expected error for nonexistent network")
	}
	if result.Error.Code != providerv1.ErrCodeNotFound {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeNotFound, result.Error.Code)
	}
}

// ----------------------------------------------------------------------------
// VM operation tests
// ----------------------------------------------------------------------------

func TestVMCreate_Success(t *testing.T) {
	p := NewProvider()
	req := &providerv1.VMCreateRequest{
		Name: "test-vm",
		Spec: providerv1.VMSpec{
			Memory: 1024,
			VCPUs:  2,
		},
	}

	result := p.VMCreate(req)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got: %v", result.Error)
	}

	vmState, ok := result.Resource.(*providerv1.VMState)
	if !ok {
		t.Fatalf("expected *VMState, got %T", result.Resource)
	}
	if vmState.Name != "test-vm" {
		t.Errorf("expected name 'test-vm', got %q", vmState.Name)
	}
	if vmState.Status != "running" {
		t.Errorf("expected status 'running', got %q", vmState.Status)
	}
	if vmState.IP == "" {
		t.Error("expected non-empty IP")
	}
	if vmState.MAC == "" {
		t.Error("expected non-empty MAC")
	}
	if vmState.UUID == "" {
		t.Error("expected non-empty UUID")
	}
	if vmState.SSHCommand == "" {
		t.Error("expected non-empty SSH command")
	}
	if vmState.CreatedAt == "" {
		t.Error("expected non-empty created at timestamp")
	}
}

func TestVMCreate_AlreadyExists(t *testing.T) {
	p := NewProvider()
	req := &providerv1.VMCreateRequest{
		Name: "test-vm",
		Spec: providerv1.VMSpec{},
	}

	// First creation should succeed
	result := p.VMCreate(req)
	if !result.Success {
		t.Fatalf("first creation failed: %v", result.Error)
	}

	// Second creation should fail
	result = p.VMCreate(req)

	if result.Success {
		t.Fatal("expected failure for duplicate VM")
	}
	if result.Error == nil {
		t.Fatal("expected error for duplicate VM")
	}
	if result.Error.Code != providerv1.ErrCodeAlreadyExists {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeAlreadyExists, result.Error.Code)
	}
}

func TestVMGet_Success(t *testing.T) {
	p := NewProvider()

	// Create a VM first
	p.VMCreate(&providerv1.VMCreateRequest{
		Name: "test-vm",
		Spec: providerv1.VMSpec{},
	})

	// Get the VM
	result := p.VMGet("test-vm")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	vmState, ok := result.Resource.(*providerv1.VMState)
	if !ok {
		t.Fatalf("expected *VMState, got %T", result.Resource)
	}
	if vmState.Name != "test-vm" {
		t.Errorf("expected name 'test-vm', got %q", vmState.Name)
	}
}

func TestVMGet_NotFound(t *testing.T) {
	p := NewProvider()

	result := p.VMGet("nonexistent-vm")

	if result.Success {
		t.Fatal("expected failure for nonexistent VM")
	}
	if result.Error == nil {
		t.Fatal("expected error for nonexistent VM")
	}
	if result.Error.Code != providerv1.ErrCodeNotFound {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeNotFound, result.Error.Code)
	}
}

func TestVMList_Empty(t *testing.T) {
	p := NewProvider()

	result := p.VMList(nil)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	vms, ok := result.Resource.([]*providerv1.VMState)
	if !ok {
		t.Fatalf("expected []*VMState, got %T", result.Resource)
	}
	if len(vms) != 0 {
		t.Errorf("expected empty list, got %d VMs", len(vms))
	}
}

func TestVMList_WithVMs(t *testing.T) {
	p := NewProvider()

	// Create some VMs
	for _, name := range []string{"vm1", "vm2", "vm3"} {
		p.VMCreate(&providerv1.VMCreateRequest{
			Name: name,
			Spec: providerv1.VMSpec{},
		})
	}

	result := p.VMList(nil)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	vms, ok := result.Resource.([]*providerv1.VMState)
	if !ok {
		t.Fatalf("expected []*VMState, got %T", result.Resource)
	}
	if len(vms) != 3 {
		t.Errorf("expected 3 VMs, got %d", len(vms))
	}
}

func TestVMDelete_Success(t *testing.T) {
	p := NewProvider()

	// Create a VM first
	p.VMCreate(&providerv1.VMCreateRequest{
		Name: "test-vm",
		Spec: providerv1.VMSpec{},
	})

	// Delete the VM
	result := p.VMDelete("test-vm")

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	// Verify VM is deleted
	getResult := p.VMGet("test-vm")
	if getResult.Success {
		t.Error("expected VM to be deleted")
	}
}

func TestVMDelete_NotFound(t *testing.T) {
	p := NewProvider()

	result := p.VMDelete("nonexistent-vm")

	if result.Success {
		t.Fatal("expected failure for nonexistent VM")
	}
	if result.Error == nil {
		t.Fatal("expected error for nonexistent VM")
	}
	if result.Error.Code != providerv1.ErrCodeNotFound {
		t.Errorf("expected error code %q, got %q", providerv1.ErrCodeNotFound, result.Error.Code)
	}
}

// ----------------------------------------------------------------------------
// Concurrency tests
// ----------------------------------------------------------------------------

func TestConcurrentOperations(t *testing.T) {
	p := NewProvider()
	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	errChan := make(chan *providerv1.OperationError, numGoroutines*numOperations)

	// Run concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				// Create unique resource names per goroutine/operation
				keyName := keyNameForConcurrency(goroutineID, j)
				netName := netNameForConcurrency(goroutineID, j)
				vmName := vmNameForConcurrency(goroutineID, j)

				// Create resources
				keyResult := p.KeyCreate(&providerv1.KeyCreateRequest{
					Name: keyName,
					Spec: providerv1.KeySpec{Type: "ed25519"},
				})
				if !keyResult.Success {
					errChan <- keyResult.Error
				}

				netResult := p.NetworkCreate(&providerv1.NetworkCreateRequest{
					Name: netName,
					Kind: "bridge",
					Spec: providerv1.NetworkSpec{},
				})
				if !netResult.Success {
					errChan <- netResult.Error
				}

				vmResult := p.VMCreate(&providerv1.VMCreateRequest{
					Name: vmName,
					Spec: providerv1.VMSpec{},
				})
				if !vmResult.Success {
					errChan <- vmResult.Error
				}

				// Get resources
				if !p.KeyGet(keyName).Success {
					t.Errorf("failed to get key %s", keyName)
				}
				if !p.NetworkGet(netName).Success {
					t.Errorf("failed to get network %s", netName)
				}
				if !p.VMGet(vmName).Success {
					t.Errorf("failed to get VM %s", vmName)
				}

				// Delete resources
				if !p.KeyDelete(keyName).Success {
					t.Errorf("failed to delete key %s", keyName)
				}
				if !p.NetworkDelete(netName).Success {
					t.Errorf("failed to delete network %s", netName)
				}
				if !p.VMDelete(vmName).Success {
					t.Errorf("failed to delete VM %s", vmName)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	errorCount := 0
	for err := range errChan {
		if err != nil {
			errorCount++
			t.Logf("concurrent operation error: %v", err)
		}
	}

	if errorCount > 0 {
		t.Errorf("had %d errors during concurrent operations", errorCount)
	}

	// Verify all resources are cleaned up
	keyList := p.KeyList(nil)
	keys := keyList.Resource.([]*providerv1.KeyState)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after cleanup, got %d", len(keys))
	}

	netList := p.NetworkList(nil)
	networks := netList.Resource.([]*providerv1.NetworkState)
	if len(networks) != 0 {
		t.Errorf("expected 0 networks after cleanup, got %d", len(networks))
	}

	vmList := p.VMList(nil)
	vms := vmList.Resource.([]*providerv1.VMState)
	if len(vms) != 0 {
		t.Errorf("expected 0 VMs after cleanup, got %d", len(vms))
	}
}

// Helper functions for generating unique names in concurrency test
func keyNameForConcurrency(goroutineID, opID int) string {
	return concurrencyResourceName("key", goroutineID, opID)
}

func netNameForConcurrency(goroutineID, opID int) string {
	return concurrencyResourceName("net", goroutineID, opID)
}

func vmNameForConcurrency(goroutineID, opID int) string {
	return concurrencyResourceName("vm", goroutineID, opID)
}

func concurrencyResourceName(prefix string, goroutineID, opID int) string {
	return prefix + "-" + intToString(goroutineID) + "-" + intToString(opID)
}

// Simple int to string conversion without importing strconv
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// ----------------------------------------------------------------------------
// Table-driven tests for comprehensive coverage
// ----------------------------------------------------------------------------

func TestKeyOperations_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*Provider)
		operation      func(*Provider) *providerv1.OperationResult
		expectSuccess  bool
		expectErrCode  string
		validateResult func(*testing.T, *providerv1.OperationResult)
	}{
		{
			name:  "create key with rsa type",
			setup: func(p *Provider) {},
			operation: func(p *Provider) *providerv1.OperationResult {
				return p.KeyCreate(&providerv1.KeyCreateRequest{
					Name: "rsa-key",
					Spec: providerv1.KeySpec{Type: "rsa", Bits: 4096},
				})
			},
			expectSuccess: true,
			validateResult: func(t *testing.T, result *providerv1.OperationResult) {
				ks := result.Resource.(*providerv1.KeyState)
				if ks.Type != "rsa" {
					t.Errorf("expected type 'rsa', got %q", ks.Type)
				}
			},
		},
		{
			name: "get after delete returns not found",
			setup: func(p *Provider) {
				p.KeyCreate(&providerv1.KeyCreateRequest{
					Name: "temp-key",
					Spec: providerv1.KeySpec{Type: "ed25519"},
				})
				p.KeyDelete("temp-key")
			},
			operation: func(p *Provider) *providerv1.OperationResult {
				return p.KeyGet("temp-key")
			},
			expectSuccess: false,
			expectErrCode: providerv1.ErrCodeNotFound,
		},
		{
			name: "list returns created keys",
			setup: func(p *Provider) {
				p.KeyCreate(&providerv1.KeyCreateRequest{Name: "k1", Spec: providerv1.KeySpec{Type: "ed25519"}})
				p.KeyCreate(&providerv1.KeyCreateRequest{Name: "k2", Spec: providerv1.KeySpec{Type: "ed25519"}})
			},
			operation: func(p *Provider) *providerv1.OperationResult {
				return p.KeyList(nil)
			},
			expectSuccess: true,
			validateResult: func(t *testing.T, result *providerv1.OperationResult) {
				keys := result.Resource.([]*providerv1.KeyState)
				if len(keys) != 2 {
					t.Errorf("expected 2 keys, got %d", len(keys))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider()
			tt.setup(p)

			result := tt.operation(p)

			if result.Success != tt.expectSuccess {
				t.Errorf("expected success=%v, got %v (error: %v)", tt.expectSuccess, result.Success, result.Error)
			}

			if tt.expectErrCode != "" && result.Error != nil {
				if result.Error.Code != tt.expectErrCode {
					t.Errorf("expected error code %q, got %q", tt.expectErrCode, result.Error.Code)
				}
			}

			if tt.validateResult != nil && result.Success {
				tt.validateResult(t, result)
			}
		})
	}
}

func TestNetworkOperations_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*Provider)
		operation      func(*Provider) *providerv1.OperationResult
		expectSuccess  bool
		expectErrCode  string
		validateResult func(*testing.T, *providerv1.OperationResult)
	}{
		{
			name:  "create network with custom CIDR",
			setup: func(p *Provider) {},
			operation: func(p *Provider) *providerv1.OperationResult {
				return p.NetworkCreate(&providerv1.NetworkCreateRequest{
					Name: "custom-net",
					Kind: "bridge",
					Spec: providerv1.NetworkSpec{CIDR: "172.16.0.0/16"},
				})
			},
			expectSuccess: true,
			validateResult: func(t *testing.T, result *providerv1.OperationResult) {
				ns := result.Resource.(*providerv1.NetworkState)
				if ns.CIDR != "172.16.0.0/16" {
					t.Errorf("expected CIDR '172.16.0.0/16', got %q", ns.CIDR)
				}
			},
		},
		{
			name: "double delete returns not found",
			setup: func(p *Provider) {
				p.NetworkCreate(&providerv1.NetworkCreateRequest{
					Name: "temp-net",
					Kind: "bridge",
					Spec: providerv1.NetworkSpec{},
				})
				p.NetworkDelete("temp-net")
			},
			operation: func(p *Provider) *providerv1.OperationResult {
				return p.NetworkDelete("temp-net")
			},
			expectSuccess: false,
			expectErrCode: providerv1.ErrCodeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider()
			tt.setup(p)

			result := tt.operation(p)

			if result.Success != tt.expectSuccess {
				t.Errorf("expected success=%v, got %v (error: %v)", tt.expectSuccess, result.Success, result.Error)
			}

			if tt.expectErrCode != "" && result.Error != nil {
				if result.Error.Code != tt.expectErrCode {
					t.Errorf("expected error code %q, got %q", tt.expectErrCode, result.Error.Code)
				}
			}

			if tt.validateResult != nil && result.Success {
				tt.validateResult(t, result)
			}
		})
	}
}

func TestVMOperations_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*Provider)
		operation      func(*Provider) *providerv1.OperationResult
		expectSuccess  bool
		expectErrCode  string
		validateResult func(*testing.T, *providerv1.OperationResult)
	}{
		{
			name:  "create VM with spec",
			setup: func(p *Provider) {},
			operation: func(p *Provider) *providerv1.OperationResult {
				return p.VMCreate(&providerv1.VMCreateRequest{
					Name: "spec-vm",
					Spec: providerv1.VMSpec{
						Memory: 2048,
						VCPUs:  4,
					},
				})
			},
			expectSuccess: true,
			validateResult: func(t *testing.T, result *providerv1.OperationResult) {
				vs := result.Resource.(*providerv1.VMState)
				if vs.Name != "spec-vm" {
					t.Errorf("expected name 'spec-vm', got %q", vs.Name)
				}
				if vs.Status != "running" {
					t.Errorf("expected status 'running', got %q", vs.Status)
				}
			},
		},
		{
			name: "create after delete succeeds",
			setup: func(p *Provider) {
				p.VMCreate(&providerv1.VMCreateRequest{
					Name: "recycled-vm",
					Spec: providerv1.VMSpec{},
				})
				p.VMDelete("recycled-vm")
			},
			operation: func(p *Provider) *providerv1.OperationResult {
				return p.VMCreate(&providerv1.VMCreateRequest{
					Name: "recycled-vm",
					Spec: providerv1.VMSpec{},
				})
			},
			expectSuccess: true,
			validateResult: func(t *testing.T, result *providerv1.OperationResult) {
				vs := result.Resource.(*providerv1.VMState)
				if vs.Name != "recycled-vm" {
					t.Errorf("expected name 'recycled-vm', got %q", vs.Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider()
			tt.setup(p)

			result := tt.operation(p)

			if result.Success != tt.expectSuccess {
				t.Errorf("expected success=%v, got %v (error: %v)", tt.expectSuccess, result.Success, result.Error)
			}

			if tt.expectErrCode != "" && result.Error != nil {
				if result.Error.Code != tt.expectErrCode {
					t.Errorf("expected error code %q, got %q", tt.expectErrCode, result.Error.Code)
				}
			}

			if tt.validateResult != nil && result.Success {
				tt.validateResult(t, result)
			}
		})
	}
}
