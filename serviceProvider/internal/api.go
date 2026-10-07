package internal

import (
	"bytes"
	_ "embed"
	"html/template"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
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

type httpStatus struct {
	Status string `json:"status" example:"Success"`
}

type httpError struct {
	Err string `json:"error" example:"Error message"`
}

// ListenAPI builds the API server, instrumented with OpenTelemetry.
func ListenAPI(conf API) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	gin.DisableConsoleColor()

	r := gin.New()
	r.Use(gin.Recovery(), otelgin.Middleware(serviceName))

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, httpError{Err: "not found"})
	})

	prefix := r.Group(conf.Prefix)
	prefix.GET("/_heartbeat", heartbeat)
	prefix.GET("/_htmx.min.js", htmxScript)
	prefix.GET("/", rootPage)

	return &http.Server{
		Addr:    net.JoinHostPort(conf.Host, strconv.Itoa(int(conf.Port))),
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

// htmxScript serves the htmx library that the input page uses.
func htmxScript(c *gin.Context) {
	c.Data(http.StatusOK, "text/javascript; charset=utf-8", htmxJS)
}
