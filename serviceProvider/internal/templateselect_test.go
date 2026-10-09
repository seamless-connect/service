package internal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/seamlessdns/service/providerFormats"
)

// sha256Sum is the digest that the RSA and ECDSA signatures are made over.
func sha256Sum(t *testing.T, data string) []byte {
	t.Helper()
	digest := sha256.Sum256([]byte(data))
	return digest[:]
}

// testTemplates returns templates of the shape the Domain Connect templates
// have: one that groups its records, one that does not, and one without
// variables at all.
func testTemplates() Templates {
	return Templates{
		// Grouped, and the two groups hold different variables.
		"grouped": {
			Template: providerFormats.Template{
				ProviderID: "exampleservice.domainconnect.org",
				ServiceID:  "grouped",
				Version:    3,
				Records: providerFormats.Records{
					{Type: "A", Host: "@", PointsTo: "%IP%", GroupID: "web"},
					{Type: "TXT", Host: "@", Data: "%TOKEN%", GroupID: "verify"},
					// The same variable twice is asked for once.
					{Type: "CNAME", Host: "www", PointsTo: "%TOKEN%.example.net", GroupID: "verify"},
				},
			},
		},
		// No groupId, so every record is scanned for variables.
		"flat": {
			Template: providerFormats.Template{
				ProviderID: "exampleservice.domainconnect.org",
				ServiceID:  "flat",
				Version:    1,
				Records: providerFormats.Records{
					{Type: "A", Host: "@", PointsTo: "%IP%"},
					{Type: "TXT", Host: "@", Data: "shm:%TIMESTAMP%:%MESSAGE%"},
					{Type: "CNAME", Host: "www", PointsTo: "@"},
				},
			},
		},
		// Nothing to fill in.
		"novariables": {
			Template: providerFormats.Template{
				ProviderID: "other.domainconnect.org",
				ServiceID:  "novariables",
				Version:    1,
				Records:    providerFormats.Records{{Type: "CNAME", Host: "www", PointsTo: "@"}},
			},
		},
		// A template that was not read has no definition to describe it
		// with, so it must not end up in the choice.
		"unloaded": {Source: "file:///does/not/exist.json"},
	}
}

// supportingProvider puts a DNS Provider in the memory of the service the way
// the search of a domain leaves it: the definition of the provider is read for
// example.com and kept, and the provider answers for every template of the
// configuration with the version that template declares.  Nothing is warned
// about unless a test asks for it.
func supportingProvider(t *testing.T) {
	t.Helper()
	provider := startProvider(t, func(providerID, serviceID string) (int, string) {
		// The question is asked of the DNS Provider for a template, and
		// the template is told apart by the Service Provider that owns it.
		if providerID != "exampleservice.domainconnect.org" && providerID != "other.domainconnect.org" {
			return http.StatusNotFound, ""
		}
		tmpl, ok := findTemplateByService(testTemplates(), serviceID)
		if !ok {
			return http.StatusNotFound, ""
		}
		return http.StatusOK, fmt.Sprintf(`{"version": %d}`, tmpl.Version)
	})
	if _, err := readDefinition(context.Background(), "example.com", provider); err != nil {
		t.Fatalf("the definition of the provider of the test cannot be read: %v", err)
	}
}

// startProvider runs a DNS Provider of the test.  It answers the definition of
// a domain, with its own address as the API it states, and the question
// whether it supports a template with whatever the given function answers.
//
// The question names the provider and the service of the template, which is who
// owns the template rather than who the DNS Provider is, so the path is split
// on the identifiers of the template.
func startProvider(t *testing.T, support func(providerID, serviceID string) (status int, body string)) string {
	t.Helper()
	// The definition states the address of the API, and the support question
	// is asked of that API, so the address of the server is only known once
	// the server is running.
	var host string
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/example.com/settings" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"providerId": "cloudflare", "providerName": "Cloudflare",`+
				` "urlAPI": "https://%s"}`, host)
			return
		}
		// /v2/domainTemplates/providers/{providerId}/services/{serviceId}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v2/domainTemplates/providers/"), "/")
		if len(parts) != 3 || parts[1] != "services" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		status, body := support(parts[0], parts[2])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
	host = startTLSServer(t, handler)
	return host
}

