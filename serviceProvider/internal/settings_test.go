package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// validDefinition is a definition in the shape the specification has.
const validDefinition = `{
  "providerId": "cloudflare",
  "providerName": "Cloudflare",
  "providerDisplayName": "Cloudflare",
  "urlAPI": "https://api.cloudflare.com/client/v4/dns/domainconnect",
  "urlSyncUX": "https://dash.cloudflare.com/?domain=%s",
  "nameServers": ["ns1.cloudflare.com", "ns2.cloudflare.com"]
}`

// The address of the definition is the host of the TXT record followed by the
// path of the specification.
func TestSettingsURL(t *testing.T) {
	accepted := map[string]string{
		"api.cloudflare.com/client/v4/dns/domainconnect": "https://api.cloudflare.com/client/v4/dns/domainconnect/v2/example.com/settings",
		"api.cloudflare.com":                             "https://api.cloudflare.com/v2/example.com/settings",
		"example.com:8443/api/v4":                        "https://example.com:8443/api/v4/v2/example.com/settings",
	}
	for host, want := range accepted {
		got, err := settingsURL(host, "example.com")
		if err != nil {
			t.Errorf("settingsURL(%q) failed: %v", host, err)
			continue
		}
		if got != want {
			t.Errorf("settingsURL(%q) = %q, want %q", host, got, want)
		}
	}

	// The host comes out of DNS, so anything that is not a plain host is
	// refused rather than asked.
	rejected := []string{
		"",
		"   ",
		"api.example.com@evil.example.com",
		"api.example.com?a=b",
		"api.example.com#fragment",
		"http://api.example.com",
		"//api.example.com",
		"api.example.com:port",
		"exa mple.com",
		"localhost",
	}
	for _, host := range rejected {
		if got, err := settingsURL(host, "example.com"); err == nil {
			t.Errorf("settingsURL(%q) = %q, want an error", host, got)
		}
	}
}

// A definition that reads is both shown and held in memory.
func TestReadDefinition(t *testing.T) {
	host := startDefinitionServer(t, http.StatusOK, validDefinition, nil)

	definitions = newDefinitionCache(settingsTTL)
	definition, err := readDefinition(context.Background(), "example.com", host)
	if err != nil {
		t.Fatalf("readDefinition() failed: %v", err)
	}
	if definition.Provider.ProviderID != "cloudflare" {
		t.Errorf("providerId = %q, want %q", definition.Provider.ProviderID, "cloudflare")
	}
	if definition.Provider.URLapi != "https://api.cloudflare.com/client/v4/dns/domainconnect" {
		t.Errorf("urlAPI = %q", definition.Provider.URLapi)
	}
	if len(definition.Provider.Nameservers) != 2 {
		t.Errorf("nameServers = %v, want two", definition.Provider.Nameservers)
	}
	// The response is shown as it came, only indented.
	if !strings.Contains(definition.JSON, "\n  \"providerId\": \"cloudflare\"") {
		t.Errorf("the json is not the response as it came:\n%s", definition.JSON)
	}
	// The definition is kept in memory for the apply of a template.
	cached, ok := definitions.get("example.com")
	if !ok {
		t.Fatal("the definition was not kept in memory")
	}
	if cached.Provider.ProviderID != "cloudflare" {
		t.Errorf("the cached definition has providerId %q", cached.Provider.ProviderID)
	}
}

// A domain whose definition is already in memory is not asked about again.
func TestReadDefinitionFromMemory(t *testing.T) {
	var asked int
	host := startDefinitionServer(t, http.StatusOK, validDefinition, &asked)

	definitions = newDefinitionCache(settingsTTL)
	ctx := context.Background()
	if _, err := readDefinition(ctx, "example.com", host); err != nil {
		t.Fatalf("readDefinition() failed: %v", err)
	}
	if _, err := readDefinition(ctx, "example.com", host); err != nil {
		t.Fatalf("readDefinition() failed: %v", err)
	}
	if asked != 1 {
		t.Errorf("the DNS Provider was asked %d times, want 1", asked)
	}
}

