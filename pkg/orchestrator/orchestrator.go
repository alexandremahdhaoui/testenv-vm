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

package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/client"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/image"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

type IsolationConfig struct {
	NamePrefix         string
	OriginalCIDRPrefix string
	NewCIDRPrefix      string
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:4])
}

func hashToOctet(testID string) int {
	h := sha256.Sum256([]byte(testID))
	val := binary.BigEndian.Uint16(h[:2])
	return int(val%252) + 2
}

func newIsolationConfig(testID string, specNetworks []v1.NetworkResource) *IsolationConfig {
	prefix := shortHash(testID)
	octet := hashToOctet(testID)

	originalPrefix := "192.168.100."
	for _, net := range specNetworks {
		if net.Spec.Cidr != "" {
			parts := strings.Split(net.Spec.Cidr, ".")
			if len(parts) >= 3 {
				originalPrefix = parts[0] + "." + parts[1] + "." + parts[2] + "."
				break
			}
		}
	}

	newPrefix := fmt.Sprintf("192.168.%d.", octet)

	return &IsolationConfig{
		NamePrefix:         prefix,
		OriginalCIDRPrefix: originalPrefix,
		NewCIDRPrefix:      newPrefix,
	}
}

type Config struct {
	StateDir         string
	ImageCacheDir    string
	CleanupOnFailure bool
}

type Orchestrator struct {
	config   Config
	manager  *provider.Manager
	store    *state.Store
	executor *Executor
}

type CreateResult struct {
	Artifact    *v1.TestEnvArtifact
	Provisioner *client.RuntimeProvisioner
}

func NewOrchestrator(config Config) (*Orchestrator, error) {
	manager := provider.NewManager()

	store := state.NewStore(config.StateDir)

	imageCacheDir := config.ImageCacheDir
	if imageCacheDir == "" {
		imageCacheDir = os.Getenv("TESTENV_VM_IMAGE_CACHE_DIR")
	}
	if imageCacheDir == "" {
		imageCacheDir = "/tmp/testenv-vm/images"
	}

	imageMgr, err := image.NewCacheManager(imageCacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create image cache manager: %w", err)
	}

	executor := NewExecutor(manager, store, imageMgr)

	return &Orchestrator{
		config:   config,
		manager:  manager,
		store:    store,
		executor: executor,
	}, nil
}