// signedTestTemplates returns the templates of the test with a signing key in
// them, the way the configuration of a Service Provider that signs has them.
func signedTestTemplates(t *testing.T, key any, syncPubKeyDomain string) Templates {
	t.Helper()
	tmpls := testTemplates()
	for id, conf := range tmpls {
		if conf.Template.ServiceID == "" {
			continue
		}
		conf.SigningKey = key
		conf.SecretType = "Ed25519"
		conf.PubKeyHost = "blue"
		conf.Template.SyncPubKeyDomain = syncPubKeyDomain
		tmpls[id] = conf
	}
	return tmpls
}

// templatesWithoutKey returns templates that were configured with no signing
// key, so that nothing can be signed with them.
func templatesWithoutKey() Templates {
	tmpls := testTemplates()
	for id, conf := range tmpls {
		if conf.Template.ServiceID == "" {
			continue
		}
		conf.PubKeyHost = "blue"
		conf.SecretType = "Ed25519"
		conf.Template.SyncPubKeyDomain = "exampleservice.domainconnect.org"
		tmpls[id] = conf
	}
	return tmpls
}

// templatesWithoutPubKeyHost returns templates whose configuration names no
// host for the public key, so that the DNS Provider cannot find it.
func templatesWithoutPubKeyHost() Templates {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	tmpls := testTemplates()
	for id, conf := range tmpls {
		if conf.Template.ServiceID == "" {
			continue
		}
		conf.SigningKey = key
		conf.SecretType = "Ed25519"
		conf.Template.SyncPubKeyDomain = "exampleservice.domainconnect.org"
		tmpls[id] = conf
	}
	return tmpls
}

// seedProviderWith puts a DNS Provider in the memory of the service that offers
// the given synchronous flow and supports every template of the configuration.
func seedProviderWith(t *testing.T, tmpls Templates, urlSyncUX string) {
	t.Helper()
	host := startProvider(t, func(_, serviceID string) (int, string) {
		tmpl, ok := findTemplateByService(tmpls, serviceID)
		if !ok {
			return http.StatusNotFound, ""
		}
		return http.StatusOK, fmt.Sprintf(`{"version": %d}`, tmpl.Version)
	})
	provider := providerFormats.DNSProvider{
		ProviderID: "cloudflare",
		URLapi:     "https://" + host,
		URLSyncUX:  urlSyncUX,
	}
	definitions.put("example.com", dnsProviderDefinition{Provider: provider})
}

// postApply asks the endpoint that builds the apply request, the way the form
// of the variables does.
func postApply(t *testing.T, tmpls Templates, path string, values url.Values) string {
	t.Helper()
	handler := ListenAPI(SPConf{
		API:       API{Host: "127.0.0.1", Prefix: "/example"},
		Templates: tmpls,
	}).Handler
	form := values.Encode()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s: status = %d, want %d", path, rec.Code, http.StatusOK)
	}
	return html.UnescapeString(rec.Body.String())
}

// signatureFromFragment takes the signature out of what the page shows, which
// is what a DNS Provider would read out of the request.
func signatureFromFragment(t *testing.T, fragment string) string {
	t.Helper()
	const marker = "&sig="
	at := strings.Index(fragment, marker)
	if at < 0 {
		t.Fatalf("the fragment has no signature:\n%s", fragment)
	}
	rest := fragment[at+len(marker):]
	end := strings.Index(rest, "&")
	if end < 0 {
		t.Fatalf("the signature is not followed by the key:\n%s", fragment)
	}
	signature, err := url.QueryUnescape(rest[:end])
	if err != nil {
		t.Fatalf("the signature is not url encoded: %v", err)
	}
	return signature
}

func findTemplateByService(tmpls Templates, serviceID string) (providerFormats.Template, bool) {
	for _, conf := range tmpls {
		if conf.Template.ServiceID == serviceID {
			return conf.Template, true
		}
	}
	return providerFormats.Template{}, false
}

// get asks an endpoint of the API for a fragment, with a DNS Provider that
// supports every template of the configuration.
func get(t *testing.T, tmpls Templates, path string) string {
	t.Helper()
	definitions = newDefinitionCache(settingsTTL)
	supportingProvider(t)
	return getWithoutProvider(t, tmpls, path)
}

