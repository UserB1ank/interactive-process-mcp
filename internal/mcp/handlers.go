package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mac01/interactive-process-mcp/internal/session"
)

func getString(args map[string]any, key, def string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func getBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

func getFloat64(args map[string]any, key string, def float64) float64 {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case int64:
			return float64(n)
		}
	}
	return def
}

func getStringSlice(args map[string]any, key string) []string {
	if v, ok := args[key]; ok {
		if arr, ok := v.([]any); ok {
			var result []string
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

func (s *Server) handleStartProcess(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	command := getString(args, "command", "")
	if command == "" {
		return mcpgo.NewToolResultError("command is required"), nil
	}

	sess, err := s.sessMgr.Create(session.Config{
		Command: command,
		Args:    getStringSlice(args, "args"),
		Mode:    getString(args, "mode", "pty"),
		Name:    getString(args, "name", ""),
		Rows:    int(getFloat64(args, "rows", 24)),
		Cols:    int(getFloat64(args, "cols", 80)),
	})
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}

	time.Sleep(100 * time.Millisecond)
	initial, _ := sess.ReadOutput(500*time.Millisecond, true, 0)

	result := map[string]any{
		"session_id":     sess.ID,
		"pid":            sess.PID,
		"initial_output": initial,
	}
	data, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Server) handleSendInput(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")
	text := getString(args, "text", "")
	pressEnter := getBool(args, "press_enter", false)

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	if err := sess.SendInput(text, pressEnter); err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultText(`{"success":true}`), nil
}

func (s *Server) handleReadOutput(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")
	stripAnsi := getBool(args, "strip_ansi", true)
	timeout := getFloat64(args, "timeout", 5.0)
	maxLines := int(getFloat64(args, "max_lines", 0))
	readerID := int(getFloat64(args, "reader_id", 0))

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	output, err := sess.ReadOutputForReader(readerID, time.Duration(timeout*float64(time.Second)), stripAnsi, maxLines)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	result := map[string]any{
		"output":         output,
		"has_more":       sess.HasMoreOutput(readerID),
		"lines_returned": strings.Count(output, "\n"),
		"bytes_returned": len(output),
	}
	data, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Server) handleSendAndRead(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")
	text := getString(args, "text", "")
	pressEnter := getBool(args, "press_enter", false)
	stripAnsi := getBool(args, "strip_ansi", true)
	timeout := getFloat64(args, "timeout", 5.0)
	maxLines := int(getFloat64(args, "max_lines", 0))
	readerID := int(getFloat64(args, "reader_id", 0))

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	if err := sess.SendInput(text, pressEnter); err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	time.Sleep(100 * time.Millisecond)
	output, err := sess.ReadOutputForReader(readerID, time.Duration(timeout*float64(time.Second)), stripAnsi, maxLines)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	result := map[string]any{
		"output":         output,
		"has_more":       sess.HasMoreOutput(readerID),
		"lines_returned": strings.Count(output, "\n"),
		"bytes_returned": len(output),
	}
	data, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Server) handleListSessions(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	sessions := s.sessMgr.ListAll()
	result := map[string]any{"sessions": sessions}
	data, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetSessionInfo(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	info := sess.Info()
	data, _ := json.Marshal(info)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Server) handleTerminateProcess(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")
	force := getBool(args, "force", false)
	gracePeriod := getFloat64(args, "grace_period", 5.0)

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	s.sessMgr.Terminate(sessionID, force, gracePeriod)
	return mcpgo.NewToolResultText(`{"success":true}`), nil
}

func (s *Server) handleDeleteSession(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	if sess.Info().Status == "running" {
		return mcpgo.NewToolResultError("cannot delete a running session, terminate it first"), nil
	}
	s.sessMgr.Delete(sessionID)
	return mcpgo.NewToolResultText(`{"success":true}`), nil
}

func (s *Server) handleResizePty(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")
	rows := int(getFloat64(args, "rows", 24))
	cols := int(getFloat64(args, "cols", 80))

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	if err := sess.ResizePty(rows, cols); err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultText(`{"success":true}`), nil
}

func (s *Server) handleListMessages(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")

	entries, err := s.msgMgr.List(sessionID)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	result := map[string]any{"messages": entries}
	data, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetMessage(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")
	msgIDs := getStringSlice(args, "message_ids")

	if len(msgIDs) == 0 {
		if id := getString(args, "message_id", ""); id != "" {
			msgIDs = append(msgIDs, id)
		}
	}

	messages, err := s.msgMgr.GetMany(sessionID, msgIDs)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	result := map[string]any{"messages": messages}
	data, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Server) handleRegisterReader(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	readerID := sess.RegisterReader()
	result := map[string]any{"reader_id": readerID}
	data, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Server) handleUnregisterReader(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := request.GetArguments()
	sessionID := getString(args, "session_id", "")
	readerID := int(getFloat64(args, "reader_id", 0))

	sess := s.sessMgr.Get(sessionID)
	if sess == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("Session '%s' not found", sessionID)), nil
	}
	sess.UnregisterReader(readerID)
	return mcpgo.NewToolResultText(`{"success":true}`), nil
}
