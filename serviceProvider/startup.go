package serviceProvider

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/seamlessdns/service/serviceProvider/internal"

	"github.com/rs/zerolog/log"
)

const (
	EXIT_SUCCESS = 0
	EXIT_FAILURE = 1
)

// shutdownTimeout is the maximum time a graful shutdown is allowed to wait.
// FIXME: make configurable?
const shutdownTimeout = 42 * time.Second

func Run() int {
	configPath := flag.String("config", "/etc/domain-connect/serviceProvider.toml", "path to configuration")
	flag.Parse()

	config, err := internal.ReadConfig(*configPath)
	if err != nil {
		log.Error().Err(err).Msg("cannot read configuration")
		return EXIT_FAILURE
	}
	internal.SetupLogging(config.Loglevel)

	// Telemetry init is required before api.
	telemetry, err := internal.SetupTelemetry(config.OTel, config.Metrics)
	if err != nil {
		log.Error().Err(err).Msg("cannot set up telemetry")
		return EXIT_FAILURE
	}
	apiServer := internal.ListenAPI(config.API)

	// Both API and telemetry report exit on the same channel.
	type result struct {
		name string
		err  error
	}
	results := make(chan result, 2)
	var listeners sync.WaitGroup
	serve := func(name string, server *http.Server) {
		listeners.Add(1)
		go func() {
			defer listeners.Done()
			log.Info().Str("server", name).Str("address", server.Addr).Msg("listening")
			err := server.ListenAndServe()
			// A closed listener is the expected outcome of a shutdown, not a
			// failure to report.
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			results <- result{name: name, err: err}
		}()
	}
	serve("api", apiServer)
	serve("metrics", telemetry.ScrapeServer())

	// Signals are buffered to avoid losing them.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	exit := EXIT_SUCCESS
	select {
	case sig := <-signals:
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
	case res := <-results:
		// Did listerner fail?
		if res.err != nil {
			log.Error().Err(res.err).Str("server", res.name).Msg("server stopped unexpectedly")
			exit = EXIT_FAILURE
		}
	}
	// Stop intercepting signals. Folloging signals are handled by the runtime
	// default and kills the process outright instead of being swallowed while
	// we are still draining.
	signal.Stop(signals)

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownServer(ctx, "api", apiServer)
	shutdownServer(ctx, "metrics", telemetry.ScrapeServer())
	telemetry.Shutdown(ctx)

	listeners.Wait()
	close(results)
	for res := range results {
		if res.err != nil {
			log.Error().Err(res.err).Str("server", res.name).Msg("server exited with error")
			exit = EXIT_FAILURE
		}
	}
	if exit == EXIT_SUCCESS {
		log.Info().Msg("shutdown complete")
	}
	return exit
}

// shutdownServer drains a listener.
func shutdownServer(ctx context.Context, name string, server *http.Server) {
	log.Info().Str("server", name).Msg("draining")
	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Str("server", name).Msg("server did not drain cleanly")
		if err := server.Close(); err != nil {
			log.Error().Err(err).Str("server", name).Msg("cannot close server")
		}
		return
	}
	log.Info().Str("server", name).Msg("drained")
}
