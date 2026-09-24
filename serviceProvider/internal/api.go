package internal

import (
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type httpStatus struct {
	Status string `json:"status" example:"Success"`
}

type httpError struct {
	Err string `json:"error" example:"Error message"`
}

func ListenAPI(conf API, errgr *errgroup.Group) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	gin.DisableConsoleColor()

	r := gin.New()
	r.Use(gin.Recovery())

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, httpError{Err: "not found"})
	})

	prefix := r.Group(conf.Prefix)
	prefix.GET("/_heartbeat", heartbeat)

	listenAddr := net.JoinHostPort(conf.Host, strconv.Itoa(int(conf.Port)))
	apiServer := &http.Server{
		Addr:    listenAddr,
		Handler: r,
	}
	errgr.Go(func() error {
		err := apiServer.ListenAndServe()
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	})
	return apiServer
}

func heartbeat(c *gin.Context) {
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write([]byte("ok"))
}
