package tree

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/malt3/rules_img/src/api"
)

type Recorder struct {
	tf api.TarWriter
}

func (r Recorder) RegularFile(filePath, target string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	fInfo, err := file.Stat()
	if err != nil {
		return err
	}
	realHdr, err := tar.FileInfoHeader(fInfo, "")
	if err != nil {
		return err
	}
	if realHdr.Typeflag != tar.TypeReg {
		return errors.New("recorder for regular files invoked on mismatching type")
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     path.Join(target, path.Base(filePath)),
		Size:     realHdr.Size,
		Mode:     realHdr.Mode,
		// leave out any extra metadata (for better reproducibility)
	}
	if err := r.tf.WriteHeader(hdr); err != nil {
		return err
	}
	n, err := io.Copy(r.tf, file)
	if err != nil {
		return err
	}
	if n != fInfo.Size() {
		return fmt.Errorf("recorder expected to write %d bytes, but wrote %d", fInfo.Size(), n)
	}
	return r.tf.Flush()
}

func (r Recorder) Executable(binaryPath, target string) error {
	// First, record the executable together with the (optional)
	// repo mapping and runfiles manifest files
	// <executable>
	// <executable>.repo_mapping
	// <executable>.runfiles_manifest
	if err := r.RegularFile(binaryPath, target); err != nil {
		return err
	}
	if err := r.RegularFile(binaryPath+".repo_mapping", target); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := r.RegularFile(binaryPath+".runfiles_manifest", target); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := r.tf.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     binaryPath + ".runfiles/",
		Mode:     0o555,
	}); err != nil {
		return err
	}

	panic("TODO: collect runfiles")
	// TODO: collect runfiles
	// return fs.WalkDir(os.DirFS(binaryPath+".runfiles/"), "", func(path string, d fs.DirEntry, err error) error {
	// 	originalInfo, err := d.Info()
	// 	if err != nil {
	// 		return err
	// 	}
	// 	mode := originalInfo.Mode()
	// 	if mode & os.ModeSymlink {
	// 	}
	// })
}
