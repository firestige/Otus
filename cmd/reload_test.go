package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockClient 实现 ClientInterface
type MockClient struct {
	mock.Mock
}

func (m *MockClient) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockClient) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockClient) Reload(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// 测试成功场景
func TestRunReload_Success(t *testing.T) {
	// 准备
	mockClient := new(MockClient)
	mockClient.On("Reload", mock.Anything).Return(nil)

	var buf bytes.Buffer
	ctx := context.Background()

	// 执行
	err := runReload(ctx, mockClient, &buf)

	// 断言
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "✓ Configuration reloaded successfully")
	mockClient.AssertExpectations(t)
}

// 测试失败场景
func TestRunReload_Failure(t *testing.T) {
	// 准备
	mockClient := new(MockClient)
	expectedErr := errors.New("connection failed")
	mockClient.On("Reload", mock.Anything).Return(expectedErr)

	var buf bytes.Buffer
	ctx := context.Background()

	// 执行
	err := runReload(ctx, mockClient, &buf)

	// 断言
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to reload")
	assert.Contains(t, err.Error(), "connection failed")
	assert.Empty(t, buf.String())
	mockClient.AssertExpectations(t)
}

// 测试 Cobra 命令集成
func TestReloadCmd_Execute(t *testing.T) {
	// 准备
	mockClient := new(MockClient)
	mockClient.On("Reload", mock.Anything).Return(nil)

	// 使用 SetClient 注入 mock
	originalCli := GetClient()
	SetClient(mockClient)
	defer SetClient(originalCli) // 测试结束后恢复

	// 创建根命令
	rootCmd := &cobra.Command{Use: "otus"}
	rootCmd.AddCommand(reloadCmd)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"reload"})

	// 执行
	err := rootCmd.Execute()

	// 断言
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "✓ Configuration reloaded successfully")
	mockClient.AssertExpectations(t)
}

// 表驱动测试
func TestRunReload_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		mockError      error
		expectedError  bool
		expectedOutput string
	}{
		{
			name:           "成功重载",
			mockError:      nil,
			expectedError:  false,
			expectedOutput: "✓ Configuration reloaded successfully",
		},
		{
			name:           "网络错误",
			mockError:      errors.New("network timeout"),
			expectedError:  true,
			expectedOutput: "",
		},
		{
			name:           "守护进程未运行",
			mockError:      errors.New("daemon not running"),
			expectedError:  true,
			expectedOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 准备
			mockClient := new(MockClient)
			mockClient.On("Reload", mock.Anything).Return(tt.mockError)

			var buf bytes.Buffer
			ctx := context.Background()

			// 执行
			err := runReload(ctx, mockClient, &buf)

			// 断言
			if tt.expectedError {
				assert.Error(t, err)
				if tt.mockError != nil {
					assert.Contains(t, err.Error(), tt.mockError.Error())
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedOutput != "" {
				assert.Contains(t, buf.String(), tt.expectedOutput)
			}

			mockClient.AssertExpectations(t)
		})
	}
}
