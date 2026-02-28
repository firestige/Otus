package task

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"icc.tech/capture-agent/internal/config"
)

func testPersistedTask(id, state string) PersistedTask {
	now := time.Now().UTC().Truncate(time.Second)
	return PersistedTask{
		Version: persistenceVersion,
		Config: config.TaskConfig{
			ID: id,
		},
		State:     TaskState(state),
		CreatedAt: now,
	}
}

func newTestStore(t *testing.T) *FileTaskStore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewFileTaskStore(filepath.Join(dir, "tasks"))
	if err != nil {
		t.Fatalf("NewFileTaskStore: %v", err)
	}
	return store
}

// ---------------------------------------------------------------------------
// Basic CRUD
// ---------------------------------------------------------------------------

func TestFileTaskStore_SaveLoad(t *testing.T) {
	store := newTestStore(t)
	pt := testPersistedTask("abc123", "running")

	if err := store.Save(pt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load("abc123")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Config.ID != pt.Config.ID {
		t.Errorf("Config.ID: got %q, want %q", got.Config.ID, pt.Config.ID)
	}
	if got.State != pt.State {
		t.Errorf("State: got %q, want %q", got.State, pt.State)
	}
	if got.Version != persistenceVersion {
		t.Errorf("Version: got %q, want %q", got.Version, persistenceVersion)
	}
}

func TestFileTaskStore_Load_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Load("does-not-exist")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestFileTaskStore_Delete(t *testing.T) {
	store := newTestStore(t)
	pt := testPersistedTask("del1", "stopped")

	if err := store.Save(pt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Delete("del1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Load("del1")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist after Delete, got %v", err)
	}
}

func TestFileTaskStore_Delete_Idempotent(t *testing.T) {
	store := newTestStore(t)
	if err := store.Delete("ghost"); err != nil {
		t.Errorf("deleting non-existent task should not error, got %v", err)
	}
}

func TestFileTaskStore_List(t *testing.T) {
	store := newTestStore(t)
	ids := []string{"t1", "t2", "t3"}
	for _, id := range ids {
		if err := store.Save(testPersistedTask(id, "running")); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(ids) {
		t.Errorf("List len: got %d, want %d", len(list), len(ids))
	}
}

func TestFileTaskStore_List_Empty(t *testing.T) {
	store := newTestStore(t)
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
}

// ---------------------------------------------------------------------------
// Overwrite / update
// ---------------------------------------------------------------------------

func TestFileTaskStore_SaveOverwrites(t *testing.T) {
	store := newTestStore(t)
	pt := testPersistedTask("upd1", "running")

	if err := store.Save(pt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	pt.State = StateStopped
	stopped := time.Now().UTC().Truncate(time.Second)
	pt.StoppedAt = &stopped

	if err := store.Save(pt); err != nil {
		t.Fatalf("Save (update): %v", err)
	}
	got, err := store.Load("upd1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.State != StateStopped {
		t.Errorf("State: got %q, want %q", got.State, StateStopped)
	}
	if got.StoppedAt == nil || *got.StoppedAt != stopped {
		t.Errorf("StoppedAt mismatch")
	}
}

// ---------------------------------------------------------------------------
// Atomic write: no .tmp file left after Save
// ---------------------------------------------------------------------------

func TestFileTaskStore_AtomicWrite_NoTmpFileAfterSave(t *testing.T) {
	store := newTestStore(t)

	if err := store.Save(testPersistedTask("atomic1", "running")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// After a successful Save there must be no leftover .tmp files.
	entries, err := os.ReadDir(store.dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("unexpected .tmp file after Save: %s", e.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrent writes
// ---------------------------------------------------------------------------

func TestFileTaskStore_ConcurrentSave(t *testing.T) {
	store := newTestStore(t)
	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			pt := testPersistedTask("concurrent-task", "running")
			pt.RestartCount = i
			errs[i] = store.Save(pt)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d Save error: %v", i, err)
		}
	}
	if _, err := store.Load("concurrent-task"); err != nil {
		t.Errorf("Load after concurrent saves: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Corrupted file is skipped by List
// ---------------------------------------------------------------------------

func TestFileTaskStore_List_SkipsCorruptedFile(t *testing.T) {
	store := newTestStore(t)

	if err := store.Save(testPersistedTask("good", "running")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	corrupt := filepath.Join(store.dir, "bad.json")
	if err := os.WriteFile(corrupt, []byte("{invalid json"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 valid record, got %d", len(list))
	}
	if len(list) == 1 && list[0].Config.ID != "good" {
		t.Errorf("wrong task returned: %q", list[0].Config.ID)
	}
}

// ---------------------------------------------------------------------------
// List ignores .tmp files
// ---------------------------------------------------------------------------

func TestFileTaskStore_List_IgnoresTmpFiles(t *testing.T) {
	store := newTestStore(t)

	if err := store.Save(testPersistedTask("real", "running")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Simulate a stray .tmp file left by a crash (matching the new naming pattern).
	stray := filepath.Join(store.dir, ".real.12345.tmp")
	if err := os.WriteFile(stray, []byte("{}"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, pt := range list {
		if strings.Contains(pt.Config.ID, ".tmp") {
			t.Errorf("List returned .tmp entry: %q", pt.Config.ID)
		}
	}
	if len(list) != 1 {
		t.Errorf("expected 1 record, got %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// noopStore
// ---------------------------------------------------------------------------

func TestNoopStore(t *testing.T) {
	var s TaskStore = noopStore{}

	if err := s.Save(testPersistedTask("x", "running")); err != nil {
		t.Errorf("noopStore.Save error: %v", err)
	}
	_, err := s.Load("x")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("noopStore.Load: expected os.ErrNotExist, got %v", err)
	}
	if err := s.Delete("x"); err != nil {
		t.Errorf("noopStore.Delete error: %v", err)
	}
	list, err := s.List()
	if err != nil {
		t.Errorf("noopStore.List error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("noopStore.List: expected empty, got %d", len(list))
	}
}
