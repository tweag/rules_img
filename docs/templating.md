# Templating and Stamping

The `image_manifest`, `image_index`, `image_push`, and `image_load` rules support Go templates for dynamic configuration. This feature enables flexible image naming and metadata based on build settings and Bazel's workspace status (stamping).

## Overview

Templates allow you to:
- Use different registries/repositories for different environments (dev, staging, prod)
- Include version information from your build system
- Add git commit hashes or timestamps to tags and labels
- Create conditional logic for tag naming
- Inject dynamic metadata into container labels, environment variables, and annotations

## Basic Templating with Build Settings

### 1. Define String Flags

First, create string flags using `bazel_skylib`:

```starlark
load("@bazel_skylib//rules:common_settings.bzl", "string_flag")

string_flag(
    name = "environment",
    build_setting_default = "dev",
)

string_flag(
    name = "region",
    build_setting_default = "us-east-1",
)
```

### 2. Use Templates in image_push

Reference the flags in your `image_push` rule:

```starlark
load("@rules_img//img:push.bzl", "image_push")

image_push(
    name = "push",
    image = ":my_image",

    # Use Go template syntax
    registry = "{{.region}}.registry.example.com",
    repository = "myapp/{{.environment}}",
    tag_list = [
        "latest",
        "{{.environment}}-latest",
    ],

    # Map build settings
    build_settings = {
        "environment": ":environment",
        "region": ":region",
    },
)
```

### 3. Override at Build Time

```bash
# Use default values (dev, us-east-1)
bazel run //:push

# Override for production
bazel run //:push --//:environment=prod --//:region=eu-west-1
```

This would push to:
- Registry: `eu-west-1.registry.example.com`
- Repository: `myapp/prod`
- Tags: `latest`, `prod-latest`

## Stamping with Workspace Status

Stamping allows you to include dynamic build information like git commits, timestamps, and version numbers in your container tags, labels, environment variables, and annotations.

### Requirements for Stamping

**Important**: Stamping requires explicit opt-in at two levels:

1. **Bazel level**: Enable stamping with the `--stamp` flag
   - By default, Bazel disables stamping for build reproducibility and performance
   - You must explicitly add `--stamp` to your build command or `.bazelrc`

2. **Target level**: Enable stamping for specific `image_push`, `image_load`, `image_manifest`, or `image_index` targets
   - Set `stamp = "enabled"` on the target, OR
   - Set `stamp = "auto"` (the default) and use `--@rules_img//img/settings:stamp=enabled`

Both levels must be enabled for stamping to work. If either is disabled, stamp variables will not be available in templates.

### Configure Workspace Status

Create a script that outputs key-value pairs:

```bash
#!/usr/bin/env bash
# File: workspace_status.sh

# Variables prefixed with STABLE_ are included in the cache key.
# If their value changes, the target must be rebuilt.
# Only use for values that rarely update for better performance.
echo "STABLE_CONTAINER_VERSION_TAG v1.2.3"

# Variables without STABLE_ prefix are volatile.
# These variables are not included in the cache key.
# If their values changes, a target may still include
# a stale value from a previous build.
echo "BUILD_TIMESTAMP $(date +%s)"
echo "GIT_COMMIT $(git rev-parse HEAD 2>/dev/null || echo 'unknown')"
echo "GIT_BRANCH $(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'unknown')"
echo "GIT_DIRTY $(if git diff --quiet 2>/dev/null; then echo 'clean'; else echo 'dirty'; fi)"
```

Make it executable:
```bash
chmod +x workspace_status.sh
```

Add to `.bazelrc`:
```bash
# Configure workspace status script
build --workspace_status_command=./workspace_status.sh
```

### Stamp Attribute Values

| Value | Behavior |
|-------|----------|
| `"enabled"` | Always use stamp values (if Bazel --stamp is set) |
| `"disabled"` | Never use stamp values |
| `"auto"` | Use the `--@rules_img//img/settings:stamp=enabled` flag (default) |

### Troubleshooting Stamping

**Stamp variables are empty or not replaced:**
1. Check that `--stamp` is set at Bazel level
2. Check that `stamp = "enabled"` or proper flags are set
3. Verify workspace_status_command is executable and in .bazelrc
4. Test your script: `./workspace_status.sh` should output key-value pairs

**Build not reproducible:**
- Use `stamp = "disabled"` for development builds
- Only enable stamping for release builds using configs

**Testing stamp values:**
```bash
# Check what values are available
bazel build --stamp //:push
cat bazel-bin/push_template.json  # Shows template with stamp placeholders
cat bazel-bin/push.json            # Shows expanded values
```

## Advanced Template Features

### Conditional Logic

```starlark
tag_list = [
    # Use version tag if available, otherwise "dev"
    "{{if .STABLE_CONTAINER_VERSION_TAG}}{{.STABLE_CONTAINER_VERSION_TAG}}{{else}}dev{{end}}",

    # Add suffix only in dev environment
    "latest{{if eq .environment \"dev\"}}-dev{{end}}",

    # Complex conditions
    "{{if and .GIT_COMMIT (ne .GIT_BRANCH \"main\")}}{{.GIT_BRANCH}}-{{.GIT_COMMIT}}{{end}}",
]
```

### Combining Build Settings and Stamping

You can use both build settings and stamp values together:

```starlark
image_push(
    name = "push",
    image = ":my_image",

    # Combine region from build setting with stamp info
    registry = "{{.region}}.registry.example.com",
    repository = "{{.organization}}/{{.STABLE_BUILD_USER}}/myapp",
    tag_list = [
        "{{.environment}}-{{.STABLE_CONTAINER_VERSION_TAG}}",
        "{{.environment}}-{{.GIT_COMMIT}}",
    ],

    build_settings = {
        "environment": ":environment",
        "region": ":region",
        "organization": ":organization",
    },
    stamp = "enabled",
)
```
