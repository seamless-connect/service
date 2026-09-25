package internal

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

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
