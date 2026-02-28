// Package daemon implements the daemon lifecycle manager.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"icc.tech/capture-agent/internal/command"
	"icc.tech/capture-agent/internal/config"
	logpkg "icc.tech/capture-agent/internal/log"
	"icc.tech/capture-agent/internal/metrics"
	"icc.tech/capture-agent/internal/task"
)

// Daemon manages the capture-agent daemon process lifecycle.
type Daemon struct {
	// Configuration
	config     *config.GlobalConfig
	configPath string
	socketPath string
	pidFile    string

	// Core components
	taskManager   *task.TaskManager
	cmdHandler    *command.CommandHandler
	udsServer     *command.UDSServer
	kafkaConsumer *command.KafkaCommandConsumer // nil if command channel disabled
	metricsServer *metrics.Server               // nil if metrics disabled

	// Lifecycle management
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownChan chan struct{}
	sigChan      chan os.Signal // promoted from Run() local for cleanup in Stop()
}

// New creates a new Daemon instance.
func New(configPath, socketPath, pidFile string) (*Daemon, error) {
	// Load global configuration
	globalConfig, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Create daemon instance
	d := &Daemon{
		config:       globalConfig,
		configPath:   configPath,
		socketPath:   socketPath,
		pidFile:      pidFile,
		shutdownChan: make(chan struct{}),
	}

	// Create context for lifecycle management
	d.ctx, d.cancel = context.WithCancel(context.Background())

	return d, nil
}

// Start initializes and starts all daemon components.
func (d *Daemon) Start() error {
	slog.Info("starting capture-agent daemon",
		"version", "0.1.0",
		"hostname", d.config.Node.Hostname,
		"config", d.configPath,
		"socket", d.socketPath,
	)

	// 1. Initialize logging system
	if err := d.initLogging(); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	// 2. Write PID file
	if err := d.writePIDFile(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// 3. Start metrics server
	if err := d.startMetrics(); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	// 4. Create task manager with optional persistence store.
	var taskStore task.TaskStore
	if d.config.TaskPersistence.Enabled {
		storeDir := filepath.Join(d.config.DataDir, "tasks")
		store, storeErr := task.NewFileTaskStore(storeDir)
		if storeErr != nil {
			slog.Warn("failed to initialise task store, persistence disabled",
				"dir", storeDir, "error", storeErr)
		} else {
			taskStore = store
		}
	}
	d.taskManager = task.NewTaskManager(d.config.Node.Hostname, taskStore)

	// Restore previously active tasks from the persistent store.
	if d.config.TaskPersistence.Enabled && taskStore != nil {
		d.taskManager.Restore(d.config.TaskPersistence.AutoRestart)
	}

	// Start in-process GC goroutine to prune stale task history records.
	if d.config.TaskPersistence.Enabled && taskStore != nil {
		gcInterval, err := time.ParseDuration(d.config.TaskPersistence.GCInterval)
		if err != nil {
			slog.Warn("invalid task_persistence.gc_interval, defaulting to 1h",
				"value", d.config.TaskPersistence.GCInterval, "error", err)
			gcInterval = time.Hour
		}
		go func() {
			ticker := time.NewTicker(gcInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					d.taskManager.GCOldTasks(d.config.TaskPersistence.MaxTaskHistory)
				case <-d.ctx.Done():
					return
				}
			}
		}()
	}

	// 5. Create command handler
	d.cmdHandler = command.NewCommandHandler(d.taskManager, d)

	// 6. Wire shutdown handler so daemon_shutdown command can trigger graceful stop
	d.cmdHandler.SetShutdownFunc(func() {
		slog.Info("shutdown triggered via daemon_shutdown command")
		close(d.shutdownChan)
	})

	// 7. Start UDS server for CLI control
	d.udsServer = command.NewUDSServer(d.socketPath, d.cmdHandler)
	go func() {
		if err := d.udsServer.Start(d.ctx); err != nil && err != context.Canceled {
			slog.Error("uds server failed", "error", err)
		}
	}()

	// 8. Start Kafka command consumer (if enabled)
	if d.config.CommandChannel.Enabled && d.config.CommandChannel.Type == "kafka" {
		if err := d.startKafkaConsumer(); err != nil {
			slog.Error("failed to start kafka consumer", "error", err)
			// Non-fatal: daemon can still run with UDS-only control
		}
	}

	slog.Info("daemon started successfully")
	return nil
}

