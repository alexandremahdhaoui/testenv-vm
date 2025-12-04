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

// Package provider implements MCP client communication with provider processes.
package provider

import (
	"testing"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

// TestRegisterCapabilities tests the RegisterCapabilities method.
func TestRegisterCapabilities(t *testing.T) {
	t.Run("register for new provider", func(t *testing.T) {
		m := NewManager()
		caps := &providerv1.CapabilitiesResponse{
			ProviderName: "test-provider",
			Version:      "1.0.0",
			Resources: []providerv1.ResourceCapability{
				{
					Kind:       "vm",
					Operations: []string{"create", "get", "list", "delete"},
				},
			},
		}

		m.RegisterCapabilities("test-provider", caps)

		info, exists := m.GetInfo("test-provider")
		if !exists {
			t.Fatal("expected provider to exist after RegisterCapabilities")
		}
		if info.Capabilities != caps {
			t.Error("capabilities not stored correctly")
		}
		if info.Status != StatusStopped {
			t.Errorf("expected status=stopped for new provider, got %s", info.Status)
		}
	})

	t.Run("update existing provider capabilities", func(t *testing.T) {
		m := NewManager()

		// First, add a provider
		m.mu.Lock()
		m.providers["test-provider"] = &ProviderInfo{
			Status: StatusRunning,
		}
		m.mu.Unlock()

		caps := &providerv1.CapabilitiesResponse{
			ProviderName: "test-provider",
			Version:      "2.0.0",
		}

		m.RegisterCapabilities("test-provider", caps)

		info, _ := m.GetInfo("test-provider")
		if info.Capabilities != caps {
			t.Error("capabilities not updated correctly")
		}
		// Status should remain unchanged
		if info.Status != StatusRunning {
			t.Errorf("status should remain running, got %s", info.Status)
		}
	})
}

// TestSupportsResource tests the SupportsResource method.
func TestSupportsResource(t *testing.T) {
	m := NewManager()

	// Set up a provider with capabilities
	m.mu.Lock()
	m.providers["vm-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "vm-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "vm", Operations: []string{"create", "delete"}},
				{Kind: "network", Operations: []string{"create"}},
			},
		},
	}
	m.providers["key-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "key-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "key", Operations: []string{"create", "delete"}},
			},
		},
	}
	m.providers["no-caps-provider"] = &ProviderInfo{
		Status:       StatusRunning,
		Capabilities: nil,
	}
	m.mu.Unlock()

	tests := []struct {
		name     string
		provider string
		kind     string
		expected bool
	}{
		{"vm-provider supports vm", "vm-provider", "vm", true},
		{"vm-provider supports network", "vm-provider", "network", true},
		{"vm-provider does not support key", "vm-provider", "key", false},
		{"key-provider supports key", "key-provider", "key", true},
		{"key-provider does not support vm", "key-provider", "vm", false},
		{"non-existent provider", "non-existent", "vm", false},
		{"provider with nil capabilities", "no-caps-provider", "vm", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := m.SupportsResource(tc.provider, tc.kind)
			if result != tc.expected {
				t.Errorf("SupportsResource(%s, %s) = %v, expected %v",
					tc.provider, tc.kind, result, tc.expected)
			}
		})
	}
}

// TestSupportsOperation tests the SupportsOperation method.
func TestSupportsOperation(t *testing.T) {
	m := NewManager()

	// Set up a provider with capabilities
	m.mu.Lock()
	m.providers["test-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "test-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "vm", Operations: []string{"create", "get", "delete"}},
				{Kind: "network", Operations: []string{"create"}},
			},
		},
	}
	m.providers["no-caps-provider"] = &ProviderInfo{
		Status:       StatusRunning,
		Capabilities: nil,
	}
	m.mu.Unlock()

	tests := []struct {
		name      string
		provider  string
		kind      string
		operation string
		expected  bool
	}{
		{"vm create supported", "test-provider", "vm", "create", true},
		{"vm get supported", "test-provider", "vm", "get", true},
		{"vm delete supported", "test-provider", "vm", "delete", true},
		{"vm list not supported", "test-provider", "vm", "list", false},
		{"network create supported", "test-provider", "network", "create", true},
		{"network delete not supported", "test-provider", "network", "delete", false},
		{"key kind not supported", "test-provider", "key", "create", false},
		{"non-existent provider", "non-existent", "vm", "create", false},
		{"nil capabilities", "no-caps-provider", "vm", "create", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := m.SupportsOperation(tc.provider, tc.kind, tc.operation)
			if result != tc.expected {
				t.Errorf("SupportsOperation(%s, %s, %s) = %v, expected %v",
					tc.provider, tc.kind, tc.operation, result, tc.expected)
			}
		})
	}
}

