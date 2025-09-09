package load

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/bazelbuild/rules_go/go/runfiles"
	ocigodigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/tweag/rules_img/src/pkg/api"
	"github.com/tweag/rules_img/src/pkg/auth/credential"
	"github.com/tweag/rules_img/src/pkg/auth/protohelper"
	"github.com/tweag/rules_img/src/pkg/cas"
	"github.com/tweag/rules_img/src/pkg/containerd"
)

const defaultWorkers = 4

type Request struct {
	Command   string           `json:"command"`
	Daemon    string           `json:"daemon"`
	Strategy  string           `json:"strategy"`
	Tag       string           `json:"tag,omitempty"`
	Manifest  *ManifestRequest `json:"manifest,omitempty"`
	Index     *IndexRequest    `json:"index,omitempty"`
	Platforms []string         `json:"platforms,omitempty"`
}

type ManifestRequest struct {
	ManifestPath string   `json:"manifest"`
	ConfigPath   string   `json:"config"`
	Layers       []string `json:"layers"`
	MissingBlobs []string `json:"missing_blobs,omitempty"`
	api.PullInfo `json:",inline"`
}

type IndexRequest struct {
	IndexPath string            `json:"index"`
	Manifests []ManifestRequest `json:"manifests"`
}

type blobWorkItem struct {
	desc       ocispec.Descriptor
	reader     func() (io.ReadCloser, error)
	sourcePath string
	labels     map[string]string
}

type imageLoader interface {
	collectBlobs(ctx context.Context, rf *runfiles.Runfiles, strategy string, casReader BlobReader) ([]blobWorkItem, error)
	getTarget(ctx context.Context, rf *runfiles.Runfiles, contentStore containerd.Store) (ocispec.Descriptor, error)
}

type manifestLoader struct {
	manifest     *ManifestRequest
	manifestData []byte
}

type indexLoader struct {
	index     *IndexRequest
	indexData []byte
	ociIndex  *ocispec.Index
	platforms []string
}

type BlobReader interface {
	ReaderForBlob(ctx context.Context, digest cas.Digest) (io.ReadCloser, error)
}

func Load(ctx context.Context, req *Request) error {
	rf, err := runfiles.New()
	if err != nil {
		return fmt.Errorf("getting runfiles: %w", err)
	}

	switch req.Daemon {
	case "containerd", "docker": // Supported daemons
	default:
		return fmt.Errorf("unsupported daemon: %s", req.Daemon)
	}

	client, err := ConnectToContainerd(ctx)
	if err != nil && req.Daemon == "docker" {
		fmt.Printf("Connecting to containerd failed: %v\n", err)
		// Print warning about performance impact
		fmt.Fprintln(os.Stderr, "\n\033[33mWARNING: Docker is not using containerd storage backend.\033[0m")
		fmt.Fprintln(os.Stderr, "This will use 'docker load' which is significantly slower than direct containerd loading.")
		fmt.Fprintln(os.Stderr, "To improve performance, configure Docker to use containerd:")
		fmt.Fprintln(os.Stderr, "  https://docs.docker.com/storage/containerd/")
		fmt.Fprintln(os.Stderr, "")

		// Use the docker load path
		return LoadViaDocker(ctx, rf, req)
	} else if err != nil {
		return fmt.Errorf("connecting to containerd: %w", err)
	}
	defer client.Close()

	ctx = containerd.WithNamespace(ctx, "moby")

	casReader, err := setupCASReader(req.Strategy)
	if err != nil {
		return err
	}

	if req.Manifest != nil {
		return LoadManifest(ctx, rf, client, casReader, req.Manifest, req.Strategy, req.Tag)
	} else if req.Index != nil {
		return LoadIndex(ctx, rf, client, casReader, req.Index, req.Strategy, req.Tag, req.Platforms)
	}

	return fmt.Errorf("no manifest or index provided")
}

func setupCASReader(strategy string) (BlobReader, error) {
	if strategy != "lazy" {
		return nil, nil
	}

	reapiEndpoint := os.Getenv("IMG_REAPI_ENDPOINT")
	if reapiEndpoint == "" {
		return nil, fmt.Errorf("IMG_REAPI_ENDPOINT environment variable must be set for lazy load strategy")
	}

	credentialHelper := getCredentialHelper()
	grpcClientConn, err := protohelper.Client(reapiEndpoint, credentialHelper)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client connection: %w", err)
	}

	casReader, err := cas.New(grpcClientConn)
	if err != nil {
		return nil, fmt.Errorf("creating CAS client: %w", err)
	}

	return casReader, nil
}

