package containerd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	api "github.com/containerd/containerd/api/services/content/v1"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Store is the content store interface
type Store interface {
	Info(ctx context.Context, dgst digest.Digest) (Info, error)
	Writer(ctx context.Context, opts ...WriterOpt) (Writer, error)
}

type contentStore struct {
	client api.ContentClient
}

// Info returns the info for a content
func (s *contentStore) Info(ctx context.Context, dgst digest.Digest) (Info, error) {
	resp, err := s.client.Info(ctx, &api.InfoRequest{
		Digest: dgst.String(),
	})
	if err != nil {
		// Convert gRPC not found errors to a standard error
		if status.Code(err) == codes.NotFound {
			return Info{}, fmt.Errorf("content %s: not found", dgst)
		}
		return Info{}, err
	}

	return Info{
		Digest: digest.Digest(resp.Info.Digest),
		Size:   resp.Info.Size,
		Labels: resp.Info.Labels,
	}, nil
}

// Writer creates a new content writer
func (s *contentStore) Writer(ctx context.Context, opts ...WriterOpt) (Writer, error) {
	var wOpts WriterOpts
	for _, opt := range opts {
		opt(&wOpts)
	}

	// Generate a unique ref if not provided
	if wOpts.Ref == "" {
		wOpts.Ref = generateRef()
	}

	stream, err := s.client.Write(ctx)
	if err != nil {
		return nil, err
	}

	return &contentWriter{
		ctx:      ctx,
		stream:   stream,
		client:   s.client,
		ref:      wOpts.Ref,
		expected: wOpts.Digest,
		total:    wOpts.Size,
		offset:   0,
		buffer:   make([]byte, 0, 4096), // 4KB buffer
	}, nil
}

// Writer is a content writer
type Writer interface {
	io.WriteCloser
	Commit(ctx context.Context, size int64, expected digest.Digest, opts ...Opt) error
	Status() (Status, error)
	Digest() digest.Digest
	Truncate(size int64) error
}

type contentWriter struct {
	ctx      context.Context
	stream   api.Content_WriteClient
	client   api.ContentClient
	ref      string
	expected digest.Digest
	total    int64
	offset   int64
	buffer   []byte
	mu       sync.Mutex
	closed   bool
	started  bool
	digester digest.Digester
}

// Write writes data to the content
func (w *contentWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, fmt.Errorf("writer is closed")
	}

	// If this is the first write and we haven't started yet, do the STAT
	if !w.started && len(p) == 0 {
		// Empty write, just return
		return 0, nil
	}

	// Send initial write request with empty data if not started
	if !w.started {
		req := &api.WriteContentRequest{
			Action:   api.WriteAction_WRITE,
			Ref:      w.ref,
			Offset:   0,
			Data:     []byte{}, // Empty data to allocate the ref
			// Don't set Total/Expected here - save it for COMMIT
		}
		if err := w.stream.Send(req); err != nil {
			return 0, fmt.Errorf("sending initial write request: %w", err)
		}

		// Receive initial response
		resp, err := w.stream.Recv()
		if err != nil {
			return 0, fmt.Errorf("receiving initial write response: %w", err)
		}

		// Update offset from response
		w.offset = resp.Offset
		w.started = true

	}

	if w.digester == nil {
		w.digester = digest.SHA256.Digester()
	}

	// Write to digester
	w.digester.Hash().Write(p)

	// Buffer data and send in chunks
	w.buffer = append(w.buffer, p...)

	// Send data when buffer is large enough (4KB for smaller files)
	if len(w.buffer) >= 4*1024 {
		if err := w.flush(); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (w *contentWriter) flush() error {
	if len(w.buffer) == 0 {
		return nil
	}

	req := &api.WriteContentRequest{
		Action: api.WriteAction_WRITE,
		Ref:    w.ref,
		Offset: w.offset,
		Data:   w.buffer,
	}

	if err := w.stream.Send(req); err != nil {
		return err
	}

	// Receive acknowledgment for the write
	resp, err := w.stream.Recv()
	if err != nil {
		return fmt.Errorf("receiving write ack: %w", err)
	}

	// Update offset from server response
	w.offset = resp.Offset
	w.buffer = w.buffer[:0]

	return nil
}

// Close closes the writer
func (w *contentWriter) Close() error {
	// We don't need to do anything here since Commit handles everything
	return nil
}

// Commit commits the content
func (w *contentWriter) Commit(ctx context.Context, size int64, expected digest.Digest, opts ...Opt) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Make sure we've written something
	if !w.started {
		return fmt.Errorf("commit called before any write")
	}

	var cOpts CommitOpts
	for _, opt := range opts {
		opt(&cOpts)
	}

	// Build COMMIT; include any pending bytes so server writes+commits atomically
	req := &api.WriteContentRequest{
		Action:   api.WriteAction_COMMIT,
		Ref:      w.ref,
		Offset:   w.offset,          // current confirmed offset
		Total:    size,              // expected total size
		Expected: expected.String(), // expected digest
		Labels:   cOpts.Labels,
	}

	// Include any buffered data in the COMMIT request
	if len(w.buffer) > 0 {
		req.Data = w.buffer
	}


	if err := w.stream.Send(req); err != nil {
		return fmt.Errorf("sending commit: %w", err)
	}

	// Half-close send side
	if err := w.stream.CloseSend(); err != nil {
		return fmt.Errorf("closing send: %w", err)
	}

	// Drain responses; final one should have Action==COMMIT
	for {
		resp, err := w.stream.Recv()
		if err == io.EOF {
			// Some impls may EOF without an explicit final response
			break
		}
		if err != nil {
			return fmt.Errorf("receiving commit response: %w", err)
		}

		// Update local offset if the server reports it
		if resp.Offset > 0 {
			w.offset = resp.Offset
		}

		if resp.Action == api.WriteAction_COMMIT {
			// Optional sanity check if you provided size
			if size > 0 && resp.Offset != size {
				return fmt.Errorf("commit response reports partial write: %d of %d", resp.Offset, size)
			}
			break
		}
	}

	// Clear buffer now that it has been sent with COMMIT
	w.buffer = w.buffer[:0]
	w.closed = true
	return nil
}