func (o *Orchestrator) Create(ctx context.Context, input *v1.CreateInput) (*CreateResult, error) {
	log.Printf("Creating test environment: testID=%s, stage=%s", input.TestID, input.Stage)

	testenvSpec, err := v1.SpecFromMap(input.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse spec: %w", err)
	}

	isoConfig := newIsolationConfig(input.TestID, testenvSpec.Networks)
	log.Printf("Isolation config: prefix=%s, originalCIDR=%s, newCIDR=%s",
		isoConfig.NamePrefix, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)

	templatedFields, err := spec.ValidateEarly(testenvSpec)
	if err != nil {
		return nil, fmt.Errorf("spec validation failed: %w", err)
	}

	artifactDir := filepath.Join(input.TmpDir, input.TestID)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory %q: %w", artifactDir, err)
	}
	log.Printf("Created artifact directory: %s", artifactDir)

	for _, providerCfg := range testenvSpec.Providers {
		if err := o.manager.Start(providerCfg); err != nil {
			return nil, fmt.Errorf("failed to start provider %q: %w", providerCfg.Name, err)
		}
	}

	dag, err := BuildDAG(testenvSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to build DAG: %w", err)
	}

	phases, err := dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("failed to compute execution phases: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	envState := &v1.EnvironmentState{
		ID:          input.TestID,
		Stage:       input.Stage,
		Status:      v1.StatusCreating,
		CreatedAt:   now,
		UpdatedAt:   now,
		Spec:        testenvSpec,
		ArtifactDir: artifactDir,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: buildExecutionPlan(phases),
		Errors:        []v1.ErrorRecord{},
	}

	if err := o.store.Save(envState); err != nil {
		return nil, fmt.Errorf("failed to save initial state: %w", err)
	}

	templateCtx := spec.NewTemplateContext()

	if input.Env != nil {
		for k, v := range input.Env {
			templateCtx.Env[k] = v
		}
	}

	result, err := o.executor.ExecuteCreate(ctx, testenvSpec, phases, templateCtx, envState, templatedFields, isoConfig)
	if err != nil {
		return nil, fmt.Errorf("execution error: %w", err)
	}

	if !result.Success {
		if o.config.CleanupOnFailure {
			log.Printf("Execution failed, performing rollback")
			rollbackErrors := o.executor.Rollback(ctx, envState, isoConfig)
			if len(rollbackErrors) > 0 {
				log.Printf("Rollback completed with %d errors", len(rollbackErrors))
			}
		}

		envState.Status = v1.StatusFailed
		envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if saveErr := o.store.Save(envState); saveErr != nil {
			log.Printf("Failed to save failed state: %v", saveErr)
		}

		var errMsgs []string
		for _, e := range result.Errors {
			errMsgs = append(errMsgs, e.Error())
		}
		return nil, fmt.Errorf("create failed: %v", errMsgs)
	}

	envState.Status = v1.StatusReady
	envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := o.store.Save(envState); err != nil {
		return nil, fmt.Errorf("failed to save ready state: %w", err)
	}

	artifact := o.buildArtifact(input.TestID, envState, isoConfig)

	provisioner, err := client.NewRuntimeProvisioner(client.RuntimeProvisionerConfig{
		Manager:     o.manager,
		Store:       o.store,
		EnvState:    envState,
		TemplateCtx: templateCtx,
		Spec:        testenvSpec,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create RuntimeProvisioner: %w", err)
	}

	log.Printf("Test environment created successfully: testID=%s", input.TestID)
	return &CreateResult{
		Artifact:    artifact,
		Provisioner: provisioner,
	}, nil
}

func (o *Orchestrator) Delete(ctx context.Context, input *v1.DeleteInput) error {
	log.Printf("Deleting test environment: testID=%s", input.TestID)

	envState, err := o.store.Load(input.TestID)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("State not found for testID %s, assuming already deleted", input.TestID)
			return nil
		}
		if isNotFoundError(err) {
			log.Printf("State not found for testID %s, assuming already deleted", input.TestID)
			return nil
		}
		return fmt.Errorf("failed to load state: %w", err)
	}

	envState.Status = v1.StatusDestroying
	envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := o.store.Save(envState); err != nil {
		log.Printf("Failed to save destroying state: %v", err)
	}

	if envState.Spec != nil {
		for _, providerCfg := range envState.Spec.Providers {
			if _, exists := o.manager.GetInfo(providerCfg.Name); exists {
				continue
			}
			if err := o.manager.Start(providerCfg); err != nil {
				log.Printf("Failed to start provider %q for deletion: %v", providerCfg.Name, err)
			}
		}
	}

	var networks []v1.NetworkResource
	if envState.Spec != nil {
		networks = envState.Spec.Networks
	}
	isoConfig := newIsolationConfig(input.TestID, networks)

	if err := o.executor.ExecuteDelete(ctx, envState, isoConfig); err != nil {
		log.Printf("Delete completed with errors: %v", err)
	}

	if err := o.store.Delete(input.TestID); err != nil {
		log.Printf("Failed to delete state file: %v", err)
	}

	if envState.ArtifactDir != "" {
		if err := os.RemoveAll(envState.ArtifactDir); err != nil {
			log.Printf("Failed to remove artifact directory %q: %v", envState.ArtifactDir, err)
		}
	}

	log.Printf("Test environment deleted: testID=%s", input.TestID)
	return nil
}

func (o *Orchestrator) Close() error {
	return o.manager.StopAll()
}

func buildExecutionPlan(phases [][]v1.ResourceRef) *v1.ExecutionPlan {
	if len(phases) == 0 {
		return nil
	}

	plan := &v1.ExecutionPlan{
		Phases: make([]v1.Phase, len(phases)),
	}

	for i, phase := range phases {
		plan.Phases[i] = v1.Phase{
			Resources: phase,
		}
	}

	return plan
}

