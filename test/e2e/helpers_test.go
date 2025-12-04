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
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/client/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestEnv wraps a test environment with its resources and state.
type TestEnv struct {
	// ID is the unique test environment identifier.
	ID string
	// Spec is the parsed test environment specification.
	Spec *v1.TestenvSpec
	// Artifact is the output artifact from creation.
	Artifact *v1.TestEnvArtifact
	// State is the persisted environment state.
	State *v1.EnvironmentState
	// Orch is the orchestrator instance used for this environment.
	Orch *orchestrator.Orchestrator
	// TmpDir is the temporary directory for artifacts.
	TmpDir string
	// StateDir is the directory for state files.
	StateDir string
}

// loadScenario loads a YAML scenario file and returns the parsed TestenvSpec.
// The path is relative to the scenarios directory.
func loadScenario(t *testing.T, scenarioName string) *v1.TestenvSpec {
	t.Helper()

	// Get the path to the scenarios directory
	scenariosDir := getScenarioDir(t)
	scenarioPath := filepath.Join(scenariosDir, scenarioName)

	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatalf("failed to read scenario file %s: %v", scenarioPath, err)
	}

	testenvSpec, err := spec.Parse(data)
	if err != nil {
		t.Fatalf("failed to parse scenario file %s: %v", scenarioPath, err)
	}

	return testenvSpec
}

// getScenarioDir returns the absolute path to the scenarios directory.
func getScenarioDir(t *testing.T) string {
	t.Helper()

	// Try to find the scenarios directory relative to the test file
	// First, try the current directory structure
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	// Check if we're in the test/e2e directory
	scenariosDir := filepath.Join(cwd, "scenarios")
	if _, err := os.Stat(scenariosDir); err == nil {
		return scenariosDir
	}

	// Check if we're in the project root
	scenariosDir = filepath.Join(cwd, "test", "e2e", "scenarios")
	if _, err := os.Stat(scenariosDir); err == nil {
		return scenariosDir
	}

	t.Fatalf("could not find scenarios directory from %s", cwd)
	return ""
}

// createTestenv creates a test environment from a spec.
// It returns a TestEnv struct containing all information about the created environment.
func createTestenv(t *testing.T, testenvSpec *v1.TestenvSpec) *TestEnv {
	t.Helper()

	// Generate a unique test ID
	testID := generateTestID(t)

	// Create temporary directories for this test
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	artifactDir := filepath.Join(tmpDir, "artifacts")

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		t.Fatalf("failed to create artifact directory: %v", err)
	}

	// Fix provider engine paths to be absolute
	projectRoot := getProjectRoot(t)
	fixProviderPaths(testenvSpec, projectRoot)

	// Create orchestrator with configuration
	orch, err := orchestrator.NewOrchestrator(orchestrator.Config{
		StateDir:         stateDir,
		CleanupOnFailure: true,
	})
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}

	// Convert spec to map for CreateInput
	specMap, err := specToMap(testenvSpec)
	if err != nil {
		t.Fatalf("failed to convert spec to map: %v", err)
	}

	// Create the test environment
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	input := &v1.CreateInput{
		TestID:   testID,
		Stage:    "e2e",
		TmpDir:   artifactDir,
		Spec:     specMap,
		Env:      map[string]string{},
		RootDir:  projectRoot,
		Metadata: map[string]string{},
	}

	artifact, err := orch.Create(ctx, input)
	if err != nil {
		// Clean up on failure
		orch.Close()
		t.Fatalf("failed to create test environment: %v", err)
	}

	return &TestEnv{
		ID:       testID,
		Spec:     testenvSpec,
		Artifact: artifact,
		Orch:     orch,
		TmpDir:   tmpDir,
		StateDir: stateDir,
	}
}

// fixProviderPaths converts relative provider engine paths to absolute paths.
// This is necessary because the tests may run from different working directories.
func fixProviderPaths(testenvSpec *v1.TestenvSpec, projectRoot string) {
	for i := range testenvSpec.Providers {
		engine := testenvSpec.Providers[i].Engine
		// Check if it's a relative path (starts with ./)
		if len(engine) > 2 && engine[:2] == "./" {
			testenvSpec.Providers[i].Engine = filepath.Join(projectRoot, engine[2:])
		}
	}
}

