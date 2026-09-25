package internal

import (
	"github.com/BurntSushi/toml"
)

type SPConf struct {
	API API
	// The OpenTelemetry setup.  The section is called [telemetry] rather
	// than [otel], because that is what the settings as a whole describe.
	OTel     OTel `toml:"telemetry"`
	// Metrics are the OTel metric scrape end point.
	Metrics Metrics
	Loglevel string
}

type API struct {
	Host   string
	Port   uint16
	Prefix string
}

// OTel is the OpenTelemetry setup.  Traces and metrics are pushed to an OTLP
// collector over gRPC.  See also Metrics for scrapes.
type OTel struct {
	// Endpoint is the host:port of the OTLP collector, "127.0.0.1:4317"
	// for a local one.  When empty nothing is pushed and only the scrape
	// endpoint reports.
	Endpoint string
	// Insecure turns off TLS towards the collector.
	Insecure bool
	// Ratio is the fraction of traces that is recorded, between 0 and 1.
	// It is ignored for spans that arrive with a sampling decision of
	// their own.
	Ratio float64
	// RuntimeMetrics adds Go runtime and process metrics to the output.
	RuntimeMetrics bool
}


// Metrics is the address the OpenTelemetry metrics are exposed on for  scraping.
type Metrics struct {
	Host string
	Port uint16
	Path string
}

func ReadConfig(path string) (SPConf, error) {
	var conf SPConf

	_, err := toml.DecodeFile(path, &conf)
	if err != nil {
		return conf, err
	}

	return conf, nil
}
