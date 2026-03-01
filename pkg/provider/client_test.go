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

// Package provider implements MCP client communication with provider processes.
package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// mockProcess simulates an MCP provider process for testing.
type mockProcess struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser

	// For simulating the process
	stdinReader  io.ReadCloser
	stdoutWriter io.WriteCloser

	responses []string
	respIndex int
	mu        sync.Mutex

	closed bool
}

// newMockProcess creates a mock process with predefined responses.
func newMockProcess(responses []string) *mockProcess {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	mp := &mockProcess{
		stdin:        stdinWriter,
		stdout:       stdoutReader,
		stdinReader:  stdinReader,
		stdoutWriter: stdoutWriter,
		responses:    responses,
	}

	// Start a goroutine to handle requests
	go mp.handleRequests()

	return mp
}

func (mp *mockProcess) handleRequests() {
	scanner := bufio.NewScanner(mp.stdinReader)
	for scanner.Scan() {
		mp.mu.Lock()
		if mp.respIndex < len(mp.responses) {
			resp := mp.responses[mp.respIndex]
			mp.respIndex++
			mp.mu.Unlock()
			mp.stdoutWriter.Write([]byte(resp + "\n"))
		} else {
			mp.mu.Unlock()
			// No more responses
			break
		}
	}
}

func (mp *mockProcess) Close() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.closed {
		return nil
	}
	mp.closed = true
	mp.stdin.Close()
	mp.stdout.Close()
	mp.stdinReader.Close()
	mp.stdoutWriter.Close()
	return nil
}

// newTestClient creates a Client with a mock process for synchronous (callSync) tests.
// The responseRouter is NOT started; use this for callSync tests only.
func newTestClient(mp *mockProcess) *Client {
	return &Client{
		stdin:      mp.stdin,
		stdout:     mp.stdout,
		scanner:    bufio.NewScanner(mp.stdout),
		encoder:    json.NewEncoder(mp.stdin),
		timeout:    5 * time.Second,
		pending:    make(map[int]chan jsonrpcResponse),
		routerDone: make(chan struct{}),
	}
}

// newTestClientWithRouter creates a Client with pipes and starts the responseRouter.
// Returns the client, a stdinReader (to read what client sends), a stdoutWriter
// (to send responses to the client), and a cleanup function.
func newTestClientWithRouter(t *testing.T) (*Client, io.ReadCloser, io.WriteCloser, func()) {
	t.Helper()
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	client := &Client{
		stdin:       stdinWriter,
		stdout:      stdoutReader,
		scanner:     bufio.NewScanner(stdoutReader),
		encoder:     json.NewEncoder(stdinWriter),
		timeout:     5 * time.Second,
		initialized: true,
		pending:     make(map[int]chan jsonrpcResponse),
		routerDone:  make(chan struct{}),
	}

	go client.responseRouter()

	cleanup := func() {
		stdinWriter.Close()
		stdoutWriter.Close()
		stdinReader.Close()
		// Wait for router to exit
		<-client.routerDone
	}

	return client, stdinReader, stdoutWriter, cleanup
}

