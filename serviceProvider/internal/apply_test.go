package internal

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/seamlessdns/service/providerFormats"
)

// The query that is signed is the domain first, then the variables of the
// records being applied, and then the group when the template has one.
func TestApplyQuery(t *testing.T) {
	tmpl := testTemplates()["flat"].Template
	values := url.Values{
		"IP":        {"192.0.2.10"},
		"TIMESTAMP": {"1700000000"},
		"MESSAGE":   {"hello world & more"},
		// A parameter that the template does not declare must not end up
		// in what is signed.
		"unexpected": {"injected"},
	}
	got := applyQuery("example.com", "", tmpl, values.Get)
	want := "domain=example.com&IP=192.0.2.10&TIMESTAMP=1700000000&MESSAGE=hello+world+%26+more"
	if got != want {
		t.Errorf("applyQuery() = %q, want %q", got, want)
	}
}

// A group is told apart in the request, so that the DNS Provider applies that
// group only.
func TestApplyQueryGroup(t *testing.T) {
	tmpl := testTemplates()["grouped"].Template
	got := applyQuery("example.com", "verify", tmpl, func(string) string { return "abc" })
	want := "domain=example.com&TOKEN=abc&groupId=verify"
	if got != want {
		t.Errorf("applyQuery() = %q, want %q", got, want)
	}
}

// The signature and the key are the last two parameters of the address, and
// neither of them is part of what was signed.
func TestApplyURL(t *testing.T) {
	got, err := applyURL("https://dash.example.com/domainconnect", "wpengine.com", "arecord",
		"domain=example.com&ip=192.0.2.10", "sig+/=", "blue")
	if err != nil {
		t.Fatalf("applyURL() failed: %v", err)
	}
	want := "https://dash.example.com/domainconnect/v2/domainTemplates/providers/wpengine.com" +
		"/services/arecord/apply?domain=example.com&ip=192.0.2.10&sig=sig%2B%2F%3D&key=blue"
	if got != want {
		t.Errorf("applyURL() = %q, want %q", got, want)
	}
}

// A DNS Provider without a synchronous flow cannot have a template applied to
// it, so there is no address to send the user to.
func TestApplyURLRefuses(t *testing.T) {
	tests := map[string]struct{ url, provider, service string }{
		"no address":     {"", "p", "s"},
		"not http":       {"ftp://example.com", "p", "s"},
		"no host":        {"https://", "p", "s"},
		"no provider":    {"https://example.com", "", "s"},
		"no service":     {"https://example.com", "p", ""},
		"not an address": {"https://exa mple.com", "p", "s"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got, err := applyURL(test.url, test.provider, test.service, "domain=example.com", "s", "k"); err == nil {
				t.Errorf("applyURL(%q) = %q, want an error", test.url, got)
			}
		})
	}
}

// The DNS name the public key is looked up at is the host of the configuration
// in front of the domain of the template, and it is the domain of the post data.
func TestPubKeyName(t *testing.T) {
	conf := Template{
		PubKeyHost: "blue",
		Template:   providerFormats.Template{SyncPubKeyDomain: "landmark.wpesvc.net"},
	}
	if got, want := pubKeyName(conf), "blue.landmark.wpesvc.net"; got != want {
		t.Errorf("pubKeyName() = %q, want %q", got, want)
	}
	// Without both halves there is no name to look the key up at.
	noHost := Template{Template: providerFormats.Template{SyncPubKeyDomain: "landmark.wpesvc.net"}}
	if got := pubKeyName(noHost); got != "" {
		t.Errorf("pubKeyName() without a host = %q, want none", got)
	}
}

