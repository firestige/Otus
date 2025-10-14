package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const (
	sockPath = "/tmp/otusd.sock"
	pidFile  = "/tmp/otusd.pid"
)

// EnsureDaemonRunning 确保守护进程运行，如果未运行则自动启动
func EnsureDaemonRunning() error {
	// 检查 socket 是否存在且可连接
	if isSocketAlive() {
		return nil
	}

	// 启动守护进程
	return startDaemon()
}

// StopDaemon 停止守护进程
func StopDaemon() error {
	pid, err := readPidFile()
	if err != nil {
		return fmt.Errorf("daemon not running")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	// 发送 SIGTERM 信号
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	// 等待进程退出
	time.Sleep(500 * time.Millisecond)
	os.Remove(sockPath)
	os.Remove(pidFile)
	return nil
}

func startDaemon() error {
	// 查找 otusd 可执行文件
	execPath, err := findDaemonExecutable()
	if err != nil {
		return err
	}

	// 以守护进程方式启动
	cmd := exec.Command(execPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // 创建新会话
	}

	// 可选：重定向日志
	logFile, _ := os.OpenFile("/tmp/otusd.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// 写入 PID 文件
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		return err
	}

	// 等待 socket 就绪
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if isSocketAlive() {
			return nil
		}
	}

	return fmt.Errorf("daemon started but socket not ready")
}

func isSocketAlive() bool {
	_, err := os.Stat(sockPath)
	return err == nil
}

func readPidFile() (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	return pid, nil
}

func findDaemonExecutable() (string, error) {
	// 1. 检查同目录
	execPath, _ := os.Executable()
	dir := filepath.Dir(execPath)
	daemonPath := filepath.Join(dir, "otusd")
	if _, err := os.Stat(daemonPath); err == nil {
		return daemonPath, nil
	}

	// 2. 检查 PATH 环境变量
	path, err := exec.LookPath("otusd")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("otusd executable not found")
}
