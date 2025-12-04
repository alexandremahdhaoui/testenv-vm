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

func TestEnvironmentState_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		state EnvironmentState
	}{
		{
			name: "minimal state",
			state: EnvironmentState{
				ID:        "test-123",
				Stage:     "e2e",
				Status:    StatusPending,
				CreatedAt: "2024-01-01T00:00:00Z",
				UpdatedAt: "2024-01-01T00:00:00Z",
				Resources: ResourceMap{},
			},
		},
		{
			name: "full state",
			state: EnvironmentState{
				ID:        "test-456",
				Stage:     "integration",
				Status:    StatusReady,
				CreatedAt: "2024-01-01T00:00:00Z",
				UpdatedAt: "2024-01-01T01:00:00Z",
				Spec: &TestenvSpec{
					StateDir: "/tmp/state",
					Providers: []ProviderConfig{
						{Name: "stub", Engine: "go://stub"},
					},
				},
				Resources: ResourceMap{
					Keys: map[string]*ResourceState{
						"vm-ssh": {
							Provider:  "stub",
							Status:    StatusReady,
							State:     map[string]any{"publicKey": "ssh-ed25519 AAAA..."},
							CreatedAt: "2024-01-01T00:00:00Z",
							UpdatedAt: "2024-01-01T00:00:00Z",
						},
					},
					Networks: map[string]*ResourceState{
						"test-net": {
							Provider:  "stub",
							Status:    StatusReady,
							State:     map[string]any{"ip": "192.168.100.1"},
							CreatedAt: "2024-01-01T00:00:00Z",
							UpdatedAt: "2024-01-01T00:00:00Z",
						},
					},
					VMs: map[string]*ResourceState{
						"test-vm": {
							Provider:  "stub",
							Status:    StatusReady,
							State:     map[string]any{"ip": "192.168.100.10"},
							CreatedAt: "2024-01-01T00:00:00Z",
							UpdatedAt: "2024-01-01T00:00:00Z",
						},
					},
				},
				ExecutionPlan: &ExecutionPlan{
					Phases: []Phase{
						{Resources: []ResourceRef{{Kind: "key", Name: "vm-ssh"}}},
						{Resources: []ResourceRef{{Kind: "network", Name: "test-net"}}},
						{Resources: []ResourceRef{{Kind: "vm", Name: "test-vm"}}},
					},
				},
				Errors: []ErrorRecord{
					{
						Resource:  ResourceRef{Kind: "vm", Name: "failed-vm"},
						Operation: "create",
						Error:     "some error",
						Timestamp: "2024-01-01T00:30:00Z",
					},
				},
				ArtifactDir: "/tmp/artifacts",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.state)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Unmarshal back
			var got EnvironmentState
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Compare
			if !reflect.DeepEqual(tt.state, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.state, got)
			}
		})
	}
}

func TestEnvironmentState_JSONFieldNames(t *testing.T) {
	state := EnvironmentState{
		ID:          "test-123",
		Stage:       "unit",
		Status:      StatusCreating,
		CreatedAt:   "2024-01-01T00:00:00Z",
		UpdatedAt:   "2024-01-01T00:00:00Z",
		Resources:   ResourceMap{},
		ArtifactDir: "/artifacts",
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	// Verify expected field names
	expectedFields := []string{"id", "stage", "status", "createdAt", "updatedAt", "resources", "artifactDir"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}
}

func TestResourceState_JSONRoundtrip(t *testing.T) {
	state := ResourceState{
		Provider:  "stub",
		Status:    StatusReady,
		State:     map[string]any{"ip": "192.168.1.1", "port": float64(22)},
		CreatedAt: "2024-01-01T00:00:00Z",
		UpdatedAt: "2024-01-01T00:00:00Z",
		Error:     "",
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got ResourceState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(state, got) {
		t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", state, got)
	}
}

func TestResourceState_WithError(t *testing.T) {
	state := ResourceState{
		Provider: "stub",
		Status:   StatusFailed,
		Error:    "connection refused",
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got ResourceState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if got.Error != "connection refused" {
		t.Errorf("Expected error %q, got %q", state.Error, got.Error)
	}
	if got.Status != StatusFailed {
		t.Errorf("Expected status %q, got %q", StatusFailed, got.Status)
	}
}

func TestExecutionPlan_JSONRoundtrip(t *testing.T) {
	plan := ExecutionPlan{
		Phases: []Phase{
			{
				Resources: []ResourceRef{
					{Kind: "key", Name: "ssh-key", Provider: "stub"},
				},
			},
			{
				Resources: []ResourceRef{
					{Kind: "network", Name: "net1", Provider: "stub"},
					{Kind: "network", Name: "net2", Provider: "stub"},
				},
			},
			{
				Resources: []ResourceRef{
					{Kind: "vm", Name: "vm1", Provider: "stub"},
				},
			},
		},
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got ExecutionPlan
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(plan, got) {
		t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", plan, got)
	}
}

func TestErrorRecord_JSONRoundtrip(t *testing.T) {
	record := ErrorRecord{
		Resource:  ResourceRef{Kind: "vm", Name: "test-vm", Provider: "stub"},
		Operation: "create",
		Error:     "timeout waiting for IP",
		Timestamp: "2024-01-01T00:30:00Z",
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got ErrorRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(record, got) {
		t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", record, got)
	}
}

func TestStatusConstants(t *testing.T) {
	// Verify status constants have expected values
	tests := []struct {
		constant string
		value    string
	}{
		{StatusPending, "pending"},
		{StatusCreating, "creating"},
		{StatusReady, "ready"},
		{StatusFailed, "failed"},
		{StatusDestroying, "destroying"},
		{StatusDestroyed, "destroyed"},
	}

	for _, tt := range tests {
		if tt.constant != tt.value {
			t.Errorf("Status constant mismatch: expected %q, got %q", tt.value, tt.constant)
		}
	}
}
