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
	"strings"
	"testing"
)

func TestNewOperationError(t *testing.T) {
	tests := []struct {
		name            string
		code            string
		message         string
		wantRetryable   bool
	}{
		{
			name:          "retryable timeout",
			code:          ErrCodeTimeout,
			message:       "operation timed out",
			wantRetryable: true,
		},
		{
			name:          "retryable resource busy",
			code:          ErrCodeResourceBusy,
			message:       "resource is busy",
			wantRetryable: true,
		},
		{
			name:          "non-retryable not found",
			code:          ErrCodeNotFound,
			message:       "resource not found",
			wantRetryable: false,
		},
		{
			name:          "non-retryable invalid spec",
			code:          ErrCodeInvalidSpec,
			message:       "invalid specification",
			wantRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewOperationError(tt.code, tt.message)

			if err.Code != tt.code {
				t.Errorf("Code = %q, want %q", err.Code, tt.code)
			}
			if err.Message != tt.message {
				t.Errorf("Message = %q, want %q", err.Message, tt.message)
			}
			if err.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", err.Retryable, tt.wantRetryable)
			}
		})
	}
}

func TestNewOperationErrorWithDetails(t *testing.T) {
	details := map[string]any{
		"host": "192.168.1.1",
		"port": 22,
	}

	err := NewOperationErrorWithDetails(ErrCodeProviderError, "connection failed", details)

	if err.Code != ErrCodeProviderError {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeProviderError)
	}
	if err.Message != "connection failed" {
		t.Errorf("Message = %q, want %q", err.Message, "connection failed")
	}
	if err.Details["host"] != "192.168.1.1" {
		t.Errorf("Details[host] = %v, want %v", err.Details["host"], "192.168.1.1")
	}
}

func TestNewNotImplementedError(t *testing.T) {
	err := NewNotImplementedError("vm", "resize")

	if err.Code != ErrCodeNotImplemented {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeNotImplemented)
	}
	if !strings.Contains(err.Message, "vm") {
		t.Errorf("Message should contain 'vm': %q", err.Message)
	}
	if !strings.Contains(err.Message, "resize") {
		t.Errorf("Message should contain 'resize': %q", err.Message)
	}
	if err.Retryable {
		t.Error("NotImplementedError should not be retryable")
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("vm", "test-vm")

	if err.Code != ErrCodeNotFound {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeNotFound)
	}
	if !strings.Contains(err.Message, "vm") {
		t.Errorf("Message should contain 'vm': %q", err.Message)
	}
	if !strings.Contains(err.Message, "test-vm") {
		t.Errorf("Message should contain 'test-vm': %q", err.Message)
	}
	if err.Retryable {
		t.Error("NotFoundError should not be retryable")
	}
}

func TestNewAlreadyExistsError(t *testing.T) {
	err := NewAlreadyExistsError("network", "prod-net")

	if err.Code != ErrCodeAlreadyExists {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeAlreadyExists)
	}
	if !strings.Contains(err.Message, "network") {
		t.Errorf("Message should contain 'network': %q", err.Message)
	}
	if !strings.Contains(err.Message, "prod-net") {
		t.Errorf("Message should contain 'prod-net': %q", err.Message)
	}
	if err.Retryable {
		t.Error("AlreadyExistsError should not be retryable")
	}
}

func TestNewInvalidSpecError(t *testing.T) {
	err := NewInvalidSpecError("memory must be positive")

	if err.Code != ErrCodeInvalidSpec {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeInvalidSpec)
	}
	if err.Message != "memory must be positive" {
		t.Errorf("Message = %q, want %q", err.Message, "memory must be positive")
	}
	if err.Retryable {
		t.Error("InvalidSpecError should not be retryable")
	}
}

