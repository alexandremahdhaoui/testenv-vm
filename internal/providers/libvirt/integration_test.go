//go:build integration

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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

const (
	defaultImageName = "ubuntu-24.04-server-cloudimg-amd64.img"
	defaultImageURL  = "https://cloud-images.ubuntu.com/releases/noble/release/" + defaultImageName
)

func integrationTestProvider(t *testing.T) (*Provider, func()) {
	t.Helper()

	if !checkLibvirtAvailable(t) {
		t.Skip("libvirt not available, skipping integration test")
	}

	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available, skipping integration test")
	}

	isoToolAvailable := false
	for _, tool := range []string{"genisoimage", "mkisofs", "xorriso"} {
		if _, err := exec.LookPath(tool); err == nil {
			isoToolAvailable = true
			break
		}
	}
	if !isoToolAvailable {
		t.Skip("no ISO generation tool available (genisoimage/mkisofs/xorriso), skipping integration test")
	}

	tmpDir, err := os.MkdirTemp("", "libvirt-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}

	prepareLibvirtDir(t, tmpDir)

	disksDir := filepath.Join(tmpDir, "disks")
	if err := os.MkdirAll(disksDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create disks directory: %v", err)
	}
	prepareLibvirtDir(t, disksDir)

	cloudinitDir := filepath.Join(tmpDir, "cloudinit")
	if err := os.MkdirAll(cloudinitDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create cloudinit directory: %v", err)
	}
	prepareLibvirtDir(t, cloudinitDir)

	uri := os.Getenv("TESTENV_VM_LIBVIRT_URI")
	if uri == "" {
		uri = "qemu:///system"
	}

	os.Setenv("TESTENV_VM_LIBVIRT_URI", uri)
	os.Setenv("TESTENV_VM_STATE_DIR", tmpDir)

	provider, err := NewProvider()
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Skipf("failed to create provider (libvirt may not be running): %v", err)
	}

	cleanup := func() {
		provider.Close()
		os.RemoveAll(tmpDir)
	}

	return provider, cleanup
}

func consoleOutputFor(provider *Provider, vmName string) string {
	return filepath.Join(provider.config.StateDir, "console", vmName+".log")
}

func checkLibvirtAvailable(t *testing.T) bool {
	t.Helper()

	cmd := exec.Command("virsh", "--connect", "qemu:///system", "version")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func prepareLibvirtDir(t *testing.T, dir string) {
	t.Helper()

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Logf("Warning: failed to chmod directory %s: %v", dir, err)
	}

	libvirtGroups := []string{"libvirt", "libvirt-qemu", "kvm", "qemu"}
	setfaclPath, err := exec.LookPath("setfacl")
	if err != nil {
		return
	}

	for _, group := range libvirtGroups {
		checkCmd := exec.Command("getent", "group", group)
		if err := checkCmd.Run(); err != nil {
			continue
		}

		aclCmd := exec.Command(setfaclPath, "-m", "g:"+group+":rwx", dir)
		if err := aclCmd.Run(); err != nil {
			t.Logf("Warning: failed to set ACL for group %s: %v", group, err)
			continue
		}

		defaultAclCmd := exec.Command(setfaclPath, "-d", "-m", "g:"+group+":rwx", dir)
		if err := defaultAclCmd.Run(); err != nil {
			t.Logf("Warning: failed to set default ACL for group %s: %v", group, err)
		}

		t.Logf("Set ACL on %s for group %s", dir, group)
	}
}

func ensureBaseImage(t *testing.T) string {
	t.Helper()

	cacheDir := os.Getenv("TESTENV_VM_IMAGE_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "/tmp/testenv-vm-images"
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("failed to create image cache directory: %v", err)
	}

	prepareLibvirtDir(t, cacheDir)

	imagePath := filepath.Join(cacheDir, defaultImageName)

	if _, err := os.Stat(imagePath); err == nil {
		t.Logf("Using cached base image: %s", imagePath)
		return imagePath
	}

	if _, err := exec.LookPath("wget"); err != nil {
		t.Fatalf("wget not found, cannot download base image")
	}

	t.Logf("Downloading base image from %s to %s (this may take a few minutes)...", defaultImageURL, imagePath)

	cmd := exec.Command("wget",
		"--progress=dot:giga",
		"-O", imagePath,
		defaultImageURL,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Remove(imagePath)
		t.Fatalf("failed to download base image: %v", err)
	}

	t.Logf("Successfully downloaded base image: %s", imagePath)
	return imagePath
}

