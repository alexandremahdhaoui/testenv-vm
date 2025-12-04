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

package providerv1

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestOperationResult_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name   string
		result OperationResult
	}{
		{
			name: "success with vm resource",
			result: OperationResult{
				Success: true,
				Resource: map[string]any{
					"name":   "test-vm",
					"status": "running",
					"ip":     "192.168.100.10",
				},
			},
		},
		{
			name: "failure with error",
			result: OperationResult{
				Success: false,
				Error: &OperationError{
					Code:      ErrCodeProviderError,
					Message:   "failed to create VM",
					Retryable: true,
					Details:   map[string]any{"reason": "disk full"},
				},
			},
		},
		{
			name: "success without resource",
			result: OperationResult{
				Success: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got OperationResult
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.result, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.result, got)
			}
		})
	}
}

func TestOperationError_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		err  OperationError
	}{
		{
			name: "minimal error",
			err: OperationError{
				Code:    ErrCodeNotFound,
				Message: "resource not found",
			},
		},
		{
			name: "full error",
			err: OperationError{
				Code:      ErrCodeProviderError,
				Message:   "connection failed",
				Retryable: true,
				Details: map[string]any{
					"host":    "192.168.1.1",
					"port":    float64(22),
					"timeout": "30s",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.err)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got OperationError
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.err, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.err, got)
			}
		})
	}
}

func TestCapabilitiesResponse_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		resp CapabilitiesResponse
	}{
		{
			name: "minimal response",
			resp: CapabilitiesResponse{
				ProviderName: "stub",
				Version:      "1.0.0",
				Resources:    []ResourceCapability{},
			},
		},
		{
			name: "full response",
			resp: CapabilitiesResponse{
				ProviderName: "libvirt",
				Version:      "2.0.0",
				Resources: []ResourceCapability{
					{
						Kind:       "vm",
						Operations: []string{"create", "get", "list", "delete"},
						VMFeatures: []string{"cloud-init", "uefi", "virtio-fs"},
					},
					{
						Kind:         "network",
						Operations:   []string{"create", "get", "list", "delete"},
						NetworkKinds: []string{"bridge", "nat", "isolated"},
					},
					{
						Kind:       "key",
						Operations: []string{"create", "get", "list", "delete"},
						KeyTypes:   []string{"rsa", "ed25519", "ecdsa"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got CapabilitiesResponse
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.resp, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.resp, got)
			}
		})
	}
}

func TestGetRequest_JSONRoundtrip(t *testing.T) {
	req := GetRequest{Name: "test-resource"}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got GetRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(req, got) {
		t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", req, got)
	}
}

func TestListRequest_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		req  ListRequest
	}{
		{
			name: "empty filter",
			req:  ListRequest{},
		},
		{
			name: "with filter",
			req: ListRequest{
				Filter: map[string]any{
					"status": "running",
					"tag":    "production",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got ListRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.req, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.req, got)
			}
		})
	}
}

func TestDeleteRequest_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		req  DeleteRequest
	}{
		{
			name: "simple delete",
			req: DeleteRequest{
				Name: "test-resource",
			},
		},
		{
			name: "force delete",
			req: DeleteRequest{
				Name:  "test-resource",
				Force: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got DeleteRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.req, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.req, got)
			}
		})
	}
}

func TestListResult_JSONRoundtrip(t *testing.T) {
	tests := []struct {
		name   string
		result ListResult
	}{
		{
			name: "success with resources",
			result: ListResult{
				Success: true,
				Resources: []any{
					map[string]any{"name": "vm1", "status": "running"},
					map[string]any{"name": "vm2", "status": "stopped"},
				},
			},
		},
		{
			name: "failure",
			result: ListResult{
				Success: false,
				Error: &OperationError{
					Code:    ErrCodeProviderError,
					Message: "failed to list",
				},
			},
		},
		{
			name: "empty list",
			result: ListResult{
				Success:   true,
				Resources: nil, // nil and empty slice both marshal to [], but unmarshal to nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got ListResult
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(tt.result, got) {
				t.Errorf("Roundtrip mismatch:\noriginal: %+v\ngot:      %+v", tt.result, got)
			}
		})
	}
}

func TestOperationResult_JSONFieldNames(t *testing.T) {
	result := OperationResult{
		Success: true,
		Error: &OperationError{
			Code:      "ERR",
			Message:   "msg",
			Retryable: true,
			Details:   map[string]any{"k": "v"},
		},
		Resource: map[string]any{"name": "test"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{"success", "error", "resource"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}

	// Check error field names
	errData := raw["error"].(map[string]any)
	errFields := []string{"code", "message", "retryable", "details"}
	for _, field := range errFields {
		if _, ok := errData[field]; !ok {
			t.Errorf("Expected error JSON field %q not found", field)
		}
	}
}

func TestCapabilitiesResponse_JSONFieldNames(t *testing.T) {
	resp := CapabilitiesResponse{
		ProviderName: "test",
		Version:      "1.0",
		Resources: []ResourceCapability{
			{
				Kind:         "vm",
				Operations:   []string{"create"},
				NetworkKinds: []string{"bridge"},
				KeyTypes:     []string{"rsa"},
				VMFeatures:   []string{"uefi"},
			},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{"providerName", "version", "resources"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("Expected JSON field %q not found", field)
		}
	}

	// Check resource capability field names
	resources := raw["resources"].([]any)
	if len(resources) > 0 {
		resData := resources[0].(map[string]any)
		resFields := []string{"kind", "operations", "networkKinds", "keyTypes", "vmFeatures"}
		for _, field := range resFields {
			if _, ok := resData[field]; !ok {
				t.Errorf("Expected resource JSON field %q not found", field)
			}
		}
	}
}

func TestErrorCodes(t *testing.T) {
	// Verify error code constants have expected values
	tests := []struct {
		constant string
		value    string
	}{
		{ErrCodeNotImplemented, "NOT_IMPLEMENTED"},
		{ErrCodeNotFound, "NOT_FOUND"},
		{ErrCodeAlreadyExists, "ALREADY_EXISTS"},
		{ErrCodeInvalidSpec, "INVALID_SPEC"},
		{ErrCodeProviderError, "PROVIDER_ERROR"},
		{ErrCodeTimeout, "TIMEOUT"},
		{ErrCodePermissionDenied, "PERMISSION_DENIED"},
		{ErrCodeResourceBusy, "RESOURCE_BUSY"},
		{ErrCodeDependencyFailed, "DEPENDENCY_FAILED"},
	}

	for _, tt := range tests {
		if tt.constant != tt.value {
			t.Errorf("Error code mismatch: expected %q, got %q", tt.value, tt.constant)
		}
	}
}