// An expired definition is read again, because a DNS Provider may have changed
// what it supports.
func TestReadDefinitionExpired(t *testing.T) {
	var asked int
	host := startDefinitionServer(t, http.StatusOK, validDefinition, &asked)

	definitions = newDefinitionCache(time.Hour)
	ctx := context.Background()
	if _, err := readDefinition(ctx, "example.com", host); err != nil {
		t.Fatalf("readDefinition() failed: %v", err)
	}
	// Expire what was kept, as the time to live would.
	definitions.mu.Lock()
	definition := definitions.entries["example.com"]
	definition.Expires = time.Now().Add(-time.Second)
	definitions.entries["example.com"] = definition
	definitions.mu.Unlock()

	if _, err := readDefinition(ctx, "example.com", host); err != nil {
		t.Fatalf("readDefinition() failed: %v", err)
	}
	if asked != 2 {
		t.Errorf("the DNS Provider was asked %d times, want 2", asked)
	}
}

// Every way a definition can fail to read is told apart, because the page
// reports which one it was.
func TestReadDefinitionErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "not found",
			status: http.StatusNotFound,
			body:   `no such domain here`,
			want:   "answered 404 Not Found: no such domain here",
		},
		{
			name:   "server error",
			status: http.StatusInternalServerError,
			body:   `something broke`,
			want:   "answered 500 Internal Server Error: something broke",
		},
		{
			name:   "not json",
			status: http.StatusOK,
			body:   `<html><body>hello</body></html>`,
			want:   "did not answer json",
		},
		{
			// A field of a definition is a number, and the response has
			// a word in it.
			name:   "wrong type",
			status: http.StatusOK,
			body:   `{"providerId": ["cloudflare"]}`,
			want:   "answered json that is not a DNS Provider definition",
		},
		{
			name:   "reserved field",
			status: http.StatusOK,
			body:   `{"providerId": "cloudflare", "urlAPI": "https://example.com", "width": 800}`,
			want:   `answers the field "width", which the specification has reserved`,
		},
		{
			name:   "empty body",
			status: http.StatusOK,
			body:   ``,
			want:   "did not answer json",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := startDefinitionServer(t, test.status, test.body, nil)

			definitions = newDefinitionCache(settingsTTL)
			_, err := readDefinition(context.Background(), "example.com", host)
			if err == nil {
				t.Fatalf("readDefinition() succeeded, want the reason it failed")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("readDefinition() failed with %q, want it to say %q", err, test.want)
			}
			// A definition that could not be read is not held in memory,
			// so that the next request tries again.
			if _, ok := definitions.get("example.com"); ok {
				t.Error("a definition that failed to read was kept in memory")
			}
		})
	}
}

// A DNS Provider that answers with a page of its own must not have all of it
// put into an error message.
func TestBodySnippet(t *testing.T) {
	long := strings.Repeat("x", maxSnippetLength*2)
	got := bodySnippet([]byte(long))
	if len(got) > maxSnippetLength+3 {
		t.Errorf("the snippet is %d characters long, want at most %d", len(got), maxSnippetLength+3)
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("a cut snippet is not marked as cut")
	}
	if got := bodySnippet([]byte("  \n ")); got != "" {
		t.Errorf("an empty body reads as %q", got)
	}
	// The cut can land inside a character, and the message is still text.
	if got := bodySnippet([]byte("\xff\xfe rubbish")); strings.ContainsRune(got, 0xFFFD) {
		t.Errorf("the snippet %q kept an invalid character", got)
	}
}