func TestIntegration_KeyLifecycle(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	t.Run("CreateED25519Key", func(t *testing.T) {
		req := &providerv1.KeyCreateRequest{
			Name: "integration-test-ed25519",
			Spec: providerv1.KeySpec{
				Type: "ed25519",
			},
		}

		result := provider.KeyCreate(req)
		if !result.Success {
			t.Fatalf("KeyCreate failed: %v", result.Error)
		}

		keyState := result.Resource.(*providerv1.KeyState)

		if keyState.Name != "integration-test-ed25519" {
			t.Errorf("expected name 'integration-test-ed25519', got %s", keyState.Name)
		}
		if keyState.Type != "ed25519" {
			t.Errorf("expected type 'ed25519', got %s", keyState.Type)
		}
		if !strings.HasPrefix(keyState.PublicKey, "ssh-ed25519") {
			t.Errorf("expected public key to start with 'ssh-ed25519', got %s", keyState.PublicKey[:20])
		}
		if !strings.HasPrefix(keyState.Fingerprint, "SHA256:") {
			t.Errorf("expected fingerprint to start with 'SHA256:', got %s", keyState.Fingerprint)
		}

		if _, err := os.Stat(keyState.PrivateKeyPath); err != nil {
			t.Errorf("private key file should exist: %v", err)
		}
		if _, err := os.Stat(keyState.PublicKeyPath); err != nil {
			t.Errorf("public key file should exist: %v", err)
		}
	})

	t.Run("CreateRSAKey", func(t *testing.T) {
		req := &providerv1.KeyCreateRequest{
			Name: "integration-test-rsa",
			Spec: providerv1.KeySpec{
				Type: "rsa",
				Bits: 2048,
			},
		}

		result := provider.KeyCreate(req)
		if !result.Success {
			t.Fatalf("KeyCreate failed: %v", result.Error)
		}

		keyState := result.Resource.(*providerv1.KeyState)
		if keyState.Type != "rsa" {
			t.Errorf("expected type 'rsa', got %s", keyState.Type)
		}
		if !strings.HasPrefix(keyState.PublicKey, "ssh-rsa") {
			t.Errorf("expected public key to start with 'ssh-rsa'")
		}
	})

	t.Run("ListKeys", func(t *testing.T) {
		result := provider.KeyList(nil)
		if !result.Success {
			t.Fatalf("KeyList failed: %v", result.Error)
		}

		keyList := result.Resource.([]*providerv1.KeyState)
		if len(keyList) != 2 {
			t.Errorf("expected 2 keys, got %d", len(keyList))
		}
	})

	t.Run("GetKey", func(t *testing.T) {
		result := provider.KeyGet("integration-test-ed25519")
		if !result.Success {
			t.Fatalf("KeyGet failed: %v", result.Error)
		}

		keyState := result.Resource.(*providerv1.KeyState)
		if keyState.Name != "integration-test-ed25519" {
			t.Errorf("expected name 'integration-test-ed25519', got %s", keyState.Name)
		}
	})

	t.Run("DeleteKeys", func(t *testing.T) {
		result := provider.KeyDelete("integration-test-ed25519")
		if !result.Success {
			t.Fatalf("KeyDelete failed: %v", result.Error)
		}

		getResult := provider.KeyGet("integration-test-ed25519")
		if getResult.Success {
			t.Error("key should be deleted")
		}

		result = provider.KeyDelete("integration-test-rsa")
		if !result.Success {
			t.Fatalf("KeyDelete failed: %v", result.Error)
		}

		listResult := provider.KeyList(nil)
		if !listResult.Success {
			t.Fatalf("KeyList failed: %v", listResult.Error)
		}
		keyList := listResult.Resource.([]*providerv1.KeyState)
		if len(keyList) != 0 {
			t.Errorf("expected 0 keys after deletion, got %d", len(keyList))
		}
	})
}

