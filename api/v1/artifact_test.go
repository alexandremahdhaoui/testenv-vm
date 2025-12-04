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

package v1

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCreateInput_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input CreateInput
	}{
		{
			name: "minimal input",
			input: CreateInput{
				TestID:  "test-123",
				Stage:   "e2e",
				TmpDir:  "/tmp/test",
				Spec:    map[string]any{},
				RootDir: "/workspace",
			},
		},
		{
			name: "full input",
			input: CreateInput{
				TestID:   "test-456",
				Stage:    "integration",
				TmpDir:   "/tmp/test-456",
				Metadata: map[string]string{"build": "12345"},
				Spec: map[string]any{
					"providers": []any{
						map[string]any{"name": "stub", "engine": "go://stub"},
					},
					"vms": []any{
						map[string]any{"name": "vm1", "memory": float64(2048)},
					},
				},
				Env:     map[string]string{"CI": "true", "DEBUG": "1"},
				RootDir: "/home/user/project",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got CreateInput
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.input, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.input, got)
			}
		})
	}
}

func TestCreateInput_JSONFieldNames(t *testing.T) {
	input := CreateInput{
		TestID:   "test-123",
		Stage:    "e2e",
		TmpDir:   "/tmp",
		Metadata: map[string]string{"key": "val"},
		Spec:     map[string]any{"option": "value"},
		Env:      map[string]string{"VAR": "val"},
		RootDir:  "/root",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	// Verify expected field names
	expectedFields := []string{"testID", "stage", "tmpDir", "metadata", "spec", "env", "rootDir"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}
}

func TestDeleteInput_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input DeleteInput
	}{
		{
			name: "minimal input",
			input: DeleteInput{
				TestID: "test-123",
			},
		},
		{
			name: "full input",
			input: DeleteInput{
				TestID:           "test-456",
				Metadata:         map[string]string{"created": "2024-01-01"},
				ManagedResources: []string{"/tmp/state.json", "/tmp/artifacts/key"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got DeleteInput
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.input, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.input, got)
			}
		})
	}
}

func TestDeleteInput_JSONFieldNames(t *testing.T) {
	input := DeleteInput{
		TestID:           "test-123",
		Metadata:         map[string]string{"key": "val"},
		ManagedResources: []string{"/path/1"},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{"testID", "metadata", "managedResources"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}
}

func TestTestEnvArtifact_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name     string
		artifact TestEnvArtifact
	}{
		{
			name: "minimal artifact",
			artifact: TestEnvArtifact{
				TestID: "test-123",
			},
		},
		{
			name: "full artifact",
			artifact: TestEnvArtifact{
				TestID: "test-456",
				Files: map[string]string{
					"testenv-vm.ssh-key":         "keys/vm-ssh",
					"testenv-vm.ssh-key.pub":     "keys/vm-ssh.pub",
					"testenv-vm.cloud-init.yaml": "cloud-init/user-data.yaml",
				},
				Metadata: map[string]string{
					"testenv-vm.vm-ip":      "192.168.100.10",
					"testenv-vm.vm-mac":     "52:54:00:12:34:56",
					"testenv-vm.network-ip": "192.168.100.1",
				},
				ManagedResources: []string{
					"/var/lib/testenv/test-456/state.json",
					"/var/lib/testenv/test-456/artifacts",
				},
				Env: map[string]string{
					"TEST_VM_IP":   "192.168.100.10",
					"TEST_SSH_KEY": "/tmp/test-456/keys/vm-ssh",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.artifact)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got TestEnvArtifact
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.artifact, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.artifact, got)
			}
		})
	}
}

func TestTestEnvArtifact_JSONFieldNames(t *testing.T) {
	artifact := TestEnvArtifact{
		TestID:           "test-123",
		Files:            map[string]string{"f1": "p1"},
		Metadata:         map[string]string{"k1": "v1"},
		ManagedResources: []string{"/path/1"},
		Env:              map[string]string{"VAR": "val"},
	}

	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{"testID", "files", "metadata", "managedResources", "env"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}
}
