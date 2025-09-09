package load

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bazelbuild/rules_go/go/runfiles"

	"github.com/tweag/rules_img/src/pkg/load"
)

// LoadProcess is the main entry point for the load command
func LoadProcess(ctx context.Context, args []string) {
	// Parse command-line flags
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	var platforms string
	fs.StringVar(&platforms, "platform", "", "Comma-separated list of platforms to load (e.g., linux/amd64,linux/arm64). If not set, all platforms are loaded.")

	// Parse args
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Parse platforms
	var platformList []string
	if platforms != "" {
		platformList = strings.Split(platforms, ",")
		// Trim whitespace from each platform
		for i, p := range platformList {
			platformList[i] = strings.TrimSpace(p)
		}
	}

	rf, err := runfiles.New()
	if err != nil {
		loadFromArgs(ctx, fs.Args())
		return
	}
	requestPath, err := rf.Rlocation("dispatch.json")
	if err != nil {
		loadFromArgs(ctx, fs.Args())
		return
	}
	if err := LoadFromFile(ctx, requestPath, args); err != nil {
		fmt.Fprintf(os.Stderr, "loading image based on request file %s: %v\n", requestPath, err)
		os.Exit(1)
	}
}

// LoadFromFile loads an image based on a JSON request file
func LoadFromFile(ctx context.Context, requestPath string, args []string) error {
	// Parse command-line flags
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	var platforms string
	fs.StringVar(&platforms, "platform", "", "Comma-separated list of platforms to load (e.g., linux/amd64,linux/arm64). If not set, all platforms are loaded.")

	// Parse args
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Parse platforms
	var platformList []string
	if platforms != "" {
		platformList = strings.Split(platforms, ",")
		// Trim whitespace from each platform
		for i, p := range platformList {
			platformList[i] = strings.TrimSpace(p)
		}
	}

	rawRequest, err := os.ReadFile(requestPath)
	if err != nil {
		return fmt.Errorf("reading request file: %w", err)
	}

	var req load.Request
	if err := json.Unmarshal(rawRequest, &req); err != nil {
		return fmt.Errorf("unmarshalling request file: %w", err)
	}

	// Set platforms in the request
	req.Platforms = platformList

	return load.Load(ctx, &req)
}

func loadFromArgs(ctx context.Context, args []string) {
	panic("not implemented")
}
