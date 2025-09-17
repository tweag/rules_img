package push

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"

	"github.com/bazelbuild/rules_go/go/runfiles"

	"github.com/tweag/rules_img/src/pkg/api"
	"github.com/tweag/rules_img/src/pkg/auth/credential"
	"github.com/tweag/rules_img/src/pkg/auth/protohelper"
	"github.com/tweag/rules_img/src/pkg/cas"
	"github.com/tweag/rules_img/src/pkg/proto/blobcache"
	"github.com/tweag/rules_img/src/pkg/push"
)

func PushProcess(ctx context.Context, args []string) {
	rf, err := runfiles.New()
	if err != nil {
		pushFromArgs(ctx, args)
		return
	}
	requestPath, err := rf.Rlocation("dispatch.json")
	if err != nil {
		pushFromArgs(ctx, args)
		return
	}
	if err := PushFromFile(ctx, requestPath); err != nil {
		fmt.Fprintf(os.Stderr, "pushing image based on request file %s: %v\n", requestPath, err)
		return
	}
}

func PushFromFile(ctx context.Context, requestPath string) error {
	// Parse command-line arguments from os.Args
	var additionalTags stringSliceFlag
	var overrideRegistry string

	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.Var(&additionalTags, "tag", "Additional tag to apply (can be used multiple times)")
	fs.Var(&additionalTags, "t", "Additional tag to apply (can be used multiple times)")
	fs.StringVar(&overrideRegistry, "registry", "", "Override registry to push to")
	fs.StringVar(&overrideRegistry, "r", "", "Override registry to push to")

	// Parse os.Args, skipping the program name
	if len(os.Args) > 1 {
		if err := fs.Parse(os.Args[1:]); err != nil {
			// Continue with no additional flags if parsing fails
			fmt.Fprintf(os.Stderr, "Warning: failed to parse flags: %v\n", err)
		}
	}

	return PushFromFileWithExtras(ctx, requestPath, []string(additionalTags), overrideRegistry)
}

func PushFromFileWithExtras(ctx context.Context, requestPath string, additionalTags []string, overrideRegistry string) error {
	rawRequest, err := os.ReadFile(requestPath)
	if err != nil {
		return fmt.Errorf("reading request file: %w", err)
	}
	var req request
	if err := json.Unmarshal(rawRequest, &req); err != nil {
		return fmt.Errorf("unmarshalling request file: %w", err)
	}

	reapiEndpoint := os.Getenv("IMG_REAPI_ENDPOINT")
	blobcacheEndpoint := os.Getenv("IMG_BLOB_CACHE_ENDPOINT")
	credentialHelperPath := credentialHelperPath()
	var credentialHelper credential.Helper
	if credentialHelperPath != "" {
		credentialHelper = credential.New(credentialHelperPath)
	} else {
		credentialHelper = credential.NopHelper()
	}

	pusher := push.New()

	// Use override registry if provided, otherwise use original
	registry := req.Registry
	if overrideRegistry != "" {
		registry = overrideRegistry
	}
	baseReference := registry + "/" + req.Repository

	var digest string
	if req.Manifest.ManifestPath != "" {
		manifestReq := push.PushManifestRequest{
			ManifestPath:   req.Manifest.ManifestPath,
			ConfigPath:     req.Manifest.ConfigPath,
			Layers:         req.Manifest.Layers,
			MissingBlobs:   req.Manifest.MissingBlobs,
			RemoteBlobInfo: req.PullInfo,
		}
		var err error
		digest, err = pusher.PushManifest(ctx, baseReference, manifestReq)
		if err != nil {
			return fmt.Errorf("pushing manifest: %w", err)
		}
	} else if req.Index.IndexPath != "" {
		indexReq := push.PushIndexRequest{
			IndexPath:        req.Index.IndexPath,
			ManifestRequests: make([]push.PushManifestRequest, len(req.Index.Manifests)),
		}
		for i, manifestReq := range req.Index.Manifests {
			indexReq.ManifestRequests[i] = push.PushManifestRequest{
				ManifestPath:   manifestReq.ManifestPath,
				ConfigPath:     manifestReq.ConfigPath,
				Layers:         manifestReq.Layers,
				MissingBlobs:   manifestReq.MissingBlobs,
				RemoteBlobInfo: req.PullInfo,
			}
		}
		var err error
		digest, err = pusher.PushIndex(ctx, baseReference, indexReq)
		if err != nil {
			return fmt.Errorf("pushing index: %w", err)
		}
	} else if req.Command == api.PushMetadata {
		var metadataRequest pushMetadata
		if err := json.Unmarshal(rawRequest, &metadataRequest); err != nil {
			return fmt.Errorf("unmarshalling request file: %w", err)
		}
		if len(metadataRequest.Blobs) == 0 {
			return fmt.Errorf("no descriptors provided for push metadata command")
		}
		switch metadataRequest.Strategy {
		case "lazy":
			if reapiEndpoint == "" {
				return fmt.Errorf("IMG_REAPI_ENDPOINT environment variable must be set for lazy push strategy")
			}
			grpcClientConn, err := protohelper.Client(reapiEndpoint, credentialHelper)
			if err != nil {
				return fmt.Errorf("Failed to create gRPC client connection: %w", err)
			}
			casReader, err := cas.New(grpcClientConn)
			if err != nil {
				return fmt.Errorf("creating CAS client: %w", err)
			}
			if digest, err = push.NewLazy(casReader).Push(ctx, baseReference, metadataRequest.PushRequest); err != nil {
				return fmt.Errorf("pushing image with lazy strategy: %w", err)
			}
		case "cas_registry":
			if blobcacheEndpoint == "" {
				return fmt.Errorf("IMG_BLOB_CACHE_ENDPOINT environment variable must be set for cas_registry push strategy")
			}
			grpcClientConn, err := protohelper.Client(blobcacheEndpoint, credentialHelper)
			if err != nil {
				return fmt.Errorf("Failed to create gRPC client connection: %w", err)
			}
			blobcacheClient := blobcache.NewBlobsClient(grpcClientConn)
			if digest, err = push.NewCASRegistryPusher(blobcacheClient).Push(ctx, baseReference, metadataRequest.PushRequest); err != nil {
				return fmt.Errorf("pushing image with CAS registry strategy: %w", err)
			}
		case "bes":
			fmt.Fprintln(os.Stderr, `You don't need to "bazel run" the target in this mode. Image is pushed as a side-effect of uploading BEP data to the BES service.`)
			return nil
		default:
			return fmt.Errorf("unknown push strategy %q", req.Strategy)
		}
	} else {
		return fmt.Errorf("no manifest or index path provided")
	}

	// Combine original tags with additional tags, deduplicate and sort
	allTags := deduplicateAndSort(append(req.Tags, additionalTags...))

	// Apply tags if any were specified
	if len(allTags) > 0 {
		if err := push.PushTags(ctx, baseReference, digest, allTags); err != nil {
			return fmt.Errorf("applying tags: %w", err)
		}
		for _, tag := range allTags {
			fmt.Printf("%s:%s\n", baseReference, tag)
		}
	}

	fmt.Printf("%s/%s@%s\n", registry, req.Repository, digest)
	return nil
}