// getWithoutProvider asks an endpoint of the API for a fragment without
// putting a DNS Provider in the memory of the service.
func getWithoutProvider(t *testing.T, tmpls Templates, path string) string {
	t.Helper()
	handler := ListenAPI(SPConf{
		API:       API{Host: "127.0.0.1", Prefix: "/example"},
		Templates: tmpls,
	}).Handler
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want %d", path, rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Errorf("GET %s: Content-Type = %q, want %q", path, got, want)
	}
	// The fragment is HTML, so what the reader of the page sees is what the
	// escaping in the markup turns back into.
	return html.UnescapeString(rec.Body.String())
}

// The choice names what is being selected as "providerId.serviceId", and every
// template that has been read can be chosen.  The choice is part of the search
// result, because only the templates of the DNS Provider that was found for
// the domain can be applied.
func TestTemplateSelection(t *testing.T) {
	startTestDNSServer(t, "_domainconnect.example.com.",
		[]string{"api.cloudflare.com/client/v4/dns/domainconnect"})
	fragment := get(t, testTemplates(), "/example/connect/dnsprovider?domain=example.com")

	for _, want := range []string{
		`<select id="template"`,
		`hx-get="connect/template?domain=example.com"`,
		// The description of a choice tells what is being selected.
		">exampleservice.domainconnect.org.grouped<",
		">exampleservice.domainconnect.org.flat<",
		">other.domainconnect.org.novariables<",
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
	// A template that could not be read has no providerId to describe it
	// with, so it must not be offered.
	if strings.Contains(fragment, `value="unloaded"`) {
		t.Errorf("an unreadable template is offered on the page:\n%s", fragment)
	}
	// Nothing has been chosen, so no variable is asked for yet.
	if strings.Contains(fragment, "IP:") {
		t.Errorf("a variable is asked for before a template is chosen:\n%s", fragment)
	}
}

// A template with groupId is asked which group is applied first, and only the
// variables of that group are collected.
func TestTemplateSelectGrouped(t *testing.T) {
	fragment := get(t, testTemplates(), "/example/connect/template?domain=example.com&template=grouped")

	for _, want := range []string{
		// Both groups are offered, in the order the records use them.
		`<select id="group"`,
		`<option value="web">web</option>`,
		`<option value="verify">verify</option>`,
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
	// No group is chosen, so no variable is asked for yet.
	for _, unwanted := range []string{"IP:", "TOKEN:"} {
		if strings.Contains(fragment, unwanted) {
			t.Errorf("%q is asked for before a group is chosen:\n%s", unwanted, fragment)
		}
	}
}

// Once a group is chosen, only the variables of the records of it are asked
// for.
func TestTemplateSelectGroup(t *testing.T) {
	fragment := get(t, testTemplates(),
		"/example/connect/template?domain=example.com&template=grouped&group=verify")

	for _, want := range []string{
		"TOKEN:",
		`<input id="var-TOKEN"`,
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
	// IP belongs to the other group.
	if strings.Contains(fragment, "IP:") {
		t.Errorf("a variable of another group is asked for:\n%s", fragment)
	}
	// The group is still there, so that it can be changed.
	if !strings.Contains(fragment, `<select id="group"`) {
		t.Errorf("the group cannot be changed:\n%s", fragment)
	}
}

// The other group asks for its own variable.
func TestTemplateSelectOtherGroup(t *testing.T) {
	fragment := get(t, testTemplates(),
		"/example/connect/template?domain=example.com&template=grouped&group=web")

	if !strings.Contains(fragment, "IP:") {
		t.Errorf("the fragment does not contain %q:\n%s", "IP:", fragment)
	}
	if strings.Contains(fragment, "TOKEN:") {
		t.Errorf("a variable of another group is asked for:\n%s", fragment)
	}
}

// A template without groupId collects the variables of every record.
func TestTemplateSelectFlat(t *testing.T) {
	fragment := get(t, testTemplates(), "/example/connect/template?domain=example.com&template=flat")

	// No group is asked for, because the records are not grouped.
	if strings.Contains(fragment, `<select id="group"`) {
		t.Errorf("a group is asked for by a template that has none:\n%s", fragment)
	}
	// Every variable of every record, including the two of one record.
	for _, want := range []string{"IP:", "TIMESTAMP:", "MESSAGE:", `<input id="var-IP"`} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
}

// A variable that is used twice is asked for once.
func TestTemplateSelectRepeatedVariable(t *testing.T) {
	fragment := get(t, testTemplates(),
		"/example/connect/template?domain=example.com&template=grouped&group=verify")
	if got := strings.Count(fragment, `id="var-TOKEN"`); got != 1 {
		t.Errorf("TOKEN is asked for %d times, want 1:\n%s", got, fragment)
	}
}

// A template without variables says so rather than showing nothing.
func TestTemplateSelectNoVariables(t *testing.T) {
	fragment := get(t, testTemplates(), "/example/connect/template?domain=example.com&template=novariables")
	if !strings.Contains(fragment, "no variables to fill in") {
		t.Errorf("the fragment does not say that there is nothing to fill in:\n%s", fragment)
	}
}

// A template or a group that is not configured is reported on the page instead
// of silently collecting the variables of another one.
func TestTemplateSelectUnknown(t *testing.T) {
	fragment := get(t, testTemplates(), "/example/connect/template?domain=example.com&template=nosuch")
	if !strings.Contains(fragment, `class="error"`) || !strings.Contains(fragment, "not configured") {
		t.Errorf("the fragment does not report the unknown template:\n%s", fragment)
	}
	if strings.Contains(fragment, "IP:") {
		t.Errorf("variables are asked for an unknown template:\n%s", fragment)
	}

	fragment = get(t, testTemplates(),
		"/example/connect/template?domain=example.com&template=grouped&group=nosuch")
	if !strings.Contains(fragment, `class="error"`) || !strings.Contains(fragment, "not part of template") {
		t.Errorf("the fragment does not report the unknown group:\n%s", fragment)
	}
}

// Changing the choice replaces what the page collects, because the selector is
// asked again rather than kept.
func TestTemplateSelectionNotPermanent(t *testing.T) {
	// The group selector of a template that has groups is what the template
	// selector replaces once a template is chosen.
	fragment := get(t, testTemplates(),
		"/example/connect/template?domain=example.com&template=grouped&group=web")

	for _, want := range []string{
		`hx-trigger="change"`,
		`hx-target="#templateVariables"`,
		`hx-swap="outerHTML"`,
		// The domain travels with the choice, because the DNS Provider of
		// the domain is what decides what a template can be applied with.
		`hx-get="connect/template?domain=example.com&template=grouped"`,
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
	// The variables live in their own element, which is what the selectors
	// replace, so that the template selector itself is left in place.
	if !strings.Contains(fragment, `<div id="templateVariables">`) {
		t.Errorf("the variables have no element of their own:\n%s", fragment)
	}
}

// The question names the provider and the service of the template, which is the
// Service Provider that owns the template, not the DNS Provider that is asked.
func TestTemplateSelectAsksForTheTemplate(t *testing.T) {
	definitions = newDefinitionCache(settingsTTL)
	var askedFor string
	host := startProvider(t, func(providerID, serviceID string) (int, string) {
		askedFor = providerID + "." + serviceID
		return http.StatusOK, `{"version": 1}`
	})
	if _, err := readDefinition(context.Background(), "example.com", host); err != nil {
		t.Fatalf("the definition of the provider of the test cannot be read: %v", err)
	}

	getWithoutProvider(t, testTemplates(), "/example/connect/template?domain=example.com&template=flat")

	// The DNS Provider of the test states providerId "cloudflare", and the
	// template of the configuration states "exampleservice.domainconnect.org".
	// The latter is the one that belongs in the path.
	if want := "exampleservice.domainconnect.org.flat"; askedFor != want {
		t.Errorf("the DNS Provider was asked about %q, want %q", askedFor, want)
	}
}

// A DNS Provider that does not support the template stops the page before any
// group or variable is asked for.
func TestTemplateSelectNotSupported(t *testing.T) {
	definitions = newDefinitionCache(settingsTTL)
	host := startProvider(t, func(string, string) (int, string) { return http.StatusNotFound, "" })
	if _, err := readDefinition(context.Background(), "example.com", host); err != nil {
		t.Fatalf("the definition of the provider of the test cannot be read: %v", err)
	}

	fragment := getWithoutProvider(t, testTemplates(),
		"/example/connect/template?domain=example.com&template=flat")

	if !strings.Contains(fragment, "does not support template") {
		t.Errorf("the fragment does not say that the template is not supported:\n%s", fragment)
	}
	for _, unwanted := range []string{"IP:", "TIMESTAMP:", "MESSAGE:"} {
		if strings.Contains(fragment, unwanted) {
			t.Errorf("%q is asked for a template that is not supported:\n%s", unwanted, fragment)
		}
	}
}

// A DNS Provider that cannot be asked is reported the same way, so that the
// page never leaves the user guessing.
func TestTemplateSelectSupportFails(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "server error", status: http.StatusInternalServerError, body: "broken", want: "answered 500 Internal Server Error: broken"},
		{name: "not json", status: http.StatusOK, body: "<html>hi</html>", want: "not the version of a template"},
		{name: "wrong version", status: http.StatusOK, body: `{"version": "four"}`, want: "not the version of a template"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definitions = newDefinitionCache(settingsTTL)
			host := startProvider(t, func(string, string) (int, string) { return test.status, test.body })
			if _, err := readDefinition(context.Background(), "example.com", host); err != nil {
				t.Fatalf("the definition of the provider of the test cannot be read: %v", err)
			}

			fragment := getWithoutProvider(t, testTemplates(),
				"/example/connect/template?domain=example.com&template=flat")
			if !strings.Contains(fragment, test.want) {
				t.Errorf("the fragment does not contain %q:\n%s", test.want, fragment)
			}
			if strings.Contains(fragment, "IP:") {
				t.Errorf("a variable is asked for although the template could not be checked:\n%s", fragment)
			}
		})
	}
}

// A version the DNS Provider does not have is worth saying out loud, but it
// does not stop the template from being used.
func TestTemplateSelectVersionMismatch(t *testing.T) {
	definitions = newDefinitionCache(settingsTTL)
	host := startProvider(t, func(string, string) (int, string) {
		// The template of the configuration is in version 1.
		return http.StatusOK, `{"version": 7}`
	})
	if _, err := readDefinition(context.Background(), "example.com", host); err != nil {
		t.Fatalf("the definition of the provider of the test cannot be read: %v", err)
	}

	fragment := getWithoutProvider(t, testTemplates(),
		"/example/connect/template?domain=example.com&template=flat")

	if !strings.Contains(fragment, "version 7") || !strings.Contains(fragment, "version 1") {
		t.Errorf("the fragment does not report the versions:\n%s", fragment)
	}
	// The variables are still asked for, because the template can be used.
	if !strings.Contains(fragment, "IP:") {
		t.Errorf("a differing version stopped the template from being used:\n%s", fragment)
	}
}

// Without the settings of the DNS Provider there is nothing to ask about the
// template with, and that is said rather than guessed at.
func TestTemplateSelectWithoutProvider(t *testing.T) {
	definitions = newDefinitionCache(settingsTTL)
	fragment := getWithoutProvider(t, testTemplates(),
		"/example/connect/template?domain=example.com&template=flat")

	if !strings.Contains(fragment, "is not known") {
		t.Errorf("the fragment does not say that the DNS Provider is not known:\n%s", fragment)
	}
	if strings.Contains(fragment, "IP:") {
		t.Errorf("a variable is asked for although nothing could be checked:\n%s", fragment)
	}
}

// The groups of a template come in the order the records use them.
func TestTemplateGroups(t *testing.T) {
	tmpl := testTemplates()["grouped"].Template
	want := []string{"web", "verify"}
	if got := templateGroups(tmpl); !slices.Equal(got, want) {
		t.Errorf("templateGroups() = %v, want %v", got, want)
	}
	if got := templateGroups(testTemplates()["flat"].Template); got != nil {
		t.Errorf("templateGroups() of a template without groups = %v, want none", got)
	}
}

// The variables of a record are taken from every field that is free text, and
// the percent signs are part of the notation rather than part of the name.
func TestRecordVariables(t *testing.T) {
	record := providerFormats.Record{
		Type: "TXT",
		Host: "%SUBDOMAIN%",
		Data: "shm:%TIMESTAMP%:hello world",
		// The type is one of a fixed set of values, so a percent sign in
		// it is not a variable.
		TxtCMP:   "%PREFIX%",
		Priority: providerFormats.SINT("%WEIGHT%"),
	}
	want := []string{"SUBDOMAIN", "TIMESTAMP", "PREFIX", "WEIGHT"}
	if got := recordVariables(record); !slices.Equal(got, want) {
		t.Errorf("recordVariables() = %v, want %v", got, want)
	}
}
