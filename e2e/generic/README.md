# Generic E2E Tests

This directory contains language-agnostic end-to-end tests for rules_img that focus on testing edge cases and special behaviors of the rules themselves.

## Test Coverage

### Layer Edge Cases
- **Empty layers**: Layers with no files
- **Empty files**: Layers containing only empty files
- **Mixed file types**: Layers with executables, binaries, text files, and unicode content
- **Deep directory structures**: Files in deeply nested paths
- **Special characters**: Files with spaces, dashes, dots, and underscores in names
- **File metadata**: Testing the new file metadata feature with custom permissions and ownership

### Manifest Edge Cases
- **Single layer manifests**: Minimal manifest configurations
- **Multi-layer manifests**: Complex layering with different file types
- **Extensive annotations**: Testing annotation handling
- **Complex configurations**: Manifests with comprehensive metadata (entrypoint, cmd, env, labels, user, workdir, volumes, ports)

### Index Edge Cases
- **Multi-platform indexes**: Combining multiple manifests into indexes

### File Content Edge Cases
- **Binary data**: Files containing binary content
- **Unicode content**: Files with international characters and emojis
- **Large files**: Files with substantial content (1000 lines)
- **Executable scripts**: Shell scripts with proper permissions

### Path and Naming Edge Cases
- **Deep nesting**: Very long directory paths
- **Special characters**: Various punctuation and spacing in file names
- **Symlinks**: Testing symbolic link creation and handling

### Metadata Edge Cases
- **Default metadata**: Global file attribute defaults
- **Per-file overrides**: Specific metadata for individual files
- **Permission variations**: Different file modes (644, 755, 666)
- **Ownership settings**: Custom user and group IDs

## Running Tests

```bash
# Run all generic e2e tests
bazel test //e2e/generic:all_tests

# Run specific test categories
bazel test //e2e/generic:layer_tests
bazel test //e2e/generic:manifest_tests
bazel test //e2e/generic:index_tests
```

## Purpose

These tests ensure that rules_img handles various edge cases correctly and maintains robust behavior across different file types, configurations, and metadata scenarios. Unlike language-specific e2e tests, these focus purely on the rules' capabilities and limitations.