// deduplicateAndSort removes duplicates and sorts a slice of strings
func deduplicateAndSort(tags []string) []string {
	if len(tags) == 0 {
		return tags
	}

	// Sort first, then compact to remove consecutive duplicates
	sort.Strings(tags)
	return slices.Compact(tags)
}

// stringSliceFlag implements flag.Value for collecting multiple string values
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("%v", []string(*s))
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func pushFromArgs(ctx context.Context, args []string) {
	panic("not implemented")
}

func credentialHelperPath() string {
	credentialHelper := os.Getenv("IMG_CREDENTIAL_HELPER")
	if credentialHelper != "" {
		return credentialHelper
	}
	workingDirectory := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
	defaultPathHelper, defaultPathHelperErr := exec.LookPath(filepath.FromSlash(path.Join(workingDirectory, "tools", "credential-helper")))
	tweagCredentialHelper, tweagErr := exec.LookPath("tweag-credential-helper")

	if defaultPathHelper != "" && defaultPathHelperErr == nil {
		// If IMG_CREDENTIAL_HELPER is not set, we look for a credential helper in the workspace.
		// This is useful for local development.
		return defaultPathHelper
	} else if tweagCredentialHelper != "" && tweagErr == nil {
		// If there is no credential helper in %workspace%/tools/credential_helper,
		// we look for the tweag-credential-helper in the PATH.
		return tweagCredentialHelper
	}
	return ""
}

type request struct {
	Command  string `json:"command"`
	Strategy string `json:"strategy,omitempty"`
	api.PushTarget
	Manifest manifestRequest `json:"manifest"`
	Index    indexRequest    `json:"index"`
	api.PullInfo
}

type indexRequest struct {
	IndexPath string `json:"index"`
	Manifests []manifestRequest
}

type manifestRequest struct {
	ManifestPath string            `json:"manifest"`
	ConfigPath   string            `json:"config"`
	Layers       []push.LayerInput `json:"layers"`
	MissingBlobs []string          `json:"missing_blobs"`
}
