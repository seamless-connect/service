package internal

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func SetupLogging(level string) {
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return filepath.Base(file) + ":" + strconv.Itoa(line)
	}
	parsedLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		log.Fatal().Str("loglevel", level).Msg("invalid loglevel")
	}
	zerolog.SetGlobalLevel(parsedLevel)

	// The hook every log record into OpenTelemetry.  The level is
	// applied before the hook runs, so the log level also decides how
	// much ends up in OpenTelemetry.
	log.Logger = zerolog.New(os.Stderr).
		Hook(newOtelHook(serviceName)).
		With().Caller().Logger()
}
