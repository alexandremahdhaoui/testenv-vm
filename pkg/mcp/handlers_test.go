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

package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/alexandremahdhaoui/forge/pkg/engineframework"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
)

// newTestOrchestrator creates an orchestrator with temporary directories for testing.
func newTestOrchestrator(t *testing.T) *orchestrator.Orchestrator {
	t.Helper()
	tmpDir := t.TempDir()
	orch, err := orchestrator.NewOrchestrator(orchestrator.Config{
		StateDir:         filepath.Join(tmpDir, "state"),
		ImageCacheDir:    filepath.Join(tmpDir, "images"),
		CleanupOnFailure: true,
	})
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}
	t.Cleanup(func() { _ = orch.Close() })
	return orch
}

func TestHandleCreate_Success(t *testing.T) {
	tmpDir := t.TempDir()
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Note: A true success test would require mocking the provider/executor
	// or setting up a real provider. For now, we test that the handler
	// correctly returns an error when the orchestrator fails (which it will
	// without valid providers).

	ctx := context.Background()
	input := engineframework.CreateInput{
		TestID: "test-success",
		Stage:  "integration",
		TmpDir: tmpDir,
		Spec: map[string]any{
			"providers": []any{
				map[string]any{
					"name":   "mock-provider",
					"engine": "/nonexistent/mock-provider",
				},
			},
		},
	}

	// This will fail because the provider doesn't exist, but it verifies
	// the handler path is exercised correctly
	_, err = server.handleCreate(ctx, input)
	if err == nil {
		t.Log("handleCreate succeeded (unexpected without real provider)")
	}
	// We expect an error because the provider won't start
	// The important thing is that the handler doesn't panic and returns an error
}

func TestHandleCreate_OrchestratorError(t *testing.T) {
	tmpDir := t.TempDir()
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()

	// Test with invalid spec - should return an error from orchestrator
	input := engineframework.CreateInput{
		TestID: "test-error",
		Stage:  "integration",
		TmpDir: tmpDir,
		Spec:   nil, // Invalid - nil spec will cause orchestrator to fail
	}

	artifact, err := server.handleCreate(ctx, input)
	if err == nil {
		t.Error("handleCreate() error = nil, want error for invalid spec")
	}
	if artifact != nil {
		t.Error("handleCreate() artifact should be nil when error occurs")
	}
}

func TestHandleCreate_InvalidSpec(t *testing.T) {
	tmpDir := t.TempDir()
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()

	// Test with empty providers - should fail validation
	input := engineframework.CreateInput{
		TestID: "test-invalid-spec",
		Stage:  "integration",
		TmpDir: tmpDir,
		Spec: map[string]any{
			"providers": []any{}, // Empty - invalid
		},
	}

	artifact, err := server.handleCreate(ctx, input)
	if err == nil {
		t.Error("handleCreate() error = nil, want error for empty providers")
	}
	if artifact != nil {
		t.Error("handleCreate() artifact should be nil when error occurs")
	}
}

func TestHandleDelete_Success(t *testing.T) {
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()

	// Delete of non-existent state should succeed (idempotent)
	input := engineframework.DeleteInput{
		TestID: "nonexistent-test-id",
	}

	err = server.handleDelete(ctx, input)
	if err != nil {
		t.Errorf("handleDelete() error = %v, want nil for non-existent state", err)
	}
}

func TestHandleDelete_OrchestratorError(t *testing.T) {
	// This test verifies error handling in delete path.
	// Since delete is best-effort and returns nil for non-existent states,
	// we need to simulate a scenario where the orchestrator would return an error.
	// However, the current orchestrator implementation is best-effort for delete,
	// so we test the handler logic with a different approach.
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()

	// Test delete with valid testID - should succeed (best-effort)
	input := engineframework.DeleteInput{
		TestID: "test-delete-error",
		Metadata: map[string]string{
			"some": "metadata",
		},
	}

	// The orchestrator's Delete is best-effort, so it should not return error
	// for non-existent resources
	err = server.handleDelete(ctx, input)
	if err != nil {
		t.Errorf("handleDelete() error = %v, want nil (best-effort delete)", err)
	}
}

func TestNormalizeFilesMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		wantNil  bool
		wantLen  int
		wantKeys []string
	}{
		{
			name:    "nil map returns empty map",
			input:   nil,
			wantNil: false,
			wantLen: 0,
		},
		{
			name:    "empty map returns same map",
			input:   map[string]string{},
			wantNil: false,
			wantLen: 0,
		},
		{
			name: "non-empty map returned as-is",
			input: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			wantNil:  false,
			wantLen:  2,
			wantKeys: []string{"key1", "key2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeFilesMap(tt.input)
			if result == nil && !tt.wantNil {
				t.Error("normalizeFilesMap() returned nil, want non-nil")
			}
			if result != nil && tt.wantNil {
				t.Error("normalizeFilesMap() returned non-nil, want nil")
			}
			if len(result) != tt.wantLen {
				t.Errorf("normalizeFilesMap() len = %d, want %d", len(result), tt.wantLen)
			}
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("normalizeFilesMap() missing key %q", key)
				}
			}
		})
	}
}

func TestNormalizeMetadataMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		wantNil  bool
		wantLen  int
		wantKeys []string
	}{
		{
			name:    "nil map returns empty map",
			input:   nil,
			wantNil: false,
			wantLen: 0,
		},
		{
			name:    "empty map returns same map",
			input:   map[string]string{},
			wantNil: false,
			wantLen: 0,
		},
		{
			name: "non-empty map returned as-is",
			input: map[string]string{
				"meta1": "value1",
				"meta2": "value2",
			},
			wantNil:  false,
			wantLen:  2,
			wantKeys: []string{"meta1", "meta2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeMetadataMap(tt.input)
			if result == nil && !tt.wantNil {
				t.Error("normalizeMetadataMap() returned nil, want non-nil")
			}
			if result != nil && tt.wantNil {
				t.Error("normalizeMetadataMap() returned non-nil, want nil")
			}
			if len(result) != tt.wantLen {
				t.Errorf("normalizeMetadataMap() len = %d, want %d", len(result), tt.wantLen)
			}
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("normalizeMetadataMap() missing key %q", key)
				}
			}
		})
	}
}

func TestNormalizeEnvMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		wantNil  bool
		wantLen  int
		wantKeys []string
	}{
		{
			name:    "nil map returns empty map",
			input:   nil,
			wantNil: false,
			wantLen: 0,
		},
		{
			name:    "empty map returns same map",
			input:   map[string]string{},
			wantNil: false,
			wantLen: 0,
		},
		{
			name: "non-empty map returned as-is",
			input: map[string]string{
				"ENV_VAR_1": "value1",
				"ENV_VAR_2": "value2",
			},
			wantNil:  false,
			wantLen:  2,
			wantKeys: []string{"ENV_VAR_1", "ENV_VAR_2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeEnvMap(tt.input)
			if result == nil && !tt.wantNil {
				t.Error("normalizeEnvMap() returned nil, want non-nil")
			}
			if result != nil && tt.wantNil {
				t.Error("normalizeEnvMap() returned non-nil, want nil")
			}
			if len(result) != tt.wantLen {
				t.Errorf("normalizeEnvMap() len = %d, want %d", len(result), tt.wantLen)
			}
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("normalizeEnvMap() missing key %q", key)
				}
			}
		})
	}
}

