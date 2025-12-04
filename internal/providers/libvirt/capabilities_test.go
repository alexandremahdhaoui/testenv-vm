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

package libvirt

import (
	"testing"
)

func TestCapabilities(t *testing.T) {
	// Create a minimal provider for testing capabilities
	// The Capabilities method doesn't use the connection or config
	p := &Provider{}

	caps := p.Capabilities()

	// Verify provider name
	if caps.ProviderName != "libvirt" {
		t.Errorf("Expected provider name 'libvirt', got %s", caps.ProviderName)
	}

	// Verify version is set
	if caps.Version == "" {
		t.Error("Version should not be empty")
	}

	// Verify we have 3 resources: key, network, vm
	if len(caps.Resources) != 3 {
		t.Errorf("Expected 3 resources, got %d", len(caps.Resources))
	}

	// Verify each resource type and operations
	expectedResources := map[string][]string{
		"key":     {"create", "get", "list", "delete"},
		"network": {"create", "get", "list", "delete"},
		"vm":      {"create", "get", "list", "delete"},
	}

	for _, res := range caps.Resources {
		expectedOps, ok := expectedResources[res.Kind]
		if !ok {
			t.Errorf("Unexpected resource kind: %s", res.Kind)
			continue
		}

		if len(res.Operations) != len(expectedOps) {
			t.Errorf("Resource %s: expected %d operations, got %d", res.Kind, len(expectedOps), len(res.Operations))
		}

		// Verify all expected operations are present
		opsSet := make(map[string]bool)
		for _, op := range res.Operations {
			opsSet[op] = true
		}
		for _, expectedOp := range expectedOps {
			if !opsSet[expectedOp] {
				t.Errorf("Resource %s: missing operation %s", res.Kind, expectedOp)
			}
		}
	}
}
