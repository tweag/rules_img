package contentmanifest

import "github.com/tweag/rules_img/src/pkg/api"

type nopexporter struct{}

func NopExporter() api.CASStateExporter {
	return nopexporter{}
}

func (nopexporter) Export(api.CASStateSupplier) error {
	return nil
}
