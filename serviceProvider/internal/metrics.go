package internal

import (
	"net"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
)

func SetupPrometheus(conf Metrics, errgr *errgroup.Group) *http.Server {
	listenAddr := net.JoinHostPort(conf.Host, strconv.Itoa(int(conf.Port)))
	metricsServer := &http.Server{
		Addr: listenAddr,
	}
	errgr.Go(func() error {
		http.Handle(conf.Path, promhttp.Handler())
		err := metricsServer.ListenAndServe()
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	})
	return metricsServer
}
