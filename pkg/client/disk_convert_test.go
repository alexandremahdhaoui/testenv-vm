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

package client

import (
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func TestConvertVMSpecCarriesTheDiskBusAndWWNToTheProvider(t *testing.T) {
	result := convertVMSpec(v1.VMSpec{
		Memory:  1024,
		Vcpus:   1,
		Network: "net",
		Disk:    v1.DiskSpec{Size: "10G", Bus: "scsi", Wwn: "5000c500a1b2c3d4"},
	})

	if result.Disk.Bus != "scsi" {
		t.Errorf("Disk.Bus = %q, want scsi", result.Disk.Bus)
	}
	if result.Disk.WWN != "5000c500a1b2c3d4" {
		t.Errorf("Disk.WWN = %q, want 5000c500a1b2c3d4", result.Disk.WWN)
	}
}
