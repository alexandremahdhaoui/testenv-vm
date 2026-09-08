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

package libvirt

import (
	"strings"
	"testing"
)

func TestParseImageFormatReadsTheFormatQemuImgReports(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantErr string
	}{
		{name: "a qcow2 image", output: `{"virtual-size": 10, "format": "qcow2"}`, want: "qcow2"},
		{name: "a raw image", output: `{"format": "raw", "actual-size": 5}`, want: "raw"},
		{name: "an output with no format", output: `{"virtual-size": 10}`, wantErr: "names no format"},
		{name: "an output that is not json", output: `qemu-img: could not open`, wantErr: "parsing qemu-img info"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseImageFormat([]byte(tt.output))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseImageFormat() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseImageFormat() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseImageFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOverlayArgsNameTheDetectedBackingFormat(t *testing.T) {
	args := overlayArgs("/images/openwrt.img", "raw", "/disks/vm.qcow2", "1G")
	want := []string{"create", "-f", "qcow2", "-F", "raw", "-b", "/images/openwrt.img", "/disks/vm.qcow2", "1G"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("overlayArgs() = %v, want %v", args, want)
	}
}

func TestCreateDiskRefusesAMissingBaseImageBeforeCallingQemuImg(t *testing.T) {
	err := createDisk("/nowhere/missing.qcow2", "/tmp/unused.qcow2", "1G", "/bin/false")
	if err == nil || !strings.Contains(err.Error(), "base image not found") {
		t.Errorf("createDisk() error = %v, want a base image not found error", err)
	}
}
