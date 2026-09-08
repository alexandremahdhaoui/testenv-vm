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

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

func TestWaitForTCPPassesWhenThePortAnswers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	opErr := waitForTCP(&providerv1.TCPReadinessSpec{Port: port, Timeout: "10s"}, "127.0.0.1")
	if opErr != nil {
		t.Errorf("waitForTCP() unexpected error: %v", opErr)
	}
}

func TestWaitForTCPTimesOutWhenThePortStaysClosed(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	opErr := waitForTCP(&providerv1.TCPReadinessSpec{Port: port, Timeout: "1s"}, "127.0.0.1")
	if opErr == nil {
		t.Fatal("waitForTCP() should time out on a closed port")
	}
	if !strings.Contains(opErr.Message, "tcp readiness") {
		t.Errorf("error should name tcp readiness, got: %v", opErr)
	}
}

func TestWaitForReadinessRunsTheTCPCheckWithoutSSH(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	opErr := waitForReadiness(&providerv1.ReadinessSpec{
		TCP: &providerv1.TCPReadinessSpec{Port: port, Timeout: "10s"},
	}, "127.0.0.1")
	if opErr != nil {
		t.Errorf("waitForReadiness() unexpected error: %v", opErr)
	}
}

func TestReadinessTimeoutFallsBackOnAnEmptyOrInvalidDuration(t *testing.T) {
	if got := readinessTimeout("", time.Minute); got != time.Minute {
		t.Errorf("readinessTimeout(\"\") = %v, want %v", got, time.Minute)
	}
	if got := readinessTimeout("soon", time.Minute); got != time.Minute {
		t.Errorf("readinessTimeout(\"soon\") = %v, want %v", got, time.Minute)
	}
	if got := readinessTimeout("5m", time.Minute); got != 5*time.Minute {
		t.Errorf("readinessTimeout(\"5m\") = %v, want %v", got, 5*time.Minute)
	}
}
