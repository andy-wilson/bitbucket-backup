// Package storage provides storage backends for backup data.
package storage

// Storage is the interface for storage backends.
type Storage interface {
	// Write writes data to the given path.
	Write(path string, data []byte) error

	// Read reads data from the given path.
	Read(path string) ([]byte, error)

	// Exists checks if a path exists.
	Exists(path string) (bool, error)

	// Delete removes a file or directory.
	Delete(path string) error

	// List returns all files under a path.
	List(path string) ([]string, error)

	// BasePath returns the base path for the storage.
	BasePath() string
}
