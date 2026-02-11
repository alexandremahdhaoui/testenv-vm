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
	"net"
	"strings"
	"testing"
	"time"
)

func TestValidateIPReachability_Success(t *testing.T) {
	// Start a TCP listener on a random available port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	// Extract the port from the listener address.
	addr := ln.Addr().(*net.TCPAddr)

	err = validateIPReachability("127.0.0.1", addr.Port, 5*time.Second)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateIPReachability_ConnectionRefused(t *testing.T) {
	// Port 1 on localhost should have nothing listening (requires no root).
	err := validateIPReachability("127.0.0.1", 1, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
	if !strings.Contains(err.Error(), "not reachable") {
		t.Errorf("expected error to contain 'not reachable', got: %v", err)
	}
}

func TestValidateIPReachability_Timeout(t *testing.T) {
	// 192.0.2.1 is RFC 5737 TEST-NET-1, guaranteed non-routable.
	err := validateIPReachability("192.0.2.1", 22, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for timeout, got nil")
	}
}
