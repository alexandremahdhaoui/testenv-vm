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

package mcp

import (
	"context"
	"log"

	"github.com/alexandremahdhaoui/forge/pkg/engineframework"
	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

// handleCreate handles the MCP "create" tool call.
// It converts the engineframework input to v1 input, calls the orchestrator,
// and converts the v1 output to engineframework output.
func (s *Server) handleCreate(ctx context.Context, input engineframework.CreateInput) (*engineframework.TestEnvArtifact, error) {
	log.Printf("Handling create request: testID=%s, stage=%s", input.TestID, input.Stage)

	// Convert engineframework.CreateInput to v1.CreateInput
	v1Input := &v1.CreateInput{
		TestID:   input.TestID,
		Stage:    input.Stage,
		TmpDir:   input.TmpDir,
		RootDir:  input.RootDir,
		Metadata: input.Metadata,
		Spec:     input.Spec,
		Env:      input.Env,
	}

	// Call the orchestrator
	createResult, err := s.orch.Create(ctx, v1Input)
	if err != nil {
		log.Printf("Create failed: %v", err)
		return nil, err
	}

	// Convert v1.TestEnvArtifact to engineframework.TestEnvArtifact
	// Note: Provisioner is not returned in MCP response (not JSON-serializable)
	result := &engineframework.TestEnvArtifact{
		TestID:           createResult.Artifact.TestID,
		Files:            normalizeFilesMap(createResult.Artifact.Files),
		Metadata:         normalizeMetadataMap(createResult.Artifact.Metadata),
		ManagedResources: createResult.Artifact.ManagedResources,
		Env:              normalizeEnvMap(createResult.Artifact.Env),
	}

	log.Printf("Create succeeded: testID=%s", input.TestID)
	return result, nil
}

// handleDelete handles the MCP "delete" tool call.
// It converts the engineframework input to v1 input and calls the orchestrator.
func (s *Server) handleDelete(ctx context.Context, input engineframework.DeleteInput) error {
	log.Printf("Handling delete request: testID=%s", input.TestID)

	// Convert engineframework.DeleteInput to v1.DeleteInput
	v1Input := &v1.DeleteInput{
		TestID:   input.TestID,
		Metadata: input.Metadata,
	}

	// Call the orchestrator
	if err := s.orch.Delete(ctx, v1Input); err != nil {
		log.Printf("Delete failed: %v", err)
		return err
	}

	log.Printf("Delete succeeded: testID=%s", input.TestID)
	return nil
}

// normalizeFilesMap ensures the map is non-nil for MCP serialization.
func normalizeFilesMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

// normalizeMetadataMap ensures the map is non-nil for MCP serialization.
func normalizeMetadataMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

// normalizeEnvMap ensures the map is non-nil for MCP serialization.
func normalizeEnvMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}