// TestCallSyncWithValidResponse tests that callSync() properly handles valid responses.
func TestCallSyncWithValidResponse(t *testing.T) {
	response := `{"jsonrpc":"2.0","id":1,"result":{"key":"value"}}`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := newTestClient(mp)

	var result map[string]string
	err := client.callSync("test_method", nil, &result)
	if err != nil {
		t.Fatalf("callSync() returned error: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("expected result[key] = value, got %v", result["key"])
	}
}

// TestCallSyncWithErrorResponse tests that callSync() properly handles error responses.
func TestCallSyncWithErrorResponse(t *testing.T) {
	response := `{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid Request"}}`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := newTestClient(mp)

	err := client.callSync("test_method", nil, nil)
	if err == nil {
		t.Fatal("expected error from callSync()")
	}

	if !strings.Contains(err.Error(), "Invalid Request") {
		t.Errorf("expected error to contain 'Invalid Request', got: %v", err)
	}
}

// TestCallSyncWithIDMismatch tests that callSync() detects response ID mismatches.
func TestCallSyncWithIDMismatch(t *testing.T) {
	// Response with wrong ID (2 instead of 1)
	response := `{"jsonrpc":"2.0","id":2,"result":{}}`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := newTestClient(mp)

	err := client.callSync("test_method", nil, nil)
	if err == nil {
		t.Fatal("expected error for ID mismatch")
	}

	if !strings.Contains(err.Error(), "response ID mismatch") {
		t.Errorf("expected error about ID mismatch, got: %v", err)
	}
}

// TestCallSyncWithMalformedResponse tests handling of invalid JSON responses in callSync.
func TestCallSyncWithMalformedResponse(t *testing.T) {
	response := `not valid json`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := newTestClient(mp)

	err := client.callSync("test_method", nil, nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}

	if !strings.Contains(err.Error(), "failed to parse response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

// TestCallWithValidResponse tests that call() with responseRouter handles valid responses.
func TestCallWithValidResponse(t *testing.T) {
	client, stdinReader, stdoutWriter, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	// Echo-style responder: reads requests, responds with matching IDs
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
			var req jsonrpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"key":"value"}}`, req.ID)
			stdoutWriter.Write([]byte(resp + "\n"))
		}
	}()

	var result map[string]string
	err := client.call(context.Background(), "test_method", nil, &result)
	if err != nil {
		t.Fatalf("call() returned error: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("expected result[key] = value, got %v", result["key"])
	}
}

// TestCallWithErrorResponse tests that call() with responseRouter handles error responses.
func TestCallWithErrorResponse(t *testing.T) {
	client, stdinReader, stdoutWriter, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	go func() {
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
			var req jsonrpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"error":{"code":-32600,"message":"Invalid Request"}}`, req.ID)
			stdoutWriter.Write([]byte(resp + "\n"))
		}
	}()

	err := client.call(context.Background(), "test_method", nil, nil)
	if err == nil {
		t.Fatal("expected error from call()")
	}

	if !strings.Contains(err.Error(), "Invalid Request") {
		t.Errorf("expected error to contain 'Invalid Request', got: %v", err)
	}
}

// TestCallWithContextTimeout tests that CallWithContext respects context deadlines.
func TestCallWithContextTimeout(t *testing.T) {
	client, stdinReader, _, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	// Drain stdin to prevent write blocking, but never respond
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := stdinReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.CallWithContext(ctx, "test_tool", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}

	if elapsed < 100*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Errorf("timeout occurred at unexpected time: %v", elapsed)
	}
}

// TestCallConcurrentAccess tests that call() is safe for concurrent access
// and all 10 concurrent calls complete with correct responses.
func TestCallConcurrentAccess(t *testing.T) {
	client, stdinReader, stdoutWriter, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	// Echo responder: responds with matching ID
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
			var req jsonrpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			resp := jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{}`),
			}
			data, _ := json.Marshal(resp)
			stdoutWriter.Write(data)
			stdoutWriter.Write([]byte("\n"))
		}
	}()

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := client.call(context.Background(), "test_method", nil, nil)
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent call failed: %v", err)
	}
}

// TestSetTimeout tests that SetTimeout correctly updates the timeout value.
func TestSetTimeout(t *testing.T) {
	client := &Client{
		timeout: DefaultTimeout,
	}

	newTimeout := 10 * time.Second
	client.SetTimeout(newTimeout)

	client.pendingMu.Lock()
	defer client.pendingMu.Unlock()

	if client.timeout != newTimeout {
		t.Errorf("expected timeout %v, got %v", newTimeout, client.timeout)
	}
}

// TestCallPublicMethod tests the public Call method that wraps internal call.
func TestCallPublicMethod(t *testing.T) {
	client, stdinReader, stdoutWriter, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	opResult := providerv1.OperationResult{
		Success: true,
		Resource: map[string]any{
			"name": "test-resource",
		},
	}
	opResultJSON, _ := json.Marshal(opResult)

	mcpResult := mcpToolCallResult{
		Content: []mcpContent{
			{Type: "text", Text: string(opResultJSON)},
		},
	}
	mcpResultJSON, _ := json.Marshal(mcpResult)

	go func() {
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
			var req jsonrpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, string(mcpResultJSON))
			stdoutWriter.Write([]byte(resp + "\n"))
		}
	}()

	result, err := client.Call("test_tool", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Call() returned error: %v", err)
	}

	if !result.Success {
		t.Error("expected success=true")
	}
}

// TestCallNotInitialized tests that Call returns error when not initialized.
func TestCallNotInitialized(t *testing.T) {
	client := &Client{
		initialized: false,
	}

	_, err := client.Call("test_tool", nil)
	if err == nil {
		t.Fatal("expected error when not initialized")
	}

	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected 'not initialized' error, got: %v", err)
	}
}

// TestInitializeRaceCondition tests that initialize() sets initialized safely.
func TestInitializeRaceCondition(t *testing.T) {
	initResponse := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","serverInfo":{"name":"test","version":"1.0.0"}}}`

	mp := newMockProcess([]string{initResponse})
	defer mp.Close()

	client := newTestClient(mp)

	err := client.initialize()
	if err != nil {
		t.Fatalf("initialize() returned error: %v", err)
	}

	if !client.initialized {
		t.Error("expected initialized=true after initialize()")
	}
}