func getCredentialHelper() credential.Helper {
	credentialHelperPath := os.Getenv("IMG_CREDENTIAL_HELPER")
	if credentialHelperPath != "" {
		return credential.New(credentialHelperPath)
	}
	return credential.NopHelper()
}

func ConnectToContainerd(ctx context.Context) (*containerd.Client, error) {
	address, err := containerd.FindContainerdSocket()
	if err != nil {
		return nil, fmt.Errorf("finding containerd socket: %w", err)
	}
	return containerd.New(address)
}

func LoadManifest(ctx context.Context, rf *runfiles.Runfiles, client *containerd.Client, casReader BlobReader, manifest *ManifestRequest, strategy, tag string) error {
	loader := &manifestLoader{manifest: manifest}
	return loadImage(ctx, rf, client, casReader, loader, strategy, tag)
}

func LoadIndex(ctx context.Context, rf *runfiles.Runfiles, client *containerd.Client, casReader BlobReader, index *IndexRequest, strategy, tag string, platforms []string) error {
	loader := &indexLoader{index: index, platforms: platforms}
	return loadImage(ctx, rf, client, casReader, loader, strategy, tag)
}

func loadImage(ctx context.Context, rf *runfiles.Runfiles, client *containerd.Client, casReader BlobReader, loader imageLoader, strategy, tag string) error {
	blobs, err := loader.collectBlobs(ctx, rf, strategy, casReader)
	if err != nil {
		return fmt.Errorf("collecting blobs: %w", err)
	}

	contentStore := client.ContentStore()
	if err := uploadBlobsParallel(ctx, contentStore, blobs, defaultWorkers); err != nil {
		return fmt.Errorf("uploading blobs: %w", err)
	}

	targetDesc, err := loader.getTarget(ctx, rf, contentStore)
	if err != nil {
		return fmt.Errorf("getting target descriptor: %w", err)
	}

	imageService := client.ImageService()
	normalizedTag := NormalizeDockerReference(tag)
	img := containerd.Image{
		Name:   normalizedTag,
		Target: targetDesc,
	}

	if tag != "" {
		_, err = imageService.Create(ctx, img)
		if err != nil && containerd.IsAlreadyExists(err) {
			_, err = imageService.Update(ctx, img)
		}
		if err != nil {
			return fmt.Errorf("creating/updating image: %w", err)
		}
	}

	fmt.Printf("%s@%s\n", normalizedTag, targetDesc.Digest)
	return nil
}

func (m *manifestLoader) collectBlobs(ctx context.Context, rf *runfiles.Runfiles, strategy string, casReader BlobReader) ([]blobWorkItem, error) {
	blobs, ociManifest, manifestData, _, err := collectManifestBlobs(rf, m.manifest, strategy, casReader)
	if err != nil {
		return nil, err
	}
	m.manifestData = manifestData

	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest(manifestData),
		Size:      int64(len(manifestData)),
	}
	blobs = append(blobs, blobWorkItem{
		desc: manifestDesc,
		reader: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(string(manifestData))), nil
		},
		sourcePath: "manifest",
		labels:     computeManifestGCLabels(ociManifest),
	})

	return blobs, nil
}

func (m *manifestLoader) getTarget(ctx context.Context, rf *runfiles.Runfiles, contentStore containerd.Store) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest(m.manifestData),
		Size:      int64(len(m.manifestData)),
	}, nil
}

func (idx *indexLoader) collectBlobs(ctx context.Context, rf *runfiles.Runfiles, strategy string, casReader BlobReader) ([]blobWorkItem, error) {
	if err := idx.loadIndexData(rf); err != nil {
		return nil, err
	}

	var allBlobs []blobWorkItem
	for i := range idx.ociIndex.Manifests {
		if i >= len(idx.index.Manifests) {
			return nil, fmt.Errorf("manifest %d not found in request", i)
		}

		// Check if this manifest's platform matches the requested platforms
		manifestDesc := &idx.ociIndex.Manifests[i]
		if manifestDesc.Platform != nil && !platformMatches(manifestDesc.Platform, idx.platforms) {
			// Skip this manifest if it doesn't match
			continue
		}

		manifestBlobs, err := idx.collectManifestBlobsForIndex(rf, i, strategy, casReader)
		if err != nil {
			return nil, err
		}
		allBlobs = append(allBlobs, manifestBlobs...)
	}

	// Only add the index blob if we're loading all platforms or if platform filtering wasn't requested
	if len(idx.platforms) == 0 {
		allBlobs = append(allBlobs, idx.createIndexBlob())
	}
	return allBlobs, nil
}

