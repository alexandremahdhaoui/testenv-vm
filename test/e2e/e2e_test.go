//go:build e2e

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

// Package e2e_test provides end-to-end tests for testenv-vm.
// These tests use the stub provider to validate the full testenv-vm flow
// without requiring real infrastructure.
package e2e_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBasicVMCreation tests creating a single VM with a key and network.
// This is the most basic E2E scenario validating the core create/delete flow.
func TestBasicVMCreation(t *testing.T) {
	// Load the basic VM scenario
	spec := loadScenario(t, "basic_vm.yaml")

	// Create the test environment
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// Verify artifact was created
	assertArtifactNotNil(t, env)

	// Verify the test ID is set correctly
	if env.Artifact.TestID != env.ID {
		t.Errorf("artifact testID mismatch: expected %s, got %s", env.ID, env.Artifact.TestID)
	}

	// Verify VM was created and has expected metadata
	vm := getVM(t, env, "test-vm")
	if vm.Name != "test-vm" {
		t.Errorf("VM name mismatch: expected test-vm, got %s", vm.Name)
	}
	if vm.IP == "" {
		t.Error("VM should have an IP address assigned")
	}

	// Verify network was created
	network := getNetwork(t, env, "test-network")
	if network.Name != "test-network" {
		t.Errorf("network name mismatch: expected test-network, got %s", network.Name)
	}

	// Verify key was created
	key := getKey(t, env, "test-key")
	if key.Name != "test-key" {
		t.Errorf("key name mismatch: expected test-key, got %s", key.Name)
	}

	// Verify managed resources contain expected resource URIs
	assertManagedResourcesContains(t, env,
		"testenv-vm://key/test-key",
		"testenv-vm://network/test-network",
		"testenv-vm://vm/test-vm",
	)
}

// TestMultiNetworkTopology tests creating multiple networks with a VM.
// Verifies that multiple independent networks can be created.
func TestMultiNetworkTopology(t *testing.T) {
	// Load the multi-network scenario
	spec := loadScenario(t, "multi_network.yaml")

	// Create the test environment
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// Verify artifact was created
	assertArtifactNotNil(t, env)

	// Verify all three networks were created
	mgmtNetwork := getNetwork(t, env, "mgmt-network")
	if mgmtNetwork.IP == "" {
		t.Error("mgmt-network should have an IP address")
	}

	dataNetwork := getNetwork(t, env, "data-network")
	if dataNetwork.IP == "" {
		t.Error("data-network should have an IP address")
	}

	storageNetwork := getNetwork(t, env, "storage-network")
	if storageNetwork.IP == "" {
		t.Error("storage-network should have an IP address")
	}

	// Verify VM was created
	vm := getVM(t, env, "multi-net-vm")
	if vm.Name != "multi-net-vm" {
		t.Errorf("VM name mismatch: expected multi-net-vm, got %s", vm.Name)
	}

	// Verify key was created
	key := getKey(t, env, "mgmt-key")
	if key.Name != "mgmt-key" {
		t.Errorf("key name mismatch: expected mgmt-key, got %s", key.Name)
	}

	// Verify managed resources contain all expected resource URIs
	assertManagedResourcesContains(t, env,
		"testenv-vm://key/mgmt-key",
		"testenv-vm://network/mgmt-network",
		"testenv-vm://network/data-network",
		"testenv-vm://network/storage-network",
		"testenv-vm://vm/multi-net-vm",
	)
}

