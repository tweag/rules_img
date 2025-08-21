package expandtemplate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// pushTemplateRequest represents the input JSON for push operations
type pushTemplateRequest struct {
	Registry      string            `json:"registry"`
	Repository    string            `json:"repository"`
	Tags          []string          `json:"tags,omitempty"`
	BuildSettings map[string]string `json:"build_settings"`
	// Preserve other fields that should pass through unchanged
	Command  string          `json:"command,omitempty"`
	Strategy string          `json:"strategy,omitempty"`
	Manifest json.RawMessage `json:"manifest,omitempty"`
	Index    json.RawMessage `json:"index,omitempty"`
	// Pull info fields
	OriginalRegistries []string `json:"original_registries,omitempty"`
	OriginalRepository string   `json:"original_repository,omitempty"`
	OriginalTag        string   `json:"original_tag,omitempty"`
	OriginalDigest     string   `json:"original_digest,omitempty"`
}

// pushExpandedRequest represents the output JSON for push operations
type pushExpandedRequest struct {
	Registry string   `json:"registry"`
	Repository string   `json:"repository"`
	Tags       []string `json:"tags,omitempty"`
	// Preserve other fields
	Command  string          `json:"command,omitempty"`
	Strategy string          `json:"strategy,omitempty"`
	Manifest json.RawMessage `json:"manifest,omitempty"`
	Index    json.RawMessage `json:"index,omitempty"`
	// Pull info fields
	OriginalRegistries []string `json:"original_registries,omitempty"`
	OriginalRepository string   `json:"original_repository,omitempty"`
	OriginalTag        string   `json:"original_tag,omitempty"`
	OriginalDigest     string   `json:"original_digest,omitempty"`
}

// loadTemplateRequest represents the input JSON for load operations
type loadTemplateRequest struct {
	Command       string            `json:"command"`
	Daemon        string            `json:"daemon"`
	Strategy      string            `json:"strategy"`
	Tag           string            `json:"tag,omitempty"`
	BuildSettings map[string]string `json:"build_settings"`
	Manifest      json.RawMessage   `json:"manifest,omitempty"`
	Index         json.RawMessage   `json:"index,omitempty"`
}

// loadExpandedRequest represents the output JSON for load operations
type loadExpandedRequest struct {
	Command  string          `json:"command"`
	Daemon   string          `json:"daemon"`
	Strategy string          `json:"strategy"`
	Tag      string          `json:"tag,omitempty"`
	Manifest json.RawMessage `json:"manifest,omitempty"`
	Index    json.RawMessage `json:"index,omitempty"`
}

// ExpandTemplateProcess is the main entry point for the expand-template subcommand
func ExpandTemplateProcess(ctx context.Context, args []string) {
	// Define flags for stamp files and kind
	var stampFiles []string
	var kind string
	flagSet := flag.NewFlagSet("expand-template", flag.ExitOnError)
	flagSet.Func("stamp", "Path to a stamp file (can be specified multiple times)", func(s string) error {
		stampFiles = append(stampFiles, s)
		return nil
	})
	flagSet.StringVar(&kind, "kind", "push", "Kind of template to expand (push or load)")

	// Parse flags
	if err := flagSet.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Validate kind
	if kind != "push" && kind != "load" {
		fmt.Fprintf(os.Stderr, "Error: --kind must be either 'push' or 'load'\n")
		os.Exit(1)
	}

	// Get positional arguments
	args = flagSet.Args()
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: img expand-template [--stamp file]... [--kind push|load] <input.json> <output.json>\n")
		os.Exit(1)
	}

	inputPath := args[0]
	outputPath := args[1]

	if err := expandTemplates(inputPath, outputPath, stampFiles, kind); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func expandTemplates(inputPath, outputPath string, stampFiles []string, kind string) error {
	// Read input JSON
	inputData, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading input file: %w", err)
	}

	// Create template data map
	templateData := make(map[string]string)

	// Read stamp files and add their key-value pairs
	for _, stampFile := range stampFiles {
		if err := readStampFile(stampFile, templateData); err != nil {
			return fmt.Errorf("reading stamp file %s: %w", stampFile, err)
		}
	}

	switch kind {
	case "push":
		return expandPushTemplates(inputData, outputPath, templateData)
	case "load":
		return expandLoadTemplates(inputData, outputPath, templateData)
	default:
		return fmt.Errorf("unknown kind: %s", kind)
	}
}

func expandPushTemplates(inputData []byte, outputPath string, templateData map[string]string) error {
	var req pushTemplateRequest
	if err := json.Unmarshal(inputData, &req); err != nil {
		return fmt.Errorf("parsing input JSON: %w", err)
	}

	// Add build settings to template data
	for k, v := range req.BuildSettings {
		templateData[k] = v
	}

	// Expand templates
	expandedRegistry, err := expandTemplate(req.Registry, templateData)
	if err != nil {
		return fmt.Errorf("expanding registry template: %w", err)
	}

	expandedRepository, err := expandTemplate(req.Repository, templateData)
	if err != nil {
		return fmt.Errorf("expanding repository template: %w", err)
	}

	expandedTags := make([]string, len(req.Tags))
	for i, tag := range req.Tags {
		expandedTags[i], err = expandTemplate(tag, templateData)
		if err != nil {
			return fmt.Errorf("expanding tag template %q: %w", tag, err)
		}
	}

	// Create output without build_settings
	output := pushExpandedRequest{
		Registry:   expandedRegistry,
		Repository: expandedRepository,
		Tags:       expandedTags,
		// Copy through other fields
		Command:            req.Command,
		Strategy:           req.Strategy,
		Manifest:           req.Manifest,
		Index:              req.Index,
		OriginalRegistries: req.OriginalRegistries,
		OriginalRepository: req.OriginalRepository,
		OriginalTag:        req.OriginalTag,
		OriginalDigest:     req.OriginalDigest,
	}

	// Write output JSON
	outputData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}

	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}

	return nil
}

func expandLoadTemplates(inputData []byte, outputPath string, templateData map[string]string) error {
	var req loadTemplateRequest
	if err := json.Unmarshal(inputData, &req); err != nil {
		return fmt.Errorf("parsing input JSON: %w", err)
	}

	// Add build settings to template data
	for k, v := range req.BuildSettings {
		templateData[k] = v
	}

	// Expand tag template
	expandedTag, err := expandTemplate(req.Tag, templateData)
	if err != nil {
		return fmt.Errorf("expanding tag template: %w", err)
	}

	// Create output without build_settings
	output := loadExpandedRequest{
		Command:  req.Command,
		Daemon:   req.Daemon,
		Strategy: req.Strategy,
		Tag:      expandedTag,
		Manifest: req.Manifest,
		Index:    req.Index,
	}

	// Write output JSON
	outputData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}

	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}

	return nil
}

func expandTemplate(tmplStr string, data map[string]string) (string, error) {
	if tmplStr == "" {
		return "", nil
	}

	tmpl, err := template.New("expand").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// readStampFile reads a Bazel stamp file and adds key-value pairs to the data map
func readStampFile(path string, data map[string]string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening stamp file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first space to get key and value
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			data[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stamp file: %w", err)
	}

	return nil
}
