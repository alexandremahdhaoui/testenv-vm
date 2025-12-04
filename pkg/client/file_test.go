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
	"encoding/base64"
	"strings"
	"testing"
)

func TestCopyToCmd(t *testing.T) {
	t.Run("encodes content correctly", func(t *testing.T) {
		content := []byte("test content")
		remotePath := "/tmp/test.txt"

		cmd := copyToCmd(content, remotePath)

		// Verify command structure
		if !strings.Contains(cmd, "echo") {
			t.Errorf("expected command to contain 'echo', got %q", cmd)
		}
		if !strings.Contains(cmd, "base64 -d") {
			t.Errorf("expected command to contain 'base64 -d', got %q", cmd)
		}
		if !strings.Contains(cmd, remotePath) {
			t.Errorf("expected command to contain %q, got %q", remotePath, cmd)
		}

		// Verify base64 encoding is correct
		encoded := base64.StdEncoding.EncodeToString(content)
		if !strings.Contains(cmd, encoded) {
			t.Errorf("expected command to contain encoded content %q", encoded)
		}
	})

	t.Run("handles empty content", func(t *testing.T) {
		content := []byte{}
		remotePath := "/tmp/empty.txt"

		cmd := copyToCmd(content, remotePath)

		if !strings.Contains(cmd, "echo") {
			t.Errorf("expected command to contain 'echo', got %q", cmd)
		}
		if !strings.Contains(cmd, "base64 -d") {
			t.Errorf("expected command to contain 'base64 -d', got %q", cmd)
		}
	})

	t.Run("quotes path with spaces", func(t *testing.T) {
		content := []byte("test")
		remotePath := "/tmp/path with spaces/test.txt"

		cmd := copyToCmd(content, remotePath)

		// Verify path is quoted
		if !strings.Contains(cmd, "'") {
			t.Errorf("expected command to quote path, got %q", cmd)
		}
	})
}

func TestCopyFromCmd(t *testing.T) {
	t.Run("generates correct command", func(t *testing.T) {
		remotePath := "/tmp/test.txt"

		cmd := copyFromCmd(remotePath)

		expected := "base64 < '/tmp/test.txt'"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})

	t.Run("quotes path with spaces", func(t *testing.T) {
		remotePath := "/tmp/path with spaces/test.txt"

		cmd := copyFromCmd(remotePath)

		if !strings.Contains(cmd, "base64 <") {
			t.Errorf("expected command to contain 'base64 <', got %q", cmd)
		}
		if !strings.Contains(cmd, "'") {
			t.Errorf("expected command to quote path, got %q", cmd)
		}
	})
}

func TestMkdirCmd(t *testing.T) {
	t.Run("generates correct command", func(t *testing.T) {
		path := "/tmp/test/dir"

		cmd := mkdirCmd(path)

		expected := "mkdir -p '/tmp/test/dir'"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})

	t.Run("quotes path with spaces", func(t *testing.T) {
		path := "/tmp/path with spaces/dir"

		cmd := mkdirCmd(path)

		if !strings.Contains(cmd, "mkdir -p") {
			t.Errorf("expected command to contain 'mkdir -p', got %q", cmd)
		}
		if !strings.Contains(cmd, "'") {
			t.Errorf("expected command to quote path, got %q", cmd)
		}
	})
}

func TestChmodCmd(t *testing.T) {
	t.Run("generates correct command", func(t *testing.T) {
		path := "/tmp/test.txt"
		mode := "755"

		cmd := chmodCmd(path, mode)

		expected := "chmod 755 '/tmp/test.txt'"
		if cmd != expected {
			t.Errorf("expected %q, got %q", expected, cmd)
		}
	})

	t.Run("handles different modes", func(t *testing.T) {
		path := "/tmp/test.txt"

		modes := []string{"644", "755", "600", "u+x", "go-rwx"}
		for _, mode := range modes {
			cmd := chmodCmd(path, mode)

			if !strings.Contains(cmd, "chmod "+mode) {
				t.Errorf("expected command to contain 'chmod %s', got %q", mode, cmd)
			}
		}
	})
}