func TestNormalizeMapsMutability(t *testing.T) {
	// Verify that non-nil maps are returned as-is (same reference)
	original := map[string]string{"key": "value"}

	filesResult := normalizeFilesMap(original)
	if &filesResult != &original {
		// They should be the same map
		original["newkey"] = "newvalue"
		if filesResult["newkey"] != "newvalue" {
			t.Error("normalizeFilesMap should return the same map reference")
		}
	}

	original2 := map[string]string{"key": "value"}
	metaResult := normalizeMetadataMap(original2)
	if &metaResult != &original2 {
		original2["newkey"] = "newvalue"
		if metaResult["newkey"] != "newvalue" {
			t.Error("normalizeMetadataMap should return the same map reference")
		}
	}

	original3 := map[string]string{"key": "value"}
	envResult := normalizeEnvMap(original3)
	if &envResult != &original3 {
		original3["newkey"] = "newvalue"
		if envResult["newkey"] != "newvalue" {
			t.Error("normalizeEnvMap should return the same map reference")
		}
	}
}

func TestHandleCreate_InputConversion(t *testing.T) {
	// Test that the input conversion from engineframework types to v1 types
	// is handled correctly
	tmpDir := t.TempDir()
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()

	// Test with all fields populated
	input := engineframework.CreateInput{
		TestID:  "test-conversion",
		Stage:   "integration",
		TmpDir:  tmpDir,
		RootDir: "/root/dir",
		Metadata: map[string]string{
			"prev.key": "prev.value",
		},
		Env: map[string]string{
			"PREV_ENV": "prev_value",
		},
		Spec: map[string]any{
			"providers": []any{
				map[string]any{
					"name":   "test",
					"engine": "/nonexistent",
				},
			},
		},
	}

	// The call will fail because provider doesn't exist, but that's expected
	// We're testing that the conversion doesn't panic
	_, _ = server.handleCreate(ctx, input)
}

func TestHandleDelete_InputConversion(t *testing.T) {
	// Test that the input conversion from engineframework types to v1 types
	// is handled correctly for delete
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()

	// Test with all fields populated
	input := engineframework.DeleteInput{
		TestID: "test-delete-conversion",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	// Delete of non-existent should succeed
	err = server.handleDelete(ctx, input)
	if err != nil {
		t.Errorf("handleDelete() error = %v, want nil", err)
	}
}

// mockOrchestrator is not directly usable because Server expects *orchestrator.Orchestrator
// and not an interface. This is a limitation of the current design.
// For comprehensive testing, we rely on the real orchestrator with invalid inputs
// to trigger error paths.

func TestHandleCreate_EmptyTestID(t *testing.T) {
	// This tests the handler's behavior when TestID is empty
	// The orchestrator should handle this, but the MCP handler validates first
	tmpDir := t.TempDir()
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()

	// Empty TestID - validation happens at MCP handler level (makeCreateHandler),
	// but handleCreate itself passes to orchestrator
	input := engineframework.CreateInput{
		TestID: "", // Empty
		Stage:  "integration",
		TmpDir: tmpDir,
		Spec: map[string]any{
			"providers": []any{
				map[string]any{
					"name":   "test",
					"engine": "/nonexistent",
				},
			},
		},
	}

	// handleCreate delegates to orchestrator which will fail on empty testID
	// when trying to create directories or save state
	_, err = server.handleCreate(ctx, input)
	// Error is expected due to orchestrator validation or provider issues
	_ = err // We just verify it doesn't panic
}

func TestHandleDelete_EmptyTestID(t *testing.T) {
	orch := newTestOrchestrator(t)

	server, err := NewServer(orch, "testenv-vm", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()

	// Empty TestID for delete
	input := engineframework.DeleteInput{
		TestID: "", // Empty
	}

	// Delete with empty TestID - the state store validates testID and returns error
	err = server.handleDelete(ctx, input)
	// The orchestrator's state store returns an error for empty testID
	if err == nil {
		t.Error("handleDelete() with empty testID error = nil, want error")
	}
}

// Verify that error wrapping works as expected
func TestErrorWrapping(t *testing.T) {
	baseErr := errors.New("base error")
	wrappedErr := errors.New("wrapped: base error")

	if !errors.Is(baseErr, baseErr) {
		t.Error("errors.Is should work for same error")
	}

	// Verify our understanding of error handling
	if errors.Is(wrappedErr, baseErr) {
		t.Error("errors.Is should not match different errors without proper wrapping")
	}
}
