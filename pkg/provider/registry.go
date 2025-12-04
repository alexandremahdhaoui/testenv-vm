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
	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

// RegisterCapabilities stores capabilities for a provider.
// This is typically called during provider Start() after fetching capabilities,
// but can also be used to update capabilities at runtime.
func (m *Manager) RegisterCapabilities(name string, caps *providerv1.CapabilitiesResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.providers[name]
	if !exists {
		// Create a new provider info entry if it doesn't exist
		info = &ProviderInfo{
			Status: StatusStopped,
		}
		m.providers[name] = info
	}

	info.Capabilities = caps
}

// SupportsResource checks if a provider supports a specific resource kind.
// Returns true if the provider has the resource kind in its capabilities.
func (m *Manager) SupportsResource(provider string, kind string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.providers[provider]
	if !exists {
		return false
	}

	if info.Capabilities == nil {
		return false
	}

	for _, res := range info.Capabilities.Resources {
		if res.Kind == kind {
			return true
		}
	}

	return false
}

// SupportsOperation checks if a provider supports a specific operation on a resource kind.
// Returns true if the provider has both the resource kind AND the operation in its capabilities.
func (m *Manager) SupportsOperation(provider, kind, operation string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.providers[provider]
	if !exists {
		return false
	}

	if info.Capabilities == nil {
		return false
	}

	for _, res := range info.Capabilities.Resources {
		if res.Kind == kind {
			for _, op := range res.Operations {
				if op == operation {
					return true
				}
			}
			return false
		}
	}

	return false
}

// GetProviderForResource selects the best provider for a resource kind.
// The selection logic is:
// 1. First, check if any provider in the list explicitly supports the kind
// 2. If no provider explicitly supports it, fall back to the default provider
// 3. Return empty string if no suitable provider is found
func (m *Manager) GetProviderForResource(kind string, providers []v1.ProviderConfig, defaultProvider string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// First pass: find a provider that explicitly supports this resource kind
	for _, pc := range providers {
		info, exists := m.providers[pc.Name]
		if !exists {
			continue
		}

		if info.Capabilities == nil {
			continue
		}

		for _, res := range info.Capabilities.Resources {
			if res.Kind == kind {
				return pc.Name
			}
		}
	}

	// Second pass: check if the default provider exists and is in the list
	if defaultProvider != "" {
		// Verify the default provider is in the provided list
		for _, pc := range providers {
			if pc.Name == defaultProvider {
				return defaultProvider
			}
		}
	}

	// No suitable provider found
	return ""
}