// Stop performs graceful shutdown of all daemon components.
func (d *Daemon) Stop() {
	slog.Info("initiating graceful shutdown")

	// 1. Stop Kafka command consumer first (no new commands)
	if d.kafkaConsumer != nil {
		slog.Info("stopping kafka command consumer")
		if err := d.kafkaConsumer.Stop(); err != nil {
			slog.Error("error stopping kafka consumer", "error", err)
		}
		d.kafkaConsumer = nil // prevent double-stop on repeated calls
	}

	// 2. Stop all running tasks
	slog.Info("stopping all tasks")
	if err := d.taskManager.StopAll(); err != nil {
		slog.Error("error stopping tasks", "error", err)
	}

	// 3. Stop UDS server (no new CLI commands)
	slog.Info("stopping uds server")
	d.udsServer.Stop()

	// 4. Stop metrics server
	if d.metricsServer != nil {
		slog.Info("stopping metrics server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.metricsServer.Stop(shutdownCtx); err != nil {
			slog.Error("error stopping metrics server", "error", err)
		}
	}

	// 5. Cancel context to signal all goroutines
	d.cancel()

	// 6. Unregister signal handler to prevent goroutine leak
	if d.sigChan != nil {
		signal.Stop(d.sigChan)
	}

	// 7. Remove PID file
	if err := d.removePIDFile(); err != nil {
		slog.Error("error removing PID file", "error", err)
	}

	// 8. Flush logs
	logpkg.Flush()

	slog.Info("daemon stopped gracefully")
}

