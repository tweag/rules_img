package push

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bazelbuild/rules_go/go/runfiles"

	"github.com/tweag/rules_img/pkg/api"
	"github.com/tweag/rules_img/pkg/auth/credential"
	"github.com/tweag/rules_img/pkg/auth/protohelper"
	"github.com/tweag/rules_img/pkg/cas"
	"github.com/tweag/rules_img/pkg/push"
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
	rawRequest, err := os.ReadFile(requestPath)
	if err != nil {
		return fmt.Errorf("reading request file: %w", err)
	}
	var req request
	if err := json.Unmarshal(rawRequest, &req); err != nil {
		return fmt.Errorf("unmarshalling request file: %w", err)
	}

	reapiEndpoint := os.Getenv("IMG_REAPI_ENDPOINT")
	credentialHelperPath := os.Getenv("IMG_CREDENTIAL_HELPER")
	var credentialHelper credential.Helper
	if credentialHelperPath != "" {
		credentialHelper = credential.New(credentialHelperPath)
	} else {
		credentialHelper = credential.NopHelper()
	}

	pusher := push.New()
	reference := req.Registry + "/" + req.Repository + ":" + req.Tag

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
		digest, err = pusher.PushManifest(ctx, reference, manifestReq)
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
		digest, err = pusher.PushIndex(ctx, reference, indexReq)
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
		digest = metadataRequest.Blobs[0].Digest
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
			if _, err := push.NewLazy(casReader).Push(ctx, reference, metadataRequest.PushRequest); err != nil {
				return fmt.Errorf("pushing image with lazy strategy: %w", err)
			}
		case "cas_registry":
			fmt.Fprintln(os.Stderr, "placeholder for cas_registry push strategy")
		case "bes":
			fmt.Fprintln(os.Stderr, `You don't need to "bazel run" the target in this mode. Image is pushed as a side-effect of uploading BEP data to the BES service.`)
		default:
			return fmt.Errorf("unknown push strategy %q", req.Strategy)
		}
	} else {
		return fmt.Errorf("no manifest or index path provided")
	}
	fmt.Printf("%s/%s@%s\n", req.Registry, req.Repository, digest)
	return nil
}

func pushFromArgs(ctx context.Context, args []string) {
	panic("not implemented")
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
