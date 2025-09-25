package cas

import "fmt"

type CASError struct {
	underlying error
}

func casErr(err error) error {
	if err == nil {
		return nil
	}
	return &CASError{underlying: err}
}

func (e *CASError) Error() string {
	return fmt.Sprintf("from Bazel remote cache: %v", e.underlying)
}

func (e *CASError) Unwrap() error {
	return e.underlying
}