// deleteTestenv deletes a test environment and cleans up resources.
// It should be called in a defer after createTestenv.
func deleteTestenv(t *testing.T, env *TestEnv) error {
	t.Helper()

	if env == nil {
		return nil
	}

	// Ensure orchestrator is closed when done
	defer func() {
		if env.Orch != nil {
			env.Orch.Close()
		}
	}()

	// Create delete input
	input := &v1.DeleteInput{
		TestID:           env.ID,
		Metadata:         map[string]string{},
		ManagedResources: []string{},
	}

	if env.Artifact != nil {
		input.Metadata = env.Artifact.Metadata
		input.ManagedResources = env.Artifact.ManagedResources
	}

	// Delete the test environment
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	return env.Orch.Delete(ctx, input)
}

// generateTestID generates a unique test ID for a test environment.
func generateTestID(t *testing.T) string {
	t.Helper()
	// Use test name and timestamp to ensure uniqueness
	return "e2e-" + time.Now().Format("20060102-150405")
}

// getProjectRoot returns the absolute path to the project root directory.
func getProjectRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	// Walk up the directory tree to find the project root (contains go.mod)
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding go.mod
			t.Fatalf("could not find project root (go.mod) from %s", cwd)
		}
		dir = parent
	}
}

// specToMap converts a TestenvSpec to a map[string]any for use in CreateInput.
func specToMap(testenvSpec *v1.TestenvSpec) (map[string]any, error) {
	// Use YAML marshal/unmarshal to convert struct to map
	// This is the same approach used in the spec package
	data, err := yaml.Marshal(testenvSpec)
	if err != nil {
		return nil, err
	}

	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return m, nil
}

// getVM retrieves VM state from the test environment by name.
func getVM(t *testing.T, env *TestEnv, name string) *providerv1.VMState {
	t.Helper()

	if env.Artifact == nil {
		t.Fatalf("artifact is nil, cannot get VM state")
	}

	// Look for VM metadata in the artifact
	// The VM state is stored in the environment state, not directly in artifact
	// For E2E tests with stub provider, we verify the VM was created through artifact metadata

	ipKey := "testenv-vm.vm." + name + ".ip"
	if ip, ok := env.Artifact.Metadata[ipKey]; ok {
		return &providerv1.VMState{
			Name:   name,
			Status: "running",
			IP:     ip,
		}
	}

	t.Fatalf("VM %s not found in artifact metadata", name)
	return nil
}

// getNetwork retrieves network state from the test environment by name.
func getNetwork(t *testing.T, env *TestEnv, name string) *providerv1.NetworkState {
	t.Helper()

	if env.Artifact == nil {
		t.Fatalf("artifact is nil, cannot get network state")
	}

	// Look for network metadata in the artifact
	ipKey := "testenv-vm.network." + name + ".ip"
	if ip, ok := env.Artifact.Metadata[ipKey]; ok {
		return &providerv1.NetworkState{
			Name:   name,
			Status: "ready",
			IP:     ip,
		}
	}

	t.Fatalf("Network %s not found in artifact metadata", name)
	return nil
}

// getKey retrieves key state from the test environment by name.
func getKey(t *testing.T, env *TestEnv, name string) *providerv1.KeyState {
	t.Helper()

	if env.Artifact == nil {
		t.Fatalf("artifact is nil, cannot get key state")
	}

	// Look for key in the artifact files
	fileKey := "testenv-vm.key." + name
	if _, ok := env.Artifact.Files[fileKey]; ok {
		return &providerv1.KeyState{
			Name: name,
		}
	}

	t.Fatalf("Key %s not found in artifact files", name)
	return nil
}

// assertArtifactNotNil asserts that the artifact is not nil.
func assertArtifactNotNil(t *testing.T, env *TestEnv) {
	t.Helper()
	if env.Artifact == nil {
		t.Fatal("expected artifact to be non-nil")
	}
}

