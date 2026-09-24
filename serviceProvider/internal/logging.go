package internal

import (
	"path/filepath"
	"strconv"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func SetupLogging(level string) {
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return filepath.Base(file) + ":" + strconv.Itoa(line)
	}
	log.Logger = log.With().Caller().Logger()
	parsedLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		log.Fatal().Str("loglevel", level).Msg("invalid loglevel")
	}
	zerolog.SetGlobalLevel(parsedLevel)
}
