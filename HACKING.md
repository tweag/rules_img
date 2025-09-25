# Hacking on rules_img

This guide provides instructions for developers working on rules_img.

## Development Environment

### Prerequisites

- [Nix](https://nixos.org/download.html) (recommended for reproducible development environment)
- OR manually install:
  - Bazel
  - Go
  - pre-commit

### Setting up the Development Environment

#### Option 1: Using Nix (Recommended)

```bash
# Enter the development shell
nix develop

# This provides:
# - Bazel with proper configuration
# - Go toolchain
# - pre-commit hooks
# - All other required tools
```

#### Option 2: Manual Setup

If not using Nix, ensure you have all prerequisites installed and set up pre-commit:

```bash
# Install pre-commit hooks
pre-commit install
```

## IDE Setup

### Visual Studio Code

For the best development experience with VSCode, especially when using Nix:

```bash
# The repository includes VSCode settings for Bazel+Nix development
# Create a symlink to use them:
ln -sf .vscode/settings-bazel-nix.json .vscode/settings.json

# If not using Nix, you may need to adjust the GOPACKAGESDRIVER path
# from gopackagesdriver-nix.sh to gopackagesdriver.sh
```

## Code Formatting and Linting

### Pre-commit Hooks

Pre-commit hooks run automatically on `git commit`. They ensure code quality and consistency.

```bash
# Run all pre-commit hooks manually
pre-commit run --all-files

# Run specific hooks
pre-commit run buildifier --all-files
pre-commit run trailing-whitespace --all-files
```

### Starlark (Bazel) Files

```bash
# Format all Bazel files (fix mode)
bazel run //util:buildifier.fix

# Check Bazel file formatting (test mode)
bazel test //util:buildifier.check

# Run Gazelle to update BUILD files
bazel run //util:gazelle
```

### Markdown Files

Generated markdown files in `docs/` are excluded from trailing whitespace and end-of-file checks.

## Building and Testing

### Building Core Components

```bash
# Build all targets in the main rules_img module
bazel build //...

# Build all targets in the rules_img_tool module (Go binaries)
bazel build @rules_img_tool//...

# Build specific Go binaries from the rules_img_tool module
bazel build @rules_img_tool//cmd/img       # Main CLI tool
bazel build @rules_img_tool//cmd/registry  # CAS-integrated registry
bazel build @rules_img_tool//cmd/bes       # BES server
bazel build @rules_img_tool//pkg/...       # Go libraries
```

### Running Tests

```bash
# Run all tests
bazel test //...

# Run tests with verbose output
bazel test --test_output=all //...
```

### Integration Testing

Integration tests are available in the `e2e/` directory:

```bash
# Run C++ integration tests
cd e2e/cc && bazel test //...

# Run Go integration tests
cd e2e/go && bazel test //...

# Test push functionality
cd e2e/go && bazel run //:push
```

### Testing with Prebuilt img Tool

When developing rules_img, you may need to test with a prebuilt version of the `img` tool. Since the tool is in a separate `rules_img_tool` module, there are a few approaches:

#### Option 1: Local Development with HTTP Server

Build the tool locally and serve it via HTTP:

```bash
# Build the img tool from the rules_img_tool module
bazel build @rules_img_tool//cmd/img

# Copy to a local directory and serve
TMPDIR=$(mktemp -d)
cp bazel-bin/external/rules_img_tool+/cmd/img/img_/img ${TMPDIR}/img
cd ${TMPDIR} && python3 -m http.server 8000
```

Then create a custom lockfile (`prebuilt_lockfile.json`):

```json
[
    {
        "version": "v0.2.3",
        "integrity": "sha256-FG5F8mJuRzvL1oiXCRXyOQ94RvJ+43HH+/yLGbWNvP8=",
        "os": "linux",
        "cpu": "amd64",
        "url_templates": [
            "http://localhost:8000/img"
        ]
    }
]
```

#### Option 2: Airgapped BCR Module

Build a complete local BCR (Bazel Central Registry) module:

```bash
# Build the BCR module and distribution directory
bazel build //img/private/release:bcr
bazel build //img/private/release/distdir

# Set environment variables
export RULES_IMG_BCR=file://$(realpath bazel-bin/img/private/release/bcr.local)
export DISTDIR=$(realpath bazel-bin/img/private/release/distdir/distdir_/distdir)
```

Then configure your test project's `.bazelrc`:

```bash
# .bazelrc
# Use the local BCR first, then fall back to the official registry
# (you need to replace the placeholder with the values from above).
common --registry=${RULES_IMG_BCR} --registry=https://bcr.bazel.build/
common --distdir=${DISTDIR}
```

This approach provides a complete isolated testing environment with all dependencies.

## Documentation

### Generating API Documentation

```bash
# Generate/update all API docs
bazel run //docs:update

# Check if docs are up to date
bazel test //docs:all
```

### Adding New Rules

When adding new public rules:

1. Create the rule in `img/private/`
2. Export it in the appropriate public `.bzl` file in `img/`
3. Add a `bzl_library` target in `img/BUILD.bazel`
4. Add documentation generation in `docs/BUILD.bazel`
5. Run `bazel run //docs:update`

## Common Development Tasks

### Adding a New Compression Algorithm

1. Implement the compressor in `src/pkg/compress/`
2. Add it to the factory in `src/pkg/compress/factory.go`
3. Update the compression attribute in `img/private/layer.bzl`
4. Add the option to `img/settings/BUILD.bazel`
5. Update documentation

### Adding a New Push Strategy

1. Implement the pusher in `src/pkg/push/`
2. Add it to the push command in `src/cmd/push/push.go`
3. Update the push strategy setting in `img/settings/BUILD.bazel`
4. Document it in `docs/push-strategies.md`

### Debugging

```bash
# Use Bazel's debugging features for rules
bazel build --sandbox_debug //target:name

# Debug Go binaries in the rules_img_tool module
bazel build --sandbox_debug @rules_img_tool//cmd/img

# Inspect action outputs
bazel aquery //target:name

# Run the img tool directly for debugging
bazel run @rules_img_tool//cmd/img -- --help

# Debug with verbose output
bazel run @rules_img_tool//cmd/img -- pull --help
```

## Repository Structure

The repository uses a **dual-module structure**:

```
rules_img/                    # Main module - Bazel rules and extensions
├── img/                      # Public Bazel rules
│   └── private/              # Implementation details
├── docs/                     # Generated documentation
├── src/                      # rules_img_tool module - Go code
│   ├── cmd/                  # Command-line tools
│   ├── pkg/                  # Go libraries
│   └── MODULE.bazel          # Separate Bazel module
└── e2e/                      # Integration tests and examples
```

### Module Breakdown:

- **`rules_img`** (root): Contains Bazel rules, extensions, and public API
- **`rules_img_tool`** (src/): Contains Go binaries and libraries used by the rules

This separation allows for better dependency management and enables the Go tools to be distributed independently.

## Troubleshooting

### Bazel Cache Issues

```bash
# Clear Bazel cache
bazel clean --expunge

# Clear specific outputs
bazel clean
```

### Go Module Issues

```bash
# Update go.mod and go.sum in the rules_img_tool module
(cd src && go mod tidy)

# Update Bazel's view of Go dependencies
bazel mod tidy

# Update all BUILD files
bazel run //util:gazelle
```

### IDE Not Finding Dependencies

1. Ensure you're using the correct GOPACKAGESDRIVER
2. Run `bazel build //...` and `bazel build @rules_img_tool//...` to generate all outputs
3. Make sure your IDE is pointed at the `src/` directory for Go development
4. Restart your IDE/language server

## Getting Help

- Check existing issues on GitHub
- Read the [API documentation](docs/)
- Review examples in `benchmark/examples/`
- Ask questions in GitHub Discussions
