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

package image

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func checkVirtCustomize() error {
	_, err := exec.LookPath("virt-customize")
	if err != nil {
		return fmt.Errorf("virt-customize not found in PATH; install libguestfs-tools (apt-get install libguestfs-tools)")
	}
	return nil
}

func createQcow2Overlay(basePath, overlayPath string) error {
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", "-F", "qcow2", "-b", basePath, overlayPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-img create overlay failed: %w\nOutput: %s", err, output)
	}
	return nil
}

func runVirtCustomize(ctx context.Context, imagePath string, spec *v1.ImageCustomizeSpec) error {
	virtArgs := []string{"-a", imagePath}

	if len(spec.Packages) > 0 {
		virtArgs = append(virtArgs, "--install", strings.Join(spec.Packages, ","))
	}

	for _, c := range spec.Runcmd {
		virtArgs = append(virtArgs, "--run-command", c)
	}

	// Build the command, prepending ELEVATED_PREPEND_CMD if set.
	// virt-customize needs root to read /boot/vmlinuz-* for the libguestfs appliance.
	var cmd *exec.Cmd
	if elevatedCmd := os.Getenv("ELEVATED_PREPEND_CMD"); elevatedCmd != "" {
		parts := strings.Fields(elevatedCmd)
		fullArgs := append(parts[1:], "virt-customize")
		fullArgs = append(fullArgs, virtArgs...)
		cmd = exec.CommandContext(ctx, parts[0], fullArgs...)
	} else {
		cmd = exec.CommandContext(ctx, "virt-customize", virtArgs...)
	}
	cmd.Env = append(os.Environ(), "LIBGUESTFS_BACKEND=direct")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("virt-customize failed: %w\nOutput: %s", err, output)
	}
	return nil
}

func cleanupPartialImage(path string) {
	_ = os.Remove(path)
}