func TestIntegration_NetworkLifecycle(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	networkName := "integration-test-net"

	_ = provider.NetworkDelete(networkName)

	t.Run("CreateNATNetwork", func(t *testing.T) {
		req := &providerv1.NetworkCreateRequest{
			Name: networkName,
			Kind: "nat",
			Spec: providerv1.NetworkSpec{
				CIDR: "192.168.222.0/24",
			},
		}

		result := provider.NetworkCreate(req)
		if !result.Success {
			t.Fatalf("NetworkCreate failed: %v", result.Error)
		}

		networkState := result.Resource.(*providerv1.NetworkState)

		if networkState.Name != networkName {
			t.Errorf("expected name '%s', got %s", networkName, networkState.Name)
		}
		if networkState.Kind != "nat" {
			t.Errorf("expected kind 'nat', got %s", networkState.Kind)
		}
		if networkState.Status != "active" {
			t.Errorf("expected status 'active', got %s", networkState.Status)
		}
		if networkState.IP == "" {
			t.Error("expected network to have an IP address")
		}
		if networkState.CIDR != "192.168.222.0/24" {
			t.Errorf("expected CIDR '192.168.222.0/24', got %s", networkState.CIDR)
		}
		if networkState.UUID == "" {
			t.Error("expected network to have a UUID")
		}
	})

	t.Run("GetNetwork", func(t *testing.T) {
		result := provider.NetworkGet(networkName)
		if !result.Success {
			t.Fatalf("NetworkGet failed: %v", result.Error)
		}

		networkState := result.Resource.(*providerv1.NetworkState)
		if networkState.Name != networkName {
			t.Errorf("expected name '%s', got %s", networkName, networkState.Name)
		}
	})

	t.Run("ListNetworks", func(t *testing.T) {
		result := provider.NetworkList(nil)
		if !result.Success {
			t.Fatalf("NetworkList failed: %v", result.Error)
		}

		networkList := result.Resource.([]*providerv1.NetworkState)
		found := false
		for _, net := range networkList {
			if net.Name == networkName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("network '%s' not found in list", networkName)
		}
	})

	t.Run("DeleteNetwork", func(t *testing.T) {
		result := provider.NetworkDelete(networkName)
		if !result.Success {
			t.Fatalf("NetworkDelete failed: %v", result.Error)
		}

		getResult := provider.NetworkGet(networkName)
		if getResult.Success {
			t.Error("network should be deleted")
		}
	})
}

func TestIntegration_IsolatedNetwork(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	networkName := "integration-test-isolated"

	_ = provider.NetworkDelete(networkName)

	req := &providerv1.NetworkCreateRequest{
		Name: networkName,
		Kind: "isolated",
		Spec: providerv1.NetworkSpec{
			CIDR: "10.99.0.0/24",
		},
	}

	result := provider.NetworkCreate(req)
	if !result.Success {
		t.Fatalf("NetworkCreate failed: %v", result.Error)
	}

	networkState := result.Resource.(*providerv1.NetworkState)

	if networkState.Kind != "isolated" {
		t.Errorf("expected kind 'isolated', got %s", networkState.Kind)
	}

	provider.NetworkDelete(networkName)
}

