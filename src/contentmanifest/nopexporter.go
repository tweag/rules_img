package contentmanifest

import "github.com/malt3/rules_img/src/api"

type nopexporter struct{}

func NopExporter() api.CASStateExporter {
	return nopexporter{}
}

func (nopexporter) Export(api.CASStateSupplier) error {
	return nil
}
