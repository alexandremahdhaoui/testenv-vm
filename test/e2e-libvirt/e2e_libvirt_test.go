//go:build e2e_libvirt

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

// Package e2e_libvirt_test provides end-to-end tests using the libvirt provider.
// These tests create real VMs using libvirt and verify the full testenv-vm flow.
//
// Prerequisites:
// - libvirt daemon running (libvirtd)
// - qemu-img installed
// - genisoimage, mkisofs, or xorriso installed
// - User must have permission to connect to libvirt (e.g., in libvirt group)
// - wget for downloading cloud images (auto-downloaded if not cached)
package e2e_libvirt_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/client/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"gopkg.in/yaml.v3"
)

const (
	// Ubuntu 24.04 LTS cloud image
	defaultImageName = "ubuntu-24.04-server-cloudimg-amd64.img"
	defaultImageURL  = "https://cloud-images.ubuntu.com/releases/noble/release/" + defaultImageName
)

// TestEnv wraps a test environment with its resources and state.
type TestEnv struct {
	ID       string
	Spec     *v1.TestenvSpec
	Artifact *v1.TestEnvArtifact
	Orch     *orchestrator.Orchestrator
	TmpDir   string
	StateDir string
}

// checkLibvirtAvailable checks if libvirt and required tools are available.
func checkLibvirtAvailable(t *testing.T) {
	t.Helper()

	// Check virsh
	cmd := exec.Command("virsh", "--connect", "qemu:///session", "version")
	if err := cmd.Run(); err != nil {
		t.Skip("libvirt not available, skipping e2e-libvirt test")
	}

	// Check qemu-img
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available, skipping e2e-libvirt test")
	}

	// Check for ISO tool
	isoToolAvailable := false
	for _, tool := range []string{"genisoimage", "mkisofs", "xorriso"} {
		if _, err := exec.LookPath(tool); err == nil {
			isoToolAvailable = true
			break
		}
	}
	if !isoToolAvailable {
		t.Skip("no ISO generation tool available, skipping e2e-libvirt test")
	}

	// Check wget for image download
	if _, err := exec.LookPath("wget"); err != nil {
		t.Skip("wget not available for image download, skipping e2e-libvirt test")
	}
}

// ensureBaseImage ensures the base cloud image is available.
func ensureBaseImage(t *testing.T) string {
	t.Helper()

	cacheDir := os.Getenv("TESTENV_VM_IMAGE_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "/tmp/testenv-vm-images"
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("failed to create image cache directory: %v", err)
	}

	// Set permissions for libvirt access
	prepareLibvirtDir(t, cacheDir)

	imagePath := filepath.Join(cacheDir, defaultImageName)

	if _, err := os.Stat(imagePath); err == nil {
		t.Logf("Using cached base image: %s", imagePath)
		return imagePath
	}

	t.Logf("Downloading base image from %s (this may take a few minutes)...", defaultImageURL)

	cmd := exec.Command("wget", "--progress=dot:giga", "-O", imagePath, defaultImageURL)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Remove(imagePath)
		t.Fatalf("failed to download base image: %v", err)
	}

	t.Logf("Successfully downloaded base image: %s", imagePath)
	return imagePath
}

// prepareLibvirtDir sets permissions for libvirt access.
func prepareLibvirtDir(t *testing.T, dir string) {
	t.Helper()

	os.Chmod(dir, 0o755)

	// Try to set ACLs for libvirt groups
	setfaclPath, err := exec.LookPath("setfacl")
	if err != nil {
		return
	}

	for _, group := range []string{"libvirt", "libvirt-qemu", "kvm", "qemu"} {
		checkCmd := exec.Command("getent", "group", group)
		if err := checkCmd.Run(); err != nil {
			continue
		}
		exec.Command(setfaclPath, "-m", "g:"+group+":rwx", dir).Run()
		exec.Command(setfaclPath, "-d", "-m", "g:"+group+":rwx", dir).Run()
	}
}