func TestNewProviderError(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		retryable bool
	}{
		{
			name:      "retryable provider error",
			message:   "temporary network issue",
			retryable: true,
		},
		{
			name:      "non-retryable provider error",
			message:   "unsupported configuration",
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewProviderError(tt.message, tt.retryable)

			if err.Code != ErrCodeProviderError {
				t.Errorf("Code = %q, want %q", err.Code, ErrCodeProviderError)
			}
			if err.Message != tt.message {
				t.Errorf("Message = %q, want %q", err.Message, tt.message)
			}
			if err.Retryable != tt.retryable {
				t.Errorf("Retryable = %v, want %v", err.Retryable, tt.retryable)
			}
		})
	}
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("create_vm")

	if err.Code != ErrCodeTimeout {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeTimeout)
	}
	if !strings.Contains(err.Message, "create_vm") {
		t.Errorf("Message should contain 'create_vm': %q", err.Message)
	}
	if !err.Retryable {
		t.Error("TimeoutError should be retryable")
	}
}

func TestNewPermissionDeniedError(t *testing.T) {
	err := NewPermissionDeniedError("insufficient privileges")

	if err.Code != ErrCodePermissionDenied {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodePermissionDenied)
	}
	if err.Message != "insufficient privileges" {
		t.Errorf("Message = %q, want %q", err.Message, "insufficient privileges")
	}
	if err.Retryable {
		t.Error("PermissionDeniedError should not be retryable")
	}
}

func TestNewResourceBusyError(t *testing.T) {
	err := NewResourceBusyError("vm", "test-vm")

	if err.Code != ErrCodeResourceBusy {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeResourceBusy)
	}
	if !strings.Contains(err.Message, "vm") {
		t.Errorf("Message should contain 'vm': %q", err.Message)
	}
	if !strings.Contains(err.Message, "test-vm") {
		t.Errorf("Message should contain 'test-vm': %q", err.Message)
	}
	if !err.Retryable {
		t.Error("ResourceBusyError should be retryable")
	}
}

func TestNewDependencyFailedError(t *testing.T) {
	err := NewDependencyFailedError("vm", "network")

	if err.Code != ErrCodeDependencyFailed {
		t.Errorf("Code = %q, want %q", err.Code, ErrCodeDependencyFailed)
	}
	if !strings.Contains(err.Message, "vm") {
		t.Errorf("Message should contain 'vm': %q", err.Message)
	}
	if !strings.Contains(err.Message, "network") {
		t.Errorf("Message should contain 'network': %q", err.Message)
	}
	if err.Retryable {
		t.Error("DependencyFailedError should not be retryable")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  *OperationError
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "retryable error",
			err:  &OperationError{Retryable: true},
			want: true,
		},
		{
			name: "non-retryable error",
			err:  &OperationError{Retryable: false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRetryableCode(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{ErrCodeTimeout, true},
		{ErrCodeResourceBusy, true},
		{ErrCodeNotFound, false},
		{ErrCodeNotImplemented, false},
		{ErrCodeAlreadyExists, false},
		{ErrCodeInvalidSpec, false},
		{ErrCodeProviderError, false},
		{ErrCodePermissionDenied, false},
		{ErrCodeDependencyFailed, false},
		{"UNKNOWN_CODE", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := IsRetryableCode(tt.code); got != tt.want {
				t.Errorf("IsRetryableCode(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestErrorResult(t *testing.T) {
	opErr := &OperationError{
		Code:    ErrCodeNotFound,
		Message: "not found",
	}

	result := ErrorResult(opErr)

	if result.Success {
		t.Error("ErrorResult should have Success = false")
	}
	if result.Error != opErr {
		t.Error("ErrorResult should contain the provided error")
	}
	if result.Resource != nil {
		t.Error("ErrorResult should have nil Resource")
	}
}

func TestSuccessResult(t *testing.T) {
	resource := &VMState{
		Name:   "test-vm",
		Status: "running",
	}

	result := SuccessResult(resource)

	if !result.Success {
		t.Error("SuccessResult should have Success = true")
	}
	if result.Error != nil {
		t.Error("SuccessResult should have nil Error")
	}
	if result.Resource != resource {
		t.Error("SuccessResult should contain the provided resource")
	}
}

func TestSuccessResultWithNilResource(t *testing.T) {
	result := SuccessResult(nil)

	if !result.Success {
		t.Error("SuccessResult should have Success = true")
	}
	if result.Error != nil {
		t.Error("SuccessResult should have nil Error")
	}
	if result.Resource != nil {
		t.Error("SuccessResult with nil should have nil Resource")
	}
}
