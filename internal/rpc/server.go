package rpc

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pb "firestige.xyz/otus/pkg/pb"
)

const version = "1.0.0"

type Server struct {
	pb.UnimplementedDaemonServiceServer

	running   atomic.Bool
	startTime time.Time
	mu        sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		startTime: time.Now(),
	}
}

func (s *Server) Start(ctx context.Context, req *pb.StartRequest) (*pb.Response, error) {
	if s.running.Load() {
		return &pb.Response{
			Success: false,
			Message: "Service is already running",
		}, nil
	}

	// if err := s.worker.Start(); err != nil {
	// 	return &pb.Response{
	// 		Success: false,
	// 		Message: fmt.Sprintf("Failed to start: %v", err),
	// 	}, nil
	// }

	s.running.Store(true)
	s.startTime = time.Now()

	return &pb.Response{
		Success: true,
		Message: "Service started successfully",
	}, nil
}

func (s *Server) Stop(ctx context.Context, req *pb.StopRequest) (*pb.Response, error) {
	if !s.running.Load() {
		return &pb.Response{
			Success: false,
			Message: "Service is not running",
		}, nil
	}

	// s.worker.Stop()
	s.running.Store(false)

	return &pb.Response{
		Success: true,
		Message: "Service stopped successfully",
	}, nil
}

func (s *Server) Reload(ctx context.Context, req *pb.ReloadRequest) (*pb.Response, error) {
	// if err := s.worker.Reload(); err != nil {
	// 	return &pb.Response{
	// 		Success: false,
	// 		Message: fmt.Sprintf("Failed to reload: %v", err),
	// 	}, nil
	// }

	return &pb.Response{
		Success: true,
		Message: "Configuration reloaded successfully",
	}, nil
}

// Shutdown 优雅关闭
func (s *Server) Shutdown() {
	if s.running.Load() {
		// s.worker.Stop()
		s.running.Store(false)
	}
}
