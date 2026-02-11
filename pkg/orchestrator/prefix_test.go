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

package orchestrator

import (
	"regexp"
	"testing"
)

func TestResourcePrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "known input produces deterministic output",
			input: "test-e2e-20260210-abc123",
			want:  "d3f1a2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResourcePrefix(tt.input)
			if got != tt.want {
				t.Errorf("ResourcePrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	// Test length property
	for _, input := range []string{"a", "abc", "test-e2e-20260210-abc123"} {
		got := ResourcePrefix(input)
		if len(got) != 6 {
			t.Errorf("ResourcePrefix(%q) length = %d, want 6", input, len(got))
		}
	}

	// Test hex-only characters
	hexRegex := regexp.MustCompile(`^[0-9a-f]+$`)
	for _, input := range []string{"a", "abc", "test-e2e-20260210-abc123"} {
		got := ResourcePrefix(input)
		if !hexRegex.MatchString(got) {
			t.Errorf("ResourcePrefix(%q) = %q, contains non-hex characters", input, got)
		}
	}

	// Test deterministic
	input := "test-deterministic"
	first := ResourcePrefix(input)
	for i := 0; i < 10; i++ {
		got := ResourcePrefix(input)
		if got != first {
			t.Errorf("ResourcePrefix(%q) returned different results: %q vs %q", input, first, got)
		}
	}

	// Test uniqueness for different inputs
	seen := map[string]string{}
	for _, input := range []string{"a", "b", "c", "test-1", "test-2"} {
		got := ResourcePrefix(input)
		if prev, ok := seen[got]; ok {
			t.Errorf("ResourcePrefix(%q) and ResourcePrefix(%q) both returned %q", prev, input, got)
		}
		seen[got] = input
	}
}

func TestSubnetOctet(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "empty string returns 100",
			input: "",
			want:  100,
		},
		{
			name:  "known input produces deterministic output",
			input: "test-e2e-20260210-abc123",
			want:  87,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubnetOctet(tt.input)
			if got != tt.want {
				t.Errorf("SubnetOctet(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}

	// Test range property [20, 219]
	for _, input := range []string{"a", "abc", "test-1", "test-2", "test-e2e-20260210-abc123"} {
		got := SubnetOctet(input)
		if got < 20 || got > 219 {
			t.Errorf("SubnetOctet(%q) = %d, want in range [20, 219]", input, got)
		}
	}

	// Test deterministic
	input := "test-deterministic"
	first := SubnetOctet(input)
	for i := 0; i < 10; i++ {
		got := SubnetOctet(input)
		if got != first {
			t.Errorf("SubnetOctet(%q) returned different results: %d vs %d", input, first, got)
		}
	}
}

func TestPrefixName(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rname  string
		want   string
	}{
		{
			name:   "empty prefix returns name unchanged",
			prefix: "",
			rname:  "my-network",
			want:   "my-network",
		},
		{
			name:   "prefix prepended with dash",
			prefix: "d3f1a2",
			rname:  "my-network",
			want:   "d3f1a2-my-network",
		},
		{
			name:   "works with empty name",
			prefix: "abc123",
			rname:  "",
			want:   "abc123-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrefixName(tt.prefix, tt.rname)
			if got != tt.want {
				t.Errorf("PrefixName(%q, %q) = %q, want %q", tt.prefix, tt.rname, got, tt.want)
			}
		})
	}
}
