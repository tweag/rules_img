package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bazel-contrib/rules_img/pull_tool/cmd/internal/pull"
)

const usage = `Usage: pull_tool [COMMAND] [ARGS...]

Commands:
  pull             pulls an image from a registry`

func Run(ctx context.Context, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	command := args[1]
	switch command {
	case "pull":
		pull.PullProcess(ctx, args[2:])
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}
}

func main() {
	ctx := context.Background()
	Run(ctx, os.Args)
}