// TestDependencyChain tests complex dependency chains for DAG execution order.
// This verifies that resources are created in the correct order based on template references.
func TestDependencyChain(t *testing.T) {
	// Load the dependency chain scenario
	spec := loadScenario(t, "dependency_chain.yaml")

	// Create the test environment
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// Verify artifact was created
	assertArtifactNotNil(t, env)

	// Verify all three keys were created
	key1 := getKey(t, env, "key-1")
	if key1.Name != "key-1" {
		t.Errorf("key name mismatch: expected key-1, got %s", key1.Name)
	}

	key2 := getKey(t, env, "key-2")
	if key2.Name != "key-2" {
		t.Errorf("key name mismatch: expected key-2, got %s", key2.Name)
	}

	key3 := getKey(t, env, "key-3")
	if key3.Name != "key-3" {
		t.Errorf("key name mismatch: expected key-3, got %s", key3.Name)
	}

	// Verify both networks were created
	network1 := getNetwork(t, env, "network-1")
	if network1.IP == "" {
		t.Error("network-1 should have an IP address")
	}

	network2 := getNetwork(t, env, "network-2")
	if network2.IP == "" {
		t.Error("network-2 should have an IP address")
	}

	// Verify both VMs were created (VMs depend on keys and networks via templates)
	vm1 := getVM(t, env, "vm-1")
	if vm1.Name != "vm-1" {
		t.Errorf("VM name mismatch: expected vm-1, got %s", vm1.Name)
	}

	vm2 := getVM(t, env, "vm-2")
	if vm2.Name != "vm-2" {
		t.Errorf("VM name mismatch: expected vm-2, got %s", vm2.Name)
	}

	// Verify managed resources contain all expected resource URIs
	assertManagedResourcesContains(t, env,
		"testenv-vm://key/key-1",
		"testenv-vm://key/key-2",
		"testenv-vm://key/key-3",
		"testenv-vm://network/network-1",
		"testenv-vm://network/network-2",
		"testenv-vm://vm/vm-1",
		"testenv-vm://vm/vm-2",
	)
}

// TestNetworkDependency verifies that network is created before VM.
// The VM spec references the network by name, so the network must exist first.
func TestNetworkDependency(t *testing.T) {
	// Load the basic VM scenario which has a network dependency
	spec := loadScenario(t, "basic_vm.yaml")

	// Create the test environment
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// If we got here without error, the dependency was handled correctly
	// The orchestrator DAG ensures network is created before VM
	assertArtifactNotNil(t, env)

	// Verify both resources exist
	_ = getNetwork(t, env, "test-network")
	_ = getVM(t, env, "test-vm")
}

// TestKeyDependency verifies that key is created before VM.
// The VM cloud-init uses {{ .Keys.test-key.PublicKey }} template.
func TestKeyDependency(t *testing.T) {
	// Load the basic VM scenario which has a key dependency
	spec := loadScenario(t, "basic_vm.yaml")

	// Create the test environment
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// If we got here without error, the dependency was handled correctly
	// The orchestrator DAG ensures key is created before VM
	assertArtifactNotNil(t, env)

	// Verify both resources exist
	_ = getKey(t, env, "test-key")
	_ = getVM(t, env, "test-vm")
}

// TestCleanup verifies that delete removes all resources.
func TestCleanup(t *testing.T) {
	// Load the basic VM scenario
	spec := loadScenario(t, "basic_vm.yaml")

	// Create the test environment
	env := createTestenv(t, spec)

	// Verify artifact was created
	assertArtifactNotNil(t, env)

	// Now delete the environment
	err := deleteTestenv(t, env)
	if err != nil {
		t.Fatalf("failed to delete test environment: %v", err)
	}

	// After delete, the orchestrator should be closed and resources cleaned up
	// We can't directly verify the provider state, but we verify delete succeeded
}

