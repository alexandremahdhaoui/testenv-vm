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

// Package mcp provides the MCP server for testenv-vm.
package mcp

import (
	"context"
	"fmt"
	"log"

	"github.com/alexandremahdhaoui/forge/pkg/engineframework"
	"github.com/alexandremahdhaoui/forge/pkg/mcputil"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server for testenv-vm.
type Server struct {
	server *mcp.Server
	orch   *orchestrator.Orchestrator
	name   string
}

// NewServer creates a new MCP server for testenv-vm.
// It uses Forge's engineframework types and mcputil for consistency.
func NewServer(orch *orchestrator.Orchestrator, name, version string) (*Server, error) {
	if orch == nil {
		return nil, fmt.Errorf("orchestrator cannot be nil")
	}

	// Create the MCP server using the MCP SDK directly
	server := mcp.NewServer(&mcp.Implementation{
		Name:    name,
		Version: version,
	}, nil)

	s := &Server{
		server: server,
		orch:   orch,
		name:   name,
	}

	// Register create tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create",
		Description: fmt.Sprintf("Create a test environment resource using %s", name),
	}, s.makeCreateHandler(name))

	// Register delete tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete",
		Description: fmt.Sprintf("Delete a test environment resource using %s", name),
	}, s.makeDeleteHandler(name))

	log.Printf("MCP server initialized: name=%s, version=%s", name, version)
	return s, nil
}

// Run starts the MCP server loop.
// It reads JSON-RPC requests from stdin and writes responses to stdout.
// All logs go to stderr to avoid corrupting the JSON-RPC stream.
func (s *Server) Run(ctx context.Context) error {
	log.Printf("Starting MCP server...")
	if err := s.server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Printf("MCP server failed: %v", err)
		return err
	}
	return nil
}

// RunDefault starts the MCP server with a background context.
func (s *Server) RunDefault() error {
	return s.Run(context.Background())
}

// makeCreateHandler creates an MCP handler function for the create tool.
func (s *Server) makeCreateHandler(name string) func(context.Context, *mcp.CallToolRequest, engineframework.CreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input engineframework.CreateInput) (*mcp.CallToolResult, any, error) {
		log.Printf("Creating test environment resource: testID=%s, stage=%s using %s", input.TestID, input.Stage, name)

		// Validate required input fields
		if result := mcputil.ValidateRequiredWithPrefix("Create failed", map[string]string{
			"testID": input.TestID,
			"stage":  input.Stage,
			"tmpDir": input.TmpDir,
		}); result != nil {
			return result, nil, nil
		}

		// Call the handler
		artifact, err := s.handleCreate(ctx, input)
		if err != nil {
			return mcputil.ErrorResult(fmt.Sprintf("Create failed: %v", err)), nil, nil
		}

		// Check if artifact is nil (shouldn't happen, but defensive)
		if artifact == nil {
			return mcputil.ErrorResult("Create function returned nil artifact"), nil, nil
		}

		// Convert artifact to map[string]interface{} for MCP serialization
		artifactMap := map[string]interface{}{
			"testID":           artifact.TestID,
			"files":            artifact.Files,
			"metadata":         artifact.Metadata,
			"managedResources": artifact.ManagedResources,
			"env":              artifact.Env,
		}

		// Return success with artifact
		result, returnedArtifact := mcputil.SuccessResultWithArtifact(
			fmt.Sprintf("Created test environment resource using %s", name),
			artifactMap,
		)
		return result, returnedArtifact, nil
	}
}

// makeDeleteHandler creates an MCP handler function for the delete tool.
func (s *Server) makeDeleteHandler(name string) func(context.Context, *mcp.CallToolRequest, engineframework.DeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input engineframework.DeleteInput) (*mcp.CallToolResult, any, error) {
		log.Printf("Deleting test environment resource: testID=%s using %s", input.TestID, name)

		// Validate required input fields
		if result := mcputil.ValidateRequiredWithPrefix("Delete failed", map[string]string{
			"testID": input.TestID,
		}); result != nil {
			return result, nil, nil
		}

		// Call the handler
		if err := s.handleDelete(ctx, input); err != nil {
			return mcputil.ErrorResult(fmt.Sprintf("Delete failed: %v", err)), nil, nil
		}

		// Return success
		return mcputil.SuccessResult(fmt.Sprintf("Deleted test environment resource using %s", name)), nil, nil
	}
}
