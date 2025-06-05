package reapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	registry "github.com/malt3/go-containerregistry/pkg/registry"
	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
	bytestream_proto "google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"

	remoteexecution_proto "github.com/tweag/rules_img/pkg/proto/remote-apis/build/bazel/remote/execution/v2"
	combined "github.com/tweag/rules_img/pkg/serve/registry"
)

type REAPIBlobHandler struct {
	upstream         registry.BlobStatHandler
	casClient        remoteexecution_proto.ContentAddressableStorageClient
	byteStreamClient bytestream_proto.ByteStreamClient
	blobSizeCache    *combined.BlobSizeCache
	capabilities     capabilities
}

func New(upstream registry.BlobStatHandler, clientConn *grpc.ClientConn, blobSizeCache *combined.BlobSizeCache) (*REAPIBlobHandler, error) {
	casClient := remoteexecution_proto.NewContentAddressableStorageClient(clientConn)
	byteStreamClient := bytestream_proto.NewByteStreamClient(clientConn)
	capabilitiesClient := remoteexecution_proto.NewCapabilitiesClient(clientConn)
	capabilities, err := learnCapabilities(context.Background(), capabilitiesClient)
	if err != nil {
		return nil, fmt.Errorf("failed to learn capabilities: %w", err)
	}
	if !capabilities.DigestFunctionSHA256 {
		return nil, errors.New("REAPI does not support SHA256 digest function")
	}

	return &REAPIBlobHandler{
		upstream:         upstream,
		casClient:        casClient,
		byteStreamClient: byteStreamClient,
		blobSizeCache:    blobSizeCache,
		capabilities:     capabilities,
	}, nil
}

func (h *REAPIBlobHandler) Get(ctx context.Context, repo string, hash registryv1.Hash) (io.ReadCloser, error) {
	switch hash.Algorithm {
	case "sha256":
		if !h.capabilities.DigestFunctionSHA256 {
			return nil, fmt.Errorf("unsupported digest algorithm: %s", hash.Algorithm)
		}
	case "sha512":
		if !h.capabilities.DigestFunctionSHA512 {
			return nil, fmt.Errorf("unsupported digest algorithm: %s", hash.Algorithm)
		}
	default:
		return nil, fmt.Errorf("unsupported digest algorithm: %s", hash.Algorithm)
	}

	// since we need to know the size of the blob for any REAPI operations,
	// we ask the cache or upstream registry to find out if the blob exists.
	var upstreamSize int64
	if cachedSize, ok := h.blobSizeCache.Get(hash); ok {
		upstreamSize = cachedSize
	} else {
		var upstreamErr error
		upstreamSize, upstreamErr = h.upstream.Stat(ctx, repo, hash)
		if upstreamErr != nil {
			return nil, upstreamErr
		}
	}

	if upstreamSize < 0 {
		return nil, errors.New("unexpected negative blob size")
	}
	if upstreamSize == 0 {
		return io.NopCloser(bytes.NewReader(nil)), nil // Blob is empty.
	}
	if upstreamSize < h.capabilities.MaxBatchTotalSizeBytes {
		// If the blob is small enough, we can use BatchReadBlobs.
		resp, err := h.casClient.BatchReadBlobs(ctx, protoBatchRead(hash, upstreamSize))
		if err != nil {
			return nil, err
		}
		if len(resp.Responses) != 1 {
			return nil, errors.New("unexpected number of responses from BatchReadBlobs")
		}
		return io.NopCloser(bytes.NewReader(resp.Responses[0].Data)), nil
	}
	// For larger blobs, we use ByteStream to read the blob in chunks.
	ctx, cancel := context.WithCancel(ctx)
	stream, err := h.byteStreamClient.Read(ctx, protoByteStreamRead(hash, upstreamSize))
	if err != nil {
		cancel()
		return nil, err
	}
	return &byteStreamReadCloser{
		stream: stream,
		cancel: cancel,
		limit:  upstreamSize,
	}, nil
}

func (h *REAPIBlobHandler) Stat(ctx context.Context, repo string, hash registryv1.Hash) (int64, error) {
	switch hash.Algorithm {
	case "sha256":
		if !h.capabilities.DigestFunctionSHA256 {
			return 0, fmt.Errorf("unsupported digest algorithm: %s", hash.Algorithm)
		}
	case "sha512":
		if !h.capabilities.DigestFunctionSHA512 {
			return 0, fmt.Errorf("unsupported digest algorithm: %s", hash.Algorithm)
		}
	default:
		return 0, fmt.Errorf("unsupported digest algorithm: %s", hash.Algorithm)
	}

	// since we need to know the size of the blob for any REAPI operations,
	// we ask the upstream registry to find out if the blob exists.
	upstreamSize, upstreamErr := h.upstream.Stat(ctx, repo, hash)
	if upstreamErr != nil {
		return upstreamSize, upstreamErr
	}
	if upstreamSize == 0 {
		return 0, nil
	}

	resp, err := h.casClient.FindMissingBlobs(ctx, protoStat(hash, upstreamSize))
	if err != nil {
		return 0, err
	}
	if len(resp.MissingBlobDigests) == 0 {
		return upstreamSize, nil // Blob is present.
	}
	return 0, registry.ErrNotFound // Blob is missing.
}

