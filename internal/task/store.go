// Package task implements task lifecycle management.
package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"icc.tech/capture-agent/internal/config"
)

// TaskStore is the persistence interface for task state (ADR-030).
// All implementations must be safe for concurrent use.
type TaskStore interface {
	// Save persists a PersistedTask, overwriting any existing record for the same ID.
	Save(pt PersistedTask) error
	// Load retrieves a single PersistedTask by ID.
	// Returns os.ErrNotExist (via errors.Is) when not found.
	Load(id string) (PersistedTask, error)
	// Delete removes the persisted record for a task.
	// Returns nil when the record does not exist (idempotent).
	Delete(id string) error
	// List returns all persisted tasks; corrupt/unreadable entries are logged and skipped.
	List() ([]PersistedTask, error)
}

// PersistedTask is the on-disk wire format for a task (ADR-030 v1).
type PersistedTask struct {
	Version       string            `json:"version"`                  // "v1"
	Config        config.TaskConfig `json:"config"`                   // full TaskConfig
	State         TaskState         `json:"state"`                    // last known state
	CreatedAt     time.Time         `json:"created_at"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	StoppedAt     *time.Time        `json:"stopped_at,omitempty"`
	FailureReason string            `json:"failure_reason,omitempty"`
	RestartCount  int               `json:"restart_count"`
}

// persistenceVersion is the current wire format version.
const persistenceVersion = "v1"

// FileTaskStore persists tasks as individual JSON files under a directory.
// Write operations use temp-file + atomic rename to guarantee crash safety.
type FileTaskStore struct {
	dir string // absolute path to the tasks directory
}

// NewFileTaskStore creates a FileTaskStore rooted at dir.
// The directory is created (including parents) if it does not exist.
func NewFileTaskStore(dir string) (*FileTaskStore, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("task store: create directory %q: %w", dir, err)
	}
	return &FileTaskStore{dir: dir}, nil
}

// Save atomically writes pt using a unique temp file + rename.
// Using os.CreateTemp ensures concurrent saves for the same ID each get their
// own temp file, preventing races on the .tmp path.
func (s *FileTaskStore) Save(pt PersistedTask) error {
	if pt.Version == "" {
		pt.Version = persistenceVersion
	}

	data, err := json.MarshalIndent(pt, "", "  ")
	if err != nil {
		return fmt.Errorf("task store: marshal %q: %w", pt.Config.ID, err)
	}

	// Create a unique temp file in the same directory so rename is atomic.
	tmpFile, err := os.CreateTemp(s.dir, "."+pt.Config.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("task store: create temp file for %q: %w", pt.Config.ID, err)
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("task store: write temp file for %q: %w", pt.Config.ID, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("task store: close temp file for %q: %w", pt.Config.ID, err)
	}

	final := s.path(pt.Config.ID)
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("task store: rename temp → %q: %w", final, err)
	}

	slog.Debug("task state persisted", "task_id", pt.Config.ID, "state", pt.State)
	return nil
}

// Load reads and deserialises the persisted Task with the given id.
// Returns an error satisfying errors.Is(err, os.ErrNotExist) when not found.
func (s *FileTaskStore) Load(id string) (PersistedTask, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PersistedTask{}, fmt.Errorf("task store: %q not found: %w", id, os.ErrNotExist)
		}
		return PersistedTask{}, fmt.Errorf("task store: read %q: %w", id, err)
	}
	var pt PersistedTask
	if err := json.Unmarshal(data, &pt); err != nil {
		return PersistedTask{}, fmt.Errorf("task store: unmarshal %q: %w", id, err)
	}
	return pt, nil
}

// Delete removes the persisted file for id.
// Returns nil when the file does not exist (idempotent).
func (s *FileTaskStore) Delete(id string) error {
	err := os.Remove(s.path(id))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("task store: delete %q: %w", id, err)
	}
	slog.Debug("task state file removed", "task_id", id)
	return nil
}

// List reads all {id}.json files in the directory.
// Files that cannot be read or decoded are logged and skipped.
// Unrecognised file names (including .tmp files) are ignored.
func (s *FileTaskStore) List() ([]PersistedTask, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("task store: read directory %q: %w", s.dir, err)
	}

	var tasks []PersistedTask
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		pt, err := s.Load(id)
		if err != nil {
			slog.Warn("task store: skipping unreadable file",
				"file", filepath.Join(s.dir, name),
				"error", err,
			)
			continue
		}
		tasks = append(tasks, pt)
	}
	return tasks, nil
}

// path returns the absolute path to the JSON file for a given task ID.
func (s *FileTaskStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// noopStore is a TaskStore that does nothing, used when persistence is disabled.
type noopStore struct{}

func (noopStore) Save(_ PersistedTask) error           { return nil }
func (noopStore) Load(_ string) (PersistedTask, error) { return PersistedTask{}, os.ErrNotExist }
func (noopStore) Delete(_ string) error                { return nil }
func (noopStore) List() ([]PersistedTask, error)       { return nil, nil }

// Ensure noopStore satisfies the TaskStore interface at compile time.
var _ TaskStore = noopStore{}