// Run runs the daemon main loop, blocking until shutdown is triggered.
// Shutdown can be triggered by:
//  1. OS signals (SIGTERM, SIGINT)
//  2. daemon_shutdown command via UDS/Kafka
//  3. SIGHUP triggers config reload
func (d *Daemon) Run() error {
	// Setup signal handling
	d.sigChan = make(chan os.Signal, 1)
	signal.Notify(d.sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	slog.Info("daemon running, waiting for signals or commands")

	for {
		select {
		case sig := <-d.sigChan:
			switch sig {
			case syscall.SIGTERM, syscall.SIGINT:
				slog.Info("received shutdown signal", "signal", sig)
				d.Stop()
				return nil

			case syscall.SIGHUP:
				slog.Info("received reload signal")
				if err := d.Reload(); err != nil {
					slog.Error("failed to reload config", "error", err)
				} else {
					slog.Info("configuration reloaded successfully")
				}
			}

		case <-d.shutdownChan:
			// Shutdown triggered by daemon_shutdown command
			slog.Info("shutdown triggered by command")
			d.Stop()
			return nil

		case <-d.ctx.Done():
			// Context cancelled externally
			slog.Info("context cancelled", "error", d.ctx.Err())
			d.Stop()
			return d.ctx.Err()
		}
	}
}

// Reload reloads the global configuration.
// Hot-reloadable: log level/format, metrics collect interval.
// Cold (requires restart): node.hostname, task definitions, listen addresses.
// Implements ConfigReloader interface for CommandHandler.
func (d *Daemon) Reload() error {
	slog.Info("reloading configuration", "path", d.configPath)

	newConfig, err := config.Load(d.configPath)
	if err != nil {
		return fmt.Errorf("failed to load new config: %w", err)
	}

	// Track what was hot-reloaded for the log message
	hotReloaded := []string{}

	// 1. Re-initialize logging with new config (log level + format)
	oldLevel := d.config.Log.Level
	oldFormat := d.config.Log.Format
	d.config = newConfig
	if err := d.initLogging(); err != nil {
		slog.Error("failed to reinitialize logging", "error", err)
		// Non-fatal: old logging continues
	} else if newConfig.Log.Level != oldLevel || newConfig.Log.Format != oldFormat {
		hotReloaded = append(hotReloaded, "log")
	}

	// 2. Update metrics collection interval if changed
	if newConfig.Metrics.CollectInterval != "" {
		if interval, err := time.ParseDuration(newConfig.Metrics.CollectInterval); err == nil && interval > 0 {
			d.taskManager.UpdateMetricsInterval(interval)
			hotReloaded = append(hotReloaded, "metrics_interval")
		} else if err != nil {
			slog.Warn("invalid metrics.collect_interval, ignoring",
				"value", newConfig.Metrics.CollectInterval,
				"error", err)
		}
	}

	// 3. Warn about cold-reload items that changed
	requiresRestart := []string{}
	if newConfig.Node.Hostname != d.config.Node.Hostname {
		requiresRestart = append(requiresRestart, "node.hostname")
	}
	if newConfig.Metrics.Listen != d.config.Metrics.Listen {
		requiresRestart = append(requiresRestart, "metrics.listen")
	}

	slog.Info("configuration reloaded",
		"hot_reloaded", hotReloaded,
		"requires_restart", requiresRestart,
	)

	return nil
}

// TriggerShutdown triggers graceful shutdown from external caller (e.g., daemon_shutdown command).
func (d *Daemon) TriggerShutdown() {
	select {
	case d.shutdownChan <- struct{}{}:
		// Shutdown signal sent
	default:
		// Channel already has a value or is closed, no-op
	}
}

// initLogging initializes the logging system from config.
func (d *Daemon) initLogging() error {
	if err := logpkg.Init(d.config.Log); err != nil {
		return err
	}

	// Update global slog default to use the configured logger
	slog.SetDefault(logpkg.Get())

	slog.Debug("logging initialized",
		"level", d.config.Log.Level,
		"format", d.config.Log.Format,
	)

	return nil
}

// startKafkaConsumer starts the Kafka command consumer in background.
func (d *Daemon) startKafkaConsumer() error {
	consumer, err := command.NewKafkaCommandConsumer(
		d.config.CommandChannel,
		d.config.Node.Hostname,
		d.cmdHandler,
	)
	if err != nil {
		return fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	d.kafkaConsumer = consumer

	// Start consumer in background goroutine
	go func() {
		if err := consumer.Start(d.ctx); err != nil && err != context.Canceled {
			slog.Error("kafka consumer stopped with error", "error", err)
		}
	}()

	return nil
}

// startMetrics starts the metrics HTTP server if enabled.
func (d *Daemon) startMetrics() error {
	if !d.config.Metrics.Enabled {
		slog.Info("metrics server disabled")
		return nil
	}

	d.metricsServer = metrics.NewServer(d.config.Metrics.Listen, d.config.Metrics.Path)
	if err := d.metricsServer.Start(d.ctx); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	slog.Info("metrics server started",
		"addr", d.config.Metrics.Listen,
		"path", d.config.Metrics.Path,
	)

	return nil
}

// writePIDFile writes the current process ID to the PID file.
func (d *Daemon) writePIDFile() error {
	if d.pidFile == "" {
		return nil
	}

	pid := os.Getpid()
	data := []byte(strconv.Itoa(pid) + "\n")

	if err := os.WriteFile(d.pidFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write PID file %s: %w", d.pidFile, err)
	}

	slog.Debug("PID file written", "path", d.pidFile, "pid", pid)
	return nil
}

// removePIDFile removes the PID file.
func (d *Daemon) removePIDFile() error {
	if d.pidFile == "" {
		return nil
	}

	if err := os.Remove(d.pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file %s: %w", d.pidFile, err)
	}

	slog.Debug("PID file removed", "path", d.pidFile)
	return nil
}
