package compress

type Option interface {
	apply(*options)
}

type ContentType string

type HashAlgorithm string

type CompressionAlgorithm string

type CompressionLevel int

type options struct {
	contentType      ContentType
	compressionLevel *CompressionLevel
}

func (c ContentType) apply(opts *options)      { opts.contentType = c }
func (l CompressionLevel) apply(opts *options) { opts.compressionLevel = &l }
