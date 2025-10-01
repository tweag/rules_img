package validate

import (
	"context"
	"fmt"
	"os"

	layerpresence "github.com/bazel-contrib/rules_img/src/cmd/validate/layer-presence"
)

const usage = `Usage img validate [COMMAND] [ARGS...]

Commands:
  layer-presence  Checks that layers used for deduplication are present in a final image.`

func ValidationProcess(ctx context.Context, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	command := args[0]
	switch command {
	case "layer-presence":
		layerpresence.LayerPresenceProcess(ctx, args[1:])
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}
}
