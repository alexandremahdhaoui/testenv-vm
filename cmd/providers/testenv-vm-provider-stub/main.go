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

// Package main implements the stub provider MCP server binary.
// This provider simulates real provider behavior for E2E testing without real infrastructure.
// It stores resources in memory and returns mock values.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"github.com/alexandremahdhaoui/testenv-vm/internal/providers/stub"
)

// Version information (set via ldflags during build)
var (
	Version        = "dev"
	CommitSHA      = "unknown"
	BuildTimestamp = "unknown"
)

func main() {
	mcpFlag := flag.Bool("mcp", false, "Run as MCP server")
	versionFlag := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("testenv-vm-provider-stub %s (commit: %s, built: %s)\n", Version, CommitSHA, BuildTimestamp)
		os.Exit(0)
	}

	if !*mcpFlag {
		fmt.Fprintln(os.Stderr, "This binary must be run with --mcp flag")
		fmt.Fprintln(os.Stderr, "Usage: testenv-vm-provider-stub --mcp")
		os.Exit(1)
	}

	if err := runMCPServer(); err != nil {
		log.Fatalf("MCP server failed: %v", err)
	}
}

// runMCPServer starts the stub provider MCP server with stdio transport.
func runMCPServer() error {
	provider := stub.NewProvider()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "testenv-vm-provider-stub",
		Version: Version,
	}, nil)

	// Register provider_capabilities tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "provider_capabilities",
		Description: "Get provider capabilities",
	}, makeCapabilitiesHandler(provider))

	// Register key tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "key_create",
		Description: "Create an SSH key",
	}, makeKeyCreateHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "key_get",
		Description: "Get an SSH key by name",
	}, makeKeyGetHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "key_list",
		Description: "List all SSH keys",
	}, makeKeyListHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "key_delete",
		Description: "Delete an SSH key by name",
	}, makeKeyDeleteHandler(provider))

	// Register network tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "network_create",
		Description: "Create a network",
	}, makeNetworkCreateHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "network_get",
		Description: "Get a network by name",
	}, makeNetworkGetHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "network_list",
		Description: "List all networks",
	}, makeNetworkListHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "network_delete",
		Description: "Delete a network by name",
	}, makeNetworkDeleteHandler(provider))

	// Register VM tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vm_create",
		Description: "Create a virtual machine",
	}, makeVMCreateHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vm_get",
		Description: "Get a virtual machine by name",
	}, makeVMGetHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vm_list",
		Description: "List all virtual machines",
	}, makeVMListHandler(provider))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vm_delete",
		Description: "Delete a virtual machine by name",
	}, makeVMDeleteHandler(provider))

	// Ensure logs go to stderr (not stdout, which is for JSON-RPC)
	log.SetOutput(os.Stderr)
	log.Printf("Starting testenv-vm-provider-stub MCP server (version: %s)", Version)

	return server.Run(context.Background(), &mcp.StdioTransport{})
}

// EmptyInput is used for tools that don't require input.
type EmptyInput struct{}

// errorResult creates a standardized MCP error result.
func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
		IsError: true,
	}
}

// toMCPResult converts a provider OperationResult to an MCP CallToolResult.
// The OperationResult is serialized as JSON and included in the text content,
// which allows the client to parse it correctly.
func toMCPResult(result *providerv1.OperationResult) (*mcp.CallToolResult, any) {
	if !result.Success {
		errMsg := "operation failed"
		if result.Error != nil {
			errMsg = result.Error.Message
		}
		return errorResult(errMsg), nil
	}

	// Serialize the full OperationResult to JSON for the response
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return errorResult("failed to serialize result: " + err.Error()), nil
	}

	// Return the OperationResult JSON in the text content
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(resultJSON)},
		},
		IsError: false,
	}, nil
}

// makeCapabilitiesHandler creates the handler for provider_capabilities tool.
func makeCapabilitiesHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, EmptyInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input EmptyInput) (*mcp.CallToolResult, any, error) {
		log.Printf("provider_capabilities called")
		caps := p.Capabilities()

		// Wrap capabilities in OperationResult for consistency with other handlers
		result := providerv1.SuccessResult(caps)
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return errorResult("failed to serialize capabilities: " + err.Error()), nil, nil
		}

		// Return the OperationResult JSON in the text content
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(resultJSON)},
			},
			IsError: false,
		}, nil, nil
	}
}

// makeKeyCreateHandler creates the handler for key_create tool.
func makeKeyCreateHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.KeyCreateRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.KeyCreateRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("key_create called: name=%s", input.Name)
		result := p.KeyCreate(&input)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeKeyGetHandler creates the handler for key_get tool.
func makeKeyGetHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.GetRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.GetRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("key_get called: name=%s", input.Name)
		result := p.KeyGet(input.Name)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeKeyListHandler creates the handler for key_list tool.
func makeKeyListHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.ListRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.ListRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("key_list called")
		result := p.KeyList(input.Filter)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeKeyDeleteHandler creates the handler for key_delete tool.
func makeKeyDeleteHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.DeleteRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.DeleteRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("key_delete called: name=%s", input.Name)
		result := p.KeyDelete(input.Name)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeNetworkCreateHandler creates the handler for network_create tool.
func makeNetworkCreateHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.NetworkCreateRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.NetworkCreateRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("network_create called: name=%s, kind=%s", input.Name, input.Kind)
		result := p.NetworkCreate(&input)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeNetworkGetHandler creates the handler for network_get tool.
func makeNetworkGetHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.GetRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.GetRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("network_get called: name=%s", input.Name)
		result := p.NetworkGet(input.Name)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeNetworkListHandler creates the handler for network_list tool.
func makeNetworkListHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.ListRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.ListRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("network_list called")
		result := p.NetworkList(input.Filter)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeNetworkDeleteHandler creates the handler for network_delete tool.
func makeNetworkDeleteHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.DeleteRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.DeleteRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("network_delete called: name=%s", input.Name)
		result := p.NetworkDelete(input.Name)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeVMCreateHandler creates the handler for vm_create tool.
func makeVMCreateHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.VMCreateRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.VMCreateRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("vm_create called: name=%s", input.Name)
		result := p.VMCreate(&input)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeVMGetHandler creates the handler for vm_get tool.
func makeVMGetHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.GetRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.GetRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("vm_get called: name=%s", input.Name)
		result := p.VMGet(input.Name)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeVMListHandler creates the handler for vm_list tool.
func makeVMListHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.ListRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.ListRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("vm_list called")
		result := p.VMList(input.Filter)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}

// makeVMDeleteHandler creates the handler for vm_delete tool.
func makeVMDeleteHandler(p *stub.Provider) func(context.Context, *mcp.CallToolRequest, providerv1.DeleteRequest) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input providerv1.DeleteRequest) (*mcp.CallToolResult, any, error) {
		log.Printf("vm_delete called: name=%s", input.Name)
		result := p.VMDelete(input.Name)
		mcpResult, artifact := toMCPResult(result)
		return mcpResult, artifact, nil
	}
}
