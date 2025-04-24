package main

import (
	"context"
	"fmt"
	"os"

	"github.com/malt3/rules_img/cmd/layer"
)

const usage = `Usage: img [COMMAND] [ARGS...]

Commands:
  layer  creates a layer from files`

func Run(ctx context.Context, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	command := args[1]
	switch command {
	case "layer":
		layer.LayerProcess(ctx, args[2:])
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}
}

func main() {
	ctx := context.Background()
	Run(ctx, os.Args)
}