func (o *Orchestrator) buildArtifact(testID string, envState *v1.EnvironmentState, isoConfig *IsolationConfig) *v1.TestEnvArtifact {
	artifact := &v1.TestEnvArtifact{
		TestID:           testID,
		Files:            make(map[string]string),
		Metadata:         make(map[string]string),
		ManagedResources: []string{},
		Env:              make(map[string]string),
	}

	if isoConfig != nil {
		artifact.Env["TESTENV_VM_NAME_PREFIX"] = isoConfig.NamePrefix
	}

	for name, keyState := range envState.Resources.Keys {
		if keyState.State != nil {
			if privateKeyPath, ok := keyState.State["privateKeyPath"].(string); ok && privateKeyPath != "" {
				relPath := privateKeyPath
				if envState.ArtifactDir != "" {
					if rel, err := filepath.Rel(envState.ArtifactDir, privateKeyPath); err == nil {
						relPath = rel
					}
				}
				artifact.Files[fmt.Sprintf("testenv-vm.key.%s", name)] = relPath
				artifact.ManagedResources = append(artifact.ManagedResources, privateKeyPath)
				artifact.Env[fmt.Sprintf("TESTENV_KEY_%s_PRIVATE_PATH", toEnvVarName(name))] = privateKeyPath
			}

			if publicKeyPath, ok := keyState.State["publicKeyPath"].(string); ok && publicKeyPath != "" {
				artifact.ManagedResources = append(artifact.ManagedResources, publicKeyPath)
			}

			if fingerprint, ok := keyState.State["fingerprint"].(string); ok && fingerprint != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.key.%s.fingerprint", name)] = fingerprint
			}
		}

		artifact.ManagedResources = append(artifact.ManagedResources,
			fmt.Sprintf("testenv-vm://key/%s", name))
	}

	for name, networkState := range envState.Resources.Networks {
		if networkState.State != nil {
			if ip, ok := networkState.State["ip"].(string); ok && ip != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.network.%s.ip", name)] = ip
				artifact.Env[fmt.Sprintf("TESTENV_NETWORK_%s_IP", toEnvVarName(name))] = ip
			}

			if ifName, ok := networkState.State["interfaceName"].(string); ok && ifName != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.network.%s.interface", name)] = ifName
				artifact.Env[fmt.Sprintf("TESTENV_NETWORK_%s_INTERFACE", toEnvVarName(name))] = ifName
			}
		}

		artifact.ManagedResources = append(artifact.ManagedResources,
			fmt.Sprintf("testenv-vm://network/%s", name))
	}

	for name, vmState := range envState.Resources.VMs {
		if vmState.State != nil {
			if ip, ok := vmState.State["ip"].(string); ok && ip != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.vm.%s.ip", name)] = ip
				artifact.Env[fmt.Sprintf("TESTENV_VM_%s_IP", toEnvVarName(name))] = ip
			}

			if domain, ok := vmState.State["name"].(string); ok && domain != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.vm.%s.domain", name)] = domain
				artifact.Env[fmt.Sprintf("TESTENV_VM_%s_DOMAIN", toEnvVarName(name))] = domain
			}

			if consoleOutput, ok := vmState.State["consoleOutput"].(string); ok && consoleOutput != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.vm.%s.consoleOutput", name)] = consoleOutput
				artifact.Env[fmt.Sprintf("TESTENV_VM_%s_CONSOLE_OUTPUT", toEnvVarName(name))] = consoleOutput
			}

			if sshCmd, ok := vmState.State["sshCommand"].(string); ok && sshCmd != "" {
				artifact.Env[fmt.Sprintf("TESTENV_VM_%s_SSH", toEnvVarName(name))] = sshCmd
			}

			if mac, ok := vmState.State["mac"].(string); ok && mac != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.vm.%s.mac", name)] = mac
			}
		}

		artifact.ManagedResources = append(artifact.ManagedResources,
			fmt.Sprintf("testenv-vm://vm/%s", name))
	}

	return artifact
}

func toEnvVarName(s string) string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist")
}
