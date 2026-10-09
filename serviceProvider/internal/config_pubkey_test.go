package internal

import (
	"os"
	"testing"
)

// The pubKeyHost of a template is what the key parameter of an apply request
// carries, so the name in the configuration has to reach the field.
func TestReadConfigPubKeyHost(t *testing.T) {
	const conf = `
loglevel = "debug"
[api]
    host = "0.0.0.0"
    port = 9270
    prefix = "/example"
[templates.t]
    source = "file:///tmp/x.json"
    secretKey = "./k.pem"
    secretType = "RS256"
    pubKeyHost = "dc-1"
`
	path := t.TempDir() + "/serviceProvider.toml"
	if err := os.WriteFile(path, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfig(path)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	tmpl, ok := got.Templates["t"]
	if !ok {
		t.Fatal("the template is missing from the configuration")
	}
	if tmpl.PubKeyHost != "dc-1" {
		t.Errorf("PubKeyHost = %q, want %q", tmpl.PubKeyHost, "dc-1")
	}
	if tmpl.SecretType != "RS256" || tmpl.SecretKey != "./k.pem" {
		t.Errorf("the rest of the template did not read: %+v", tmpl)
	}
}
