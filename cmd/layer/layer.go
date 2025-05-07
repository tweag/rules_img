package layer

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/malt3/rules_img/src/api"
	"github.com/malt3/rules_img/src/compress"
	"github.com/malt3/rules_img/src/contentmanifest"
	"github.com/malt3/rules_img/src/tarcas"
	"github.com/malt3/rules_img/src/tree"
	"github.com/malt3/rules_img/src/tree/runfiles"
)

func LayerProcess(ctx context.Context, args []string) {
	var addFiles addFiles
	var addFromFile addFromFileArgs
	var importTarFlags importTars
	var runfilesFlags runfilesForExecutables
	var executableFlags executables
	var symlinkFlags symlinks
	var symlinksFromFiles symlinksFromFileArgs
	var formatFlag string
	var metadataOutputFlag string
	var contentManifestOutputFlag string

	flagSet := flag.NewFlagSet("layer", flag.ExitOnError)
	flagSet.Usage = func() {
		fmt.Fprintf(flagSet.Output(), "Creates a compressed tar file which can be used as a container image layer while deduplicating the contents.\n\n")
		fmt.Fprintf(flagSet.Output(), "Usage: img layer [--add path_in_image=target] [--add-from-file param_file] [--import-tar tar_file] [--executable path_in_image=executable_file] [--runfiles executable_file=param_file] [--symlink path_in_image=target] [--symlinks-from-file=param_file] [--metadata=metadata_output_file] [--content-manifest=manifest_output_file] [output]\n")
		flagSet.PrintDefaults()
		examples := []string{
			"img layer --add /etc/passwd=./passwd --executable /bin/myapp=./myapp layer.tgz",
			"img layer --add-from-file param_file.txt layer.tgz",
			"img layer --add --executable /bin/app=./app --runfiles ./app=runfiles_list.txt layer.tgz",
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
	flagSet.Var(&importTarFlags, "import-tar", `Import all files from the given tar file into the image layer while deduplicating the contents.`)
	flagSet.Var(&executableFlags, "executable", `Add the executable file at the specified path in the image. This should be combined with the --runfiles flag to include the runfiles of the executable.`)
	flagSet.Var(&runfilesFlags, "runfiles", `Add the runfiles of an executable file. The runfiles are read from the specified parameter file with the same encoding used by --add-from-file. The parameter file is usually written by Bazel.`)
	flagSet.Var(&symlinkFlags, "symlink", `Add a symlink to the image layer. The parameter is a string of the form <path_in_image>=<target> where <path_in_image> is the path in the image and <target> is the target of the symlink.`)
	flagSet.Var(&symlinksFromFiles, "symlinks-from-file", `Add all symlinks listed in the parameter file to the image layer. The parameter file is usually written by Bazel.`)
	flagSet.StringVar(&formatFlag, "format", "", `The compression format of the output layer. Can be "gzip" or "none". Default is to guess the algorithm based on the filename, but fall back to "gzip".`)
	flagSet.StringVar(&metadataOutputFlag, "metadata", "", `Write the metadata to the specified file. The metadata is a JSON file containing info needed to use the layer as part of an OCI image.`)
	flagSet.StringVar(&contentManifestOutputFlag, "content-manifest", "", `Write a manifest of the contents of the layer to the specified file. The manifest uses a custom binary format listing all blobs, nodes, and trees in the layer after deduplication.`)

	if err := flagSet.Parse(args); err != nil {
		flagSet.Usage()
		os.Exit(1)
	}

	if flagSet.NArg() != 1 {
		flagSet.Usage()
		os.Exit(1)
	}

	outputFilePath := flagSet.Arg(0)

	var compressionAlgorithm api.CompressionAlgorithm
	switch formatFlag {
	case "":
		if filepath.Ext(outputFilePath) == ".tar" {
			compressionAlgorithm = api.Uncompressed
		} else if filepath.Ext(outputFilePath) == ".tgz" || filepath.Ext(outputFilePath) == ".gz" {
			compressionAlgorithm = api.Gzip
		} else {
			compressionAlgorithm = api.Gzip
		}
	case "gzip":
		compressionAlgorithm = api.Gzip
	case "none", "uncompressed", "tar":
		compressionAlgorithm = api.Uncompressed
	default:
		fmt.Fprintf(os.Stderr, "Unknown format %s. Supported formats are gzip and uncompressed.\n", formatFlag)
		os.Exit(1)
	}

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

	// read the symlinksFromFile parameter file and create a list of operations
	for _, paramFile := range symlinksFromFiles {
		symlinkOpsFromParamFile, err := readSymlinkParamFile(paramFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading symlink parameter file: %v\n", err)
			os.Exit(1)
		}
		symlinkFlags = append(symlinkFlags, symlinkOpsFromParamFile...)
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

	var casExporter api.CASStateExporter
	if len(contentManifestOutputFlag) > 0 {
		casExporter = contentmanifest.New(contentManifestOutputFlag, api.SHA256)
	} else {
		casExporter = contentmanifest.NopExporter()
	}

	compressorState, err := handleLayerState(compressionAlgorithm, addFiles, importTarFlags, executableFlags, symlinkFlags, outputFile, casExporter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Writing layer: %v\n", err)
		os.Exit(1)
	}

	if len(metadataOutputFlag) > 0 {
		metadataOutputFile, err := os.OpenFile(metadataOutputFlag, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening metadata output file: %v\n", err)
			os.Exit(1)
		}
		defer metadataOutputFile.Close()

		if err := writeMetadata(compressionAlgorithm, compressorState, metadataOutputFile); err != nil {
			fmt.Fprintf(os.Stderr, "Writing metadata: %v\n", err)
			os.Exit(1)
		}
	}
}

func handleLayerState(compressionAlgorithm api.CompressionAlgorithm, addFiles addFiles, importTars importTars, addExecutables executables, addSymlinks symlinks, outputFile io.Writer, casExporter api.CASStateExporter) (compressorState api.AppenderState, err error) {
	compressor, err := compress.AppenderFactory("sha256", string(compressionAlgorithm), outputFile)
	if err != nil {
		return compressorState, fmt.Errorf("creating compressor: %w", err)
	}
	defer func() {
		var compressorCloseErr error
		compressorState, compressorCloseErr = compressor.Finalize()
		if compressorCloseErr != nil {
			fmt.Fprintf(os.Stderr, "Error closing compressor: %v\n", compressorCloseErr)
			os.Exit(1)
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
	if err := writeLayer(recorder, addFiles, importTars, addExecutables, addSymlinks); err != nil {
		return compressorState, err
	}

	return compressorState, tw.Export(casExporter)
}

func writeLayer(recorder tree.Recorder, addFiles addFiles, importTars importTars, addExecutables executables, addSymlinks symlinks) error {
	for _, tarFile := range importTars {
		if err := recorder.ImportTar(tarFile); err != nil {
			return fmt.Errorf("importing tar file: %w", err)
		}
	}

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

	for _, op := range addSymlinks {
		if err := recorder.Symlink(op.Target, op.LinkName); err != nil {
			return fmt.Errorf("writing symlink: %w", err)
		}
	}

	return nil
}

func writeMetadata(compressionAlgorithm api.CompressionAlgorithm, compressorState api.AppenderState, outputFile io.Writer) error {
	var mediaType string
	switch compressionAlgorithm {
	case api.Uncompressed:
		mediaType = "application/vnd.oci.image.layer.v1.tar"
	case api.Gzip:
		mediaType = "application/vnd.oci.image.layer.v1.tar+gzip"
	default:
		return fmt.Errorf("unsupported compression algorithm: %s", compressionAlgorithm)
	}

	metadata := api.LayerMetadata{
		DiffID:    fmt.Sprintf("sha256:%x", compressorState.ContentHash),
		MediaType: mediaType,
		Digest:    fmt.Sprintf("sha256:%x", compressorState.OuterHash),
		Size:      compressorState.CompressedSize,
	}

	json.NewEncoder(outputFile).SetIndent("", "  ")
	if err := json.NewEncoder(outputFile).Encode(metadata); err != nil {
		return fmt.Errorf("encoding metadata: %w", err)
	}
	return nil
}
