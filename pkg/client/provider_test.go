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

import "testing"

// Compile-time check that implementations satisfy interface
var _ ClientProvider = (*testProvider)(nil)

// testProvider is a minimal implementation for testing.
type testProvider struct{}

func (t *testProvider) GetVMInfo(vmName string) (*VMInfo, error) {
	return &VMInfo{
		Host:       "192.168.1.100",
		Port:       "22",
		User:       "testuser",
		PrivateKey: []byte("fake-key"),
	}, nil
}

// TestClientProviderInterface verifies the interface is correctly defined.
func TestClientProviderInterface(t *testing.T) {
	var provider ClientProvider = &testProvider{}
	vmInfo, err := provider.GetVMInfo("test-vm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vmInfo.Host != "192.168.1.100" {
		t.Errorf("expected host 192.168.1.100, got %s", vmInfo.Host)
	}
	if vmInfo.Port != "22" {
		t.Errorf("expected port 22, got %s", vmInfo.Port)
	}
	if vmInfo.User != "testuser" {
		t.Errorf("expected user testuser, got %s", vmInfo.User)
	}
	if string(vmInfo.PrivateKey) != "fake-key" {
		t.Errorf("expected PrivateKey 'fake-key', got %s", string(vmInfo.PrivateKey))
	}
}
