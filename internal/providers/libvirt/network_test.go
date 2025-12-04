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

package libvirt

import (
	"regexp"
	"testing"

	"github.com/digitalocean/go-libvirt"
)

func TestFormatUUID(t *testing.T) {
	tests := []struct {
		name     string
		uuid     libvirt.UUID
		expected string
	}{
		{
			name:     "Standard UUID",
			uuid:     libvirt.UUID{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0},
			expected: "12345678-9abc-def0-1234-56789abcdef0",
		},
		{
			name:     "All zeros",
			uuid:     libvirt.UUID{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: "00000000-0000-0000-0000-000000000000",
		},
		{
			name:     "All ones",
			uuid:     libvirt.UUID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			expected: "ffffffff-ffff-ffff-ffff-ffffffffffff",
		},
		{
			name:     "Real-looking UUID",
			uuid:     libvirt.UUID{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00},
			expected: "550e8400-e29b-41d4-a716-446655440000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatUUID(tt.uuid)
			if result != tt.expected {
				t.Errorf("formatUUID() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestFormatUUID_ValidFormat(t *testing.T) {
	// Generate a sample UUID
	uuid := libvirt.UUID{0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a}
	result := formatUUID(uuid)

	// Verify format: 8-4-4-4-12 (36 characters total)
	uuidPattern := `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	matched, err := regexp.MatchString(uuidPattern, result)
	if err != nil {
		t.Fatalf("Regex error: %v", err)
	}
	if !matched {
		t.Errorf("UUID format invalid: %s", result)
	}

	if len(result) != 36 {
		t.Errorf("UUID should be 36 characters, got %d", len(result))
	}
}
