package layer

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/malt3/rules_img/src/api"
	"github.com/malt3/rules_img/src/compress"
	"github.com/malt3/rules_img/src/tarcas"
	"github.com/malt3/rules_img/src/tree"
	"github.com/malt3/rules_img/src/tree/runfiles"
)

func LayerProcess(ctx context.Context, args []string) {
	var addFiles addFiles
	var addFromFile addFromFileArgs
	var runfilesFlags runfilesForExecutables
	var executableFlags executables

	flagSet := flag.NewFlagSet("layer", flag.ExitOnError)
	flagSet.Usage = func() {
		fmt.Fprintf(flagSet.Output(), "Creates or appends a compressed tar file which can be used as a container image layer while deduplicating the contents.\n\n")
		fmt.Fprintf(flagSet.Output(), "Usage: img layer [--add-from-file param_file] [--executable path_in_image=executable_file] [--runfiles executable_file=param_file] [output]\n")
		flagSet.PrintDefaults()
		examples := []string{
			"image layer --add /etc/passwd=./passwd --executable /bin/myapp=./myapp layer.tgz",
			"image layer --add-from-file param_file.txt layer.tgz",
			"image layer --add --executable /bin/app=./app --runfiles ./app=runfiles_list.txt layer.tgz",
		}
		fmt.Fprintf(flagSet.Output(), "\nExamples:\n")
		for _, example := range examples {
			fmt.Fprintf(flagSet.Output(), "  $ %s\n", example)
		}
		os.Exit(1)
	}
	flagSet.Var(&addFiles, "add", `Add a file to the image layer. The parameter is a string of the form <path_in_image>=<file> where <path_in_image> is the path in the image and <file> is the path in the host filesystem.`)
	flagSet.Var(&addFromFile, "add-from-file", `Add all files listed in the parameter file to the image layer. The parameter file is usually written by Bazel.
The file contains one line per file, where each line contains a path in the image and a path in the host filesystem, separated by a a null byte and a single character indicating the type of the file.
The type is either 'f' for regular files, 'd' for directories. The parameter file is usually written by Bazel.`)
	flagSet.Var(&executableFlags, "executable", `Add the executable file at the specified path in the image. This should be combined with the --runfiles flag to include the runfiles of the executable.`)
	flagSet.Var(&runfilesFlags, "runfiles", `Add the runfiles of an executable file. The runfiles are read from the specified parameter file with the same encoding used by --add-from-file. The parameter file is usually written by Bazel.`)

	if err := flagSet.Parse(args); err != nil {
		flagSet.Usage()
		os.Exit(1)
	}

	if flagSet.NArg() != 1 {
		flagSet.Usage()
		os.Exit(1)
	}

	outputFilePath := flagSet.Arg(0)
	outputFile, err := os.OpenFile(outputFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening output file: %v\n", err)
		os.Exit(1)
	}
	defer outputFile.Close()

	// read the addFromFile parameter file and create a list of operations
	for _, paramFile := range addFromFile {
		addFileOpsFromParamFile, err := readParamFile(paramFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading parameter file: %v\n", err)
			os.Exit(1)
		}
		addFiles = append(addFiles, addFileOpsFromParamFile...)
	}

	// first, due to the way Bazel attributes work, we need to find out if a pathInImage is used multiple times
	// If so, we add the basename of each file to the pathInImage
	pathsInImageCount := make(map[string]int)
	for _, op := range addFiles {
		pathsInImageCount[op.PathInImage]++
	}
	for _, op := range executableFlags {
		pathsInImageCount[op.PathInImage]++
	}

	// now, we fixup the operations
	for i, op := range addFiles {
		if pathsInImageCount[op.PathInImage] > 1 {
			addFiles[i].PathInImage = fmt.Sprintf("%s/%s", op.PathInImage, filepath.Base(op.File))
		}
	}
	for i, op := range executableFlags {
		if pathsInImageCount[op.PathInImage] > 1 {
			executableFlags[i].PathInImage = fmt.Sprintf("%s/%s", op.PathInImage, filepath.Base(op.Executable))
		}
		// try to match the runfiles parameter file to the executable
		// This is inefficient, but we don't expect a lot of executables
		// to be added.
		for _, runfilesOp := range runfilesFlags {
			if runfilesOp.Executable == op.Executable {
				executableFlags[i].RunfilesParameterFile = runfilesOp.RunfilesFromFile
				break
			}
		}
	}

	_, err = handleLayerState(addFiles, executableFlags, outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Writing layer: %v\n", err)
		os.Exit(1)
	}
}

func handleLayerState(addFiles addFiles, addExecutables executables, outputFile io.Writer) (compressorState api.AppenderState, err error) {
	compressor, err := compress.AppenderFactory("sha256", "gzip", outputFile)
	if err != nil {
		return compressorState, fmt.Errorf("creating compressor: %w", err)
	}
	defer func() {
		var compressorCloseErr error
		compressorState, compressorCloseErr = compressor.Finalize()
		if compressorCloseErr != nil {
			fmt.Fprintf(os.Stderr, "Error closing compressor: %v\n", compressorCloseErr)
		}
	}()

	tw, err := tarcas.CASFactory("sha256", compressor)
	if err != nil {
		return compressorState, fmt.Errorf("creating Content-addressable storage inside tar file: %w", err)
	}
	defer func() {
		if err := tw.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing tar writer: %v\n", err)
		}
	}()

	recorder := tree.NewRecorder(tw)
	if err := writeLayer(recorder, addFiles, addExecutables); err != nil {
		return compressorState, err
	}

	return compressorState, nil
}

func writeLayer(recorder tree.Recorder, addFiles addFiles, addExecutables executables) error {
	for _, op := range addFiles {
		switch op.FileType {
		case api.RegularFile:
			if err := recorder.RegularFileFromPath(op.File, op.PathInImage); err != nil {
				return fmt.Errorf("writing regular file: %w", err)
			}
		case api.Directory:
			if err := recorder.TreeFromPath(op.File, op.PathInImage); err != nil {
				return fmt.Errorf("writing directory: %w", err)
			}
		default:
			return fmt.Errorf("unknown type %s for file %s", op.FileType.String(), op.File)
		}
	}

	for _, op := range addExecutables {
		runfilesList, err := readParamFile(op.RunfilesParameterFile)
		if err != nil {
			return fmt.Errorf("reading runfiles parameter file: %w", err)
		}
		accessor := runfiles.NewRunfilesFS()
		for _, f := range runfilesList {
			accessor.Add(f.PathInImage, f)
		}
		if err := recorder.Executable(op.Executable, op.PathInImage, accessor); err != nil {
			return fmt.Errorf("writing executable: %w", err)
		}
	}

	return nil
}