// The definition must be shown between the DNS answer and the choice of a
// template, and a definition that cannot be read must say why.
func TestDNSProviderSearchDefinition(t *testing.T) {
	// A DNS Provider that answers the search with the host of the definition
	// server of the test.
	host := startDefinitionServer(t, http.StatusOK, validDefinition, nil)
	startTestDNSServer(t, "_domainconnect.example.com.", []string{host})

	definitions = newDefinitionCache(settingsTTL)
	fragment := getWithoutProvider(t, testTemplates(), "/example/connect/dnsprovider?domain=example.com")

	answer := strings.Index(fragment, ";; ANSWER SECTION:")
	definition := strings.Index(fragment, `<div id="dnsproviderDefinition">`)
	selector := strings.Index(fragment, `<select id="template"`)
	if answer < 0 || definition < 0 || selector < 0 {
		t.Fatalf("the fragment is missing a part:\n%s", fragment)
	}
	if answer > definition || definition > selector {
		t.Errorf("the order is answer at %d, definition at %d, selector at %d, want that order",
			answer, definition, selector)
	}
	for _, want := range []string{`"providerId": "cloudflare"`, "/v2/example.com/settings"} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
}

// A DNS Provider that does not answer must not take the rest of the result
// with it, because the search itself worked.
func TestDNSProviderSearchDefinitionFailed(t *testing.T) {
	host := startDefinitionServer(t, http.StatusNotFound, "no such domain", nil)
	startTestDNSServer(t, "_domainconnect.example.com.", []string{host})

	definitions = newDefinitionCache(settingsTTL)
	fragment := getWithoutProvider(t, testTemplates(), "/example/connect/dnsprovider?domain=example.com")

	// The answer is still shown, and the choice of a template still opens.
	for _, want := range []string{
		";; ANSWER SECTION:",
		"answered 404 Not Found",
		`<select id="template"`,
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
}

// The definition has to be held in memory for the apply that comes later, not
// only rendered once.
func TestDNSProviderSearchKeepsDefinition(t *testing.T) {
	host := startDefinitionServer(t, http.StatusOK, validDefinition, nil)
	startTestDNSServer(t, "_domainconnect.example.com.", []string{host})

	definitions = newDefinitionCache(settingsTTL)
	getWithoutProvider(t, testTemplates(), "/example/connect/dnsprovider?domain=example.com")

	definition, ok := definitions.get("example.com")
	if !ok {
		t.Fatal("the definition was not kept in memory for the apply")
	}
	// The apply works on the struct, not on the json that was rendered.
	if definition.Provider.ProviderID != "cloudflare" || definition.Provider.URLapi == "" {
		t.Errorf("the definition in memory is %+v", definition.Provider)
	}
}

// startTLSServer runs a server that speaks tls, and makes this service ask it
// in place of the DNS Providers for as long as the test runs.  The host that a
// TXT record of the test would name is returned.
//
// The settings are always asked over https, so the server speaks it and the
// ask is made with the client of the server, which trusts it.
func startTLSServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(func() { server.Close() })

	// The TXT record of a domain names a host, never a scheme, so what the
	// test puts in DNS is the host of the server without its scheme.
	host := strings.TrimPrefix(server.URL, "https://")

	oldClient := settingsClient
	settingsClient = server.Client()
	t.Cleanup(func() { settingsClient = oldClient })
	return host
}

// startDefinitionServer runs a server that answers with the given status and
// body at the path of the definition of a domain.
//
// Every ask is counted into asked, which may be nil when the test does not care
// how many there were.
func startDefinitionServer(t *testing.T, status int, body string, asked *int) string {
	t.Helper()
	return startTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/example.com/settings" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if asked != nil {
			*asked++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

// The definition is read into the struct that the formats of this service use,
// so that the fields are the ones the apply reads.
func TestParseDefinitionStruct(t *testing.T) {
	provider, err := parseDefinition("https://example.com", []byte(validDefinition))
	if err != nil {
		t.Fatalf("parseDefinition() failed: %v", err)
	}
	if provider.ProviderName != "Cloudflare" || provider.ProviderDN != "Cloudflare" {
		t.Errorf("the provider names are %+v", provider)
	}
	if provider.URLSyncUX != "https://dash.cloudflare.com/?domain=%s" {
		t.Errorf("urlSyncUX = %q", provider.URLSyncUX)
	}
	// Whatever comes out has to be json again, which is what is shown.
	if _, err := json.Marshal(provider); err != nil {
		t.Errorf("the definition cannot be written back as json: %v", err)
	}
}
