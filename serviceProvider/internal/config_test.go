package internal

import (
	"os"
	"testing"
)

// The [telemetry] section name does not match the OTel field name, so the
// mapping depends on the struct tag.  Without it the whole section is silently
// dropped and the service starts with no telemetry settings at all.
func TestReadConfigTelemetrySection(t *testing.T) {
	const conf = `
loglevel = "warn"
[api]
    host = "127.0.0.1"
    port = 9270
    prefix = "/example"
[metrics]
    host = "127.0.0.1"
    port = 9271
    path = "/metrics"
[telemetry]
    endpoint = "127.0.0.1:4317"
    insecure = true
    ratio = 0.25
    runtimemetrics = true
`
	path := t.TempDir() + "/serviceProvider.toml"
	if err := os.WriteFile(path, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadConfig(path)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	want := OTel{
		Endpoint:       "127.0.0.1:4317",
		Insecure:       true,
		Ratio:          0.25,
		RuntimeMetrics: true,
	}
	if got.OTel != want {
		t.Errorf("OTel = %+v, want %+v", got.OTel, want)
	}
	if got.API.Port != 9270 || got.API.Prefix != "/example" {
		t.Errorf("API = %+v", got.API)
	}
	if got.Metrics.Port != 9271 || got.Metrics.Path != "/metrics" {
		t.Errorf("Metrics = %+v", got.Metrics)
	}
}
