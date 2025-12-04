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
	"encoding/base64"
	"fmt"
	"strings"
)

// copyToCmd generates the command to copy content to a remote file.
// Uses base64 encoding: echo <base64> | base64 -d > <path>
// Note: Base64 encoding has ~100KB practical limit for single transfers.
//
//nolint:unused // Will be used by Client type in Task 10
func copyToCmd(content []byte, remotePath string) string {
	encoded := base64.StdEncoding.EncodeToString(content)
	return fmt.Sprintf("echo %s | base64 -d > %s", encoded, quotePath(remotePath))
}

// copyFromCmd generates the command to copy content from a remote file.
// Uses base64 encoding: base64 < <path>
//
//nolint:unused // Will be used by Client type in Task 10
func copyFromCmd(remotePath string) string {
	return fmt.Sprintf("base64 < %s", quotePath(remotePath))
}

// mkdirCmd generates the command to create a directory.
// Uses: mkdir -p <path>
//
//nolint:unused // Will be used by Client type in Task 10
func mkdirCmd(path string) string {
	return fmt.Sprintf("mkdir -p %s", quotePath(path))
}

// chmodCmd generates the command to set file permissions.
// Uses: chmod <mode> <path>
//
//nolint:unused // Will be used by Client type in Task 10
func chmodCmd(path string, mode string) string {
	return fmt.Sprintf("chmod %s %s", mode, quotePath(path))
}

// fileExistsCmd generates the command to check if a file exists.
// Uses: test -f <path> && echo "exists" || echo "not_exists"
//
//nolint:unused // Will be used by Client type in Task 10
func fileExistsCmd(path string) string {
	return fmt.Sprintf("test -f %s && echo \"exists\" || echo \"not_exists\"", quotePath(path))
}

// parseFileExists parses the output of fileExistsCmd.
//
//nolint:unused // Will be used by Client type in Task 10
func parseFileExists(stdout string) bool {
	return strings.TrimSpace(stdout) == "exists"
}

// decodeBase64 decodes base64 content from copyFromCmd output.
//
//nolint:unused // Will be used by Client type in Task 10
func decodeBase64(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
}

// quotePath quotes a path for safe shell usage.
// This ensures paths with spaces or special characters are handled correctly.
//
//nolint:unused // Will be used by Client type in Task 10
func quotePath(path string) string {
	// Simple shell quoting - wrap in single quotes and escape any single quotes in the path
	escaped := strings.ReplaceAll(path, "'", "'\"'\"'")
	return fmt.Sprintf("'%s'", escaped)
}
