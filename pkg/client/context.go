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
	"fmt"
	"strings"
)

// PrivilegeEscalation configures command privilege escalation (e.g., sudo, doas).
// When Enabled is true, the Cmd prefix is prepended to commands.
type PrivilegeEscalation struct {
	// Enabled indicates whether to use privilege escalation
	Enabled bool
	// Cmd is the command prefix for privilege escalation
	// Example: []string{"sudo"} or []string{"sudo", "-E"}
	Cmd []string
}

// PrivilegeEscalationNone returns a PrivilegeEscalation with Enabled=false.
func PrivilegeEscalationNone() *PrivilegeEscalation {
	return &PrivilegeEscalation{
		Enabled: false,
		Cmd:     nil,
	}
}

// PrivilegeEscalationSudo returns a PrivilegeEscalation configured for sudo.
func PrivilegeEscalationSudo() *PrivilegeEscalation {
	return &PrivilegeEscalation{
		Enabled: true,
		Cmd:     []string{"sudo"},
	}
}

// PrivilegeEscalationSudoPreserveEnv returns a PrivilegeEscalation configured
// for sudo with environment preservation (-E flag).
func PrivilegeEscalationSudoPreserveEnv() *PrivilegeEscalation {
	return &PrivilegeEscalation{
		Enabled: true,
		Cmd:     []string{"sudo", "-E"},
	}
}

// ExecutionContext contains environment variables and privilege escalation settings.
// It follows an immutable builder pattern where With* methods return new instances.
type ExecutionContext struct {
	envs                map[string]string
	privilegeEscalation *PrivilegeEscalation
}

// NewExecutionContext creates a new ExecutionContext with no environment variables
// and no privilege escalation.
func NewExecutionContext() *ExecutionContext {
	return &ExecutionContext{
		envs:                make(map[string]string),
		privilegeEscalation: PrivilegeEscalationNone(),
	}
}

// Envs returns a copy of the environment variables.
func (c *ExecutionContext) Envs() map[string]string {
	if c.envs == nil {
		return make(map[string]string)
	}
	out := make(map[string]string, len(c.envs))
	for k, v := range c.envs {
		out[k] = v
	}
	return out
}

// PrivilegeEscalation returns the privilege escalation configuration.
func (c *ExecutionContext) PrivilegeEscalation() *PrivilegeEscalation {
	return c.privilegeEscalation
}

// WithEnvs returns a new ExecutionContext with the given environment variables,
// replacing any existing environment variables.
func (c *ExecutionContext) WithEnvs(envs map[string]string) *ExecutionContext {
	newEnvs := make(map[string]string, len(envs))
	for k, v := range envs {
		newEnvs[k] = v
	}
	return &ExecutionContext{
		envs:                newEnvs,
		privilegeEscalation: c.privilegeEscalation,
	}
}

// WithEnv returns a new ExecutionContext with the given environment variable added.
// Existing environment variables are preserved.
func (c *ExecutionContext) WithEnv(key, value string) *ExecutionContext {
	newEnvs := make(map[string]string, len(c.envs)+1)
	for k, v := range c.envs {
		newEnvs[k] = v
	}
	newEnvs[key] = value
	return &ExecutionContext{
		envs:                newEnvs,
		privilegeEscalation: c.privilegeEscalation,
	}
}

// WithPrivilegeEscalation returns a new ExecutionContext with the given
// privilege escalation configuration.
func (c *ExecutionContext) WithPrivilegeEscalation(pe *PrivilegeEscalation) *ExecutionContext {
	newEnvs := make(map[string]string, len(c.envs))
	for k, v := range c.envs {
		newEnvs[k] = v
	}
	return &ExecutionContext{
		envs:                newEnvs,
		privilegeEscalation: pe,
	}
}

// unquotable contains shell operators that should not be quoted.
var unquotable = map[string]struct{}{
	"&&": {},
	"||": {},
	";":  {},
	":":  {},
	"&":  {},
}

// safelyAppendToCmd appends a string to a command, quoting it unless it's
// a shell operator.
func safelyAppendToCmd(cmd string, s string) string {
	if _, ok := unquotable[s]; ok {
		return fmt.Sprintf("%s%s ", cmd, s)
	}
	return fmt.Sprintf("%s%q ", cmd, s)
}

// FormatCmd formats a command with environment variables and privilege escalation.
// It prepends environment variables in the form KEY="value", then adds privilege
// escalation commands if enabled, then adds the actual command arguments.
// Shell operators (&&, ||, ;, :, &) are not quoted.
func FormatCmd(ctx *ExecutionContext, cmd ...string) string {
	if ctx == nil {
		ctx = NewExecutionContext()
	}

	out := ""

	// Add environment variables first
	for k, v := range ctx.Envs() {
		envStr := fmt.Sprintf("%s=%q", k, v)
		out = fmt.Sprintf("%s%s ", out, envStr)
	}

	// Add privilege escalation command (if enabled)
	pe := ctx.PrivilegeEscalation()
	if pe != nil && pe.Enabled {
		for _, s := range pe.Cmd {
			out = safelyAppendToCmd(out, s)
		}
	}

	// Add the actual command
	for _, s := range cmd {
		out = safelyAppendToCmd(out, s)
	}

	return strings.TrimSpace(out)
}
