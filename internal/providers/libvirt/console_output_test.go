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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDomainXMLLogsTheSerialConsoleToTheDeclaredFile(t *testing.T) {
	xml, err := generateDomainXML(DomainConfig{
		Name:          "box",
		DiskPath:      "/disks/box.qcow2",
		ConsoleOutput: "/artifacts/test-1/console/box.log",
	})
	if err != nil {
		t.Fatalf("generateDomainXML() unexpected error: %v", err)
	}
	want := "<serial type='pty'>\n            <log file='/artifacts/test-1/console/box.log' append='on'/>\n            <target port='0'/>"
	if !strings.Contains(xml, want) {
		t.Errorf("domain XML should contain %q\n%s", want, xml)
	}
}

func TestGenerateDomainXMLGivesEveryVMAVGAScreenWithNoGraphicsListener(t *testing.T) {
	xml, err := generateDomainXML(DomainConfig{
		Name:          "box",
		DiskPath:      "/disks/box.qcow2",
		ConsoleOutput: "/artifacts/test-1/console/box.log",
	})
	if err != nil {
		t.Fatalf("generateDomainXML() unexpected error: %v", err)
	}
	want := "<video>\n            <model type='vga'/>\n        </video>"
	if !strings.Contains(xml, want) {
		t.Errorf("domain XML should contain %q\n%s", want, xml)
	}
	if strings.Contains(xml, "<graphics") {
		t.Errorf("domain XML should open no graphics listener\n%s", xml)
	}
}

func TestValidateConsoleOutputRefusesAMissingOrRelativePathByName(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "an empty path names the vm", path: "", wantErr: "vm box has no console output path"},
		{name: "a relative path names the vm and the path", path: "console/box.log", wantErr: `vm box console output path "console/box.log" is not absolute`},
		{name: "an absolute path passes", path: "/artifacts/console/box.log"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opErr := validateConsoleOutput("box", tt.path)
			if tt.wantErr == "" {
				if opErr != nil {
					t.Fatalf("validateConsoleOutput() = %v, want nil", opErr)
				}
				return
			}
			if opErr == nil || !strings.Contains(opErr.Message, tt.wantErr) {
				t.Fatalf("validateConsoleOutput() = %v, want a message containing %q", opErr, tt.wantErr)
			}
		})
	}
}

func TestCreateConsoleOutputFileTruncatesAPreviousLogAndLeavesAFileTheProviderUserCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test-1", "console", "box.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating console directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("a previous boot"), 0o644); err != nil {
		t.Fatalf("writing a previous log: %v", err)
	}
	if err := createConsoleOutputFile(path); err != nil {
		t.Fatalf("createConsoleOutputFile() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("console output file: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("console output file size = %d, want 0 after truncation", info.Size())
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Errorf("the owner cannot read the console output file: %v", err)
	}
}

func TestCreateConsoleOutputFileCreatesTheMissingParentDirectoryAndAFileOnlyItsOwnerCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test-1", "console", "box.log")
	if err := createConsoleOutputFile(path); err != nil {
		t.Fatalf("createConsoleOutputFile() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("console output file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("console output file mode = %o, want 600", got)
	}
}
