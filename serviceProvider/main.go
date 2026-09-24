package main

import (
	"context"
	"flag"
	"os"

	"github.com/seamlessdns/service/serviceProvider/internal"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

func main() {
	configPath := flag.String("config", "/etc/domain-connect/serviceProvider.toml", "path to configuration")
	flag.Parse()

	config, err := internal.ReadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot read configuration")
		os.Exit(1)
	}
	internal.SetupLogging(config.Loglevel)
	log.Debug().Msg("init done")

	ctx := context.Background()
	errgr, ctx := errgroup.WithContext(ctx)
	metricsServer := internal.SetupPrometheus(config.Metrics, errgr)
	apiServer := internal.ListenAPI(config.API, errgr)
	log.Debug().Msg("listen addresses are setup")

	// Exit handling
	errgr.Go(func() error {
		<-ctx.Done()
		unimportant := apiServer.Shutdown(context.Background())
		if unimportant != nil {
			log.Error().Err(unimportant).Msg("api server shutdown failed")
		}
		unimportant = metricsServer.Shutdown(context.Background())
		if unimportant != nil {
			log.Error().Err(unimportant).Msg("metrics server shutdown failed")
		}
		return err
	})

	if err := errgr.Wait(); err != nil {
		log.Fatal().Err(err).Msg("waitgroup reported error")
	}
}
