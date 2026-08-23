package observability

import "github.com/bruli-lab/stowmark/internal/config"

const defaultProtocol = "http/protobuf"

func observabilityProtocol(
	cfg config.ObservabilityConfig,
) string {
	if cfg.OTELExporterProtocol == nil ||
		*cfg.OTELExporterProtocol == "" {
		return defaultProtocol
	}

	return *cfg.OTELExporterProtocol
}
