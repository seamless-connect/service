package internal

import (
	"bytes"
	"context"
	_ "embed"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/rs/zerolog/log"
)

// htmxJS is htmx 2.0.4, https://htmx.org, Zero-Clause BSD licensed.  It is
// embedded in the binary so that the input page does not depend on a CDN.
//
//go:embed htmx.min.js
var htmxJS []byte

// rootPageHTML is the input page.  It is a template, rather than a constant,
// because processing the input later on has to report its errors on the page.
//
//go:embed rootPage.html
var rootPageHTML string

var rootPageTemplate = template.Must(template.New("rootPage").Parse(rootPageHTML))

// dnsProviderSearchPath is where htmx collects the DNS Provider search from
// the fragment that reports the domain back.  The path is relative to the
// input page, so that it works whatever prefix the service is configured with.
const dnsProviderSearchPath = "connect/dnsprovider"

type httpStatus struct {
	Status string `json:"status" example:"Success"`
}

type httpError struct {
	Err string `json:"error" example:"Error message"`
}

// ListenAPI builds the API server, instrumented with OpenTelemetry.  The
// templates are the ones the configuration defines, and they are what the
// input page lets the user choose from.
func ListenAPI(conf SPConf) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	gin.DisableConsoleColor()

	r := gin.New()
	r.Use(gin.Recovery(), otelgin.Middleware(serviceName))

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, httpError{Err: "not found"})
	})

	prefix := r.Group(conf.API.Prefix)
	prefix.GET("/_heartbeat", heartbeat)
	prefix.GET("/_htmx.min.js", htmxScript)
	prefix.GET("/", rootPage)
	prefix.POST("/connect", connect)
	prefix.GET("/connect/dnsprovider", dnsProviderSearchHandler(conf.Templates))
	prefix.GET("/connect/template", templateHandler(conf.Templates))
	prefix.POST("/connect/apply", curlRequestHandler(conf.Templates))

	return &http.Server{
		Addr:    net.JoinHostPort(conf.API.Host, strconv.Itoa(int(conf.API.Port))),
		Handler: r,
		// FIXME: make the timeouts configurable
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func heartbeat(c *gin.Context) {
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write([]byte("ok"))
}

func rootPage(c *gin.Context) {
	// The page is rendered into a buffer, so that a failure does not leave a
	// half written page in the response.
	var page bytes.Buffer
	if err := rootPageTemplate.Execute(&page, nil); err != nil {
		c.JSON(http.StatusInternalServerError, httpError{Err: "cannot render the input page"})
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", page.Bytes())
}

// connect receives the domain of the input page and reports it back on the
// page.  The DNS Provider search is handed over to htmx rather than made here,
// so that the domain shows up without waiting for the search to finish.
func connect(c *gin.Context) {
	domain, err := normalizeDomain(c.PostForm("domain"))
	if err != nil {
		log.Warn().Err(err).Msg("cannot use the domain of the request")
		// htmx replaces nothing on a failing status code, so the reason is
		// reported in the fragment rather than in the status.
		renderFragment(c, pagesTemplate.Lookup("connectPage"), connectPageData{Error: err.Error()})
		return
	}
	query := url.Values{"domain": []string{domain}}
	renderFragment(c, pagesTemplate.Lookup("connectPage"), connectPageData{
		Domain:    domain,
		SearchURL: dnsProviderSearchPath + "?" + query.Encode(),
	})
}

// dnsProviderSearch searches the _domainconnect.<domain> TXT record of a domain
// that connect has already reported back on the page.
func dnsProviderSearch(c *gin.Context, tmpls Templates) {
	domain, err := normalizeDomain(c.Query("domain"))
	if err != nil {
		log.Warn().Err(err).Msg("cannot use the domain of the request")
		renderFragment(c, pagesTemplate.Lookup("dnsProviderPage"), dnsProviderPageData{Error: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), dnsLookupTimeout)
	defer cancel()
	search, err := searchDNSProvider(ctx, domain)
	if err != nil {
		log.Warn().Err(err).Str("domain", domain).Msg("DNS Provider search failed")
		renderFragment(c, pagesTemplate.Lookup("dnsProviderPage"), dnsProviderPageData{Error: err.Error()})
		return
	}
	renderFragment(c, pagesTemplate.Lookup("dnsProviderPage"), dnsProviderPageData{
		Domain:   domain,
		Answer:   search.Answer,
		Template: templateSelection(domain, tmpls),
		// The definition is asked for after the search that named the DNS
		// Provider.  Not being able to read it does not fail the request:
		// the search itself worked, and the page says why the definition is
		// missing instead of pretending there is none.  It is asked with a
		// context of its own, so that a slow search does not leave it less
		// than the time it is allowed.
		Definition: readDefinitionSafely(c.Request.Context(), domain, search.Host),
	})
}

// readDefinitionSafely reads the definition of the DNS Provider of a domain,
// turning a failure into a message for the page rather than into an error that
// would leave the page without the rest of the result.
func readDefinitionSafely(ctx context.Context, domain, host string) definitionResult {
	definition, err := readDefinition(ctx, domain, host)
	if err != nil {
		log.Warn().Err(err).Str("domain", domain).Msg("cannot read DNS Provider definition")
		return definitionResult{Error: err.Error()}
	}
	log.Debug().Str("domain", domain).Str("provider", definition.Provider.ProviderID).
		Msg("DNS Provider definition read")
	return definitionResult{URL: definition.URL, JSON: definition.JSON}
}

// templateHandler binds the templates of the configuration to the handler that
// the template selection on the page is collected at.
func templateHandler(tmpls Templates) gin.HandlerFunc {
	return func(c *gin.Context) { templateSelect(c, tmpls) }
}

// dnsProviderSearchHandler binds the templates of the configuration to the
// search handler, which puts the choice of the template on the page.
func dnsProviderSearchHandler(tmpls Templates) gin.HandlerFunc {
	return func(c *gin.Context) { dnsProviderSearch(c, tmpls) }
}

// connectPageData fills the fragment that reports the received domain back.
type connectPageData struct {
	Domain string
	// SearchURL is where htmx collects the DNS Provider search from.
	SearchURL string
	// Error tells why the domain was not accepted, if it was not.
	Error string
}

// dnsProviderPageData fills the fragment with the result of the search.
type dnsProviderPageData struct {
	Domain string
	// Answer is the answer section of the DNS response.
	Answer string
	// Definition is what the DNS Provider states about itself, which comes
	// between the answer and the choice of a template.
	Definition definitionResult
	// Template is the choice of the template to apply, which follows the
	// search because only the templates of the DNS Provider that was found
	// can be applied.
	Template templateInput
	// Error tells why the search failed, if it did.
	Error string
}

// definitionResult is what the page shows of the definition of a DNS Provider.
type definitionResult struct {
	// URL is where the definition was read from.
	URL string
	// JSON is the response as the DNS Provider stated it.
	JSON string
	// Error tells why the definition could not be read, if it could not.
	Error string
}

// renderFragment sends a fragment that htmx swaps into the page.  It is
// rendered into a buffer, so that a failure does not leave a half written
// fragment in the response.
func renderFragment(c *gin.Context, page *template.Template, data any) {
	var fragment bytes.Buffer
	if err := page.Execute(&fragment, data); err != nil {
		log.Error().Err(err).Msg("cannot render the fragment")
		c.JSON(http.StatusInternalServerError, httpError{Err: "cannot render the response"})
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", fragment.Bytes())
}

// htmxScript serves the htmx library that the input page uses.
func htmxScript(c *gin.Context) {
	c.Data(http.StatusOK, "text/javascript; charset=utf-8", htmxJS)
}
