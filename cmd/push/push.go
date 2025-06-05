package push

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/tweag/rules_img/pkg/api"
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

	pusher := push.New()

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
		digest, err = pusher.PushManifest(ctx, req.Registry+"/"+req.Repository+":"+req.Tag, manifestReq)
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
		digest, err = pusher.PushIndex(ctx, req.Registry+"/"+req.Repository+":"+req.Tag, indexReq)
		if err != nil {
			return fmt.Errorf("pushing index: %w", err)
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
	Command string `json:"command"`
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
