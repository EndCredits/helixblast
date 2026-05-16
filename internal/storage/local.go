package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type LocalStore struct {
	baseDir string
}

func NewLocalStore(baseDir string) (*LocalStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create base directory: %w", err)
	}
	return &LocalStore{baseDir: baseDir}, nil
}

func (s *LocalStore) Put(ctx context.Context, key string, reader io.Reader) error {
	fullPath := filepath.Join(s.baseDir, sanitizeKey(key))
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		os.Remove(fullPath)
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (s *LocalStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.baseDir, sanitizeKey(key))
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not found: %s", key)
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(s.baseDir, sanitizeKey(key))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

func (s *LocalStore) ListExpired(ctx context.Context, ttl time.Duration) ([]string, error) {
	var expired []string
	cutoff := time.Now().Add(-ttl)

	err := filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			relPath, err := filepath.Rel(s.baseDir, path)
			if err != nil {
				return nil
			}
			expired = append(expired, relPath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	sort.Strings(expired)
	return expired, nil
}

func (s *LocalStore) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return "", fmt.Errorf("presigned URLs not supported with local storage")
}

func (s *LocalStore) PresignedUploadURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return "", fmt.Errorf("presigned URLs not supported with local storage")
}

func sanitizeKey(key string) string {
	key = strings.ReplaceAll(key, "..", "")
	key = strings.ReplaceAll(key, "\\", "/")
	key = filepath.Clean(key)
	return key
}
