package rpc

import (
	"context"
	"fmt"
	"time"

	pb "firestige.xyz/otus/pkg/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const sockPath = "/tmp/otus.sock"

type Client struct {
	conn   *grpc.ClientConn
	client pb.DaemonServiceClient
}

// NewClient 创建客户端连接
func NewClient() (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		"unix://"+sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewDaemonServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Start(ctx context.Context) error {
	resp, err := c.client.Start(ctx, &pb.StartRequest{})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}
	return nil
}

func (c *Client) Stop(ctx context.Context) error {
	resp, err := c.client.Stop(ctx, &pb.StopRequest{})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}
	return nil
}

func (c *Client) Reload(ctx context.Context) error {
	resp, err := c.client.Reload(ctx, &pb.ReloadRequest{})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}
	return nil
}