// TestIsRunning tests the IsRunning method.
func TestIsRunning(t *testing.T) {
	client := &Client{
		cmd: nil,
	}
	if client.IsRunning() {
		t.Error("expected IsRunning()=false for nil cmd")
	}

	cmd := exec.Command("echo", "test")
	client = &Client{
		cmd: cmd,
	}
	if client.IsRunning() {
		t.Error("expected IsRunning()=false for unstarted command")
	}
}

// TestCallWithParams tests that parameters are correctly serialized.
func TestCallWithParams(t *testing.T) {
	client, stdinReader, stdoutWriter, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	var capturedRequest jsonrpcRequest
	var captured sync.WaitGroup
	captured.Add(1)
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		if scanner.Scan() {
			json.Unmarshal(scanner.Bytes(), &capturedRequest)
			captured.Done()
			// Respond with matching ID
			resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{}}`, capturedRequest.ID)
			stdoutWriter.Write([]byte(resp + "\n"))
		}
		// Drain remaining
		for scanner.Scan() {
		}
	}()

	params := struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}{
		Name:  "test",
		Value: 42,
	}

	err := client.call(context.Background(), "test_method", params, nil)
	if err != nil {
		t.Fatalf("call() returned error: %v", err)
	}

	captured.Wait()

	if capturedRequest.Params["name"] != "test" {
		t.Errorf("expected params.name=test, got %v", capturedRequest.Params["name"])
	}
	if capturedRequest.Params["value"] != float64(42) {
		t.Errorf("expected params.value=42, got %v", capturedRequest.Params["value"])
	}
}

// TestCallWithToolCallError tests handling of tool call errors.
func TestCallWithToolCallError(t *testing.T) {
	client, stdinReader, stdoutWriter, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	mcpResult := mcpToolCallResult{
		IsError: true,
		Content: []mcpContent{
			{Type: "text", Text: "Something went wrong"},
		},
	}
	mcpResultJSON, _ := json.Marshal(mcpResult)

	go func() {
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
			var req jsonrpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, string(mcpResultJSON))
			stdoutWriter.Write([]byte(resp + "\n"))
		}
	}()

	result, err := client.Call("failing_tool", nil)
	if err != nil {
		t.Fatalf("Call() returned error: %v", err)
	}

	if result.Success {
		t.Error("expected success=false for tool error")
	}
	if result.Error == nil {
		t.Error("expected error details to be present")
	}
	if result.Error != nil && result.Error.Message != "Something went wrong" {
		t.Errorf("expected error message 'Something went wrong', got: %s", result.Error.Message)
	}
}

// TestNotify tests the notify method for notifications.
func TestNotify(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, _ := io.Pipe()

	client := &Client{
		stdin:      stdinWriter,
		stdout:     stdoutReader,
		scanner:    bufio.NewScanner(stdoutReader),
		encoder:    json.NewEncoder(stdinWriter),
		timeout:    5 * time.Second,
		pending:    make(map[int]chan jsonrpcResponse),
		routerDone: make(chan struct{}),
	}

	var capturedNotification struct {
		JSONRPC string         `json:"jsonrpc"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params,omitempty"`
	}
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		if scanner.Scan() {
			json.Unmarshal(scanner.Bytes(), &capturedNotification)
		}
		close(done)
	}()

	err := client.notify("test/notification", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("notify() returned error: %v", err)
	}

	stdinWriter.Close()
	<-done

	if capturedNotification.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc=2.0, got %v", capturedNotification.JSONRPC)
	}
	if capturedNotification.Method != "test/notification" {
		t.Errorf("expected method=test/notification, got %v", capturedNotification.Method)
	}
}