func (idx *indexLoader) loadIndexData(rf *runfiles.Runfiles) error {
	indexPath, err := rf.Rlocation(idx.index.IndexPath)
	if err != nil {
		return fmt.Errorf("resolving index path: %w", err)
	}
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("reading index: %w", err)
	}
	idx.indexData = indexData

	var ociIndex ocispec.Index
	if err := json.Unmarshal(indexData, &ociIndex); err != nil {
		return fmt.Errorf("parsing index: %w", err)
	}
	idx.ociIndex = &ociIndex
	return nil
}

func (idx *indexLoader) collectManifestBlobsForIndex(rf *runfiles.Runfiles, manifestIndex int, strategy string, casReader BlobReader) ([]blobWorkItem, error) {
	manifest := idx.index.Manifests[manifestIndex]
	blobs, _, _, _, err := collectManifestBlobs(rf, &manifest, strategy, casReader)
	if err != nil {
		return nil, fmt.Errorf("collecting blobs for manifest %d: %w", manifestIndex, err)
	}

	manifestBlob, err := idx.createManifestBlob(rf, &manifest, manifestIndex)
	if err != nil {
		return nil, err
	}
	blobs = append(blobs, manifestBlob)

	return blobs, nil
}

func (idx *indexLoader) createManifestBlob(rf *runfiles.Runfiles, manifest *ManifestRequest, index int) (blobWorkItem, error) {
	manifestPath, err := rf.Rlocation(manifest.ManifestPath)
	if err != nil {
		return blobWorkItem{}, fmt.Errorf("resolving manifest path: %w", err)
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return blobWorkItem{}, fmt.Errorf("reading manifest: %w", err)
	}

	var ociManifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &ociManifest); err != nil {
		return blobWorkItem{}, fmt.Errorf("parsing manifest[%d] for GC labels: %w", index, err)
	}

	manifestLabels := computeManifestGCLabels(&ociManifest)
	data := manifestData
	manifestDesc := idx.ociIndex.Manifests[index]

	return blobWorkItem{
		desc: manifestDesc,
		reader: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(string(data))), nil
		},
		sourcePath: fmt.Sprintf("manifest[%d]", index),
		labels:     manifestLabels,
	}, nil
}

func (idx *indexLoader) createIndexBlob() blobWorkItem {
	indexLabels := make(map[string]string)
	for i, manifest := range idx.ociIndex.Manifests {
		indexLabels[fmt.Sprintf("containerd.io/gc.ref.content.m.%d", i)] = manifest.Digest.String()
	}

	return blobWorkItem{
		desc: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageIndex,
			Digest:    digest(idx.indexData),
			Size:      int64(len(idx.indexData)),
		},
		reader: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(string(idx.indexData))), nil
		},
		sourcePath: "index",
		labels:     indexLabels,
	}
}

func computeManifestGCLabels(manifest *ocispec.Manifest) map[string]string {
	labels := make(map[string]string)
	labels["containerd.io/gc.ref.content.config"] = manifest.Config.Digest.String()
	for i, layer := range manifest.Layers {
		labels[fmt.Sprintf("containerd.io/gc.ref.content.l.%d", i)] = layer.Digest.String()
	}
	return labels
}

func (idx *indexLoader) getTarget(ctx context.Context, rf *runfiles.Runfiles, contentStore containerd.Store) (ocispec.Descriptor, error) {
	// If we're filtering platforms and only one platform matches, return the manifest descriptor instead of the index
	if len(idx.platforms) == 1 {
		var matchingManifests []int
		for i, manifestDesc := range idx.ociIndex.Manifests {
			if manifestDesc.Platform != nil && platformMatches(manifestDesc.Platform, idx.platforms) {
				matchingManifests = append(matchingManifests, i)
			}
		}

		if len(matchingManifests) == 0 {
			return ocispec.Descriptor{}, fmt.Errorf("no manifest matches requested platform %v", idx.platforms)
		}

		// If exactly one platform matches, return its manifest descriptor
		if len(matchingManifests) == 1 {
			return idx.ociIndex.Manifests[matchingManifests[0]], nil
		}
	} else if len(idx.platforms) > 1 {
		return ocispec.Descriptor{}, fmt.Errorf("multiple platforms requested %v- this would require recalculating the index, which is not yet supported", idx.platforms)
	}

	// Otherwise, return the index descriptor
	return ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest(idx.indexData),
		Size:      int64(len(idx.indexData)),
	}, nil
}

