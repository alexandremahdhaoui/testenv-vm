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

package provider

import (
	"strings"
	"sync"
	"testing"

	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
)

// TestMockProviderImplementsInterface verifies MockProvider implements ClientProvider.
func TestMockProviderImplementsInterface(t *testing.T) {
	// Compile-time check
	var _ client.ClientProvider = (*MockProvider)(nil)

	// Runtime check
	p := NewMockProvider()
	if p == nil {
		t.Fatal("NewMockProvider returned nil")
	}
}

// TestNewMockProvider verifies NewMockProvider returns empty provider.
func TestNewMockProvider(t *testing.T) {
	p := NewMockProvider()
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.vms == nil {
		t.Error("expected vms map to be initialized")
	}
	if len(p.vms) != 0 {
		t.Errorf("expected empty vms map, got %d entries", len(p.vms))
	}
}

// TestAddVM verifies AddVM adds VM info correctly.
func TestAddVM(t *testing.T) {
	p := NewMockProvider()

	vmInfo := &client.VMInfo{
		Host:       "192.168.1.100",
		Port:       "22",
		User:       "testuser",
		PrivateKey: []byte("test-private-key"),
	}

	p.AddVM("test-vm", vmInfo)

	// Verify VM was added
	info, err := p.GetVMInfo("test-vm")
	if err != nil {
		t.Fatalf("GetVMInfo failed: %v", err)
	}
	if info != vmInfo {
		t.Error("expected same VMInfo pointer")
	}
}

// TestGetVMInfo verifies GetVMInfo returns added VM.
func TestGetVMInfo(t *testing.T) {
	p := NewMockProvider()

	vmInfo := &client.VMInfo{
		Host:       "10.0.0.5",
		Port:       "2222",
		User:       "admin",
		PrivateKey: []byte("ssh-private-key-content"),
	}

	p.AddVM("my-vm", vmInfo)

	info, err := p.GetVMInfo("my-vm")
	if err != nil {
		t.Fatalf("GetVMInfo failed: %v", err)
	}
	if info.Host != "10.0.0.5" {
		t.Errorf("expected Host '10.0.0.5', got %q", info.Host)
	}
	if info.Port != "2222" {
		t.Errorf("expected Port '2222', got %q", info.Port)
	}
	if info.User != "admin" {
		t.Errorf("expected User 'admin', got %q", info.User)
	}
	if string(info.PrivateKey) != "ssh-private-key-content" {
		t.Errorf("expected PrivateKey 'ssh-private-key-content', got %q", string(info.PrivateKey))
	}
}

// TestGetVMInfoUnknownVM verifies GetVMInfo returns error for unknown VM.
func TestGetVMInfoUnknownVM(t *testing.T) {
	p := NewMockProvider()

	_, err := p.GetVMInfo("unknown-vm")
	if err == nil {
		t.Fatal("expected error for unknown VM, got nil")
	}

	// Verify error message contains VM name
	if !strings.Contains(err.Error(), "VM not found") {
		t.Errorf("expected error to contain 'VM not found', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "unknown-vm") {
		t.Errorf("expected error to contain 'unknown-vm', got %q", err.Error())
	}
}

// TestMultipleVMs verifies multiple VMs can be added and retrieved.
func TestMultipleVMs(t *testing.T) {
	p := NewMockProvider()

	vm1 := &client.VMInfo{
		Host:       "192.168.1.10",
		Port:       "22",
		User:       "user1",
		PrivateKey: []byte("key1"),
	}

	vm2 := &client.VMInfo{
		Host:       "192.168.1.20",
		Port:       "22",
		User:       "user2",
		PrivateKey: []byte("key2"),
	}

	vm3 := &client.VMInfo{
		Host:       "192.168.1.30",
		Port:       "2222",
		User:       "user3",
		PrivateKey: []byte("key3"),
	}

	p.AddVM("vm1", vm1)
	p.AddVM("vm2", vm2)
	p.AddVM("vm3", vm3)

	// Verify all VMs can be retrieved
	info1, err := p.GetVMInfo("vm1")
	if err != nil {
		t.Fatalf("GetVMInfo(vm1) failed: %v", err)
	}
	if info1.Host != "192.168.1.10" {
		t.Errorf("vm1: expected Host '192.168.1.10', got %q", info1.Host)
	}

	info2, err := p.GetVMInfo("vm2")
	if err != nil {
		t.Fatalf("GetVMInfo(vm2) failed: %v", err)
	}
	if info2.Host != "192.168.1.20" {
		t.Errorf("vm2: expected Host '192.168.1.20', got %q", info2.Host)
	}

	info3, err := p.GetVMInfo("vm3")
	if err != nil {
		t.Fatalf("GetVMInfo(vm3) failed: %v", err)
	}
	if info3.Host != "192.168.1.30" {
		t.Errorf("vm3: expected Host '192.168.1.30', got %q", info3.Host)
	}
	if info3.Port != "2222" {
		t.Errorf("vm3: expected Port '2222', got %q", info3.Port)
	}
}

// TestConcurrentAccess verifies thread-safety of MockProvider.
func TestConcurrentAccess(t *testing.T) {
	p := NewMockProvider()

	var wg sync.WaitGroup

	// Add VMs concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			vmInfo := &client.VMInfo{
				Host:       "192.168.1.100",
				Port:       "22",
				User:       "user",
				PrivateKey: []byte("key"),
			}
			p.AddVM(string(rune('a'+idx)), vmInfo)
		}(i)
	}

	wg.Wait()

	// Read VMs concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = p.GetVMInfo(string(rune('a' + idx)))
		}(i)
	}

	wg.Wait()
}
