package internal

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The input page must be rendered even though there is no data to fill it
// with yet, and it must point to the endpoints that the htmx attributes name.
func TestRootPage(t *testing.T) {
	handler := ListenAPI(SPConf{API: API{Host: "127.0.0.1", Prefix: "/example"}}).Handler
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/example/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	page := rec.Body.String()
	for _, want := range []string{
		// A white page with the input line and the button in the middle.
		"background-color: #ffffff",
		`size="64"`,
		"Domain Connect",
		`src="_htmx.min.js"`,
		// The input and the button are in a form, so that pressing Enter
		// posts the domain the way clicking the button does.  The form is
		// what htmx collects, which is why it carries the endpoint.
		`<form action="connect" method="post"`,
		`hx-post="connect"`,
		// A submit button, because a form with a single text field
		// submits on Enter only when it has one.
		`<button type="submit">`,
		// The domain goes out under the name the endpoint reads.
		`name="domain"`,
		// The received domain and the search result land in this element,
		// and the answer of a DNS response is shown as it comes, tabs
		// included.
		`<div id="result">`,
		"pre {",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
}

// The input has to sit inside the form, otherwise Enter in it submits nothing
// and the domain never reaches the endpoint.
func TestRootPageInputIsInTheForm(t *testing.T) {
	handler := ListenAPI(SPConf{API: API{Host: "127.0.0.1", Prefix: "/example"}}).Handler
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/example/", nil))

	page := rec.Body.String()
	form := strings.Index(page, "<form")
	input := strings.Index(page, `id="domain"`)
	closing := strings.Index(page, "</form>")
	if form < 0 || input < 0 || closing < 0 {
		t.Fatalf("the page has no form around the input:\n%s", page)
	}
	if input < form || input > closing {
		t.Errorf("the input at %d is not between the form at %d and its end at %d",
			input, form, closing)
	}
	// The result of a request is not part of the form, so that the selectors
	// the page grows are not submitted along with the domain.
	if result := strings.Index(page, `<div id="result">`); result < closing {
		t.Errorf("the result element at %d is inside the form, which ends at %d",
			result, closing)
	}
}

// The button must reach an endpoint that reports the received domain back on
// the page, and hand the DNS Provider search over to htmx afterwards.
func TestConnect(t *testing.T) {
	handler := ListenAPI(SPConf{API: API{Host: "127.0.0.1", Prefix: "/example"}}).Handler
	request := httptest.NewRequest(http.MethodPost, "/example/connect",
		strings.NewReader("domain=Example.com."))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	fragment := rec.Body.String()
	for _, want := range []string{
		// The domain is reported back in the form it is searched with.
		"example.com received",
		// The search is collected by htmx once the domain is shown, so
		// that the page does not have to wait for the DNS server.
		`hx-get="connect/dnsprovider?domain=example.com"`,
		`hx-trigger="load"`,
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
	// A domain that is not a DNS name cannot be searched for, and nothing is
	// handed over to htmx.
	for _, bad := range []string{"", "example", "exa mple.com", "example.com/../org"} {
		request := httptest.NewRequest(http.MethodPost, "/example/connect",
			strings.NewReader("domain="+url.QueryEscape(bad)))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, request)

		if rec.Code != http.StatusOK {
			t.Errorf("status for %q = %d, want %d", bad, rec.Code, http.StatusOK)
		}
		fragment := rec.Body.String()
		if strings.Contains(fragment, "hx-get") {
			t.Errorf("a search was handed over for %q:\n%s", bad, fragment)
		}
		if !strings.Contains(fragment, "is not a domain name") &&
			!strings.Contains(fragment, "is not a valid domain name") &&
			!strings.Contains(fragment, "no domain was given") {
			t.Errorf("the reason %q was rejected is not on the page:\n%s", bad, fragment)
		}
	}
}

// The answer of the DNS Provider search is shown the way dig prints it, and the
// template selection follows it.
func TestDNSProviderSearch(t *testing.T) {
	startTestDNSServer(t, "_domainconnect.example.com.",
		[]string{"api.cloudflare.com/client/v4/dns/domainconnect"})

	handler := ListenAPI(SPConf{API: API{Host: "127.0.0.1", Prefix: "/example"},
		Templates: testTemplates()}).Handler
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/example/connect/dnsprovider?domain=example.com", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	// The fragment is HTML, so what the reader of the page sees is what the
	// escaping in the markup turns back into.
	fragment := html.UnescapeString(rec.Body.String())
	for _, want := range []string{
		";; ANSWER SECTION:",
		"_domainconnect.example.com.\t3600\tIN\tTXT\t\"api.cloudflare.com/client/v4/dns/domainconnect\"",
		// The template choice comes after the search result, because only
		// the templates of the DNS Provider that was found can be applied.
		`<select id="template"`,
		">exampleservice.domainconnect.org.grouped<",
		">exampleservice.domainconnect.org.flat<",
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("the fragment does not contain %q:\n%s", want, fragment)
		}
	}
}

// A domain without the record has no DNS Provider, so the reason is reported on
// the page instead of an empty answer.
func TestDNSProviderSearchNoRecord(t *testing.T) {
	// An empty list of texts is a name that exists without a TXT record.
	startTestDNSServer(t, "_domainconnect.example.com.", []string{})

	handler := ListenAPI(SPConf{API: API{Host: "127.0.0.1", Prefix: "/example"}}).Handler
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/example/connect/dnsprovider?domain=example.com", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "_domainconnect.example.com. has no TXT record"; !strings.Contains(got, want) {
		t.Errorf("the fragment does not contain %q:\n%s", want, got)
	}
}

// The page loads htmx from this service, so that it works without a CDN.
func TestHtmxScript(t *testing.T) {
	handler := ListenAPI(SPConf{API: API{Host: "127.0.0.1", Prefix: "/example"}}).Handler
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/example/_htmx.min.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/javascript; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if !strings.Contains(rec.Body.String(), "htmx") {
		t.Error("the script served is not htmx")
	}
}
