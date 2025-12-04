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
	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// Capabilities returns the capabilities of the libvirt provider.
// This includes supported resource types, operations, and provider-specific features.
func (p *Provider) Capabilities() *providerv1.CapabilitiesResponse {
	return &providerv1.CapabilitiesResponse{
		ProviderName: "libvirt",
		Version:      "1.0.0",
		Resources: []providerv1.ResourceCapability{
			{
				Kind:       "key",
				Operations: []string{"create", "get", "list", "delete"},
			},
			{
				Kind:       "network",
				Operations: []string{"create", "get", "list", "delete"},
			},
			{
				Kind:       "vm",
				Operations: []string{"create", "get", "list", "delete"},
			},
		},
	}
}
