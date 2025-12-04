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
	"strings"
	"testing"
)

// TestPrivilegeEscalationNone verifies that PrivilegeEscalationNone returns
// a PrivilegeEscalation with Enabled=false.
func TestPrivilegeEscalationNone(t *testing.T) {
	pe := PrivilegeEscalationNone()
	if pe.Enabled {
		t.Error("expected Enabled to be false")
	}
}

// TestPrivilegeEscalationSudo verifies that PrivilegeEscalationSudo returns
// a PrivilegeEscalation with Enabled=true and Cmd=["sudo"].
func TestPrivilegeEscalationSudo(t *testing.T) {
	pe := PrivilegeEscalationSudo()
	if !pe.Enabled {
		t.Error("expected Enabled to be true")
	}
	if len(pe.Cmd) != 1 || pe.Cmd[0] != "sudo" {
		t.Errorf("expected Cmd to be [sudo], got %v", pe.Cmd)
	}
}

// TestPrivilegeEscalationSudoPreserveEnv verifies that PrivilegeEscalationSudoPreserveEnv
// returns a PrivilegeEscalation with Enabled=true and Cmd=["sudo", "-E"].
func TestPrivilegeEscalationSudoPreserveEnv(t *testing.T) {
	pe := PrivilegeEscalationSudoPreserveEnv()
	if !pe.Enabled {
		t.Error("expected Enabled to be true")
	}
	if len(pe.Cmd) != 2 || pe.Cmd[0] != "sudo" || pe.Cmd[1] != "-E" {
		t.Errorf("expected Cmd to be [sudo -E], got %v", pe.Cmd)
	}
}

// TestNewExecutionContext verifies that NewExecutionContext returns an
// empty context with no environment variables and no privilege escalation.
func TestNewExecutionContext(t *testing.T) {
	ctx := NewExecutionContext()
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if len(ctx.Envs()) != 0 {
		t.Errorf("expected empty envs, got %v", ctx.Envs())
	}
	if ctx.PrivilegeEscalation().Enabled {
		t.Error("expected privilege escalation to be disabled")
	}
}

// TestWithEnv verifies that WithEnv adds a single environment variable.
func TestWithEnv(t *testing.T) {
	ctx := NewExecutionContext()
	newCtx := ctx.WithEnv("KEY", "value")

	envs := newCtx.Envs()
	if len(envs) != 1 {
		t.Errorf("expected 1 env var, got %d", len(envs))
	}
	if envs["KEY"] != "value" {
		t.Errorf("expected KEY=value, got KEY=%s", envs["KEY"])
	}
}

// TestWithEnvs verifies that WithEnvs replaces all environment variables.
func TestWithEnvs(t *testing.T) {
	ctx := NewExecutionContext().WithEnv("OLD", "value")
	newCtx := ctx.WithEnvs(map[string]string{
		"NEW1": "value1",
		"NEW2": "value2",
	})

	envs := newCtx.Envs()
	if len(envs) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(envs))
	}
	if _, ok := envs["OLD"]; ok {
		t.Error("expected OLD to be removed")
	}
	if envs["NEW1"] != "value1" {
		t.Errorf("expected NEW1=value1, got NEW1=%s", envs["NEW1"])
	}
	if envs["NEW2"] != "value2" {
		t.Errorf("expected NEW2=value2, got NEW2=%s", envs["NEW2"])
	}
}

// TestWithPrivilegeEscalation verifies that WithPrivilegeEscalation sets
// privilege escalation.
func TestWithPrivilegeEscalation(t *testing.T) {
	ctx := NewExecutionContext()
	newCtx := ctx.WithPrivilegeEscalation(PrivilegeEscalationSudo())

	pe := newCtx.PrivilegeEscalation()
	if !pe.Enabled {
		t.Error("expected privilege escalation to be enabled")
	}
	if len(pe.Cmd) != 1 || pe.Cmd[0] != "sudo" {
		t.Errorf("expected Cmd to be [sudo], got %v", pe.Cmd)
	}
}

