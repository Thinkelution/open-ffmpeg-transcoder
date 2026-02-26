package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalStorage struct {
	Path string
}

func (l *LocalStorage) Download(_ context.Context, localPath string) error {
	src, err := os.Open(l.Path)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	return nil
}

func (l *LocalStorage) Upload(_ context.Context, localPath string) error {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open transcoded file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(l.Path)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy to output: %w", err)
	}
	return nil
}