func protoDigestFunctionFromRegistryAlgorithm(hash registryv1.Hash) remoteexecution_proto.DigestFunction_Value {
	switch hash.Algorithm {
	case "sha256":
		return remoteexecution_proto.DigestFunction_SHA256
	case "sha512":
		return remoteexecution_proto.DigestFunction_SHA512
	default:
		return remoteexecution_proto.DigestFunction_UNKNOWN
	}
}

func protoBatchRead(hash registryv1.Hash, size int64) *remoteexecution_proto.BatchReadBlobsRequest {
	return &remoteexecution_proto.BatchReadBlobsRequest{
		Digests:        []*remoteexecution_proto.Digest{{Hash: hash.Hex, SizeBytes: size}},
		DigestFunction: protoDigestFunctionFromRegistryAlgorithm(hash),
	}
}

func protoByteStreamRead(hash registryv1.Hash, size int64) *bytestream_proto.ReadRequest {
	return &bytestream_proto.ReadRequest{
		ResourceName: fmt.Sprintf("blobs/%s/%d", hash.Hex, size),
		ReadOffset:   0,
		ReadLimit:    size,
	}
}

func protoStat(hash registryv1.Hash, size int64) *remoteexecution_proto.FindMissingBlobsRequest {
	return &remoteexecution_proto.FindMissingBlobsRequest{
		BlobDigests: []*remoteexecution_proto.Digest{
			{
				Hash:      hash.Hex,
				SizeBytes: size,
			},
		},
		DigestFunction: protoDigestFunctionFromRegistryAlgorithm(hash),
	}
}

type capabilities struct {
	DigestFunctionSHA256   bool
	DigestFunctionSHA512   bool
	MaxBatchTotalSizeBytes int64
}

func learnCapabilities(ctx context.Context, capabilitiesClient remoteexecution_proto.CapabilitiesClient) (capabilities, error) {
	resp, err := capabilitiesClient.GetCapabilities(ctx, &remoteexecution_proto.GetCapabilitiesRequest{})
	if err != nil {
		return capabilities{}, err
	}
	if resp == nil {
		return capabilities{}, errors.New("capabilities response is nil")
	}
	if resp.CacheCapabilities == nil {
		return capabilities{}, errors.New("capabilities response has no cache capabilities")
	}

	var caps capabilities
	for _, f := range resp.CacheCapabilities.DigestFunctions {
		if f == remoteexecution_proto.DigestFunction_SHA256 {
			caps.DigestFunctionSHA256 = true
		}
		if f == remoteexecution_proto.DigestFunction_SHA512 {
			caps.DigestFunctionSHA512 = true
		}
	}
	caps.MaxBatchTotalSizeBytes = resp.CacheCapabilities.MaxBatchTotalSizeBytes
	if caps.MaxBatchTotalSizeBytes <= 0 {
		// Default to 64 MiB if not set.
		caps.MaxBatchTotalSizeBytes = 64 * 1024 * 1024
	}
	return caps, nil
}

type byteStreamReadCloser struct {
	stream bytestream_proto.ByteStream_ReadClient
	buf    bytes.Buffer
	eof    bool
	cancel context.CancelFunc

	limit          int64
	readFromRemote int64
	writtenToOut   int64
}

func (b *byteStreamReadCloser) Read(p []byte) (n int, err error) {
	// first, check if we have data from the previous read
	budget := len(p)
	availableFromLastRead := b.buf.Len()
	copyFromLastRead := min(budget, availableFromLastRead)
	if copyFromLastRead > 0 {
		n := copy(p, b.buf.Next(copyFromLastRead))
		if n > budget {
			// should never happen
			panic(fmt.Sprintf("copy(%d, %d) > %d (budget exceeded)", n, copyFromLastRead, budget))
		}
		if n != copyFromLastRead {
			// should never happen
			panic(fmt.Sprintf("copy(%d, %d) != %d (logic flaw)", n, copyFromLastRead, n))
		}
		b.writtenToOut += int64(n)
		budget -= n
	}
	if budget == 0 {
		// we can fulfill the request with buffered data
		return len(p), b.nilOrEOF()
	}
	// buffer was drained

	if b.eof {
		// we are at the end of the stream
		// and drained the buffer
		// the reader is done
		return 0, io.EOF
	}

	// read from the stream
	resp, err := b.stream.Recv()
	var readFromRemoteNow int
	if resp != nil {
		readFromRemoteNow = len(resp.Data)
	}
	if err == io.EOF {
		// we are at the end of the stream
		// we will also not call Recv again
		// we will return EOF after the buffer is drained
		b.eof = true
	} else if err != nil {
		return 0, err
	}
	b.readFromRemote += int64(readFromRemoteNow)

	// copy the data to the buffer
	n = 0
	if resp != nil {
		n = copy(p[copyFromLastRead:], resp.Data)
	}
	b.writtenToOut += int64(n)
	if n < readFromRemoteNow {
		// we have more data than the requested read wants
		// buffer for next call
		b.buf.Write(resp.Data[n:])
	}
	copiedToOutTotal := copyFromLastRead + n
	return copiedToOutTotal, b.nilOrEOF()
}

func (b *byteStreamReadCloser) Close() error {
	// cancel the context to
	// stop the stream from our side
	b.cancel()
	return nil
}

func (b *byteStreamReadCloser) nilOrEOF() error {
	if b.eof && b.buf.Len() == 0 {
		return io.EOF
	}
	return nil
}