// TestResponseRouterEOF tests that pending callers are unblocked when the
// provider process exits (scanner returns false).
func TestResponseRouterEOF(t *testing.T) {
	client, stdinReader, stdoutWriter, _ := newTestClientWithRouter(t)

	// Drain stdin
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := stdinReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Start a call that will never get a response
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.call(context.Background(), "test_method", nil, nil)
	}()

	// Give time for the call to register in pending
	time.Sleep(50 * time.Millisecond)

	// Close stdout to simulate provider exit -> scanner EOF -> router exit
	stdoutWriter.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error when router exits")
		}
		if !strings.Contains(err.Error(), "response router") {
			t.Errorf("expected 'response router' error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call() did not unblock after router exit")
	}

	// Wait for routerDone
	<-client.routerDone

	stdinReader.Close()
}

// TestResponseRouterOrphanedResponse tests that the router handles responses
// for unknown IDs without panicking.
func TestResponseRouterOrphanedResponse(t *testing.T) {
	client, stdinReader, stdoutWriter, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	// Drain stdin
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := stdinReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Send a response with an ID that nobody is waiting for
	orphanedResp := `{"jsonrpc":"2.0","id":9999,"result":{"key":"orphaned"}}` + "\n"
	stdoutWriter.Write([]byte(orphanedResp))

	// Give the router time to process
	time.Sleep(50 * time.Millisecond)

	// Now send a real request and verify it still works
	validResp := make(chan struct{})
	go func() {
		// Wait for the request and respond
		time.Sleep(10 * time.Millisecond)
		// We need to read the request to know its ID
		// Since we drained stdin above, just send a response for the next expected ID
		client.pendingMu.Lock()
		for id, ch := range client.pending {
			ch <- jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      id,
				Result:  json.RawMessage(`{"key":"valid"}`),
			}
			delete(client.pending, id)
			break
		}
		client.pendingMu.Unlock()
		close(validResp)
	}()

	var result map[string]string
	err := client.call(context.Background(), "test_method", nil, &result)
	if err != nil {
		t.Fatalf("call() after orphaned response returned error: %v", err)
	}

	<-validResp

	if result["key"] != "valid" {
		t.Errorf("expected result[key]=valid, got %v", result["key"])
	}
}

// TestCallContextCancellation tests that cancelling the context properly
// unblocks call() and cleans up the pending entry.
func TestCallContextCancellation(t *testing.T) {
	client, stdinReader, _, cleanup := newTestClientWithRouter(t)
	defer cleanup()

	// Drain stdin
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := stdinReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.call(ctx, "test_method", nil, nil)
	}()

	// Give time for the call to register
	time.Sleep(50 * time.Millisecond)

	// Verify there is a pending entry
	client.pendingMu.Lock()
	pendingCount := len(client.pending)
	client.pendingMu.Unlock()
	if pendingCount != 1 {
		t.Fatalf("expected 1 pending entry, got %d", pendingCount)
	}

	// Cancel the context
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call() did not unblock after context cancellation")
	}

	// Give time for defer cleanup
	time.Sleep(50 * time.Millisecond)

	// Verify pending map was cleaned up
	client.pendingMu.Lock()
	pendingCount = len(client.pending)
	client.pendingMu.Unlock()
	if pendingCount != 0 {
		t.Errorf("expected 0 pending entries after cancellation, got %d", pendingCount)
	}
}
