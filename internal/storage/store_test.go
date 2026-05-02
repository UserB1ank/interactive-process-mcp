package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mac01/interactive-process-mcp/pkg/api"
)

func TestStore_SaveLoadSessions(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	sessions := []api.Session{
		{ID: "abc123", Command: "bash", Mode: "pty", Status: api.SessionRunning},
		{ID: "def456", Command: "cat", Mode: "pipe", Status: api.SessionExited},
	}

	if err := s.SaveSessions(sessions); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(loaded))
	}
	if loaded[0].ID != "abc123" {
		t.Fatalf("expected first session ID 'abc123', got %q", loaded[0].ID)
	}
}

func TestStore_LoadSessionsEmpty(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	loaded, err := s.LoadSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(loaded))
	}
}

func TestStore_SaveLoadMessageIndex(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	entries := []api.MessageIndexEntry{
		{ID: "msg001", Type: api.MsgInput, ByteSize: 10},
		{ID: "msg002", Type: api.MsgOutput, ByteSize: 50},
	}

	if err := s.SaveMessageIndex("sess1", entries); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadMessageIndex("sess1")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	if loaded[0].ID != "msg001" {
		t.Fatalf("expected first entry ID 'msg001', got %q", loaded[0].ID)
	}
}

func TestStore_SaveLoadMessage(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	msg := api.Message{
		ID:        "msg001",
		SessionID: "sess1",
		Type:      api.MsgInput,
		Content:   "hello world",
	}

	if err := s.SaveMessage("sess1", msg); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadMessage("sess1", "msg001")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Content != "hello world" {
		t.Fatalf("expected 'hello world', got %q", loaded.Content)
	}
}

func TestStore_LoadMessageNotFound(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	_, err := s.LoadMessage("sess1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent message")
	}
}

func TestStore_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	// Saving a message should create nested directories
	msg := api.Message{ID: "m1", SessionID: "s1", Type: api.MsgOutput, Content: "x"}
	if err := s.SaveMessage("s1", msg); err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(dir, "messages", "s1", "messages", "m1.json")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Fatalf("expected file %q to exist", expected)
	}
}