// A signature has to check out with the public key, or the DNS Provider will
// not apply the template.  Each algorithm the configuration names is checked
// the way the verifier of a DNS Provider checks it.
func TestSignQuery(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edPublic, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const query = "domain=example.com&ip=192.0.2.10"

	tests := []struct {
		name       string
		key        any
		secretType string
		verify     func(t *testing.T, signature []byte)
	}{
		{
			name: "RS256", key: rsaKey, secretType: "RS256",
			verify: func(t *testing.T, signature []byte) {
				digest := sha256Sum(t, query)
				if err := rsa.VerifyPKCS1v15(&rsaKey.PublicKey, crypto.SHA256, digest, signature); err != nil {
					t.Errorf("the signature does not verify: %v", err)
				}
			},
		},
		{
			name: "PS256", key: rsaKey, secretType: "PS256",
			verify: func(t *testing.T, signature []byte) {
				digest := sha256Sum(t, query)
				if err := rsa.VerifyPSS(&rsaKey.PublicKey, crypto.SHA256, digest, signature, nil); err != nil {
					t.Errorf("the signature does not verify: %v", err)
				}
			},
		},
		{
			name: "ES256", key: ecKey, secretType: "ES256",
			verify: func(t *testing.T, signature []byte) {
				digest := sha256Sum(t, query)
				if !ecdsa.VerifyASN1(&ecKey.PublicKey, digest, signature) {
					t.Error("the signature does not verify")
				}
			},
		},
		{
			name: "Ed25519", key: edKey, secretType: "Ed25519",
			verify: func(t *testing.T, signature []byte) {
				if !ed25519.Verify(edPublic, []byte(query), signature) {
					t.Error("the signature does not verify")
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conf := Template{SigningKey: test.key, SecretType: test.secretType, PubKeyHost: "blue"}
			conf.Template.SyncPubKeyDomain = "d.example.com"
			signature, err := signQuery(conf, query)
			if err != nil {
				t.Fatalf("signQuery() failed: %v", err)
			}
			raw, err := base64.StdEncoding.DecodeString(signature)
			if err != nil {
				t.Fatalf("the signature is not base64: %v", err)
			}
			test.verify(t, raw)
		})
	}
}

// A signature over something else than the query is not a signature of the
// request, so it has to fail to verify.
func TestSignQueryIsOverTheQuery(t *testing.T) {
	edPublic, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	conf := Template{SigningKey: edKey, SecretType: "Ed25519", PubKeyHost: "blue"}
	conf.Template.SyncPubKeyDomain = "d.example.com"
	signature, err := signQuery(conf, "domain=example.com&ip=192.0.2.10")
	if err != nil {
		t.Fatalf("signQuery() failed: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		t.Fatal(err)
	}
	if ed25519.Verify(edPublic, []byte("domain=example.com&ip=192.0.2.11"), raw) {
		t.Error("a signature over a changed query verified, which it must not")
	}
}

// Every reason a request cannot be signed has to be told apart, because the
// fix is a different one for each of them.
func TestSignQueryRefuses(t *testing.T) {
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]Template{
		"no key":            {SecretType: "Ed25519"},
		"no key domain":     {SigningKey: edKey, SecretType: "Ed25519", PubKeyHost: "blue"},
		"no pub key host":   {SigningKey: edKey, SecretType: "Ed25519", Template: providerFormats.Template{SyncPubKeyDomain: "d.example.com"}},
		"unknown algorithm": {SigningKey: edKey, SecretType: "Ed255255", PubKeyHost: "blue", Template: providerFormats.Template{SyncPubKeyDomain: "d.example.com"}},
		"mismatched key":    {SigningKey: edKey, SecretType: "RS256", PubKeyHost: "blue", Template: providerFormats.Template{SyncPubKeyDomain: "d.example.com"}},
		"not a key":         {SigningKey: "not a key", SecretType: "Ed25519", PubKeyHost: "blue", Template: providerFormats.Template{SyncPubKeyDomain: "d.example.com"}},
	}
	for name, conf := range tests {
		t.Run(name, func(t *testing.T) {
			if got, err := signQuery(conf, "domain=example.com"); err == nil {
				t.Errorf("signQuery() = %q, want an error", got)
			}
		})
	}
}

// The button collects the values and asks for the request, which is shown and
// not made.
func TestCurlRequest(t *testing.T) {
	edPublic, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpls := signedTestTemplates(t, edKey, "exampleservice.domainconnect.org")

	definitions = newDefinitionCache(settingsTTL)
	seedProviderWith(t, tmpls, "https://dash.example.com/domainconnect")

	fragment := postApply(t, tmpls, "/example/connect/apply?domain=example.com&template=flat",
		url.Values{"IP": {"192.0.2.10"}, "TIMESTAMP": {"1700000000"}, "MESSAGE": {"hi"}})

	// The command carries the address, the signature, and the key, in that
	// order.
	if !strings.Contains(fragment, "https://dash.example.com/domainconnect/v2/domainTemplates/providers/exampleservice.domainconnect.org/services/flat/apply?") {
		t.Errorf("the fragment has no apply address:\n%s", fragment)
	}
	for _, want := range []string{"domain=example.com", "&sig=", "&key=blue"} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
	// The request is shown and nothing else: the fragment makes no request
	// of its own, so the page cannot apply the template by itself.
	if strings.Contains(fragment, "hx-") {
		t.Errorf("the fragment would make a request of its own:\n%s", fragment)
	}

	// The post data carries the signature, the query, and the DNS name of the
	// public key rather than the domain that is being changed.
	signature := signatureFromFragment(t, fragment)
	raw, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		t.Fatalf("the signature in the fragment is not base64: %v", err)
	}
	const query = "domain=example.com&IP=192.0.2.10&TIMESTAMP=1700000000&MESSAGE=hi"
	if !ed25519.Verify(edPublic, []byte(query), raw) {
		t.Error("the signature in the fragment is not over the query")
	}
	for _, want := range []string{
		`"domain": "blue.exampleservice.domainconnect.org"`,
		`"hash": "domain=example.com&IP=192.0.2.10&TIMESTAMP=1700000000&MESSAGE=hi"`,
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the post data does not contain %q:\n%s", want, fragment)
		}
	}
}

