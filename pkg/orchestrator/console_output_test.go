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

package orchestrator

import (
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func TestBuildArtifactExportsTheConsoleOutputOfEveryVMInEnvAndMetadata(t *testing.T) {
	orchestrator, err := NewOrchestrator(newTestConfig(t))
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	envState := &v1.EnvironmentState{
		ID:          "test-1",
		ArtifactDir: "/tmp/test-1",
		Resources: v1.ResourceMap{
			VMs: map[string]*v1.ResourceState{
				"talos-node": {
					Provider: "libvirt",
					Status:   v1.StatusReady,
					State:    map[string]any{"consoleOutput": "/tmp/test-1/console/talos-node.log"},
				},
			},
		},
	}

	artifact := orchestrator.buildArtifact("test-1", envState, nil)

	if got := artifact.Env["TESTENV_VM_TALOS_NODE_CONSOLE_OUTPUT"]; got != "/tmp/test-1/console/talos-node.log" {
		t.Errorf("env TESTENV_VM_TALOS_NODE_CONSOLE_OUTPUT = %q, want the console log path", got)
	}
	if got := artifact.Metadata["testenv-vm.vm.talos-node.consoleOutput"]; got != "/tmp/test-1/console/talos-node.log" {
		t.Errorf("metadata testenv-vm.vm.talos-node.consoleOutput = %q, want the console log path", got)
	}
}

func TestBuildArtifactExportsTheLibvirtDomainNameOfEveryVMSoAGateCanScreenshotIt(t *testing.T) {
	orchestrator, err := NewOrchestrator(newTestConfig(t))
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	envState := &v1.EnvironmentState{
		ID: "test-1",
		Resources: v1.ResourceMap{
			VMs: map[string]*v1.ResourceState{
				"talos-node": {
					Provider: "libvirt",
					Status:   v1.StatusReady,
					State:    map[string]any{"name": "1a2b3c4d-talos-node"},
				},
			},
		},
	}

	artifact := orchestrator.buildArtifact("test-1", envState, nil)

	if got := artifact.Env["TESTENV_VM_TALOS_NODE_DOMAIN"]; got != "1a2b3c4d-talos-node" {
		t.Errorf("env TESTENV_VM_TALOS_NODE_DOMAIN = %q, want the prefixed libvirt domain name", got)
	}
}
