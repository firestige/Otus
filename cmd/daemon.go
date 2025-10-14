package cmd

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"firestige.xyz/otus/internal/rpc"
	pb "firestige.xyz/otus/pkg/pb"
	"google.golang.org/grpc"
)

const sockPath = "/tmp/otus.sock"

// Run 运行守护进程
func RunDaemon() {
	log.SetPrefix("[otus-daemon] ")
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Printf("Starting daemon (PID: %d)...", os.Getpid())

	// 清理旧 socket
	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", sockPath, err)
	}
	defer func() {
		listener.Close()
		os.Remove(sockPath)
	}()

	// 设置权限（只允许当前用户访问）
	if err := os.Chmod(sockPath, 0600); err != nil {
		log.Printf("Warning: failed to set socket permissions: %v", err)
	}

	// 创建 gRPC 服务器
	grpcServer := grpc.NewServer()
	srv := rpc.NewServer()
	pb.RegisterDaemonServiceServer(grpcServer, srv)

	// 优雅退出处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down gracefully...", sig)
		srv.Shutdown() // 停止业务逻辑
		grpcServer.GracefulStop()
	}()

	log.Printf("Daemon ready (Socket: %s)", sockPath)

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("Daemon stopped")
}