// TestGetProviderForResource tests the GetProviderForResource method.
func TestGetProviderForResource(t *testing.T) {
	m := NewManager()

	// Set up providers with different capabilities
	m.mu.Lock()
	m.providers["vm-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "vm-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "vm", Operations: []string{"create"}},
			},
		},
	}
	m.providers["network-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "network-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "network", Operations: []string{"create"}},
			},
		},
	}
	m.providers["multi-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "multi-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "vm", Operations: []string{"create"}},
				{Kind: "network", Operations: []string{"create"}},
				{Kind: "key", Operations: []string{"create"}},
			},
		},
	}
	m.providers["no-caps-provider"] = &ProviderInfo{
		Status:       StatusRunning,
		Capabilities: nil,
	}
	m.mu.Unlock()

	tests := []struct {
		name            string
		kind            string
		providers       []v1.ProviderConfig
		defaultProvider string
		expected        string
	}{
		{
			name: "find provider that supports vm",
			kind: "vm",
			providers: []v1.ProviderConfig{
				{Name: "network-provider"},
				{Name: "vm-provider"},
			},
			expected: "vm-provider",
		},
		{
			name: "find first provider that supports resource",
			kind: "vm",
			providers: []v1.ProviderConfig{
				{Name: "vm-provider"},
				{Name: "multi-provider"},
			},
			expected: "vm-provider", // First one found
		},
		{
			name: "fallback to default when no explicit support",
			kind: "storage", // Not supported by any provider
			providers: []v1.ProviderConfig{
				{Name: "vm-provider"},
				{Name: "network-provider"},
			},
			defaultProvider: "vm-provider",
			expected:        "vm-provider",
		},
		{
			name:            "default provider not in list",
			kind:            "storage",
			providers:       []v1.ProviderConfig{{Name: "vm-provider"}},
			defaultProvider: "other-provider",
			expected:        "", // Default not in list
		},
		{
			name:     "no providers match",
			kind:     "storage",
			providers: []v1.ProviderConfig{
				{Name: "vm-provider"},
			},
			expected: "",
		},
		{
			name: "provider not registered in manager",
			kind: "vm",
			providers: []v1.ProviderConfig{
				{Name: "unregistered-provider"},
			},
			expected: "",
		},
		{
			name: "provider with nil capabilities",
			kind: "vm",
			providers: []v1.ProviderConfig{
				{Name: "no-caps-provider"},
			},
			expected: "",
		},
		{
			name:     "empty provider list",
			kind:     "vm",
			providers: []v1.ProviderConfig{},
			expected: "",
		},
		{
			name:            "empty provider list with default",
			kind:            "vm",
			providers:       []v1.ProviderConfig{},
			defaultProvider: "vm-provider",
			expected:        "", // Default not in empty list
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := m.GetProviderForResource(tc.kind, tc.providers, tc.defaultProvider)
			if result != tc.expected {
				t.Errorf("GetProviderForResource(%s, ..., %s) = %q, expected %q",
					tc.kind, tc.defaultProvider, result, tc.expected)
			}
		})
	}
}