func TestIntegration_VMLifecycle(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	baseImage := ensureBaseImage(t)

	vmName := "integration-test-vm"
	networkName := "integration-test-vm-net"
	keyName := "integration-test-vm-key"

	_ = provider.VMDelete(vmName)
	_ = provider.NetworkDelete(networkName)
	_ = provider.KeyDelete(keyName)

	t.Run("CreateKey", func(t *testing.T) {
		req := &providerv1.KeyCreateRequest{
			Name: keyName,
			Spec: providerv1.KeySpec{
				Type: "ed25519",
			},
		}
		result := provider.KeyCreate(req)
		if !result.Success {
			t.Fatalf("KeyCreate failed: %v", result.Error)
		}
	})

	t.Run("CreateNetwork", func(t *testing.T) {
		req := &providerv1.NetworkCreateRequest{
			Name: networkName,
			Kind: "nat",
			Spec: providerv1.NetworkSpec{
				CIDR: "192.168.223.0/24",
			},
		}
		result := provider.NetworkCreate(req)
		if !result.Success {
			t.Fatalf("NetworkCreate failed: %v", result.Error)
		}
	})

	keyResult := provider.KeyGet(keyName)
	if !keyResult.Success {
		t.Fatalf("KeyGet failed: %v", keyResult.Error)
	}
	keyState := keyResult.Resource.(*providerv1.KeyState)

	t.Run("AVMWithNoConsoleOutputPathIsRefusedByName", func(t *testing.T) {
		result := provider.VMCreate(&providerv1.VMCreateRequest{
			Name: vmName + "-no-console",
			Spec: providerv1.VMSpec{
				Network: networkName,
				Disk:    providerv1.DiskSpec{BaseImage: baseImage, Size: "5G"},
			},
		})
		if result.Success {
			t.Fatal("VMCreate succeeded with no console output path, want a refusal")
		}
		if result.Error == nil || result.Error.Code != providerv1.ErrCodeInvalidSpec || !strings.Contains(result.Error.Message, "has no console output path") {
			t.Fatalf("VMCreate error = %+v, want an invalid spec naming the missing console output path", result.Error)
		}
	})

	consoleOutput := consoleOutputFor(provider, vmName)

	t.Run("CreateVM", func(t *testing.T) {
		req := &providerv1.VMCreateRequest{
			Name: vmName,
			Spec: providerv1.VMSpec{
				Memory:  512,
				VCPUs:   1,
				Network: networkName,
				Disk: providerv1.DiskSpec{
					BaseImage: baseImage,
					Size:      "5G",
				},
				CloudInit: &providerv1.CloudInitSpec{
					Hostname: vmName,
					Users: []providerv1.UserSpec{
						{
							Name: "testuser",
							Sudo: "ALL=(ALL) NOPASSWD:ALL",
							SSHAuthorizedKeys: []string{
								strings.TrimSpace(keyState.PublicKey),
							},
						},
					},
				},
			},
			ConsoleOutput: consoleOutput,
		}

		result := provider.VMCreate(req)
		if !result.Success {
			t.Fatalf("VMCreate failed: %v", result.Error)
		}

		vmState := result.Resource.(*providerv1.VMState)

		if vmState.Name != vmName {
			t.Errorf("expected name '%s', got %s", vmName, vmState.Name)
		}
		if vmState.Status != "running" {
			t.Errorf("expected status 'running', got %s", vmState.Status)
		}
		if vmState.UUID == "" {
			t.Error("expected VM to have a UUID")
		}
		if vmState.MAC == "" {
			t.Error("expected VM to have a MAC address")
		}

		t.Logf("VM created: name=%s, uuid=%s, mac=%s, ip=%s",
			vmState.Name, vmState.UUID, vmState.MAC, vmState.IP)

		if vmState.SSHCommand != "" {
			t.Logf("SSH command: %s", vmState.SSHCommand)
		}
	})

	t.Run("TheSerialConsoleOfTheRunningVMLogsToTheRequestedFile", func(t *testing.T) {
		result := provider.VMGet(vmName)
		if !result.Success {
			t.Fatalf("VMGet failed: %v", result.Error)
		}
		if got := result.Resource.(*providerv1.VMState).ConsoleOutput; got != consoleOutput {
			t.Errorf("VMState.ConsoleOutput = %q, want %q", got, consoleOutput)
		}
		dom, err := provider.conn.DomainLookupByName(vmName)
		if err != nil {
			t.Fatalf("looking up domain %s: %v", vmName, err)
		}
		liveXML, err := provider.conn.DomainGetXMLDesc(dom, 0)
		if err != nil {
			t.Fatalf("reading the live XML of domain %s: %v", vmName, err)
		}
		if want := "<log file='" + consoleOutput + "'"; !strings.Contains(liveXML, want) {
			t.Errorf("live domain XML should contain %q\n%s", want, liveXML)
		}
		data, err := os.ReadFile(consoleOutput)
		if err != nil {
			t.Fatalf("reading console output file %s: %v", consoleOutput, err)
		}
		if len(data) == 0 {
			t.Errorf("console output file %s is empty after boot", consoleOutput)
		}
	})

	t.Run("GetVM", func(t *testing.T) {
		result := provider.VMGet(vmName)
		if !result.Success {
			t.Fatalf("VMGet failed: %v", result.Error)
		}

		vmState := result.Resource.(*providerv1.VMState)
		if vmState.Name != vmName {
			t.Errorf("expected name '%s', got %s", vmName, vmState.Name)
		}
	})

	t.Run("ListVMs", func(t *testing.T) {
		result := provider.VMList(nil)
		if !result.Success {
			t.Fatalf("VMList failed: %v", result.Error)
		}

		vmList := result.Resource.([]*providerv1.VMState)
		found := false
		for _, vm := range vmList {
			if vm.Name == vmName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("VM '%s' not found in list", vmName)
		}
	})

	time.Sleep(2 * time.Second)

	t.Run("DeleteVM", func(t *testing.T) {
		result := provider.VMDelete(vmName)
		if !result.Success {
			t.Fatalf("VMDelete failed: %v", result.Error)
		}

		getResult := provider.VMGet(vmName)
		if getResult.Success {
			t.Error("VM should be deleted")
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		provider.NetworkDelete(networkName)
		provider.KeyDelete(keyName)
	})
}

func TestIntegration_NetworkDeletionBlockedByVM(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	baseImage := ensureBaseImage(t)

	vmName := "integration-block-test-vm"
	networkName := "integration-block-test-net"

	_ = provider.VMDelete(vmName)
	_ = provider.NetworkDelete(networkName)

	netReq := &providerv1.NetworkCreateRequest{
		Name: networkName,
		Kind: "nat",
		Spec: providerv1.NetworkSpec{
			CIDR: "192.168.224.0/24",
		},
	}
	netResult := provider.NetworkCreate(netReq)
	if !netResult.Success {
		t.Fatalf("NetworkCreate failed: %v", netResult.Error)
	}

	vmReq := &providerv1.VMCreateRequest{
		Name: vmName,
		Spec: providerv1.VMSpec{
			Memory:  256,
			VCPUs:   1,
			Network: networkName,
			Disk: providerv1.DiskSpec{
				BaseImage: baseImage,
				Size:      "2G",
			},
		},
		ConsoleOutput: consoleOutputFor(provider, vmName),
	}
	vmResult := provider.VMCreate(vmReq)
	if !vmResult.Success {
		provider.NetworkDelete(networkName)
		t.Fatalf("VMCreate failed: %v", vmResult.Error)
	}

	deleteResult := provider.NetworkDelete(networkName)
	if deleteResult.Success {
		t.Error("expected network deletion to fail while VM is using it")
	}

	if deleteResult.Error == nil || deleteResult.Error.Code != providerv1.ErrCodeResourceBusy {
		t.Errorf("expected ErrCodeResourceBusy, got: %v", deleteResult.Error)
	}

	provider.VMDelete(vmName)
	provider.NetworkDelete(networkName)
}

func TestIntegration_KeyDeletionBlockedByVM(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	baseImage := ensureBaseImage(t)

	vmName := "integration-key-block-vm"
	networkName := "integration-key-block-net"
	keyName := "integration-key-block-key"

	_ = provider.VMDelete(vmName)
	_ = provider.NetworkDelete(networkName)
	_ = provider.KeyDelete(keyName)

	keyReq := &providerv1.KeyCreateRequest{
		Name: keyName,
		Spec: providerv1.KeySpec{Type: "ed25519"},
	}
	keyResult := provider.KeyCreate(keyReq)
	if !keyResult.Success {
		t.Fatalf("KeyCreate failed: %v", keyResult.Error)
	}
	keyState := keyResult.Resource.(*providerv1.KeyState)

	netReq := &providerv1.NetworkCreateRequest{
		Name: networkName,
		Kind: "nat",
		Spec: providerv1.NetworkSpec{
			CIDR: "192.168.225.0/24",
		},
	}
	netResult := provider.NetworkCreate(netReq)
	if !netResult.Success {
		provider.KeyDelete(keyName)
		t.Fatalf("NetworkCreate failed: %v", netResult.Error)
	}

	vmReq := &providerv1.VMCreateRequest{
		Name: vmName,
		Spec: providerv1.VMSpec{
			Memory:  256,
			VCPUs:   1,
			Network: networkName,
			Disk: providerv1.DiskSpec{
				BaseImage: baseImage,
				Size:      "2G",
			},
			CloudInit: &providerv1.CloudInitSpec{
				Users: []providerv1.UserSpec{
					{
						Name: "testuser",
						SSHAuthorizedKeys: []string{
							strings.TrimSpace(keyState.PublicKey),
						},
					},
				},
			},
		},
		ConsoleOutput: consoleOutputFor(provider, vmName),
	}
	vmResult := provider.VMCreate(vmReq)
	if !vmResult.Success {
		provider.NetworkDelete(networkName)
		provider.KeyDelete(keyName)
		t.Fatalf("VMCreate failed: %v", vmResult.Error)
	}

	deleteResult := provider.KeyDelete(keyName)
	if deleteResult.Success {
		t.Error("expected key deletion to fail while VM is using it")
	}

	if deleteResult.Error == nil || deleteResult.Error.Code != providerv1.ErrCodeResourceBusy {
		t.Errorf("expected ErrCodeResourceBusy, got: %v", deleteResult.Error)
	}

	provider.VMDelete(vmName)
	provider.NetworkDelete(networkName)
	provider.KeyDelete(keyName)
}

func TestIntegration_ProviderCapabilities(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	caps := provider.Capabilities()

	if caps.ProviderName != "libvirt" {
		t.Errorf("expected provider name 'libvirt', got %s", caps.ProviderName)
	}

	if caps.Version == "" {
		t.Error("expected version to be set")
	}

	resourceKinds := make(map[string]bool)
	for _, res := range caps.Resources {
		resourceKinds[res.Kind] = true
	}

	expectedKinds := []string{"key", "network", "vm"}
	for _, kind := range expectedKinds {
		if !resourceKinds[kind] {
			t.Errorf("expected resource kind '%s' to be supported", kind)
		}
	}
}

func sshExecWithKey(t *testing.T, privateKeyPath, user, host string, command string) (string, string, error) {
	t.Helper()

	cmd := exec.Command("ssh",
		"-i", privateKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		user+"@"+host,
		command,
	)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func waitForCloudInitSSH(t *testing.T, privateKeyPath, user, host string, timeout time.Duration) error {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stdout, _, err := sshExecWithKey(t, privateKeyPath, user, host, "test -f /var/lib/cloud/instance/boot-finished && echo 'ready'")
		if err == nil && strings.TrimSpace(stdout) == "ready" {
			t.Log("Cloud-init completed successfully")
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return exec.Command("false").Run()
}

func TestIntegration_CloudInitVerification(t *testing.T) {
	provider, cleanup := integrationTestProvider(t)
	defer cleanup()

	baseImage := ensureBaseImage(t)

	vmName := "cloudinit-verify-vm"
	networkName := "cloudinit-verify-net"
	keyName := "cloudinit-verify-key"
	customHostname := "my-custom-hostname"

	_ = provider.VMDelete(vmName)
	_ = provider.NetworkDelete(networkName)
	_ = provider.KeyDelete(keyName)

	keyReq := &providerv1.KeyCreateRequest{
		Name: keyName,
		Spec: providerv1.KeySpec{
			Type: "ed25519",
		},
	}
	keyResult := provider.KeyCreate(keyReq)
	if !keyResult.Success {
		t.Fatalf("KeyCreate failed: %v", keyResult.Error)
	}
	keyState := keyResult.Resource.(*providerv1.KeyState)
	t.Logf("Created key: %s, private key path: %s", keyName, keyState.PrivateKeyPath)
	privateKeyPath := keyState.PrivateKeyPath

	netReq := &providerv1.NetworkCreateRequest{
		Name: networkName,
		Kind: "nat",
		Spec: providerv1.NetworkSpec{
			CIDR: "192.168.231.0/24",
		},
	}
	netResult := provider.NetworkCreate(netReq)
	if !netResult.Success {
		provider.KeyDelete(keyName)
		t.Fatalf("NetworkCreate failed: %v", netResult.Error)
	}
	t.Logf("Created network: %s", networkName)

	vmReq := &providerv1.VMCreateRequest{
		Name: vmName,
		Spec: providerv1.VMSpec{
			Memory:  512,
			VCPUs:   1,
			Network: networkName,
			Disk: providerv1.DiskSpec{
				BaseImage: baseImage,
				Size:      "5G",
			},
			CloudInit: &providerv1.CloudInitSpec{
				Hostname: customHostname,
				Users: []providerv1.UserSpec{
					{
						Name:  "testadmin",
						Sudo:  "ALL=(ALL) NOPASSWD:ALL",
						Shell: "/bin/bash",
						SSHAuthorizedKeys: []string{
							strings.TrimSpace(keyState.PublicKey),
						},
					},
					{
						Name:  "testuser",
						Shell: "/bin/sh",
					},
				},
				Packages: []string{"curl"},
				WriteFiles: []providerv1.WriteFileSpec{
					{
						Path:        "/tmp/cloudinit-test-file.txt",
						Content:     "Hello from cloud-init!\nThis is line 2.",
						Permissions: "0644",
					},
					{
						Path:        "/tmp/cloudinit-script.sh",
						Content:     "#!/bin/bash\necho 'script works'",
						Permissions: "0755",
					},
				},
				Runcmd: []string{
					"touch /tmp/runcmd-marker-file",
					"echo 'runcmd executed' > /tmp/runcmd-output.txt",
				},
			},
		},
		ConsoleOutput: consoleOutputFor(provider, vmName),
	}

	vmResult := provider.VMCreate(vmReq)
	if !vmResult.Success {
		provider.NetworkDelete(networkName)
		provider.KeyDelete(keyName)
		t.Fatalf("VMCreate failed: %v", vmResult.Error)
	}
	vmState := vmResult.Resource.(*providerv1.VMState)
	t.Logf("Created VM: %s, IP: %s", vmName, vmState.IP)

	defer func() {
		provider.VMDelete(vmName)
		provider.NetworkDelete(networkName)
		provider.KeyDelete(keyName)
	}()

	getResult := provider.VMGet(vmName)
	if !getResult.Success {
		t.Fatalf("VMGet failed: %v", getResult.Error)
	}
	vm := getResult.Resource.(*providerv1.VMState)
	vmMAC := vm.MAC
	t.Logf("VM MAC address: %s", vmMAC)

	var vmIP string
	t.Log("Waiting for VM to get an IP address via DHCP...")
	for i := 0; i < 90; i++ {
		cmd := exec.Command("virsh", "--connect", "qemu:///system", "net-dhcp-leases", networkName)
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), strings.ToLower(vmMAC)) {
					fields := strings.Fields(line)
					for _, field := range fields {
						if strings.Contains(field, "/") && strings.Contains(field, ".") {
							vmIP = strings.Split(field, "/")[0]
							t.Logf("VM got IP: %s (after %d seconds)", vmIP, i*2)
							break
						}
					}
				}
				if vmIP != "" {
					break
				}
			}
		}
		if vmIP != "" {
			break
		}
		if i > 0 && i%15 == 0 {
			t.Logf("Still waiting for IP... (%d seconds)", i*2)
		}
		time.Sleep(2 * time.Second)
	}
	if vmIP == "" {
		t.Fatal("VM did not get an IP address within timeout (3 minutes)")
	}

	t.Log("Waiting for cloud-init to complete...")
	if err := waitForCloudInitSSH(t, privateKeyPath, "testadmin", vmIP, 5*time.Minute); err != nil {
		t.Fatalf("Cloud-init did not complete within timeout")
	}

	t.Run("VerifyHostname", func(t *testing.T) {
		stdout, _, err := sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "hostname")
		if err != nil {
			t.Fatalf("Failed to get hostname: %v", err)
		}
		hostname := strings.TrimSpace(stdout)
		if hostname != customHostname {
			t.Errorf("Expected hostname %q, got %q", customHostname, hostname)
		}
		t.Logf("Hostname verified: %s", hostname)
	})

	t.Run("VerifyWriteFiles", func(t *testing.T) {
		stdout, _, err := sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "cat /tmp/cloudinit-test-file.txt")
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}
		if !strings.Contains(stdout, "Hello from cloud-init!") {
			t.Errorf("Expected file to contain 'Hello from cloud-init!', got: %s", stdout)
		}
		if !strings.Contains(stdout, "This is line 2.") {
			t.Errorf("Expected file to contain 'This is line 2.', got: %s", stdout)
		}
		t.Log("write_files content verified")

		stdout, _, err = sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "stat -c %a /tmp/cloudinit-script.sh")
		if err != nil {
			t.Fatalf("Failed to check script permissions: %v", err)
		}
		perms := strings.TrimSpace(stdout)
		if perms != "755" {
			t.Errorf("Expected script permissions 755, got %s", perms)
		}
		t.Logf("write_files permissions verified: %s", perms)
	})

	t.Run("VerifyUsers", func(t *testing.T) {
		stdout, _, err := sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "id testadmin")
		if err != nil {
			t.Fatalf("Failed to check testadmin user: %v", err)
		}
		if !strings.Contains(stdout, "testadmin") {
			t.Errorf("testadmin user not found: %s", stdout)
		}
		t.Log("testadmin user verified")

		stdout, _, err = sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "id testuser")
		if err != nil {
			t.Fatalf("Failed to check testuser: %v", err)
		}
		if !strings.Contains(stdout, "testuser") {
			t.Errorf("testuser not found: %s", stdout)
		}
		t.Log("testuser verified")

		stdout, _, err = sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "getent passwd testuser | cut -d: -f7")
		if err != nil {
			t.Fatalf("Failed to check testuser shell: %v", err)
		}
		shell := strings.TrimSpace(stdout)
		if shell != "/bin/sh" {
			t.Errorf("Expected testuser shell /bin/sh, got %s", shell)
		}
		t.Logf("testuser shell verified: %s", shell)
	})

	t.Run("VerifyRuncmd", func(t *testing.T) {
		stdout, _, err := sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "test -f /tmp/runcmd-marker-file && echo 'exists'")
		if err != nil || strings.TrimSpace(stdout) != "exists" {
			t.Errorf("runcmd marker file not found")
		}
		t.Log("runcmd marker file verified")

		stdout, _, err = sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "cat /tmp/runcmd-output.txt")
		if err != nil {
			t.Fatalf("Failed to read runcmd output: %v", err)
		}
		if !strings.Contains(stdout, "runcmd executed") {
			t.Errorf("Expected runcmd output 'runcmd executed', got: %s", stdout)
		}
		t.Log("runcmd output verified")
	})

	t.Run("VerifyPackages", func(t *testing.T) {
		stdout, _, err := sshExecWithKey(t, privateKeyPath, "testadmin", vmIP, "which curl")
		if err != nil {
			t.Errorf("curl package not installed: %v", err)
		} else {
			t.Logf("curl installed at: %s", strings.TrimSpace(stdout))
		}
	})

	t.Log("All cloud-init verification tests passed!")
}
