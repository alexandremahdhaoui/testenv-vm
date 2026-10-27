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
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

const fakeProviderCaptureEnv = "TESTENV_VM_FAKE_PROVIDER_CAPTURE"

func TestMain(m *testing.M) {
	if capturePath := os.Getenv(fakeProviderCaptureEnv); capturePath != "" {
		os.Exit(serveFakeProvider(capturePath))
	}
	os.Exit(m.Run())
}

func serveFakeProvider(capturePath string) int {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return 1
		}
		if request.ID == nil {
			continue
		}
		result, err := fakeProviderResult(request.Method, request.Params.Name, request.Params.Arguments, capturePath)
		if err != nil {
			return 1
		}
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": *request.ID, "result": result}); err != nil {
			return 1
		}
	}
	return 0
}

func fakeProviderResult(method, tool string, arguments json.RawMessage, capturePath string) (any, error) {
	if method == "initialize" {
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]string{"name": "fake", "version": "0"},
			"capabilities":    map[string]any{},
		}, nil
	}
	if tool == "vm_create" {
		if err := os.WriteFile(capturePath, arguments, 0o600); err != nil {
			return nil, err
		}
		return toolCallText(providerv1.ErrorResult(providerv1.NewProviderError("the fake provider only captures the request", false)))
	}
	return toolCallText(providerv1.SuccessResult(providerv1.CapabilitiesResponse{ProviderName: "fake", Version: "0"}))
}

func toolCallText(result *providerv1.OperationResult) (any, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return map[string]any{"content": []map[string]string{{"type": "text", "text": string(data)}}}, nil
}

func TestCreateVMAsksTheProviderToLogTheSerialConsoleUnderTheArtifactDirectory(t *testing.T) {
	capturePath := filepath.Join(t.TempDir(), "vm_create.json")
	t.Setenv(fakeProviderCaptureEnv, capturePath)
	manager := provider.NewManager()
	if err := manager.Start(v1.ProviderConfig{Name: "fake", Engine: os.Args[0]}); err != nil {
		t.Fatalf("starting the fake provider: %v", err)
	}
	t.Cleanup(func() { _ = manager.StopAll() })

	envState := newTestEnvState("test-1")
	envState.ArtifactDir = "/artifacts/test-1"
	envState.Resources.Networks["test-net"] = &v1.ResourceState{Status: v1.StatusReady}
	rp, err := NewRuntimeProvisioner(RuntimeProvisionerConfig{
		Manager:     manager,
		Store:       state.NewStore(t.TempDir()),
		EnvState:    envState,
		TemplateCtx: spec.NewTemplateContext(),
		Spec:        newTestSpec("fake", v1.ProviderConfig{Name: "fake", Default: true}),
	})
	if err != nil {
		t.Fatalf("NewRuntimeProvisioner() error = %v", err)
	}

	_, _ = rp.CreateVM(context.Background(), "runtime-vm", v1.VMSpec{
		Memory:  512,
		Vcpus:   1,
		Network: "test-net",
		Disk:    v1.DiskSpec{Size: "1G"},
	})

	data, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("the fake provider captured no vm_create request: %v", err)
	}
	var request providerv1.VMCreateRequest
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("parsing the captured vm_create request: %v", err)
	}
	if want := "/artifacts/test-1/console/runtime-vm.log"; request.ConsoleOutput != want {
		t.Errorf("vm_create consoleOutput = %q, want %q", request.ConsoleOutput, want)
	}
}