// A request that cannot be built says why, rather than showing one that would
// not work.
func TestCurlRequestRefuses(t *testing.T) {
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed := func() Templates {
		return signedTestTemplates(t, edKey, "https://dash.example.com/domainconnect")
	}
	tests := map[string]struct {
		tmpls     func() Templates
		urlSyncUX string
		path      string
		want      string
	}{
		"no signing key": {
			tmpls:     templatesWithoutKey,
			urlSyncUX: "https://dash.example.com/domainconnect",
			path:      "/example/connect/apply?domain=example.com&template=flat",
			want:      "no signing key",
		},
		"no pub key host": {
			tmpls:     templatesWithoutPubKeyHost,
			urlSyncUX: "https://dash.example.com/domainconnect",
			path:      "/example/connect/apply?domain=example.com&template=flat",
			want:      "pubKeyHost",
		},
		"no synchronous flow": {
			tmpls:     signed,
			urlSyncUX: "",
			path:      "/example/connect/apply?domain=example.com&template=flat",
			want:      "does not offer a synchronous flow",
		},
		"unknown template": {
			tmpls:     signed,
			urlSyncUX: "https://dash.example.com/domainconnect",
			path:      "/example/connect/apply?domain=example.com&template=nosuch",
			want:      "not configured",
		},
		"no group chosen": {
			tmpls:     signed,
			urlSyncUX: "https://dash.example.com/domainconnect",
			path:      "/example/connect/apply?domain=example.com&template=grouped",
			want:      "group to apply has to be chosen first",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tmpls := test.tmpls()
			definitions = newDefinitionCache(settingsTTL)
			seedProviderWith(t, tmpls, test.urlSyncUX)
			fragment := postApply(t, tmpls, test.path, url.Values{"IP": {"192.0.2.10"}})

			if !strings.Contains(fragment, test.want) {
				t.Errorf("the fragment does not say %q:\n%s", test.want, fragment)
			}
			if strings.Contains(fragment, "curl -v") {
				t.Errorf("a command was shown although the request cannot be built:\n%s", fragment)
			}
		})
	}
}

// The values of a request come from the form of the variables, so the button
// has to be inside it.
func TestVariablesAreInAForm(t *testing.T) {
	fragment := get(t, testTemplates(), "/example/connect/template?domain=example.com&template=flat")
	for _, want := range []string{
		`<form id="applyForm"`,
		`hx-post="connect/apply?domain=example.com&template=flat"`,
		"create curl request",
		`hx-target="#applyRequest"`,
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
}

// The request lands in its own element below the values, so that the values can
// still be changed after the request has been made.
func TestApplyRequestHasItsOwnElement(t *testing.T) {
	fragment := get(t, testTemplates(), "/example/connect/template?domain=example.com&template=flat")
	if !strings.Contains(fragment, `<div id="applyRequest">`) {
		t.Errorf("the request has no element of its own:\n%s", fragment)
	}
}

// A quoted value in the command does not end the quoting, so that what is
// shown can be pasted into a shell as it stands.
func TestShellQuote(t *testing.T) {
	if got, want := shellQuote("https://example.com/?a=b c"), "'https://example.com/?a=b c'"; got != want {
		t.Errorf("shellQuote() = %q, want %q", got, want)
	}
	if got, want := shellQuote("it's"), `'it'\''s'`; got != want {
		t.Errorf("shellQuote() = %q, want %q", got, want)
	}
}
