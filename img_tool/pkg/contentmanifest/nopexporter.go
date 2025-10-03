package contentmanifest

import "github.com/bazel-contrib/rules_img/img_tool/pkg/api"

type nopexporter struct{}

func NopExporter() api.CASStateExporter {
	return nopexporter{}
}

func (nopexporter) Export(api.CASStateSupplier) error {
	return nil
}
