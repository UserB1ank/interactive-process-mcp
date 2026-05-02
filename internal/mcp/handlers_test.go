package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mac01/interactive-process-mcp/internal/message"
	"github.com/mac01/interactive-process-mcp/internal/session"
	"github.com/mac01/interactive-process-mcp/internal/sshserver"
	"github.com/mac01/interactive-process-mcp/internal/storage"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	srv := sshserver.New("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Stop() })

	dir := t.TempDir()
	store := storage.New(dir)
	msgMgr := message.NewManager(store)
	sessMgr := session.NewManager(srv.Addr(), msgMgr, store)
	return New(sessMgr, msgMgr)
}

func makeRequest(args map[string]any) mcpgo.CallToolRequest {
	return mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Arguments: args,
		},
	}
}

func parseResult(t *testing.T, result *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	text := result.Content[0].(mcpgo.TextContent).Text
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("failed to parse result: %v, text: %s", err, text)
	}
	return m
}

func TestHandleStartProcess_MissingCommand(t *testing.T) {
	s := newTestServer(t)
	req := makeRequest(map[string]any{})
	result, err := s.handleStartProcess(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing command")
	}
}

func TestHandleStartProcess_Success(t *testing.T) {
	s := newTestServer(t)
	req := makeRequest(map[string]any{
		"command": "echo",
		"args":    []any{"hello"},
		"mode":    "pipe",
	})

	result, err := s.handleStartProcess(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].(mcpgo.TextContent).Text)
	}

	m := parseResult(t, result)
	if m["session_id"] == nil {
		t.Fatal("expected session_id in result")
	}
}

func TestHandleSendInput_SessionNotFound(t *testing.T) {
	s := newTestServer(t)
	req := makeRequest(map[string]any{
		"session_id": "nonexistent",
		"text":       "hello",
	})

	result, err := s.handleSendInput(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleListSessions_Empty(t *testing.T) {
	s := newTestServer(t)
	req := makeRequest(map[string]any{})

	result, err := s.handleListSessions(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	m := parseResult(t, result)
	sessions := m["sessions"].([]any)
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestHandleTerminateProcess_SessionNotFound(t *testing.T) {
	s := newTestServer(t)
	req := makeRequest(map[string]any{
		"session_id": "nonexistent",
	})

	result, err := s.handleTerminateProcess(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleGetSessionInfo_NotFound(t *testing.T) {
	s := newTestServer(t)
	req := makeRequest(map[string]any{
		"session_id": "nonexistent",
	})

	result, err := s.handleGetSessionInfo(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleResizePty_NotFound(t *testing.T) {
	s := newTestServer(t)
	req := makeRequest(map[string]any{
		"session_id": "nonexistent",
		"rows":       float64(50),
		"cols":       float64(120),
	})

	result, err := s.handleResizePty(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleStartAndReadOutput(t *testing.T) {
	s := newTestServer(t)

	// Start a bash session
	startReq := makeRequest(map[string]any{
		"command": "bash",
		"mode":    "pty",
	})
	startResult, err := s.handleStartProcess(context.Background(), startReq)
	if err != nil {
		t.Fatal(err)
	}
	m := parseResult(t, startResult)
	sessionID := m["session_id"].(string)

	time.Sleep(300 * time.Millisecond)

	// Send input and read
	sarReq := makeRequest(map[string]any{
		"session_id":  sessionID,
		"text":        "echo handler_test",
		"press_enter": true,
		"timeout":     3.0,
	})
	sarResult, err := s.handleSendAndRead(context.Background(), sarReq)
	if err != nil {
		t.Fatal(err)
	}
	if sarResult.IsError {
		t.Fatalf("unexpected error: %s", sarResult.Content[0].(mcpgo.TextContent).Text)
	}

	sarM := parseResult(t, sarResult)
	output := sarM["output"].(string)
	if len(output) == 0 {
		t.Fatal("expected non-empty output")
	}

	// Cleanup
	termReq := makeRequest(map[string]any{
		"session_id": sessionID,
		"force":      true,
	})
	s.handleTerminateProcess(context.Background(), termReq)
}

func TestHandleListMessages(t *testing.T) {
	s := newTestServer(t)

	startReq := makeRequest(map[string]any{
		"command": "echo",
		"args":    []any{"test"},
		"mode":    "pipe",
	})
	startResult, _ := s.handleStartProcess(context.Background(), startReq)
	m := parseResult(t, startResult)
	sessionID := m["session_id"].(string)

	time.Sleep(500 * time.Millisecond)

	// List messages for this session
	listReq := makeRequest(map[string]any{
		"session_id": sessionID,
	})
	listResult, err := s.handleListMessages(context.Background(), listReq)
	if err != nil {
		t.Fatal(err)
	}

	listM := parseResult(t, listResult)
	msgs := listM["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
}
