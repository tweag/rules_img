package tree

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

type RunfilesFS struct {
	runfiles *runfiles.Runfiles
}

func NewRunfilesFS(binaryPath string) (*RunfilesFS, error) {
	rf, err := runfiles.New(runfiles.ProgramName(binaryPath))
	if err != nil {
		return nil, err
	}
	return &RunfilesFS{
		runfiles: rf,
	}, nil
}

func (r *RunfilesFS) Open(name string) (fs.File, error) {
	file, err := r.runfiles.Open(name)
	if err != nil {
		return nil, err
	}
	fInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	file.Close()

	if fInfo.IsDir() || fInfo.Mode().IsRegular() {
		return file, nil
	}
	if fInfo.Mode() & ^fs.ModeSymlink != 0 {
		// Unhandled file type
		return nil, fs.ErrInvalid
	}
	// Handle symlink
	symlinkPath, err := r.runfiles.Rlocation(name)
	if err != nil {
		return nil, err
	}
	symlinkTarget, err := filepath.EvalSymlinks(symlinkPath)
	if err != nil {
		return nil, err
	}
	symlinkTarget, err = filepath.Abs(symlinkTarget)
	if err != nil {
		return nil, err
	}
	return os.Open(symlinkTarget)
}
