package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/mac01/interactive-process-mcp/internal/config"
	mcpmod "github.com/mac01/interactive-process-mcp/internal/mcp"
	"github.com/mac01/interactive-process-mcp/internal/message"
	"github.com/mac01/interactive-process-mcp/internal/session"
	"github.com/mac01/interactive-process-mcp/internal/sshserver"
	"github.com/mac01/interactive-process-mcp/internal/storage"
)

func main() {
	cfg := config.Default()
	flag.StringVar(&cfg.Host, "host", cfg.Host, "HTTP server host")
	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Data directory for JSON storage")
	flag.StringVar(&cfg.SSHHost, "ssh-host", cfg.SSHHost, "Internal SSH server host")
	flag.IntVar(&cfg.SSHPort, "ssh-port", cfg.SSHPort, "Internal SSH server port (0 = random)")
	flag.Parse()

	// Start internal SSH server
	sshAddr := fmt.Sprintf("%s:%d", cfg.SSHHost, cfg.SSHPort)
	sshSrv := sshserver.New(sshAddr)
	if err := sshSrv.Start(); err != nil {
		log.Fatalf("Failed to start SSH server: %v", err)
	}
	actualSSHAddr := sshSrv.Addr()
	log.Printf("Internal SSH server listening on %s", actualSSHAddr)

	// Initialize storage and managers
	store := storage.New(cfg.DataDir)
	msgMgr := message.NewManager(store)
	sessMgr := session.NewManager(actualSSHAddr, msgMgr, store)

	// Start MCP SSE server
	mcpSrv := mcpmod.New(sessMgr, msgMgr)
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("MCP SSE server listening on %s", addr)

	var shuttingDown atomic.Bool

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		shuttingDown.Store(true)
		log.Println("Shutting down...")
		sessMgr.CleanupAll(true)
		sshSrv.Stop()
		mcpSrv.Stop()
	}()

	if err := mcpSrv.Start(addr); err != nil {
		if shuttingDown.Load() && errors.Is(err, http.ErrServerClosed) {
			log.Println("Server stopped")
			return
		}
		log.Fatalf("Failed to start MCP server: %v", err)
	}
}
