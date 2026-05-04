package mcp

import (
	"context"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mac01/interactive-process-mcp/internal/message"
	"github.com/mac01/interactive-process-mcp/internal/session"
)

// Server wraps the MCP SSE server and tool handlers.
type Server struct {
	mcpServer *mcpserver.MCPServer
	sseServer *mcpserver.SSEServer
	sessMgr   *session.Manager
	msgMgr    *message.Manager
}

// New creates and configures the MCP server with all tools registered.
func New(sessMgr *session.Manager, msgMgr *message.Manager) *Server {
	s := &Server{
		sessMgr: sessMgr,
		msgMgr:  msgMgr,
	}

	mcpServer := mcpserver.NewMCPServer("interactive-process", "0.1.0")

	mcpServer.AddTool(mcpgo.NewTool("start_process",
		mcpgo.WithDescription("Start an interactive process and return its session info"),
		mcpgo.WithString("command", mcpgo.Required(), mcpgo.Description("Command to execute")),
		mcpgo.WithArray("args", mcpgo.Description("Command arguments"), mcpgo.WithStringItems()),
		mcpgo.WithString("mode", mcpgo.Description("I/O mode: pty or pipe"), mcpgo.DefaultString("pty")),
		mcpgo.WithString("name", mcpgo.Description("Session name")),
		mcpgo.WithNumber("rows", mcpgo.Description("PTY rows"), mcpgo.DefaultNumber(24)),
		mcpgo.WithNumber("cols", mcpgo.Description("PTY columns"), mcpgo.DefaultNumber(80)),
	), s.handleStartProcess)

	mcpServer.AddTool(mcpgo.NewTool("send_input",
		mcpgo.WithDescription("Send text input to a running interactive process"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithString("text", mcpgo.Required(), mcpgo.Description("Text to send")),
		mcpgo.WithBoolean("press_enter", mcpgo.Description("Append newline after text"), mcpgo.DefaultBool(false)),
	), s.handleSendInput)

	mcpServer.AddTool(mcpgo.NewTool("read_output",
		mcpgo.WithDescription("Read new output from an interactive process since last read"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithBoolean("strip_ansi", mcpgo.Description("Remove ANSI escape codes"), mcpgo.DefaultBool(true)),
		mcpgo.WithNumber("timeout", mcpgo.Description("Seconds to wait for new output"), mcpgo.DefaultNumber(5)),
		mcpgo.WithNumber("max_lines", mcpgo.Description("Max lines to return (0 = unlimited)"), mcpgo.DefaultNumber(0)),
		mcpgo.WithNumber("reader_id", mcpgo.Description("Reader ID (0 = default shared reader)"), mcpgo.DefaultNumber(0)),
	), s.handleReadOutput)

	mcpServer.AddTool(mcpgo.NewTool("send_and_read",
		mcpgo.WithDescription("Send input to a process and immediately read its response"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithString("text", mcpgo.Required(), mcpgo.Description("Text to send")),
		mcpgo.WithBoolean("press_enter", mcpgo.Description("Append newline after text"), mcpgo.DefaultBool(false)),
		mcpgo.WithBoolean("strip_ansi", mcpgo.Description("Remove ANSI escape codes"), mcpgo.DefaultBool(true)),
		mcpgo.WithNumber("timeout", mcpgo.Description("Seconds to wait for response"), mcpgo.DefaultNumber(5)),
		mcpgo.WithNumber("max_lines", mcpgo.Description("Max lines to return (0 = unlimited)"), mcpgo.DefaultNumber(0)),
			mcpgo.WithNumber("reader_id", mcpgo.Description("Reader ID (0 = default shared reader)"), mcpgo.DefaultNumber(0)),
	), s.handleSendAndRead)

	mcpServer.AddTool(mcpgo.NewTool("list_sessions",
		mcpgo.WithDescription("List all interactive process sessions"),
	), s.handleListSessions)

	mcpServer.AddTool(mcpgo.NewTool("get_session_info",
		mcpgo.WithDescription("Get detailed information about a session"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
	), s.handleGetSessionInfo)

	mcpServer.AddTool(mcpgo.NewTool("terminate_process",
		mcpgo.WithDescription("Terminate an interactive process"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithBoolean("force", mcpgo.Description("Use SIGKILL directly"), mcpgo.DefaultBool(false)),
		mcpgo.WithNumber("grace_period", mcpgo.Description("Seconds to wait after SIGTERM"), mcpgo.DefaultNumber(5)),
	), s.handleTerminateProcess)

	mcpServer.AddTool(mcpgo.NewTool("delete_session",
		mcpgo.WithDescription("Delete an exited session from the registry"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
	), s.handleDeleteSession)

	mcpServer.AddTool(mcpgo.NewTool("resize_pty",
		mcpgo.WithDescription("Resize the PTY terminal dimensions for a session"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithNumber("rows", mcpgo.Description("Row count"), mcpgo.DefaultNumber(24)),
		mcpgo.WithNumber("cols", mcpgo.Description("Column count"), mcpgo.DefaultNumber(80)),
	), s.handleResizePty)

	mcpServer.AddTool(mcpgo.NewTool("list_messages",
		mcpgo.WithDescription("List the message index for a session"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
	), s.handleListMessages)

	mcpServer.AddTool(mcpgo.NewTool("get_message",
		mcpgo.WithDescription("Get the content of one or more messages"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithArray("message_ids", mcpgo.Description("Message IDs to retrieve"), mcpgo.WithStringItems()),
	), s.handleGetMessage)

	mcpServer.AddTool(mcpgo.NewTool("register_reader",
		mcpgo.WithDescription("Register a new independent reader for a session's output. Each reader has its own cursor."),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
	), s.handleRegisterReader)

	mcpServer.AddTool(mcpgo.NewTool("unregister_reader",
		mcpgo.WithDescription("Unregister a reader when it is no longer needed"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithNumber("reader_id", mcpgo.Required(), mcpgo.Description("Reader ID to unregister")),
	), s.handleUnregisterReader)

	mcpServer.AddTool(mcpgo.NewTool("upload_file",
		mcpgo.WithDescription("Upload a file to the process environment via SFTP. Max 1MB. For large files, use send_input with curl/wget instead."),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithString("content_base64", mcpgo.Required(), mcpgo.Description("File content encoded as base64")),
		mcpgo.WithString("remote_path", mcpgo.Required(), mcpgo.Description("Destination path in the process environment")),
	), s.handleUploadFile)

	mcpServer.AddTool(mcpgo.NewTool("download_file",
		mcpgo.WithDescription("Download a file from the process environment via SFTP. Text files returned as plain text, binary files as base64. Max 1MB."),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithString("remote_path", mcpgo.Required(), mcpgo.Description("Path of the file to download")),
	), s.handleDownloadFile)

	mcpServer.AddTool(mcpgo.NewTool("list_files",
		mcpgo.WithDescription("List files and directories at a path in the process environment via SFTP"),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID")),
		mcpgo.WithString("remote_path", mcpgo.Required(), mcpgo.Description("Directory path to list")),
	), s.handleListFiles)

	s.mcpServer = mcpServer
	s.sseServer = mcpserver.NewSSEServer(mcpServer)
	return s
}

// Start begins serving MCP over SSE on the given address.
func (s *Server) Start(addr string) error {
	return s.sseServer.Start(addr)
}

// Stop gracefully shuts down the SSE server.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.sseServer.Shutdown(ctx)
}
