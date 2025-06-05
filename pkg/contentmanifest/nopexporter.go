package contentmanifest

import "github.com/tweag/rules_img/pkg/api"

type nopexporter struct{}

func NopExporter() api.CASStateExporter {
	return nopexporter{}
}

func (nopexporter) Export(api.CASStateSupplier) error {
	return nil
}