// Status returns the status of the write
func (w *contentWriter) Status() (Status, error) {
	resp, err := w.client.Status(w.ctx, &api.StatusRequest{
		Ref: w.ref,
	})
	if err != nil {
		return Status{}, err
	}

	return Status{
		Ref:       resp.Status.Ref,
		Offset:    resp.Status.Offset,
		Total:     resp.Status.Total,
		StartedAt: resp.Status.StartedAt.AsTime(),
		UpdatedAt: resp.Status.UpdatedAt.AsTime(),
	}, nil
}

// Digest returns the digest of the content
func (w *contentWriter) Digest() digest.Digest {
	if w.digester != nil {
		return w.digester.Digest()
	}
	return w.expected
}

// Truncate truncates the content
func (w *contentWriter) Truncate(size int64) error {
	// Not implemented for our use case
	return fmt.Errorf("truncate not supported")
}

// Helper functions and types

func generateRef() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "write-" + hex.EncodeToString(b)
}

// WriterOpt is an option for creating a writer
type WriterOpt func(*WriterOpts)

// WriterOpts contains options for creating a writer
type WriterOpts struct {
	Ref    string
	Size   int64
	Digest digest.Digest
}

// WithDescriptor sets the descriptor for the writer
func WithDescriptor(desc ocispec.Descriptor) WriterOpt {
	return func(opts *WriterOpts) {
		opts.Size = desc.Size
		opts.Digest = desc.Digest
	}
}

// WithRef sets the ref for the writer
func WithRef(ref string) WriterOpt {
	return func(opts *WriterOpts) {
		opts.Ref = ref
	}
}

// Opt is an option for commit
type Opt func(*CommitOpts)

// CommitOpts contains options for commit
type CommitOpts struct {
	Labels map[string]string
}

// WithLabels sets labels for the commit
func WithLabels(labels map[string]string) Opt {
	return func(opts *CommitOpts) {
		opts.Labels = labels
	}
}

// Info contains content info
type Info struct {
	Digest    digest.Digest
	Size      int64
	CreatedAt time.Time
	UpdatedAt time.Time
	Labels    map[string]string
}

// Status contains write status
type Status struct {
	Ref       string
	Offset    int64
	Total     int64
	StartedAt time.Time
	UpdatedAt time.Time
}

// IsAlreadyExists returns true if the error is an already exists error
func IsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}

	s, ok := status.FromError(err)
	if !ok {
		return false
	}

	if s.Code() == codes.AlreadyExists {
		return true
	}

	// Also check for specific containerd error messages
	msg := strings.ToLower(s.Message())
	return strings.Contains(msg, "already exists")
}
