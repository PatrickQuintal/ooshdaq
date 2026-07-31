package sqliterepo

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ooshdaq/ooshdaq/internal/domain"
)

func TestCheckpointReportsAndTruncatesWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.sqlite")
	store, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	for index := range 100 {
		if err := store.UpsertUser(context.Background(), domain.User{
			ID:          "checkpoint-member-" + strconv.Itoa(index),
			DisplayName: "Checkpoint Member",
		}); err != nil {
			t.Fatalf("UpsertUser %d: %v", index, err)
		}
	}

	passive, err := store.Checkpoint(context.Background(), false)
	if err != nil {
		t.Fatalf("passive Checkpoint: %v", err)
	}
	if passive.LogFrames < 1 || passive.CheckpointedFrames < 1 || passive.Duration < 0 {
		t.Fatalf("passive checkpoint = %#v", passive)
	}
	truncated, err := store.Checkpoint(context.Background(), true)
	if err != nil {
		t.Fatalf("truncate Checkpoint: %v", err)
	}
	if truncated.Busy != 0 {
		t.Fatalf("truncate checkpoint = %#v", truncated)
	}
	info, err := os.Stat(path + "-wal")
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat WAL: %v", err)
	}
	if err == nil && info.Size() != 0 {
		t.Fatalf("WAL size after truncate = %d, want 0", info.Size())
	}
}
