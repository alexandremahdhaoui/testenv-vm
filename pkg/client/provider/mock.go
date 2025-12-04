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
	"fmt"
	"sync"

	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
)

// MockProvider is a mock implementation of ClientProvider for unit testing.
// Named "MockProvider" (not "StubProvider") to avoid confusion with
// internal/providers/stub/ which is the MCP stub provider.
type MockProvider struct {
	mu  sync.RWMutex
	vms map[string]*client.VMInfo
}

// Compile-time check that MockProvider implements client.ClientProvider
var _ client.ClientProvider = (*MockProvider)(nil)

// NewMockProvider creates a new MockProvider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		vms: make(map[string]*client.VMInfo),
	}
}

// AddVM adds a VM to the mock provider.
func (p *MockProvider) AddVM(name string, info *client.VMInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vms[name] = info
}

// GetVMInfo implements ClientProvider.
func (p *MockProvider) GetVMInfo(vmName string) (*client.VMInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	info, ok := p.vms[vmName]
	if !ok {
		return nil, fmt.Errorf("VM not found: %s", vmName)
	}
	return info, nil
}
