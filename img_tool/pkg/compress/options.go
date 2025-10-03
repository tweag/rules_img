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
    compressorJobs   *int
}

func (c ContentType) apply(opts *options)      { opts.contentType = c }
func (l CompressionLevel) apply(opts *options) { opts.compressionLevel = &l }
type CompressorJobs int
func (j CompressorJobs) apply(opts *options)   { v := int(j); opts.compressorJobs = &v }