func collectManifestBlobs(rf *runfiles.Runfiles, manifest *ManifestRequest, strategy string, casReader BlobReader) ([]blobWorkItem, *ocispec.Manifest, []byte, []byte, error) {
	manifestData, configData, ociManifest, err := loadManifestAndConfig(rf, manifest)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var blobs []blobWorkItem
	blobs = append(blobs, createConfigBlob(ociManifest.Config, configData))

	layerBlobs := collectLayerBlobs(rf, manifest, ociManifest, strategy, casReader)
	blobs = append(blobs, layerBlobs...)

	return blobs, ociManifest, manifestData, configData, nil
}

func loadManifestAndConfig(rf *runfiles.Runfiles, manifest *ManifestRequest) ([]byte, []byte, *ocispec.Manifest, error) {
	manifestPath, err := rf.Rlocation(manifest.ManifestPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolving manifest path: %w", err)
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading manifest: %w", err)
	}

	configPath, err := rf.Rlocation(manifest.ConfigPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolving config path: %w", err)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading config: %w", err)
	}

	var ociManifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &ociManifest); err != nil {
		return nil, nil, nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return manifestData, configData, &ociManifest, nil
}

func createConfigBlob(configDesc ocispec.Descriptor, configData []byte) blobWorkItem {
	return blobWorkItem{
		desc: configDesc,
		reader: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(string(configData))), nil
		},
		sourcePath: "config",
	}
}

func collectLayerBlobs(rf *runfiles.Runfiles, manifest *ManifestRequest, ociManifest *ocispec.Manifest, strategy string, casReader BlobReader) []blobWorkItem {
	var blobs []blobWorkItem
	layerPathIndex := 0

	for i, layerDesc := range ociManifest.Layers {
		isMissing := manifest.MissingBlobs != nil && contains(manifest.MissingBlobs, layerDesc.Digest.Hex())
		apiDesc := api.Descriptor{
			Digest: layerDesc.Digest.String(),
			Size:   layerDesc.Size,
		}

		if strategy == "lazy" && casReader != nil {
			blobs = append(blobs, createCASLayerBlob(layerDesc, apiDesc, casReader, i))
		} else if !isMissing && layerPathIndex < len(manifest.Layers) {
			path := manifest.Layers[layerPathIndex]
			layerPathIndex++
			blobs = append(blobs, createLocalLayerBlob(layerDesc, path, rf, i))
		} else if isMissing {
			blobs = append(blobs, createRemoteLayerBlob(layerDesc, apiDesc, manifest.PullInfo, i))
		}
	}

	return blobs
}

func createCASLayerBlob(layerDesc ocispec.Descriptor, apiDesc api.Descriptor, casReader BlobReader, index int) blobWorkItem {
	return blobWorkItem{
		desc: layerDesc,
		reader: func() (io.ReadCloser, error) {
			return GetCASLayerReader(apiDesc, casReader)
		},
		sourcePath: fmt.Sprintf("layer[%d] from CAS", index),
	}
}

func createLocalLayerBlob(layerDesc ocispec.Descriptor, path string, rf *runfiles.Runfiles, index int) blobWorkItem {
	return blobWorkItem{
		desc: layerDesc,
		reader: func() (io.ReadCloser, error) {
			resolvedPath, err := rf.Rlocation(path)
			if err != nil {
				return nil, fmt.Errorf("resolving layer path %s: %w", path, err)
			}
			return os.Open(resolvedPath)
		},
		sourcePath: fmt.Sprintf("layer[%d] from %s", index, path),
	}
}

func createRemoteLayerBlob(layerDesc ocispec.Descriptor, apiDesc api.Descriptor, pullInfo api.PullInfo, index int) blobWorkItem {
	return blobWorkItem{
		desc: layerDesc,
		reader: func() (io.ReadCloser, error) {
			return GetRemoteLayerReader(apiDesc, pullInfo)
		},
		sourcePath: fmt.Sprintf("layer[%d] from registry", index),
	}
}

