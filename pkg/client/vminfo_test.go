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
	"strings"
	"testing"
)

// TestVMInfoValidate_Valid tests that a valid VMInfo passes validation.
func TestVMInfoValidate_Valid(t *testing.T) {
	vmInfo := &VMInfo{
		Host:       "192.168.1.100",
		Port:       "22",
		User:       "testuser",
		PrivateKey: []byte("test-private-key-content"),
	}

	err := vmInfo.Validate()
	if err != nil {
		t.Errorf("expected no error for valid VMInfo, got: %v", err)
	}
}

// TestVMInfoValidate_EmptyHost tests that empty Host returns an error.
func TestVMInfoValidate_EmptyHost(t *testing.T) {
	vmInfo := &VMInfo{
		Host:       "",
		Port:       "22",
		User:       "testuser",
		PrivateKey: []byte("test-private-key-content"),
	}

	err := vmInfo.Validate()
	if err == nil {
		t.Error("expected error for empty Host, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "Host is required") {
		t.Errorf("expected error message to contain 'Host is required', got: %v", err)
	}
}

// TestVMInfoValidate_EmptyPort tests that empty Port returns an error.
func TestVMInfoValidate_EmptyPort(t *testing.T) {
	vmInfo := &VMInfo{
		Host:       "192.168.1.100",
		Port:       "",
		User:       "testuser",
		PrivateKey: []byte("test-private-key-content"),
	}

	err := vmInfo.Validate()
	if err == nil {
		t.Error("expected error for empty Port, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "Port is required") {
		t.Errorf("expected error message to contain 'Port is required', got: %v", err)
	}
}

// TestVMInfoValidate_EmptyUser tests that empty User returns an error.
func TestVMInfoValidate_EmptyUser(t *testing.T) {
	vmInfo := &VMInfo{
		Host:       "192.168.1.100",
		Port:       "22",
		User:       "",
		PrivateKey: []byte("test-private-key-content"),
	}

	err := vmInfo.Validate()
	if err == nil {
		t.Error("expected error for empty User, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "User is required") {
		t.Errorf("expected error message to contain 'User is required', got: %v", err)
	}
}

// TestVMInfoValidate_NilPrivateKey tests that nil PrivateKey returns an error.
func TestVMInfoValidate_NilPrivateKey(t *testing.T) {
	vmInfo := &VMInfo{
		Host:       "192.168.1.100",
		Port:       "22",
		User:       "testuser",
		PrivateKey: nil,
	}

	err := vmInfo.Validate()
	if err == nil {
		t.Error("expected error for nil PrivateKey, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "PrivateKey is required") {
		t.Errorf("expected error message to contain 'PrivateKey is required', got: %v", err)
	}
}

// TestVMInfoValidate_EmptyPrivateKey tests that empty PrivateKey (len=0) returns an error.
func TestVMInfoValidate_EmptyPrivateKey(t *testing.T) {
	vmInfo := &VMInfo{
		Host:       "192.168.1.100",
		Port:       "22",
		User:       "testuser",
		PrivateKey: []byte{},
	}

	err := vmInfo.Validate()
	if err == nil {
		t.Error("expected error for empty PrivateKey, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "PrivateKey is required") {
		t.Errorf("expected error message to contain 'PrivateKey is required', got: %v", err)
	}
}
