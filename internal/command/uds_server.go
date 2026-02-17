// Package command implements command channels.
package command

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
)

// UDSServer implements a JSON-RPC server over Unix Domain Socket.
type UDSServer struct {
	socketPath string
	handler    *CommandHandler
	listener   net.Listener
	
	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	wg      sync.WaitGroup
	stopped bool
}

// NewUDSServer creates a new UDS server.
func NewUDSServer(socketPath string, handler *CommandHandler) *UDSServer {
	return &UDSServer{
		socketPath: socketPath,
		handler:    handler,
		conns:      make(map[net.Conn]struct{}),
	}
}

// Start starts the UDS server.
// Blocks until context is cancelled or an error occurs.
func (s *UDSServer) Start(ctx context.Context) error {
	// Remove existing socket file if it exists
	if err := os.RemoveAll(s.socketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create Unix listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", s.socketPath, err)
	}
	s.listener = listener

	// Set socket permissions (0600 - owner only)
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	slog.Info("uds server started", "socket", s.socketPath)

	// Accept connections in background
	go s.acceptLoop(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	slog.Info("uds server stopping", "reason", ctx.Err())
	
	return s.Stop()
}

// acceptLoop accepts incoming connections.
func (s *UDSServer) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			stopped := s.stopped
			s.mu.Unlock()
			
			if stopped {
				return
			}
			
			slog.Error("failed to accept connection", "error", err)
			continue
		}

		// Track connection
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			conn.Close()
			return
		}
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		// Handle connection in goroutine
		s.wg.Add(1)
		go s.handleConnection(ctx, conn)
	}
}

// handleConnection handles a single connection.
func (s *UDSServer) handleConnection(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	slog.Debug("uds connection established", "remote", conn.RemoteAddr())

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		line := scanner.Bytes()
		
		// Parse JSON-RPC request
		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			slog.Error("failed to parse request", "error", err)
			// Send error response
			errResp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &ErrorInfo{
					Code:    ErrCodeParseError,
					Message: fmt.Sprintf("parse error: %v", err),
				},
			}
			encoder.Encode(errResp)
			continue
		}

		// Convert to internal Command format
		cmd := Command{
			Method: req.Method,
			Params: req.Params,
			ID:     fmt.Sprintf("%v", req.ID), // Convert to string
		}

		// Handle command
		resp := s.handler.Handle(ctx, cmd)

		// Convert to JSON-RPC response
		jsonrpcResp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resp.Result,
			Error:   resp.Error,
		}

		// Send response
		if err := encoder.Encode(jsonrpcResp); err != nil {
			slog.Error("failed to send response", "error", err)
			return
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("connection error", "error", err)
	}

	slog.Debug("uds connection closed", "remote", conn.RemoteAddr())
}

// Stop stops the UDS server.
func (s *UDSServer) Stop() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	s.mu.Unlock()

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Close all active connections
	s.mu.Lock()
	for conn := range s.conns {
		conn.Close()
	}
	s.mu.Unlock()

	// Wait for all handlers to finish
	s.wg.Wait()

	// Remove socket file
	os.RemoveAll(s.socketPath)

	slog.Info("uds server stopped")
	return nil
}

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}
