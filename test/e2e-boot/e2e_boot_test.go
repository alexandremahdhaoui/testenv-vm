//go:build e2e_boot

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

package e2e_boot_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
	"gopkg.in/yaml.v3"
)

const (
	talosVMName     = "e2e-boot-talos"
	openwrtVMName   = "e2e-boot-openwrt"
	openwrtLANIP    = "192.168.1.1"
	talosAPIPort    = "50000"
	openwrtSSHPort  = "22"
	openwrtHTTPPort = "80"
	libvirtURI      = "qemu:///system"
)

func TestTheTalosISOAndTheOpenWrtImageBootAnswerTheirPortsAndLeaveNothingBehind(t *testing.T) {
	requireLibvirt(t)
	projectRoot := findProjectRoot(t)

	specMap := loadScenario(t, filepath.Join(projectRoot, "test", "e2e", "scenarios", "boot_talos_openwrt.yaml"), projectRoot)

	tmpDir, err := os.MkdirTemp(os.TempDir(), "testenv-vm-e2e-boot-")
	if err != nil {
		t.Fatalf("creating work dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	stateDir := filepath.Join(tmpDir, "state")
	artifactDir := filepath.Join(tmpDir, "artifacts")
	for _, dir := range []string{tmpDir, stateDir, artifactDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
		grantLibvirtAccess(dir)
	}
	t.Setenv("TESTENV_VM_STATE_DIR", stateDir)

	imageCacheDir := os.Getenv("TESTENV_VM_IMAGE_CACHE_DIR")
	if imageCacheDir == "" {
		imageCacheDir = "/tmp/testenv-vm-images"
	}
	if err := os.MkdirAll(imageCacheDir, 0o755); err != nil {
		t.Fatalf("creating image cache dir %s: %v", imageCacheDir, err)
	}
	grantLibvirtAccess(imageCacheDir)

	orch, err := orchestrator.NewOrchestrator(orchestrator.Config{
		StateDir:         stateDir,
		ImageCacheDir:    imageCacheDir,
		CleanupOnFailure: true,
	})
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}
	defer orch.Close()

	testID := "e2e-boot-" + time.Now().Format("20060102-150405")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	t.Log("Creating the Talos and OpenWrt environment")
	result, err := orch.Create(ctx, &v1.CreateInput{
		TestID:   testID,
		Stage:    "e2e-boot",
		TmpDir:   artifactDir,
		Spec:     specMap,
		Env:      map[string]string{},
		RootDir:  projectRoot,
		Metadata: map[string]string{},
	})
	if err != nil {
		t.Fatalf("creating environment: %v", err)
	}
	artifact := result.Artifact

	talosIP := artifact.Metadata["testenv-vm.vm."+talosVMName+".ip"]
	if talosIP == "" {
		t.Errorf("Talos VM has no IP in metadata")
	}
	openwrtIP := artifact.Metadata["testenv-vm.vm."+openwrtVMName+".ip"]
	if openwrtIP != openwrtLANIP {
		t.Errorf("OpenWrt VM IP = %q, want %q", openwrtIP, openwrtLANIP)
	}

	assertPortAnswers(t, "Talos API", net.JoinHostPort(talosIP, talosAPIPort))
	assertPortAnswers(t, "OpenWrt SSH", net.JoinHostPort(openwrtLANIP, openwrtSSHPort))
	assertPortAnswers(t, "OpenWrt HTTP", net.JoinHostPort(openwrtLANIP, openwrtHTTPPort))

	assertImageCached(t, imageCacheDir, "talos-metal-amd64", "metal-amd64.iso")
	assertImageCached(t, imageCacheDir, "openwrt-x86-64", "openwrt-24.10.8-x86-64-generic-ext4-combined.img.gz")
	assertImageCached(t, imageCacheDir, "openwrt-x86-64", "openwrt-24.10.8-x86-64-generic-ext4-combined.img")

	t.Log("Deleting the environment")
	deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer deleteCancel()
	if err := orch.Delete(deleteCtx, &v1.DeleteInput{
		TestID:           testID,
		Metadata:         artifact.Metadata,
		ManagedResources: artifact.ManagedResources,
	}); err != nil {
		t.Fatalf("deleting environment: %v", err)
	}

	assertNoLibvirtObjectNamed(t, "list", "e2e-boot")
	assertNoLibvirtObjectNamed(t, "net-list", "e2e-boot")
	assertDirectoryHoldsNoFiles(t, filepath.Join(stateDir, "disks"))
	assertDirectoryHoldsNoFiles(t, filepath.Join(stateDir, "cloudinit"))
}

func requireLibvirt(t *testing.T) {
	t.Helper()
	if err := exec.Command("virsh", "--connect", libvirtURI, "version").Run(); err != nil {
		t.Skip("libvirt not available, skipping e2e-boot test")
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available, skipping e2e-boot test")
	}
}

func loadScenario(t *testing.T, path, projectRoot string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading scenario %s: %v", path, err)
	}
	var specMap map[string]any
	if err := yaml.Unmarshal(data, &specMap); err != nil {
		t.Fatalf("parsing scenario %s: %v", path, err)
	}
	providers, _ := specMap["providers"].([]any)
	for _, p := range providers {
		provider, ok := p.(map[string]any)
		if !ok {
			continue
		}
		engine, _ := provider["engine"].(string)
		if strings.HasPrefix(engine, "./") {
			provider["engine"] = filepath.Join(projectRoot, engine[2:])
		}
	}
	return specMap
}

func grantLibvirtAccess(dir string) {
	_ = os.Chmod(dir, 0o755)
	setfacl, err := exec.LookPath("setfacl")
	if err != nil {
		return
	}
	for _, group := range []string{"libvirt", "libvirt-qemu", "kvm", "qemu"} {
		if err := exec.Command("getent", "group", group).Run(); err != nil {
			continue
		}
		_ = exec.Command(setfacl, "-m", "g:"+group+":rwx", dir).Run()
		_ = exec.Command(setfacl, "-d", "-m", "g:"+group+":rwx", dir).Run()
	}
}

func assertPortAnswers(t *testing.T, what, addr string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err == nil {
			_ = conn.Close()
			t.Logf("%s answers on %s", what, addr)
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Errorf("%s does not answer on %s", what, addr)
}

func assertImageCached(t *testing.T, cacheDir, imageName, fileName string) {
	t.Helper()
	path := filepath.Join(cacheDir, imageName, fileName)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("image %s is not cached at %s: %v", imageName, path, err)
	}
}

func assertNoLibvirtObjectNamed(t *testing.T, listVerb, needle string) {
	t.Helper()
	output, err := exec.Command("virsh", "--connect", libvirtURI, listVerb, "--all", "--name").Output()
	if err != nil {
		t.Fatalf("running virsh %s: %v", listVerb, err)
	}
	for _, name := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.Contains(name, needle) {
			t.Errorf("virsh %s still shows %s", listVerb, name)
		}
	}
}

func assertDirectoryHoldsNoFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("reading %s: %v", dir, err)
	}
	for _, entry := range entries {
		t.Errorf("%s still holds %s", dir, entry.Name())
	}
}

func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}
