package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// Local implements Storage for the local filesystem.
type Local struct {
	basePath string
}

// NewLocal creates a new Local storage backend.
func NewLocal(basePath string) (*Local, error) {
	// Convert to absolute path
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolving absolute path: %w", err)
	}

	return &Local{basePath: absPath}, nil
}

// Write writes data to the given path relative to the base path.
func (l *Local) Write(path string, data []byte) error {
	fullPath := filepath.Join(l.basePath, path)

	// Ensure parent directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	// Write the file
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("writing file %s: %w", fullPath, err)
	}

	return nil
}

// Read reads data from the given path relative to the base path.
func (l *Local) Read(path string) ([]byte, error) {
	fullPath := filepath.Join(l.basePath, path)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", fullPath, err)
	}

	return data, nil
}

// Exists checks if a path exists relative to the base path.
func (l *Local) Exists(path string) (bool, error) {
	fullPath := filepath.Join(l.basePath, path)

	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking path %s: %w", fullPath, err)
}

// Delete removes a file or directory relative to the base path.
func (l *Local) Delete(path string) error {
	fullPath := filepath.Join(l.basePath, path)

	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("deleting %s: %w", fullPath, err)
	}

	return nil
}

// List returns all files under a path relative to the base path.
func (l *Local) List(path string) ([]string, error) {
	fullPath := filepath.Join(l.basePath, path)

	var files []string
	err := filepath.Walk(fullPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// Return path relative to base
			rel, err := filepath.Rel(l.basePath, p)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing %s: %w", fullPath, err)
	}

	return files, nil
}

// BasePath returns the base path for the storage.
func (l *Local) BasePath() string {
	return l.basePath
}