// assertResourceCount asserts the expected count of resources in the artifact.
func assertResourceCount(t *testing.T, env *TestEnv, expectedVMs, expectedNetworks, expectedKeys int) {
	t.Helper()

	// Count VMs from metadata
	vmCount := 0
	for key := range env.Artifact.Metadata {
		if len(key) > 15 && key[:15] == "testenv-vm.vm." && key[len(key)-3:] == ".ip" {
			vmCount++
		}
	}

	// Count networks from metadata
	networkCount := 0
	for key := range env.Artifact.Metadata {
		if len(key) > 20 && key[:20] == "testenv-vm.network." && key[len(key)-3:] == ".ip" {
			networkCount++
		}
	}

	// Count keys from files
	keyCount := 0
	for key := range env.Artifact.Files {
		if len(key) > 15 && key[:15] == "testenv-vm.key." {
			keyCount++
		}
	}

	if vmCount != expectedVMs {
		t.Errorf("expected %d VMs, got %d", expectedVMs, vmCount)
	}
	if networkCount != expectedNetworks {
		t.Errorf("expected %d networks, got %d", expectedNetworks, networkCount)
	}
	if keyCount != expectedKeys {
		t.Errorf("expected %d keys, got %d", expectedKeys, keyCount)
	}
}

// assertManagedResourcesContains checks that managed resources contain expected patterns.
func assertManagedResourcesContains(t *testing.T, env *TestEnv, patterns ...string) {
	t.Helper()

	for _, pattern := range patterns {
		found := false
		for _, resource := range env.Artifact.ManagedResources {
			if containsPattern(resource, pattern) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("managed resources do not contain pattern: %s", pattern)
		}
	}
}

// containsPattern checks if a string contains a pattern (simple substring match).
func containsPattern(s, pattern string) bool {
	return len(s) >= len(pattern) && (s == pattern || contains(s, pattern))
}

// contains is a simple substring check.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// createClientFromArtifact creates a Client from the test environment artifact.
// Uses ArtifactProvider to extract VM connection info.
func createClientFromArtifact(t *testing.T, artifact *v1.TestEnvArtifact, vmName string, opts ...client.ClientOption) *client.Client {
	t.Helper()

	prov := provider.NewArtifactProvider(artifact,
		provider.WithDefaultUser("testuser"),
	)

	c, err := client.NewClient(prov, vmName, opts...)
	require.NoError(t, err, "failed to create client for VM %s", vmName)

	return c
}

// createMockSSHRunner creates a MockSSHRunner for testing without real SSH.
func createMockSSHRunner() *client.MockSSHRunner {
	return client.NewMockSSHRunner()
}

// createClientWithMockSSH creates a Client with MockSSHRunner for testing
// command formatting and client behavior without real SSH.
func createClientWithMockSSH(t *testing.T, artifact *v1.TestEnvArtifact, vmName string) (*client.Client, *client.MockSSHRunner) {
	t.Helper()

	// Ensure stub key files exist for ArtifactProvider to read
	ensureStubKeys(t, artifact)

	mockRunner := createMockSSHRunner()
	mockRunner.DefaultStdout = "ok"

	prov := provider.NewArtifactProvider(artifact,
		provider.WithDefaultUser("testuser"),
	)

	c, err := client.NewClient(prov, vmName,
		client.WithSSHRunner(mockRunner),
	)
	require.NoError(t, err, "failed to create client with mock SSH")

	return c, mockRunner
}

// ensureStubKeys ensures that stub key files referenced in the artifact exist.
// The stub provider returns paths like /tmp/stub-keys/test-key but doesn't create the files.
// The artifact contains relative paths like ../../../../stub-keys/test-key.
// This helper creates minimal valid SSH keys at the correct location.
func ensureStubKeys(t *testing.T, artifact *v1.TestEnvArtifact) {
	t.Helper()

	// Get current working directory to resolve relative paths
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for key, path := range artifact.Files {
		if !strings.HasPrefix(key, "testenv-vm.key.") {
			continue
		}

		// Resolve the path (it's relative to cwd)
		absPath := path
		if !filepath.IsAbs(path) {
			absPath = filepath.Join(cwd, path)
			absPath = filepath.Clean(absPath)
		}

		// Create directory if needed
		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create key directory %s: %v", dir, err)
		}

		// Create a minimal valid SSH private key if it doesn't exist
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			// This is a minimal valid ed25519 private key for testing
			// It's not a real key, just enough for the SSH library to parse
			stubKey := `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACBTEST1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890AAAAA
EAAAAtAAAAEdBAAAAEdBAAAA3RFU1RLRVkAAAAAAAAA1RFU1RLRVkAAAAAAAAA1RFU1R
LRVRFVVRFVURFU1RLRVkAAAA=
-----END OPENSSH PRIVATE KEY-----`
			if err := os.WriteFile(absPath, []byte(stubKey), 0600); err != nil {
				t.Fatalf("failed to write stub key to %s: %v", absPath, err)
			}
		}
	}
}
