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

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/alexandremahdhaoui/forge/pkg/engineframework"
	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

// Delete deletes a test environment identified by testID.
// This is the main entry point called by the generated MCP server.
// Note: spec may be nil as Delete uses testID to find state.
func Delete(ctx context.Context, input engineframework.DeleteInput, spec *v1.Spec) error {
	log.Printf("Handling delete request: testID=%s", input.TestID)

	o, err := getOrchestrator()
	if err != nil {
		return fmt.Errorf("failed to get orchestrator: %w", err)
	}

	// Convert engineframework.DeleteInput to v1.DeleteInput
	// Note: ManagedResources is not available in engineframework.DeleteInput
	// The orchestrator loads state from disk using TestID instead
	v1Input := &v1.DeleteInput{
		TestID:   input.TestID,
		Metadata: input.Metadata,
	}

	// Call the orchestrator
	if err := o.Delete(ctx, v1Input); err != nil {
		log.Printf("Delete failed: %v", err)
		return err
	}

	log.Printf("Delete succeeded: testID=%s", input.TestID)
	return nil
}