// TestExecutionContextImmutability verifies that With* methods return new
// instances and do not modify the original context.
func TestExecutionContextImmutability(t *testing.T) {
	original := NewExecutionContext()

	// Add an env var
	modified := original.WithEnv("KEY", "value")

	// Original should be unchanged
	if len(original.Envs()) != 0 {
		t.Error("original context was modified by WithEnv")
	}

	// Modified should have the new env var
	if len(modified.Envs()) != 1 {
		t.Error("modified context does not have the new env var")
	}

	// Test WithPrivilegeEscalation immutability
	modified2 := original.WithPrivilegeEscalation(PrivilegeEscalationSudo())
	if original.PrivilegeEscalation().Enabled {
		t.Error("original context was modified by WithPrivilegeEscalation")
	}
	if !modified2.PrivilegeEscalation().Enabled {
		t.Error("modified2 context does not have privilege escalation enabled")
	}
}

// TestFormatCmdWithNoContext verifies that FormatCmd with nil context
// returns a quoted command.
func TestFormatCmdWithNoContext(t *testing.T) {
	result := FormatCmd(nil, "echo", "hello")
	expected := `"echo" "hello"`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestFormatCmdWithEnvs verifies that FormatCmd prepends environment variables
// in the form KEY="value".
func TestFormatCmdWithEnvs(t *testing.T) {
	ctx := NewExecutionContext().WithEnv("KEY", "value")
	result := FormatCmd(ctx, "echo", "hello")

	if !strings.Contains(result, `KEY="value"`) {
		t.Errorf("expected result to contain KEY=\"value\", got %q", result)
	}
	if !strings.Contains(result, `"echo"`) {
		t.Errorf("expected result to contain \"echo\", got %q", result)
	}
	if !strings.Contains(result, `"hello"`) {
		t.Errorf("expected result to contain \"hello\", got %q", result)
	}
}

// TestFormatCmdWithPrivilegeEscalation verifies that FormatCmd prepends
// privilege escalation commands when enabled.
func TestFormatCmdWithPrivilegeEscalation(t *testing.T) {
	ctx := NewExecutionContext().WithPrivilegeEscalation(PrivilegeEscalationSudo())
	result := FormatCmd(ctx, "whoami")

	if !strings.Contains(result, `"sudo"`) {
		t.Errorf("expected result to contain \"sudo\", got %q", result)
	}
	if !strings.Contains(result, `"whoami"`) {
		t.Errorf("expected result to contain \"whoami\", got %q", result)
	}

	// Verify sudo comes before whoami
	sudoIndex := strings.Index(result, `"sudo"`)
	whoamiIndex := strings.Index(result, `"whoami"`)
	if sudoIndex >= whoamiIndex {
		t.Errorf("expected sudo to come before whoami, got %q", result)
	}
}

// TestFormatCmdWithShellOperators verifies that shell operators are not quoted.
func TestFormatCmdWithShellOperators(t *testing.T) {
	tests := []struct {
		name     string
		cmd      []string
		expected []string // substrings that should appear
	}{
		{
			name:     "logical AND",
			cmd:      []string{"echo", "hello", "&&", "echo", "world"},
			expected: []string{`"echo"`, `"hello"`, "&&", `"world"`},
		},
		{
			name:     "logical OR",
			cmd:      []string{"false", "||", "echo", "fallback"},
			expected: []string{`"false"`, "||", `"echo"`, `"fallback"`},
		},
		{
			name:     "semicolon",
			cmd:      []string{"echo", "first", ";", "echo", "second"},
			expected: []string{`"echo"`, `"first"`, ";", `"second"`},
		},
		{
			name:     "colon",
			cmd:      []string{":", "&&", "echo", "ok"},
			expected: []string{":", "&&", `"echo"`, `"ok"`},
		},
		{
			name:     "background",
			cmd:      []string{"sleep", "10", "&"},
			expected: []string{`"sleep"`, `"10"`, "&"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewExecutionContext()
			result := FormatCmd(ctx, tt.cmd...)

			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("expected result to contain %q, got %q", expected, result)
				}
			}
		})
	}
}

