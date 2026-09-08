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

func TestGenerateDomainXMLAttachesTheDeclaredISOAsTheFirstCdrom(t *testing.T) {
	xml, err := generateDomainXML(DomainConfig{
		Name:      "talos",
		DiskPath:  "/disks/talos.qcow2",
		CdromPath: "/images/metal-amd64.iso",
		Networks:  []NetworkInterface{{Name: "default"}},
		BootOrder: []string{"cdrom"},
	})
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}
	for _, want := range []string{
		"<source file='/images/metal-amd64.iso'/>",
		"<target dev='sda' bus='sata'/>",
		"<boot dev='cdrom'/>",
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("domain XML should contain %s", want)
		}
	}
	if strings.Count(xml, "device='cdrom'") != 1 {
		t.Errorf("domain XML should hold exactly one cdrom, got %d", strings.Count(xml, "device='cdrom'"))
	}
}

func TestGenerateDomainXMLMovesTheCloudInitISOBehindTheDeclaredISO(t *testing.T) {
	xml, err := generateDomainXML(DomainConfig{
		Name:         "installer",
		DiskPath:     "/disks/installer.qcow2",
		CdromPath:    "/images/boot.iso",
		CloudInitISO: "/cloudinit/installer.iso",
		Networks:     []NetworkInterface{{Name: "default"}},
	})
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}
	bootIndex := strings.Index(xml, "/images/boot.iso")
	cloudInitIndex := strings.Index(xml, "/cloudinit/installer.iso")
	if bootIndex < 0 || cloudInitIndex < 0 || bootIndex > cloudInitIndex {
		t.Errorf("declared ISO should come before the cloud-init ISO")
	}
	if !strings.Contains(xml, "<target dev='sdb' bus='sata'/>") {
		t.Errorf("cloud-init ISO should sit on sdb when a declared ISO holds sda")
	}
	if strings.Count(xml, "device='cdrom'") != 2 {
		t.Errorf("domain XML should hold two cdroms, got %d", strings.Count(xml, "device='cdrom'"))
	}
}

func TestGenerateDomainXMLKeepsTheCloudInitISOOnSdaWithoutADeclaredISO(t *testing.T) {
	xml, err := generateDomainXML(DomainConfig{
		Name:         "plain",
		DiskPath:     "/disks/plain.qcow2",
		CloudInitISO: "/cloudinit/plain.iso",
		Networks:     []NetworkInterface{{Name: "default"}},
	})
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}
	if !strings.Contains(xml, "<target dev='sda' bus='sata'/>") {
		t.Errorf("cloud-init ISO should sit on sda")
	}
}

func TestGenerateDomainXMLEmitsNoCdromWithoutCloudInitOrADeclaredISO(t *testing.T) {
	xml, err := generateDomainXML(DomainConfig{
		Name:     "bare",
		DiskPath: "/disks/bare.qcow2",
		Networks: []NetworkInterface{{Name: "default"}},
	})
	if err != nil {
		t.Fatalf("generateDomainXML failed: %v", err)
	}
	if strings.Contains(xml, "device='cdrom'") {
		t.Errorf("domain XML should hold no cdrom")
	}
}
