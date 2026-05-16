package storage

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLocalStorePutGet(t *testing.T) {
	dir, err := os.MkdirTemp("", "helixblast-storage-test-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}

	ctx := context.Background()
	content := "hello helixblast"
	key := "test/job.json"

	if err := store.Put(ctx, key, strings.NewReader(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	reader, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()

	buf, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(buf) != content {
		t.Errorf("expected %q, got %q", content, string(buf))
	}
}

func TestLocalStoreDelete(t *testing.T) {
	dir, err := os.MkdirTemp("", "helixblast-storage-test-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}

	ctx := context.Background()
	key := "test/job.json"

	if err := store.Put(ctx, key, strings.NewReader("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Get(ctx, key)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestLocalStoreListExpired(t *testing.T) {
	dir, err := os.MkdirTemp("", "helixblast-storage-test-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}

	ctx := context.Background()
	store.Put(ctx, "old/job.json", strings.NewReader("data"))
	store.Put(ctx, "new/job.json", strings.NewReader("data"))

	expired, err := store.ListExpired(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("ListExpired: %v", err)
	}

	if len(expired) != 0 {
		t.Errorf("newly created files should not be expired, got %d", len(expired))
	}
}

func TestLocalStorePresignedNotSupported(t *testing.T) {
	dir, err := os.MkdirTemp("", "helixblast-storage-test-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}

	ctx := context.Background()

	_, err = store.PresignedGetURL(ctx, "key", time.Hour)
	if err == nil {
		t.Error("presigned URL should not be supported for local storage")
	}

	_, err = store.PresignedUploadURL(ctx, "key", time.Hour)
	if err == nil {
		t.Error("presigned upload URL should not be supported for local storage")
	}
}
