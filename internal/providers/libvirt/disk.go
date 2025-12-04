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
	"fmt"
	"os"
	"os/exec"
)

// createDisk creates a QCOW2 disk image.
// If baseImage is provided, it creates a disk with the base image as a backing store.
// If baseImage is empty, it creates a standalone disk.
func createDisk(baseImage, outputPath, size, qemuImgPath string) error {
	// Apply default size if not specified
	if size == "" {
		size = "20G"
	}

	var cmd *exec.Cmd
	if baseImage != "" {
		// Verify base image exists
		if _, err := os.Stat(baseImage); err != nil {
			return fmt.Errorf("base image not found: %s", baseImage)
		}

		// Create disk with backing store
		cmd = exec.Command(qemuImgPath, "create",
			"-f", "qcow2",
			"-F", "qcow2",
			"-b", baseImage,
			outputPath,
			size)
	} else {
		// Create standalone disk
		cmd = exec.Command(qemuImgPath, "create",
			"-f", "qcow2",
			outputPath,
			size)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create disk: %w, output: %s", err, string(output))
	}

	return nil
}
