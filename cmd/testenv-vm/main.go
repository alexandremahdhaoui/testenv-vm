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

// Package main implements the testenv-vm MCP server binary.
// This binary runs as an MCP server and is called by Forge to manage
// test environment VMs through orchestration of underlying providers.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/alexandremahdhaoui/testenv-vm/pkg/mcp"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
)

// Version information (set via ldflags during build)
var (
	Version = "dev"
)

func main() {
	mcpFlag := flag.Bool("mcp", false, "Run as MCP server")
	versionFlag := flag.Bool("version", false, "Print version")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("testenv-vm version %s\n", Version)
		return
	}

	if !*mcpFlag {
		fmt.Fprintln(os.Stderr, "This binary must be run with --mcp flag")
		fmt.Fprintln(os.Stderr, "Usage: testenv-vm --mcp")
		os.Exit(1)
	}

	if err := runMCPServer(); err != nil {
		log.Fatalf("MCP server failed: %v", err)
	}
}

// runMCPServer starts the testenv-vm MCP server with stdio transport.
func runMCPServer() error {
	// Configure from environment
	stateDir := getEnvOrDefault("TESTENV_VM_STATE_DIR", ".forge/testenv-vm/state")
	cleanupOnFailure := getEnvOrDefault("TESTENV_VM_CLEANUP_ON_FAILURE", "true") == "true"

	// Create orchestrator
	orch, err := orchestrator.NewOrchestrator(orchestrator.Config{
		StateDir:         stateDir,
		CleanupOnFailure: cleanupOnFailure,
	})
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}
	defer func() { _ = orch.Close() }()

	// Create MCP server
	server, err := mcp.NewServer(orch, "testenv-vm", Version)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	// Ensure logs go to stderr (not stdout, which is for JSON-RPC)
	log.SetOutput(os.Stderr)
	log.Printf("Starting testenv-vm MCP server (version: %s)", Version)

	return server.RunDefault()
}

// getEnvOrDefault returns the value of the environment variable with the given key,
// or the default value if the environment variable is not set or empty.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
