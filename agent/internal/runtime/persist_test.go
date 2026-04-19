package runtime_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/runtime"
)

func TestPersistentSessionStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")

	// ── Write ──────────────────────────────────────────────────────────────
	store, err := runtime.NewPersistentSessionStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	sess := store.Create("test-session-1")
	sess.AppendHistory(llm.Message{Role: llm.RoleUser, Content: "hello"})
	sess.AppendHistory(llm.Message{Role: llm.RoleAssistant, Content: "hi there"})
	sess.SetSummary("User greeted the assistant.")
	store.Persist("test-session-1")

	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	// ── Reload ─────────────────────────────────────────────────────────────
	store2, err := runtime.NewPersistentSessionStore(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	defer store2.Close()

	loaded, ok := store2.Get("test-session-1")
	if !ok {
		t.Fatal("session not found after reload")
	}
	if loaded.GetSummary() != "User greeted the assistant." {
		t.Errorf("summary = %q, want original", loaded.GetSummary())
	}
	history := loaded.GetHistory()
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Content != "hello" || history[1].Content != "hi there" {
		t.Errorf("history contents don't match original")
	}
}

func TestPersistentSessionStore_DeleteRemovesFromDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")

	store, err := runtime.NewPersistentSessionStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	store.Create("to-delete")
	store.Persist("to-delete")
	store.Delete("to-delete")
	store.Close()

	store2, err := runtime.NewPersistentSessionStore(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	defer store2.Close()

	if _, ok := store2.Get("to-delete"); ok {
		t.Error("deleted session still exists after reload")
	}
}

func TestMemorySessionStore_NoFileCreated(t *testing.T) {
	dir := t.TempDir()
	store := runtime.NewSessionStore()
	store.Create("s1")
	store.Persist("s1") // should be a no-op
	store.Close()

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("memory store created files in %s", dir)
	}
}