func uploadBlobsParallel(ctx context.Context, contentStore containerd.Store, blobs []blobWorkItem, numWorkers int) error {
	if numWorkers <= 0 {
		numWorkers = defaultWorkers
	}

	workCh := make(chan blobWorkItem, len(blobs))
	for _, blob := range blobs {
		workCh <- blob
	}
	close(workCh)

	errCh := make(chan error, len(blobs))
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go uploadWorker(ctx, contentStore, workCh, errCh, &wg)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to upload %d blobs: %v", len(errs), errs)
	}

	return nil
}

func uploadWorker(ctx context.Context, contentStore containerd.Store, workCh <-chan blobWorkItem, errCh chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	for blob := range workCh {
		if err := uploadBlob(ctx, contentStore, blob); err != nil {
			errCh <- err
		}
	}
}

func uploadBlob(ctx context.Context, contentStore containerd.Store, blob blobWorkItem) error {
	info, err := contentStore.Info(ctx, blob.desc.Digest)
	if err == nil && info.Digest == blob.desc.Digest {
		return nil
	}

	reader, err := blob.reader()
	if err != nil {
		return fmt.Errorf("getting reader for %s: %w", blob.sourcePath, err)
	}
	defer reader.Close()

	if err := storeBlob(ctx, contentStore, blob.desc, reader, blob.labels); err != nil {
		return fmt.Errorf("storing %s: %w", blob.sourcePath, err)
	}

	return nil
}

func storeBlob(ctx context.Context, store containerd.Store, desc ocispec.Descriptor, reader io.Reader, labels map[string]string) error {
	// Check if the blob already exists
	info, err := store.Info(ctx, desc.Digest)
	if err == nil && info.Digest == desc.Digest && info.Size == desc.Size {
		// Blob already exists with correct size, nothing to do
		return nil
	}

	writer, err := store.Writer(ctx,
		containerd.WithDescriptor(desc))
	if err != nil {
		return fmt.Errorf("creating writer: %w", err)
	}
	defer writer.Close()

	if _, err := io.Copy(writer, reader); err != nil {
		return fmt.Errorf("copying data to writer: %w", err)
	}

	// Prepare commit options
	commitOpts := []containerd.Opt{}
	if len(labels) > 0 {
		commitOpts = append(commitOpts, containerd.WithLabels(labels))
	}

	if err := writer.Commit(ctx, desc.Size, desc.Digest, commitOpts...); err != nil {
		if containerd.IsAlreadyExists(err) {
			// Blob was written by another process, that's ok
			return nil
		}
		return fmt.Errorf("committing data: %w", err)
	}

	return nil
}

func digest(data []byte) ocigodigest.Digest {
	return ocigodigest.FromBytes(data)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func NormalizeDockerReference(ref string) string {
	if ref == "" {
		return ""
	}

	parts := strings.SplitN(ref, ":", 2)
	name := parts[0]
	tag := ""
	if len(parts) > 1 {
		tag = parts[1]
	}

	hasHostname := false

	if strings.HasPrefix(name, "localhost/") {
		hasHostname = true
	} else {
		firstSlash := strings.Index(name, "/")
		if firstSlash > 0 {
			possibleHost := name[:firstSlash]
			if strings.Contains(possibleHost, ".") || strings.Contains(possibleHost, ":") {
				hasHostname = true
			}
		}
	}

	if !hasHostname {
		if !strings.Contains(name, "/") {
			name = "docker.io/library/" + name
		} else {
			name = "docker.io/" + name
		}
	}

	if tag != "" {
		return name + ":" + tag
	}
	return name
}

// parsePlatform parses a platform string like "linux/amd64" into an OCI Platform
func parsePlatform(platform string) (*ocispec.Platform, error) {
	parts := strings.Split(platform, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid platform format: %s", platform)
	}

	p := &ocispec.Platform{
		OS:           parts[0],
		Architecture: parts[1],
	}

	if len(parts) > 2 {
		p.Variant = parts[2]
	}

	return p, nil
}

// platformMatches checks if a manifest platform matches any of the requested platforms
func platformMatches(manifestPlatform *ocispec.Platform, requestedPlatforms []string) bool {
	if len(requestedPlatforms) == 0 {
		return true // No filter, all platforms match
	}

	for _, reqPlatStr := range requestedPlatforms {
		reqPlat, err := parsePlatform(reqPlatStr)
		if err != nil {
			continue
		}

		if manifestPlatform.OS == reqPlat.OS &&
			manifestPlatform.Architecture == reqPlat.Architecture &&
			(reqPlat.Variant == "" || manifestPlatform.Variant == reqPlat.Variant) {
			return true
		}
	}

	return false
}

// getCurrentPlatform returns the current platform string
func getCurrentPlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
