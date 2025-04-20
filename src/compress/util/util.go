package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/malt3/rules_img/src/api"
	"github.com/malt3/rules_img/src/compress"
	"github.com/malt3/rules_img/src/compress/factory"
)

var (
	stateIn              string
	stateOut             string
	compressionLevel     *int
	compressionAlgorithm string
	hashFunction         string
	inputFilePath        string
	outputFilePath       string
	append               bool
)

func main() {
	flag.StringVar(&stateIn, "state-in", "", "Path to the state file to resume from")
	flag.StringVar(&stateOut, "state-out", "", "Path to the state file to write to")
	flag.Func("compression-level", "Compression level (0-9)", func(s string) error {
		level, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		compressionLevel = &level
		return nil
	})
	flag.StringVar(&compressionAlgorithm, "compression-algorithm", "gzip", "Compression algorithm to use")
	flag.StringVar(&hashFunction, "hash-function", "sha256", "Hash function to use")
	flag.BoolVar(&append, "append", false, "Append to an existing output file")
	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}
	inputFilePath = flag.Arg(0)
	outputFilePath = flag.Arg(1)

	inputFile, err := os.Open(inputFilePath)
	if err != nil {
		fmt.Printf("Error opening input file: %v\n", err)
		os.Exit(1)
	}
	defer inputFile.Close()

	outputFileFlags := os.O_WRONLY | os.O_CREATE
	if append {
		outputFileFlags |= os.O_APPEND
	} else {
		outputFileFlags |= os.O_TRUNC
	}
	outputFile, err := os.OpenFile(outputFilePath, outputFileFlags, 0o644)
	if err != nil {
		fmt.Printf("Error opening output file: %v\n", err)
		os.Exit(1)
	}
	defer outputFile.Close()

	var state api.AppenderState
	if stateIn != "" {
		state, err = readStateFromFile(stateIn)
		if err != nil {
			fmt.Printf("Error reading input state file: %v\n", err)
			os.Exit(1)
		}
	}

	fileInfo, err := outputFile.Stat()
	if append && stateIn != "" && state.CompressedSize != fileInfo.Size() {
		fmt.Printf("Warning: The output file size %d does not match the state file size %d. The state file may be invalid.\n", fileInfo.Size(), state.CompressedSize)
	}

	opts := compress.Options{
		CompressionLevel: compressionLevel,
	}

	var appender api.Appender
	if stateIn != "" {
		appender, err = factory.ResumeFactory(hashFunction, compressionAlgorithm, state, outputFile, opts)
	} else {
		appender, err = factory.AppenderFactory(hashFunction, compressionAlgorithm, outputFile, opts)
	}
	if err != nil {
		fmt.Printf("Error creating appender: %v\n", err)
		os.Exit(1)
	}

	_, err = io.Copy(appender, inputFile)
	if err != nil {
		fmt.Printf("Error copying data: %v\n", err)
		os.Exit(1)
	}

	state, err = appender.Finalize()
	if err != nil {
		fmt.Printf("Error finalizing appender: %v\n", err)
		os.Exit(1)
	}
	err = writeStateToFile(stateOut, state)
	if err != nil {
		fmt.Printf("Error writing output state file: %v\n", err)
		os.Exit(1)
	}
}

func readStateFromFile(filePath string) (api.AppenderState, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return api.AppenderState{}, fmt.Errorf("opening state file: %w", err)
	}
	defer file.Close()

	var state api.AppenderState
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return api.AppenderState{}, fmt.Errorf("decoding state file: %w", err)
	}

	return state, nil
}

func writeStateToFile(filePath string, state api.AppenderState) error {
	var file io.Writer
	if len(filePath) > 0 {
		osFile, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("creating state file: %w", err)
		}
		defer osFile.Close()
		file = osFile
	} else {
		file = os.Stdout
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encoding state file: %w", err)
	}

	return nil
}
