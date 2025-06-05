package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/tweag/rules_img/cmd/compress"
	"github.com/tweag/rules_img/cmd/index"
	"github.com/tweag/rules_img/cmd/layer"
	"github.com/tweag/rules_img/cmd/layermeta"
	"github.com/tweag/rules_img/cmd/manifest"
	"github.com/tweag/rules_img/cmd/push"
	"github.com/tweag/rules_img/cmd/validate"
	"github.com/tweag/rules_img/pkg/api"
)

const usage = `Usage: img [COMMAND] [ARGS...]

Commands:
  compress        (re-)compresses a layer
  layer           creates a layer from files
  layer-metadata  creates a layer metadata file from a layer
  manifest        creates an image manifest and config from layers
  validate        validates layers and images
  push            pushes an image to a registry`

func Run(ctx context.Context, args []string) {
	if runfilesDispatch(ctx) {
		// Check if we got a special command
		// via runfiels root symlinks.
		// If so, we don't need to
		// invoke the normal command line interface.
		return
	}

	// Otherwise, we invoke the normal command line interface.
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	command := args[1]
	switch command {
	case "layer":
		layer.LayerProcess(ctx, args[2:])
	case "layer-metadata":
		layermeta.LayerMetadataProcess(ctx, args[2:])
	case "manifest":
		manifest.ManifestProcess(ctx, args[2:])
	case "index":
		index.IndexProcess(ctx, args[2:])
	case "validate":
		validate.ValidationProcess(ctx, args[2:])
	case "push":
		push.PushProcess(ctx, args[2:])
	case "push-metadata":
		push.PushMetadataProcess(ctx, args[2:])
	case "compress":
		compress.CompressProcess(ctx, args[2:])
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}
}

func runfilesDispatch(ctx context.Context) bool {
	// Check if the command is run from a Bazel runfiles context
	// with a special root symlink indicating that this binary is used
	// to push an image.
	rf, err := runfiles.New()
	if err != nil {
		return false
	}
	requestPath, err := rf.Rlocation("dispatch.json")
	if err != nil {
		return false
	}
	if _, err := os.Stat(requestPath); err != nil {
		return false
	}

	rawRequest, err := os.ReadFile(requestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading request file: %v\n", err)
		os.Exit(1)
	}

	var req api.Dispatch
	if err := json.Unmarshal(rawRequest, &req); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshalling request file: %v\n", err)
		os.Exit(1)
	}

	// If we got here, we are in a Bazel runfiles context
	// and we have a special root symlink indicating that this binary
	// is using a json command.

	switch req.Command {
	case api.PushCommand:
		if err := push.PushFromFile(ctx, requestPath); err != nil {
			fmt.Fprintf(os.Stderr, "pushing image based on request file %s: %v\n", requestPath, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %s\n", req.Command)
		os.Exit(1)
	}

	return true
}

func main() {
	ctx := context.Background()
	Run(ctx, os.Args)
}
