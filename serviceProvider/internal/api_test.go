package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The input page must be rendered even though there is no data to fill it
// with yet, and it must point to the endpoints that the htmx attributes name.
func TestRootPage(t *testing.T) {
	handler := ListenAPI(API{Host: "127.0.0.1", Prefix: "/example"}).Handler
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
		// The button posts the input to an endpoint of this service.
		`hx-post="connect"`,
		`hx-include="#domain"`,
		`src="_htmx.min.js"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
}

// The page loads htmx from this service, so that it works without a CDN.
func TestHtmxScript(t *testing.T) {
	handler := ListenAPI(API{Host: "127.0.0.1", Prefix: "/example"}).Handler
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
