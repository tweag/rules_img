package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/malt3/rules_img/cmd/compress"
	"github.com/malt3/rules_img/cmd/layer"
	"github.com/malt3/rules_img/cmd/layermeta"
	"github.com/malt3/rules_img/cmd/manifest"
	"github.com/malt3/rules_img/cmd/push"
)

const usage = `Usage: img [COMMAND] [ARGS...]

Commands:
  compress        (re-)compresses a layer
  layer           creates a layer from files
  layer-metadata  creates a layer metadata file from a layer
  manifest        creates an image manifest and config from layers
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
	case "push":
		push.PushProcess(ctx, args[2:])
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
	requestPath, err := rf.Rlocation("push_request.json")
	if err != nil {
		return false
	}
	if _, err := os.Stat(requestPath); err != nil {
		return false
	}
	// If we got here, we are in a Bazel runfiles context
	// and we have a special root symlink indicating that this binary
	// is used to push an image.
	if err := push.PushFromFile(ctx, requestPath); err != nil {
		fmt.Fprintf(os.Stderr, "pushing image based on request file %s: %v\n", requestPath, err)
	}
	return true
}

func main() {
	ctx := context.Background()
	Run(ctx, os.Args)
}
