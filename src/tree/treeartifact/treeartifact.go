package treeartifact

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type treeartifactFS string

func TreeArtifactFS(path string) treeartifactFS {
	return treeartifactFS(path)
}

func (t treeartifactFS) Open(name string) (fs.File, error) {
	fullname := t.join(name)
	realpath, err := filepath.EvalSymlinks(fullname)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	f, err := os.Open(realpath)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return &treeartifactFile{
		File: f,
		name: name,
	}, nil
}

func (t treeartifactFS) ReadFile(name string) ([]byte, error) {
	fullname := t.join(name)
	realpath, err := filepath.EvalSymlinks(fullname)
	if err != nil {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: err}
	}
	return os.ReadFile(realpath)
}

func (t treeartifactFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fullname := t.join(name)
	realpath, err := filepath.EvalSymlinks(fullname)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}
	dirents, err := os.ReadDir(realpath)
	if err != nil {
		return nil, err
	}
	for i, entry := range dirents {
		if entry.Type()&fs.ModeSymlink != 0 {
			// If the entry is a symlink, we need to
			// resolve it to the real path.
			realpath, err := filepath.EvalSymlinks(filepath.Join(realpath, entry.Name()))
			if err != nil {
				return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
			}
			fInfo, err := os.Stat(realpath)
			if err != nil {
				return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
			}
			dirents[i] = &treeArtifactDirEntry{
				name:     entry.Name(),
				DirEntry: fs.FileInfoToDirEntry(fInfo),
			}
		}
	}
	return dirents, nil
}

func (t treeartifactFS) join(name string) string {
	return filepath.Join(string(t), name)
}

type treeartifactFile struct {
	*os.File
	name string
}

func (f *treeartifactFile) Name() string {
	return f.name
}

func (f *treeartifactFile) Stat() (fs.FileInfo, error) {
	realStat, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	if realStat.Name() == f.name {
		return realStat, nil
	}
	return &treeartifactFileInfo{
		name:     f.name,
		realStat: realStat,
	}, nil
}

type treeartifactFileInfo struct {
	name     string
	realStat fs.FileInfo
}

func (f *treeartifactFileInfo) Name() string {
	return f.name
}

func (f *treeartifactFileInfo) Size() int64 {
	return f.realStat.Size()
}

func (f *treeartifactFileInfo) Mode() fs.FileMode {
	return f.realStat.Mode()
}

func (f *treeartifactFileInfo) ModTime() time.Time {
	return f.realStat.ModTime()
}

func (f *treeartifactFileInfo) IsDir() bool {
	return f.realStat.IsDir()
}

func (f *treeartifactFileInfo) Sys() any {
	return f.realStat.Sys()
}

type treeArtifactDirEntry struct {
	name string
	fs.DirEntry
}

func (d *treeArtifactDirEntry) Name() string {
	return d.name
}
