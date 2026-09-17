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

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

func TestGenerateDomainXMLPlacesTheDiskOnItsDeclaredBus(t *testing.T) {
	tests := []struct {
		name    string
		config  DomainConfig
		want    []string
		absent  []string
		wantErr string
	}{
		{
			name: "a scsi disk with a wwn renders bus scsi, the wwn, a virtio-scsi controller and moves the cdroms off sda",
			config: DomainConfig{
				Name:         "box",
				DiskPath:     "/disks/box.qcow2",
				DiskBus:      "scsi",
				DiskWWN:      "5000c500a1b2c3d4",
				CdromPath:    "/images/boot.iso",
				CloudInitISO: "/cloudinit/box.iso",
			},
			want: []string{
				"<controller type='scsi' model='virtio-scsi'/>",
				"<target dev='sda' bus='scsi'/>\n            <wwn>5000c500a1b2c3d4</wwn>",
				"<source file='/images/boot.iso'/>\n            <target dev='sdb' bus='sata'/>",
				"<source file='/cloudinit/box.iso'/>\n            <target dev='sdc' bus='sata'/>",
			},
		},
		{
			name:   "an ide disk with a wwn renders hda on ide and the wwn without a scsi controller",
			config: DomainConfig{Name: "ide", DiskPath: "/disks/ide.qcow2", DiskBus: "ide", DiskWWN: "5000c500a1b2c3d4", CloudInitISO: "/cloudinit/ide.iso"},
			want:   []string{"<target dev='hda' bus='ide'/>\n            <wwn>5000c500a1b2c3d4</wwn>", "<target dev='sda' bus='sata'/>"},
			absent: []string{"<controller type='scsi'"},
		},
		{
			name:   "a virtio disk renders vda on virtio with the cdroms on sda and sdb",
			config: DomainConfig{Name: "plain", DiskPath: "/disks/plain.qcow2", DiskBus: "virtio", CdromPath: "/images/boot.iso", CloudInitISO: "/cloudinit/plain.iso"},
			want:   []string{"<target dev='vda' bus='virtio'/>", "<target dev='sda' bus='sata'/>", "<target dev='sdb' bus='sata'/>"},
			absent: []string{"<wwn>", "<controller type='scsi'"},
		},
		{
			name:   "a disk with no bus renders as virtio",
			config: DomainConfig{Name: "plain", DiskPath: "/disks/plain.qcow2"},
			want:   []string{"<target dev='vda' bus='virtio'/>"},
			absent: []string{"<controller type='scsi'"},
		},
		{
			name:   "a sata disk renders sda on sata and moves the cloud init ISO to sdb",
			config: DomainConfig{Name: "sata", DiskPath: "/disks/sata.qcow2", DiskBus: "sata", CloudInitISO: "/cloudinit/sata.iso"},
			want:   []string{"<target dev='sda' bus='sata'/>", "<target dev='sdb' bus='sata'/>"},
			absent: []string{"<controller type='scsi'"},
		},
		{
			name:    "an unknown bus refuses by name",
			config:  DomainConfig{Name: "odd", DiskPath: "/disks/odd.qcow2", DiskBus: "nvme"},
			wantErr: `disk bus "nvme" is not one of virtio, sata, scsi, ide`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xml, err := generateDomainXML(tt.config)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("generateDomainXML() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("generateDomainXML() unexpected error: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(xml, want) {
					t.Errorf("domain XML should contain %q\n%s", want, xml)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(xml, absent) {
					t.Errorf("domain XML should not contain %q\n%s", absent, xml)
				}
			}
		})
	}
}

func TestValidateDiskRefusesAWWNTheBusCannotCarry(t *testing.T) {
	tests := []struct {
		name    string
		disk    providerv1.DiskSpec
		wantErr string
	}{
		{name: "a scsi disk with a 16 hex digit wwn passes", disk: providerv1.DiskSpec{Bus: "scsi", WWN: "5000C500A1B2C3D4"}},
		{name: "an ide disk with a 16 hex digit wwn passes", disk: providerv1.DiskSpec{Bus: "ide", WWN: "5000c500a1b2c3d4"}},
		{name: "a sata disk without a wwn passes", disk: providerv1.DiskSpec{Bus: "sata"}},
		{name: "a virtio disk without a wwn passes", disk: providerv1.DiskSpec{Bus: "virtio"}},
		{name: "a disk with no bus and no wwn passes", disk: providerv1.DiskSpec{}},
		{name: "a wwn on virtio refuses by name", disk: providerv1.DiskSpec{Bus: "virtio", WWN: "5000c500a1b2c3d4"}, wantErr: "disk wwn 5000c500a1b2c3d4 on bus virtio: libvirt carries a wwn only on ide or scsi"},
		{name: "a wwn on sata refuses by name", disk: providerv1.DiskSpec{Bus: "sata", WWN: "5000c500a1b2c3d4"}, wantErr: "disk wwn 5000c500a1b2c3d4 on bus sata: libvirt carries a wwn only on ide or scsi"},
		{name: "a wwn on the default bus refuses by name", disk: providerv1.DiskSpec{WWN: "5000c500a1b2c3d4"}, wantErr: "disk wwn 5000c500a1b2c3d4 on bus virtio: libvirt carries a wwn only on ide or scsi"},
		{name: "a wwn of the wrong length refuses by name", disk: providerv1.DiskSpec{Bus: "ide", WWN: "5000c500"}, wantErr: `disk wwn "5000c500" is not 16 hex digits`},
		{name: "a wwn with a non hex digit refuses by name", disk: providerv1.DiskSpec{Bus: "scsi", WWN: "5000c500a1b2c3zz"}, wantErr: `disk wwn "5000c500a1b2c3zz" is not 16 hex digits`},
		{name: "an unknown bus refuses by name", disk: providerv1.DiskSpec{Bus: "usb"}, wantErr: `disk bus "usb" is not one of virtio, sata, scsi, ide`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDisk(tt.disk)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateDisk() unexpected error: %s", err.Message)
				}
				return
			}
			if err == nil || !strings.Contains(err.Message, tt.wantErr) {
				t.Fatalf("validateDisk() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
