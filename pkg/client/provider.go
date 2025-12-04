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

// ClientProvider abstracts how VM connection information is obtained.
// Different implementations can get VM info from different sources:
// - ArtifactProvider: from TestEnvArtifact (in pkg/client/provider/)
// - MockProvider: preconfigured for testing (in pkg/client/provider/)
// - LibvirtProvider: queries libvirt directly (in pkg/client/provider/)
//
// NOTE: This interface is defined in pkg/client to avoid circular imports.
// Implementations live in pkg/client/provider/ and import this interface.
type ClientProvider interface {
	// GetVMInfo returns connection information for a VM by name.
	// Returns error if VM not found or info unavailable.
	GetVMInfo(vmName string) (*VMInfo, error)
}