// TestRegistryConcurrentAccess tests concurrent access to registry methods.
func TestRegistryConcurrentAccess(t *testing.T) {
	m := NewManager()

	// Set up initial state
	caps := &providerv1.CapabilitiesResponse{
		ProviderName: "test-provider",
		Resources: []providerv1.ResourceCapability{
			{Kind: "vm", Operations: []string{"create"}},
		},
	}
	m.RegisterCapabilities("test-provider", caps)

	providers := []v1.ProviderConfig{{Name: "test-provider"}}

	done := make(chan bool)

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			m.SupportsResource("test-provider", "vm")
			m.SupportsOperation("test-provider", "vm", "create")
			m.GetProviderForResource("vm", providers, "")
		}
		done <- true
	}()

	// Concurrent writes
	go func() {
		for i := 0; i < 100; i++ {
			m.RegisterCapabilities("test-provider", caps)
		}
		done <- true
	}()

	<-done
	<-done
}

// TestSupportsResourceEdgeCases tests edge cases for SupportsResource.
func TestSupportsResourceEdgeCases(t *testing.T) {
	m := NewManager()

	t.Run("empty resources list", func(t *testing.T) {
		m.mu.Lock()
		m.providers["empty-provider"] = &ProviderInfo{
			Status: StatusRunning,
			Capabilities: &providerv1.CapabilitiesResponse{
				ProviderName: "empty-provider",
				Resources:    []providerv1.ResourceCapability{},
			},
		}
		m.mu.Unlock()

		if m.SupportsResource("empty-provider", "vm") {
			t.Error("should not support any resource with empty resources list")
		}
	})

	t.Run("empty kind string", func(t *testing.T) {
		m.mu.Lock()
		m.providers["test-provider"] = &ProviderInfo{
			Status: StatusRunning,
			Capabilities: &providerv1.CapabilitiesResponse{
				ProviderName: "test-provider",
				Resources: []providerv1.ResourceCapability{
					{Kind: "vm", Operations: []string{"create"}},
				},
			},
		}
		m.mu.Unlock()

		if m.SupportsResource("test-provider", "") {
			t.Error("should not support empty kind")
		}
	})
}

// TestSupportsOperationEdgeCases tests edge cases for SupportsOperation.
func TestSupportsOperationEdgeCases(t *testing.T) {
	m := NewManager()

	m.mu.Lock()
	m.providers["test-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "test-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "vm", Operations: []string{}}, // Empty operations
			},
		},
	}
	m.mu.Unlock()

	t.Run("empty operations list", func(t *testing.T) {
		if m.SupportsOperation("test-provider", "vm", "create") {
			t.Error("should not support any operation with empty operations list")
		}
	})

	t.Run("empty operation string", func(t *testing.T) {
		if m.SupportsOperation("test-provider", "vm", "") {
			t.Error("should not support empty operation")
		}
	})
}

// TestGetProviderForResourcePriority tests the priority of provider selection.
func TestGetProviderForResourcePriority(t *testing.T) {
	m := NewManager()

	// Set up multiple providers that support the same resource
	m.mu.Lock()
	m.providers["first-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "first-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "vm", Operations: []string{"create"}},
			},
		},
	}
	m.providers["second-provider"] = &ProviderInfo{
		Status: StatusRunning,
		Capabilities: &providerv1.CapabilitiesResponse{
			ProviderName: "second-provider",
			Resources: []providerv1.ResourceCapability{
				{Kind: "vm", Operations: []string{"create"}},
			},
		},
	}
	m.mu.Unlock()

	// The first provider in the list that supports the resource should be returned
	providers := []v1.ProviderConfig{
		{Name: "second-provider"}, // Listed first
		{Name: "first-provider"},
	}

	result := m.GetProviderForResource("vm", providers, "")
	if result != "second-provider" {
		t.Errorf("expected second-provider (first in list), got %s", result)
	}

	// Reverse the order
	providers = []v1.ProviderConfig{
		{Name: "first-provider"}, // Now listed first
		{Name: "second-provider"},
	}

	result = m.GetProviderForResource("vm", providers, "")
	if result != "first-provider" {
		t.Errorf("expected first-provider (first in list), got %s", result)
	}
}
