package compress

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

func TestSOCIGzipWriter(t *testing.T) {
	// Create a buffer to hold compressed data
	var buf bytes.Buffer

	// Create SOCI writer with small span size for testing
	sociOpts := SOCIOptions{
		SpanSize:     1024, // 1KB for testing
		MinLayerSize: 0,    // No minimum for testing
	}

	writer, err := NewSOCIGzipWriter(&nopWriteCloser{Writer: &buf}, sociOpts)
	if err != nil {
		t.Fatalf("Failed to create SOCI writer: %v", err)
	}

	// Create a tar writer
	tw := tar.NewWriter(writer)

	// Add some files
	testFiles := []struct {
		name    string
		content string
	}{
		{"file1.txt", strings.Repeat("Hello World! ", 100)},
		{"file2.txt", strings.Repeat("SOCI Test ", 200)},
		{"dir/file3.txt", strings.Repeat("Nested file ", 150)},
	}

	for _, tf := range testFiles {
		hdr := &tar.Header{
			Name: tf.name,
			Mode: 0644,
			Size: int64(len(tf.content)),
		}

		// Track header in SOCI
		if err := writer.AppendTarHeader(hdr); err != nil {
			t.Fatalf("Failed to append tar header: %v", err)
		}

		// Write header to tar
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}

		// Write content
		if _, err := tw.Write([]byte(tf.content)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}

	// Close tar writer
	if err := tw.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}

	// Close SOCI writer
	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close SOCI writer: %v", err)
	}

	// Get ztoc info
	ztocInfo := writer.ZtocInfo()
	if ztocInfo == nil {
		t.Fatal("Expected ztoc info to be generated")
	}

	// Verify ztoc was created
	if len(ztocInfo.Bytes) == 0 {
		t.Error("Expected ztoc bytes to be non-empty")
	}

	if ztocInfo.Digest == "" {
		t.Error("Expected ztoc digest to be set")
	}

	if !strings.HasPrefix(ztocInfo.Digest, "sha256:") {
		t.Errorf("Expected digest to start with 'sha256:', got %s", ztocInfo.Digest)
	}

	if ztocInfo.Size != int64(len(ztocInfo.Bytes)) {
		t.Errorf("Expected size %d to match bytes length %d", ztocInfo.Size, len(ztocInfo.Bytes))
	}

	// Verify the compressed data is valid gzip
	gr, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	// Read all data to verify it's valid
	if _, err := io.Copy(io.Discard, gr); err != nil {
		t.Fatalf("Failed to read gzip data: %v", err)
	}
}

// nopWriteCloser wraps an io.Writer to provide Close method
type nopWriteCloser struct {
	io.Writer
}

func (n *nopWriteCloser) Close() error {
	return nil
}

func TestValidateSOCICompression(t *testing.T) {
	tests := []struct {
		name        string
		compression string
		sociEnabled bool
		requireGzip bool
		wantError   bool
	}{
		{
			name:        "SOCI disabled - any compression ok",
			compression: "zstd",
			sociEnabled: false,
			requireGzip: true,
			wantError:   false,
		},
		{
			name:        "SOCI enabled with gzip - ok",
			compression: "gzip",
			sociEnabled: true,
			requireGzip: true,
			wantError:   false,
		},
		{
			name:        "SOCI enabled with zstd and require gzip - error",
			compression: "zstd",
			sociEnabled: true,
			requireGzip: true,
			wantError:   true,
		},
		{
			name:        "SOCI enabled with zstd and no require - warning only",
			compression: "zstd",
			sociEnabled: true,
			requireGzip: false,
			wantError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSOCICompression(tt.compression, tt.sociEnabled, tt.requireGzip)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateSOCICompression() error = %v, wantError %v", err, tt.wantError)
			}
			if err != nil {
				if _, ok := err.(SOCIZstdError); !ok && tt.wantError {
					t.Errorf("Expected SOCIZstdError, got %T", err)
				}
			}
		})
	}
}
