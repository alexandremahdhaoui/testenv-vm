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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

func createDisk(baseImage, outputPath, size, qemuImgPath string) error {
	if size == "" {
		size = "20G"
	}

	if baseImage == "" {
		return runQemuImg(qemuImgPath, "create", "-f", "qcow2", outputPath, size)
	}

	if _, err := os.Stat(baseImage); err != nil {
		return fmt.Errorf("base image not found: %s", baseImage)
	}

	format, err := detectImageFormat(qemuImgPath, baseImage)
	if err != nil {
		return fmt.Errorf("detecting format of base image %s: %w", baseImage, err)
	}

	return runQemuImg(qemuImgPath, overlayArgs(baseImage, format, outputPath, size)...)
}

func overlayArgs(baseImage, backingFormat, outputPath, size string) []string {
	return []string{"create", "-f", "qcow2", "-F", backingFormat, "-b", baseImage, outputPath, size}
}

func detectImageFormat(qemuImgPath, imagePath string) (string, error) {
	output, err := exec.Command(qemuImgPath, "info", "--output=json", imagePath).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running qemu-img info on %s: %w, output: %s", imagePath, err, string(output))
	}
	return parseImageFormat(output)
}

func parseImageFormat(qemuImgInfoJSON []byte) (string, error) {
	var info struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(qemuImgInfoJSON, &info); err != nil {
		return "", fmt.Errorf("parsing qemu-img info output: %w", err)
	}
	if info.Format == "" {
		return "", fmt.Errorf("qemu-img info output names no format")
	}
	return info.Format, nil
}

func runQemuImg(qemuImgPath string, args ...string) error {
	output, err := exec.Command(qemuImgPath, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create disk: %w, output: %s", err, string(output))
	}
	return nil
}
