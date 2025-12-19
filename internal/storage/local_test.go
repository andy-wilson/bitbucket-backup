package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLocal(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewLocal(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.BasePath() != tmpDir {
		t.Errorf("expected basePath = '%s', got '%s'", tmpDir, store.BasePath())
	}
}

func TestLocal_Write_Read(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewLocal(tmpDir)

	data := []byte(`{"test": "data"}`)
	path := "subdir/test.json"

	// Write
	if err := store.Write(path, data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify file exists on disk
	fullPath := filepath.Join(tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatal("file was not created")
	}

	// Read
	readData, err := store.Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(readData) != string(data) {
		t.Errorf("expected data = '%s', got '%s'", string(data), string(readData))
	}
}

func TestLocal_Write_CreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewLocal(tmpDir)

	path := "deep/nested/path/file.txt"
	data := []byte("content")

	if err := store.Write(path, data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify nested directories were created
	fullPath := filepath.Join(tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatal("file was not created in nested path")
	}
}

func TestLocal_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewLocal(tmpDir)

	// Check non-existent
	exists, err := store.Exists("nonexistent.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected non-existent file to return false")
	}

	// Create file
	store.Write("exists.txt", []byte("data"))

	// Check exists
	exists, err = store.Exists("exists.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("expected existing file to return true")
	}
}

func TestLocal_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewLocal(tmpDir)

	// Create file
	store.Write("todelete.txt", []byte("data"))

	// Verify it exists
	exists, _ := store.Exists("todelete.txt")
	if !exists {
		t.Fatal("file should exist before delete")
	}

	// Delete
	if err := store.Delete("todelete.txt"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	exists, _ = store.Exists("todelete.txt")
	if exists {
		t.Error("file should not exist after delete")
	}
}

func TestLocal_Delete_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewLocal(tmpDir)

	// Create directory with files
	store.Write("dir/file1.txt", []byte("data1"))
	store.Write("dir/file2.txt", []byte("data2"))
	store.Write("dir/subdir/file3.txt", []byte("data3"))

	// Delete entire directory
	if err := store.Delete("dir"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	exists, _ := store.Exists("dir")
	if exists {
		t.Error("directory should not exist after delete")
	}
}

func TestLocal_List(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewLocal(tmpDir)

	// Create some files
	store.Write("root.txt", []byte("data"))
	store.Write("dir/file1.txt", []byte("data1"))
	store.Write("dir/file2.txt", []byte("data2"))
	store.Write("dir/subdir/file3.txt", []byte("data3"))

	// List all
	files, err := store.List("")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(files) != 4 {
		t.Errorf("expected 4 files, got %d: %v", len(files), files)
	}

	// List subdirectory
	files, err = store.List("dir")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files in dir, got %d: %v", len(files), files)
	}
}

func TestLocal_List_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewLocal(tmpDir)

	files, err := store.List("nonexistent")
	if err != nil {
		t.Fatalf("List should not error for nonexistent path: %v", err)
	}
	if files != nil && len(files) != 0 {
		t.Errorf("expected empty list for nonexistent path, got %v", files)
	}
}

func TestLocal_Read_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewLocal(tmpDir)

	_, err := store.Read("nonexistent.txt")
	if err == nil {
		t.Error("expected error reading nonexistent file")
	}
}
