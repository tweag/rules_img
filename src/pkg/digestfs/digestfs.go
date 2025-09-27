package digestfs

import (
	"hash"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// HashProvider creates new hash instances for digest calculation
type HashProvider interface {
	New() hash.Hash
}

// DigestFile represents a file with cached digest calculation and seeking capabilities
type DigestFile interface {
	io.ReadSeeker
	io.Closer

	// Path returns the real path of the file (after resolving symlinks)
	Path() string

	// Size returns the file size
	Size() int64

	// Digest returns the cached digest of the file contents
	Digest() ([]byte, error)
}

// FileSystem provides digest-cached file access with memoization
type FileSystem struct {
	mu           sync.RWMutex
	digestCache  map[string][]byte // realpath -> digest
	hashProvider HashProvider
}

type cachedDigestFile struct {
	file     *os.File
	realPath string
	size     int64
	fs       *FileSystem
}

// New creates a new digest file system with the given hash provider
func New(hashProvider HashProvider) *FileSystem {
	return &FileSystem{
		digestCache:  make(map[string][]byte),
		hashProvider: hashProvider,
	}
}

// OpenFile opens a file for reading with digest caching capabilities
func (fs *FileSystem) OpenFile(path string) (DigestFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		file.Close()
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	return &cachedDigestFile{
		file:     file,
		realPath: realPath,
		size:     stat.Size(),
		fs:       fs,
	}, nil
}

func (f *cachedDigestFile) Read(p []byte) (n int, err error) {
	return f.file.Read(p)
}

func (f *cachedDigestFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *cachedDigestFile) Close() error {
	return f.file.Close()
}

func (f *cachedDigestFile) Path() string {
	return f.realPath
}

func (f *cachedDigestFile) Size() int64 {
	return f.size
}

func (f *cachedDigestFile) Digest() ([]byte, error) {
	// Check cache with read lock first
	f.fs.mu.RLock()
	if digest, exists := f.fs.digestCache[f.realPath]; exists {
		f.fs.mu.RUnlock()
		return digest, nil
	}
	f.fs.mu.RUnlock()

	// Acquire write lock for computation
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()

	// Double-check cache after acquiring write lock
	if digest, exists := f.fs.digestCache[f.realPath]; exists {
		return digest, nil
	}

	// Save current position and reset to start
	currentPos, err := f.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	if _, err := f.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	defer f.file.Seek(currentPos, io.SeekStart)

	// Calculate digest
	h := f.fs.hashProvider.New()
	if _, err := io.Copy(h, f.file); err != nil {
		return nil, err
	}

	digest := h.Sum(nil)
	f.fs.digestCache[f.realPath] = digest
	return digest, nil
}

// InvalidateCache removes the cached digest for a file path
func (fs *FileSystem) InvalidateCache(path string) {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return
	}

	fs.mu.Lock()
	delete(fs.digestCache, realPath)
	fs.mu.Unlock()
}

// GetCachedDigest returns the cached digest for a file path, if available
func (fs *FileSystem) GetCachedDigest(path string) ([]byte, bool) {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, false
	}

	fs.mu.RLock()
	digest, exists := fs.digestCache[realPath]
	fs.mu.RUnlock()
	return digest, exists
}

// CacheStats returns statistics about the digest cache
func (fs *FileSystem) CacheStats() (entries int, totalDigests int) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return len(fs.digestCache), len(fs.digestCache)
}
