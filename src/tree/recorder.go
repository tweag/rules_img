package tree

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path"
	"strings"

	"github.com/malt3/rules_img/src/api"
	"github.com/malt3/rules_img/src/tree/runfiles"
)

type Recorder struct {
	tf api.TarCAS
}

func NewRecorder(tf api.TarCAS) Recorder {
	return Recorder{
		tf: tf,
	}
}

func (r Recorder) RegularFileFromPath(filePath, target string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	fInfo, err := file.Stat()
	if err != nil {
		return err
	}
	return r.RegularFile(file, fInfo, target)
}

func (r Recorder) RegularFile(f io.Reader, info fs.FileInfo, target string) error {
	realHdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	if realHdr.Typeflag != tar.TypeReg {
		return errors.New("recorder for regular files invoked on mismatching type")
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     target,
		Size:     realHdr.Size,
		Mode:     realHdr.Mode,
		// leave out any extra metadata (for better reproducibility)
	}
	if err := r.tf.WriteHeader(hdr); err != nil {
		return err
	}
	n, err := io.Copy(r.tf, f)
	if err != nil {
		return err
	}
	if n != info.Size() {
		return fmt.Errorf("recorder expected to write %d bytes, but wrote %d", info.Size(), n)
	}
	return r.tf.Flush()
}

func (r Recorder) TreeFromPath(dirPath, target string) error {
	fsys := os.DirFS(dirPath)
	return r.Tree(fsys, target)
}

// Tree records a directory tree (including all files and subdirectories).
// It creates a symlink in the tar file that points to the root of the tree.
func (r Recorder) Tree(fsys fs.FS, target string) error {
	rootHash, err := r.tf.StoreTree(fsys)
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeSymlink,
		Name:     target,
		Linkname: relativeSymlinkTarget(fmt.Sprintf(".cas/tree/%x", rootHash), target),
	}
	return r.tf.WriteHeader(hdr)
}

func (r Recorder) Executable(binaryPath, target string, accessor runfilesSupplier) error {
	// First, record the executable itself.
	if err := r.RegularFileFromPath(binaryPath, target); err != nil {
		return err
	}
	// Next, record the root directory of the runfiles tree.
	if err := r.tf.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     target + ".runfiles/",
		Mode:     0o555,
	}); err != nil {
		return err
	}

	// Finally, record the contents of the runfiles tree.
	for p, node := range accessor.Items() {
		switch node.Type() {
		case api.RegularFile:
			f, err := node.Open()
			if err != nil {
				return err
			}
			info, err := f.Stat()
			if err != nil {
				f.Close()
				return err
			}
			if err := r.RegularFile(f, info, path.Join(target+".runfiles", p)); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case api.Directory:
			fsys, err := node.Tree()
			if err != nil {
				return err
			}
			if err := r.Tree(fsys, path.Join(target+".runfiles", p)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported runfiles node type: %s", node.Type())
		}
	}

	return nil
}

func relativeSymlinkTarget(target, linkName string) string {
	sourceDir := path.Dir(linkName)
	sourceParts := strings.Split(path.Clean(sourceDir), "/")
	targetParts := strings.Split(path.Clean(target), "/")

	// remove common prefix
	for i := 0; i < len(sourceParts) && i < len(targetParts); i++ {
		if sourceParts[i] != targetParts[i] {
			sourceParts = sourceParts[i:]
			targetParts = targetParts[i:]
			break
		}
	}

	var relParts []string

	for range sourceParts {
		relParts = append(relParts, "..")
	}
	relParts = append(relParts, targetParts...)

	return strings.Join(relParts, "/")
}

type runfilesSupplier interface {
	Items() iter.Seq2[string, runfiles.Node]
}