// TestLibvirtVMLifecycle tests the full VM lifecycle using the libvirt provider
// through the testenv-vm orchestrator.
func TestLibvirtVMLifecycle(t *testing.T) {
	checkLibvirtAvailable(t)

	// Ensure base image is available
	baseImage := ensureBaseImage(t)

	// Load scenario
	scenarioPath := filepath.Join(getProjectRoot(t), "test", "e2e", "scenarios", "libvirt_vm.yaml")
	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatalf("failed to read scenario: %v", err)
	}

	testenvSpec, err := spec.Parse(data)
	if err != nil {
		t.Fatalf("failed to parse scenario: %v", err)
	}

	// Fix provider paths
	projectRoot := getProjectRoot(t)
	for i := range testenvSpec.Providers {
		engine := testenvSpec.Providers[i].Engine
		if len(engine) > 2 && engine[:2] == "./" {
			testenvSpec.Providers[i].Engine = filepath.Join(projectRoot, engine[2:])
		}
	}

	// Create temp directories with libvirt-accessible permissions
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	artifactDir := filepath.Join(tmpDir, "artifacts")

	os.MkdirAll(stateDir, 0o755)
	os.MkdirAll(artifactDir, 0o755)
	prepareLibvirtDir(t, tmpDir)
	prepareLibvirtDir(t, stateDir)
	prepareLibvirtDir(t, artifactDir)

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
	specMap, err := specToMap(testenvSpec)
	if err != nil {
		t.Fatalf("failed to convert spec: %v", err)
	}

	// Create test environment
	testID := "e2e-libvirt-" + time.Now().Format("20060102-150405")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	input := &v1.CreateInput{
		TestID: testID,
		Stage:  "e2e-libvirt",
		TmpDir: artifactDir,
		Spec:   specMap,
		Env: map[string]string{
			"TESTENV_VM_BASE_IMAGE": baseImage,
		},
		RootDir:  projectRoot,
		Metadata: map[string]string{},
	}

	t.Log("Creating test environment...")
	result, err := orch.Create(ctx, input)
	if err != nil {
		t.Fatalf("failed to create test environment: %v", err)
	}
	artifact := result.Artifact

	// Store for cleanup test
	envStateFile := filepath.Join(tmpDir, "env_state.yaml")
	stateData, _ := yaml.Marshal(map[string]any{
		"testID":           testID,
		"artifact":         artifact,
		"managedResources": artifact.ManagedResources,
	})
	os.WriteFile(envStateFile, stateData, 0o644)

	t.Log("Verifying resources were created...")

	// Verify VM was created
	vmIPKey := "testenv-vm.vm.e2e-libvirt-vm.ip"
	vmIP, ok := artifact.Metadata[vmIPKey]
	if !ok {
		t.Logf("VM IP not found in metadata (VM may still be booting)")
	} else {
		t.Logf("VM IP: %s", vmIP)
	}

	// Verify network was created
	networkIPKey := "testenv-vm.network.e2e-libvirt-network.ip"
	if _, ok := artifact.Metadata[networkIPKey]; !ok {
		t.Error("Network IP not found in metadata")
	}

	// Verify key was created
	keyFileKey := "testenv-vm.key.e2e-libvirt-key"
	keyPath, ok := artifact.Files[keyFileKey]
	if !ok {
		t.Error("Key file not found in artifact")
	} else {
		t.Logf("Key path: %s", keyPath)
	}

	// Verify SSH command is available
	sshEnvKey := "TESTENV_VM_E2E_LIBVIRT_VM_SSH"
	sshCmd, ok := artifact.Env[sshEnvKey]
	if !ok {
		t.Log("SSH command not in env (VM may not have IP yet)")
	} else {
		t.Logf("SSH command: %s", sshCmd)
	}

	// Verify managed resources
	expectedResources := []string{
		"testenv-vm://key/e2e-libvirt-key",
		"testenv-vm://network/e2e-libvirt-network",
		"testenv-vm://vm/e2e-libvirt-vm",
	}
	for _, expected := range expectedResources {
		found := false
		for _, managed := range artifact.ManagedResources {
			if strings.Contains(managed, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected managed resource %s not found", expected)
		}
	}

	t.Log("Test environment created successfully!")

	// Test SSH connectivity using LibvirtProvider
	// Note: We use LibvirtProvider instead of ArtifactProvider because the
	// orchestrator's IP resolution may timeout before the VM gets a DHCP lease.
	// LibvirtProvider queries libvirt directly and will retry until IP is available.
	t.Log("Testing SSH connectivity (using LibvirtProvider for IP resolution)...")
	testSSHConnectivity(t, artifact, "e2e-libvirt-vm")

	// Cleanup
	t.Log("Deleting test environment...")
	deleteInput := &v1.DeleteInput{
		TestID:           testID,
		Metadata:         artifact.Metadata,
		ManagedResources: artifact.ManagedResources,
	}

	deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer deleteCancel()

	if err := orch.Delete(deleteCtx, deleteInput); err != nil {
		t.Errorf("failed to delete test environment: %v", err)
	}

	t.Log("Test environment deleted successfully!")
}