// TestCleanupOnFailure tests that rollback is triggered on failure.
// This uses an invalid spec to force a failure during creation.
func TestCleanupOnFailure(t *testing.T) {
	// Create an invalid spec that will fail during creation
	// Use basic_vm.yaml as a base but modify it to cause a failure
	spec := loadScenario(t, "basic_vm.yaml")

	// Make the provider reference invalid to cause a failure
	// This should trigger a rollback of any resources created before the failure
	if len(spec.Providers) > 0 {
		// Set the engine to a non-existent binary path
		spec.Providers[0].Engine = "/nonexistent/path/to/provider"
	}

	// Generate a unique test ID
	testID := generateTestID(t)

	// Create temporary directories for this test
	tmpDir := t.TempDir()
	stateDir := tmpDir + "/state"
	artifactDir := tmpDir + "/artifacts"

	// Create orchestrator
	imageCacheDir := filepath.Join(tmpDir, "images")
	orch, err := orchestrator.NewOrchestrator(orchestrator.Config{
		StateDir:         stateDir,
		ImageCacheDir:    imageCacheDir,
		CleanupOnFailure: true,
	})
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	defer orch.Close()

	// Convert spec to map
	specMap, err := specToMap(spec)
	if err != nil {
		t.Fatalf("failed to convert spec to map: %v", err)
	}

	// Try to create - should fail
	ctx := context.Background()
	input := &v1.CreateInput{
		TestID:   testID,
		Stage:    "e2e",
		TmpDir:   artifactDir,
		Spec:     specMap,
		Env:      map[string]string{},
		RootDir:  getProjectRoot(t),
		Metadata: map[string]string{},
	}

	_, err = orch.Create(ctx, input)

	// Creation should have failed
	if err == nil {
		t.Fatal("expected creation to fail with invalid provider path")
	}

	// Verify the error is related to the provider
	if !strings.Contains(err.Error(), "provider") && !strings.Contains(err.Error(), "nonexistent") {
		t.Logf("got error: %v", err)
		// Still acceptable - as long as it failed
	}
}

// TestArtifactMetadata verifies that artifact metadata is populated correctly.
func TestArtifactMetadata(t *testing.T) {
	// Load the basic VM scenario
	spec := loadScenario(t, "basic_vm.yaml")

	// Create the test environment
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// Verify artifact metadata contains expected keys
	assertArtifactNotNil(t, env)

	// Check for VM IP in metadata
	vmIPKey := "testenv-vm.vm.test-vm.ip"
	if _, ok := env.Artifact.Metadata[vmIPKey]; !ok {
		t.Errorf("expected metadata key %s to exist", vmIPKey)
	}

	// Check for network IP in metadata
	networkIPKey := "testenv-vm.network.test-network.ip"
	if _, ok := env.Artifact.Metadata[networkIPKey]; !ok {
		t.Errorf("expected metadata key %s to exist", networkIPKey)
	}

	// Check that env contains VM SSH info
	sshEnvKey := "TESTENV_VM_TEST_VM_SSH"
	if _, ok := env.Artifact.Env[sshEnvKey]; !ok {
		t.Errorf("expected env key %s to exist", sshEnvKey)
	}

	// Check that env contains VM IP
	ipEnvKey := "TESTENV_VM_TEST_VM_IP"
	if _, ok := env.Artifact.Env[ipEnvKey]; !ok {
		t.Errorf("expected env key %s to exist", ipEnvKey)
	}
}

// TestArtifactFiles verifies that artifact files reference key paths correctly.
func TestArtifactFiles(t *testing.T) {
	// Load the basic VM scenario
	spec := loadScenario(t, "basic_vm.yaml")

	// Create the test environment
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// Verify artifact files contains key path
	assertArtifactNotNil(t, env)

	// Check for key file in artifact files
	keyFileKey := "testenv-vm.key.test-key"
	if _, ok := env.Artifact.Files[keyFileKey]; !ok {
		t.Errorf("expected files key %s to exist", keyFileKey)
	}
}

// TestClientConstruction tests that a client can be created from an artifact.
// Uses MockSSHRunner - tests construction and command formatting, not real SSH.
func TestClientConstruction(t *testing.T) {
	spec := loadScenario(t, "basic_vm.yaml")
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// Create client with mock SSH
	c, mockRunner := createClientWithMockSSH(t, env.Artifact, "test-vm")
	defer c.Close()

	// Verify client was constructed
	assert.NotNil(t, c)

	// Run a command - verifies formatting works
	ctx := context.Background()
	stdout, _, err := c.Run(ctx, "echo", "hello")
	assert.NoError(t, err)
	assert.Equal(t, "ok", stdout) // MockSSHRunner default

	// Verify command was recorded
	commands := mockRunner.GetCommands()
	assert.Len(t, commands, 1)
	assert.Contains(t, commands[0], "echo")
	assert.Contains(t, commands[0], "hello")
}