// TestFormatCmdWithEnvsAndPrivilegeEscalation verifies that FormatCmd correctly
// combines environment variables and privilege escalation.
func TestFormatCmdWithEnvsAndPrivilegeEscalation(t *testing.T) {
	ctx := NewExecutionContext().
		WithEnv("PATH", "/usr/local/bin").
		WithPrivilegeEscalation(PrivilegeEscalationSudoPreserveEnv())

	result := FormatCmd(ctx, "echo", "test")

	// Verify all components are present
	if !strings.Contains(result, `PATH="/usr/local/bin"`) {
		t.Errorf("expected result to contain PATH env var, got %q", result)
	}
	if !strings.Contains(result, `"sudo"`) {
		t.Errorf("expected result to contain sudo, got %q", result)
	}
	if !strings.Contains(result, `"-E"`) {
		t.Errorf("expected result to contain -E flag, got %q", result)
	}
	if !strings.Contains(result, `"echo"`) {
		t.Errorf("expected result to contain echo, got %q", result)
	}
	if !strings.Contains(result, `"test"`) {
		t.Errorf("expected result to contain test, got %q", result)
	}

	// Verify order: env vars -> sudo -> command
	pathIndex := strings.Index(result, "PATH=")
	sudoIndex := strings.Index(result, `"sudo"`)
	echoIndex := strings.Index(result, `"echo"`)

	if pathIndex >= sudoIndex {
		t.Errorf("expected PATH to come before sudo, got %q", result)
	}
	if sudoIndex >= echoIndex {
		t.Errorf("expected sudo to come before echo, got %q", result)
	}
}

// TestFormatCmdEmptyCommand verifies that FormatCmd handles empty commands.
func TestFormatCmdEmptyCommand(t *testing.T) {
	ctx := NewExecutionContext()
	result := FormatCmd(ctx)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// TestFormatCmdMultipleEnvs verifies that FormatCmd handles multiple env vars.
func TestFormatCmdMultipleEnvs(t *testing.T) {
	ctx := NewExecutionContext().
		WithEnv("KEY1", "value1").
		WithEnv("KEY2", "value2")

	result := FormatCmd(ctx, "echo", "test")

	if !strings.Contains(result, `KEY1="value1"`) {
		t.Errorf("expected result to contain KEY1, got %q", result)
	}
	if !strings.Contains(result, `KEY2="value2"`) {
		t.Errorf("expected result to contain KEY2, got %q", result)
	}
}

// TestFormatCmdDisabledPrivilegeEscalation verifies that FormatCmd does not
// prepend privilege escalation commands when Enabled is false.
func TestFormatCmdDisabledPrivilegeEscalation(t *testing.T) {
	ctx := NewExecutionContext().WithPrivilegeEscalation(PrivilegeEscalationNone())
	result := FormatCmd(ctx, "whoami")

	if strings.Contains(result, "sudo") {
		t.Errorf("expected result not to contain sudo when disabled, got %q", result)
	}
	if !strings.Contains(result, `"whoami"`) {
		t.Errorf("expected result to contain whoami, got %q", result)
	}
}

// TestEnvsWithNilMap verifies that Envs() handles nil envs map correctly.
func TestEnvsWithNilMap(t *testing.T) {
	ctx := &ExecutionContext{
		envs:                nil,
		privilegeEscalation: PrivilegeEscalationNone(),
	}
	envs := ctx.Envs()
	if envs == nil {
		t.Error("expected non-nil envs map, got nil")
	}
	if len(envs) != 0 {
		t.Errorf("expected empty envs map, got %v", envs)
	}
}