// testSSHConnectivity tests SSH connection using the client library.
// Uses LibvirtProvider to query libvirt directly for VM IP (more reliable
// than ArtifactProvider when IP isn't available in metadata yet).
func testSSHConnectivity(t *testing.T, artifact *v1.TestEnvArtifact, vmName string) {
	t.Helper()

	// Get key path from artifact
	keyPath := ""
	for k, v := range artifact.Files {
		if strings.Contains(k, "key") {
			keyPath = v
			break
		}
	}
	if keyPath == "" {
		t.Fatal("no SSH key found in artifact files")
	}

	// Make keyPath absolute if it's relative
	if !filepath.IsAbs(keyPath) {
		// The key path might be relative to tmpDir or a global state dir
		// Check common locations for the key
		possiblePaths := []string{
			filepath.Join(getProjectRoot(t), keyPath),
			"/tmp/testenv-vm-1000/keys/e2e-libvirt-key",
		}
		for _, p := range possiblePaths {
			if _, err := os.Stat(p); err == nil {
				keyPath = p
				break
			}
		}
	}

	// Use LibvirtProvider which queries libvirt directly for the IP
	// This is more reliable than ArtifactProvider when the orchestrator
	// didn't get the IP during initial creation (VM still booting)
	// Note: Use qemu:///system because go-libvirt creates VMs in system mode
	// even when using session URI (due to library behavior with polkit)
	prov := provider.NewLibvirtProvider(
		provider.WithUser("testuser"),
		provider.WithKeyPath(keyPath),
		provider.WithConnectionURI("qemu:///system"),
	)
	c, err := client.NewClient(prov, vmName)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer c.Close()

	// Wait for VM to be ready (SSH + cloud-init)
	// This replaces the hard-coded 60s sleep
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Log("Waiting for VM to be ready (SSH + cloud-init)...")
	err = c.WaitReady(ctx, 2*time.Minute)
	if err != nil {
		t.Fatalf("VM failed to become ready: %v", err)
	}

	// Test SSH connectivity
	stdout, stderr, err := c.Run(ctx, "echo", "SSH connection successful")
	if err != nil {
		t.Fatalf("SSH command failed: %v, stderr: %s", err, stderr)
	}

	if strings.Contains(stdout, "SSH connection successful") {
		t.Log("SSH connection successful!")
	}

	// Test hostname command
	stdout, _, err = c.Run(ctx, "hostname")
	if err != nil {
		t.Errorf("hostname command failed: %v", err)
	} else {
		t.Logf("VM hostname: %s", strings.TrimSpace(stdout))
	}
}

func getProjectRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root from %s", cwd)
		}
		dir = parent
	}
}

func specToMap(s *v1.TestenvSpec) (map[string]any, error) {
	data, err := yaml.Marshal(s)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
