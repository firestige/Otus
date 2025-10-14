package cmd

import (
	"context"
)

// ClientInterface 定义所有命令需要的客户端方法
type ClientInterface interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Reload(ctx context.Context) error
	Close() error
}