func TestFileExistsCmd(t *testing.T) {
	t.Run("generates correct command", func(t *testing.T) {
		path := "/tmp/test.txt"

		cmd := fileExistsCmd(path)

		if !strings.Contains(cmd, "test -f") {
			t.Errorf("expected command to contain 'test -f', got %q", cmd)
		}
		if !strings.Contains(cmd, "exists") {
			t.Errorf("expected command to contain 'exists', got %q", cmd)
		}
		if !strings.Contains(cmd, "not_exists") {
			t.Errorf("expected command to contain 'not_exists', got %q", cmd)
		}
	})

	t.Run("quotes path with spaces", func(t *testing.T) {
		path := "/tmp/path with spaces/test.txt"

		cmd := fileExistsCmd(path)

		if !strings.Contains(cmd, "'") {
			t.Errorf("expected command to quote path, got %q", cmd)
		}
	})
}

func TestParseFileExists(t *testing.T) {
	t.Run("returns true for exists", func(t *testing.T) {
		stdout := "exists"
		result := parseFileExists(stdout)

		if !result {
			t.Errorf("expected true for %q, got false", stdout)
		}
	})

	t.Run("returns true for exists with whitespace", func(t *testing.T) {
		stdout := "  exists  \n"
		result := parseFileExists(stdout)

		if !result {
			t.Errorf("expected true for %q, got false", stdout)
		}
	})

	t.Run("returns false for not_exists", func(t *testing.T) {
		stdout := "not_exists"
		result := parseFileExists(stdout)

		if result {
			t.Errorf("expected false for %q, got true", stdout)
		}
	})

	t.Run("returns false for not_exists with whitespace", func(t *testing.T) {
		stdout := "  not_exists  \n"
		result := parseFileExists(stdout)

		if result {
			t.Errorf("expected false for %q, got true", stdout)
		}
	})

	t.Run("returns false for empty string", func(t *testing.T) {
		stdout := ""
		result := parseFileExists(stdout)

		if result {
			t.Errorf("expected false for empty string, got true")
		}
	})

	t.Run("returns false for unexpected output", func(t *testing.T) {
		stdout := "something else"
		result := parseFileExists(stdout)

		if result {
			t.Errorf("expected false for %q, got true", stdout)
		}
	})
}

func TestDecodeBase64(t *testing.T) {
	t.Run("decodes correctly", func(t *testing.T) {
		original := []byte("test content")
		encoded := base64.StdEncoding.EncodeToString(original)

		decoded, err := decodeBase64(encoded)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(decoded) != string(original) {
			t.Errorf("expected %q, got %q", original, decoded)
		}
	})

	t.Run("decodes with whitespace", func(t *testing.T) {
		original := []byte("test content")
		encoded := base64.StdEncoding.EncodeToString(original)
		encodedWithWhitespace := "  " + encoded + "  \n"

		decoded, err := decodeBase64(encodedWithWhitespace)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(decoded) != string(original) {
			t.Errorf("expected %q, got %q", original, decoded)
		}
	})

	t.Run("handles invalid base64", func(t *testing.T) {
		invalidBase64 := "not valid base64!!!"

		_, err := decodeBase64(invalidBase64)

		if err == nil {
			t.Error("expected error for invalid base64, got nil")
		}
	})

	t.Run("decodes empty string", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte{})

		decoded, err := decodeBase64(encoded)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(decoded) != 0 {
			t.Errorf("expected empty slice, got %q", decoded)
		}
	})
}

func TestQuotePath(t *testing.T) {
	t.Run("quotes simple path", func(t *testing.T) {
		path := "/tmp/test.txt"

		quoted := quotePath(path)

		expected := "'/tmp/test.txt'"
		if quoted != expected {
			t.Errorf("expected %q, got %q", expected, quoted)
		}
	})

	t.Run("quotes path with spaces", func(t *testing.T) {
		path := "/tmp/path with spaces/test.txt"

		quoted := quotePath(path)

		expected := "'/tmp/path with spaces/test.txt'"
		if quoted != expected {
			t.Errorf("expected %q, got %q", expected, quoted)
		}
	})

	t.Run("escapes single quotes in path", func(t *testing.T) {
		path := "/tmp/path's/test.txt"

		quoted := quotePath(path)

		// Single quote should be escaped as '"'"'
		expected := "'/tmp/path'\"'\"'s/test.txt'"
		if quoted != expected {
			t.Errorf("expected %q, got %q", expected, quoted)
		}
	})

	t.Run("handles path with special characters", func(t *testing.T) {
		path := "/tmp/$test/file*.txt"

		quoted := quotePath(path)

		// Should be wrapped in single quotes which prevent shell expansion
		if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
			t.Errorf("expected path to be wrapped in single quotes, got %q", quoted)
		}
	})
}
