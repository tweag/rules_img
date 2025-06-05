package layer

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/tweag/rules_img/pkg/api"
	"github.com/tweag/rules_img/pkg/tree/treeartifact"
)

type addFile struct {
	PathInImage string
	File        string
	FileType    api.FileType
}

func (a addFile) Type() api.FileType {
	return a.FileType
}

func (a addFile) Open() (fs.File, error) {
	return os.Open(a.File)
}

func (a addFile) Tree() (fs.FS, error) {
	if a.FileType != api.Directory {
		return nil, fmt.Errorf("cannot get tree for non-directory file: %s", a.File)
	}
	// TODO: consider using a special
	// file system for tree artifacts
	// that filters out non-regular files.
	return treeartifact.TreeArtifactFS(a.File), nil
}

type addFiles []addFile

func (a *addFiles) String() string {
	var sb strings.Builder
	for i, a := range *a {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(a.PathInImage)
		sb.WriteString("=")
		sb.WriteString(a.File)
	}
	return sb.String()
}

func (a *addFiles) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format for --add: %s", value)
	}
	if len(parts[0]) == 0 {
		return fmt.Errorf("path in image cannot be empty: %s", value)
	}
	if parts[0][0] == '/' {
		// remove leading slash in target
		parts[0] = parts[0][1:]
	}
	fInfo, err := os.Stat(parts[1])
	if err != nil {
		return fmt.Errorf("file %s does not exist: %w", parts[1], err)
	}
	var fileType api.FileType
	if fInfo.IsDir() {
		fileType = api.Directory
	} else {
		fileType = api.RegularFile
	}
	*a = append(*a, addFile{
		PathInImage: parts[0],
		File:        parts[1],
		FileType:    fileType,
	})
	return nil
}

type addFromFileArgs []string

func (a *addFromFileArgs) String() string {
	return strings.Join(*a, ", ")
}

func (a *addFromFileArgs) Set(value string) error {
	if _, err := os.Stat(value); err != nil {
		return fmt.Errorf("file %s does not exist: %w", value, err)
	}
	*a = append(*a, value)
	return nil
}

type importTars []string

func (i *importTars) String() string {
	return strings.Join(*i, ", ")
}

func (i *importTars) Set(value string) error {
	if _, err := os.Stat(value); err != nil {
		return fmt.Errorf("file %s does not exist: %w", value, err)
	}
	*i = append(*i, value)
	return nil
}

type runfilesForExecutable struct {
	Executable       string
	RunfilesFromFile string
}

type executable struct {
	PathInImage           string
	Executable            string
	RunfilesParameterFile string
}

type runfilesForExecutables []runfilesForExecutable

func (r *runfilesForExecutables) String() string {
	var sb strings.Builder
	for i, r := range *r {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(r.Executable)
		sb.WriteString("=")
		sb.WriteString(r.RunfilesFromFile)
	}
	return sb.String()
}

func (r *runfilesForExecutables) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format for --runfiles: %s", value)
	}
	if _, err := os.Stat(parts[1]); err != nil {
		return fmt.Errorf("parameter file %s does not exist: %w", parts[1], err)
	}
	*r = append(*r, runfilesForExecutable{
		Executable:       parts[0],
		RunfilesFromFile: parts[1],
	})
	return nil
}

type executables []executable

func (e *executables) String() string {
	var sb strings.Builder
	for i, e := range *e {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(e.PathInImage)
		sb.WriteString("=")
		sb.WriteString(e.Executable)
	}
	return sb.String()
}

func (e *executables) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format for --executable: %s", value)
	}
	if _, err := os.Stat(parts[1]); err != nil {
		return fmt.Errorf("executable %s does not exist: %w", parts[1], err)
	}
	if len(parts[0]) == 0 {
		return fmt.Errorf("path in image cannot be empty: %s", value)
	}
	if parts[0][0] == '/' {
		// remove leading slash in target
		parts[0] = parts[0][1:]
	}
	*e = append(*e, executable{
		PathInImage: parts[0],
		Executable:  parts[1],
	})
	return nil
}

type symlinksFromFileArgs []string

func (s *symlinksFromFileArgs) String() string {
	return strings.Join(*s, ", ")
}

func (s *symlinksFromFileArgs) Set(value string) error {
	if _, err := os.Stat(value); err != nil {
		return fmt.Errorf("file %s does not exist: %w", value, err)
	}
	*s = append(*s, value)
	return nil
}

type symlink struct {
	LinkName string
	Target   string
}

type symlinks []symlink

func (s *symlinks) String() string {
	var sb strings.Builder
	for i, s := range *s {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(s.LinkName)
		sb.WriteString(" → ")
		sb.WriteString(s.Target)
	}
	return sb.String()
}

func (s *symlinks) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format for --symlink: %s", value)
	}
	if len(parts[0]) == 0 {
		return fmt.Errorf("link name cannot be empty: %s", value)
	}
	if parts[0][0] == '/' {
		// remove leading slash in link name
		parts[0] = parts[0][1:]
	}
	*s = append(*s, symlink{
		LinkName: parts[0],
		Target:   parts[1],
	})
	return nil
}

type contentManifests []string

func (m *contentManifests) String() string {
	return strings.Join(*m, ", ")
}

func (m *contentManifests) Set(value string) error {
	if _, err := os.Stat(value); err != nil {
		return fmt.Errorf("file %s does not exist: %w", value, err)
	}
	*m = append(*m, value)
	return nil
}