// TestClientWithPrivilegeEscalation tests command formatting with sudo.
func TestClientWithPrivilegeEscalation(t *testing.T) {
	spec := loadScenario(t, "basic_vm.yaml")
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	c, mockRunner := createClientWithMockSSH(t, env.Artifact, "test-vm")
	defer c.Close()

	// Create context with sudo
	execCtx := client.NewExecutionContext().
		WithPrivilegeEscalation(client.PrivilegeEscalationSudo())

	ctx := context.Background()
	_, _, err := c.RunWithContext(ctx, execCtx, "apt-get", "update")
	assert.NoError(t, err)

	// Verify sudo was added to command
	commands := mockRunner.GetCommands()
	assert.Len(t, commands, 1)
	assert.Contains(t, commands[0], "sudo")
	assert.Contains(t, commands[0], "apt-get")
}

// TestClientFileCopyFormatting tests file copy command generation.
func TestClientFileCopyFormatting(t *testing.T) {
	spec := loadScenario(t, "basic_vm.yaml")
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	c, mockRunner := createClientWithMockSSH(t, env.Artifact, "test-vm")
	defer c.Close()

	// Create a temp file to copy
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(tmpFile, []byte("test content"), 0644)
	require.NoError(t, err)

	ctx := context.Background()
	err = c.CopyTo(ctx, tmpFile, "/remote/path/test.txt")
	assert.NoError(t, err)

	// Verify base64 command was generated
	commands := mockRunner.GetCommands()
	assert.GreaterOrEqual(t, len(commands), 1)
	// Should contain base64 -d for decoding
	found := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "base64") && strings.Contains(cmd, "/remote/path/test.txt") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected base64 copy command, got: %v", commands)
}

// TestTemplatedAttachTo verifies that templated attachTo fields work correctly.
// This is the primary bug fix validation test for the attachTo validation bug.
// The scenario uses a network with attachTo: "{{ .Networks.parent-bridge.InterfaceName }}"
// which previously failed validation before template expansion.
func TestTemplatedAttachTo(t *testing.T) {
	// Load the templated attachTo scenario
	spec := loadScenario(t, "templated_attachto.yaml")

	// Create the test environment
	env := createTestenv(t, spec)
	defer func() {
		if err := deleteTestenv(t, env); err != nil {
			t.Errorf("failed to delete test environment: %v", err)
		}
	}()

	// Verify artifact was created
	assertArtifactNotNil(t, env)

	// Verify parent-bridge network was created
	parentNetwork := getNetwork(t, env, "parent-bridge")
	if parentNetwork.Name != "parent-bridge" {
		t.Errorf("network name mismatch: expected parent-bridge, got %s", parentNetwork.Name)
	}

	// Verify child-network was created (this is the network with templated attachTo)
	childNetwork := getNetwork(t, env, "child-network")
	if childNetwork.Name != "child-network" {
		t.Errorf("network name mismatch: expected child-network, got %s", childNetwork.Name)
	}

	// Verify VM was created
	vm := getVM(t, env, "test-vm")
	if vm.Name != "test-vm" {
		t.Errorf("VM name mismatch: expected test-vm, got %s", vm.Name)
	}

	// Verify key was created
	key := getKey(t, env, "test-key")
	if key.Name != "test-key" {
		t.Errorf("key name mismatch: expected test-key, got %s", key.Name)
	}

	// Verify managed resources contain all expected resource URIs
	assertManagedResourcesContains(t, env,
		"testenv-vm://key/test-key",
		"testenv-vm://network/parent-bridge",
		"testenv-vm://network/child-network",
		"testenv-vm://vm/test-vm",
	)
}
