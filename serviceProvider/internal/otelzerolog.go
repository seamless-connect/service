package internal

import (
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
)

// otelHook mirrors every zerolog record into OpenTelemetry.
type otelHook struct {
	logger otellog.Logger
}

// newOtelHook builds the hook around the global LoggerProvider.
func newOtelHook(name string) zerolog.Hook {
	return &otelHook{logger: global.GetLoggerProvider().Logger(name)}
}

// Run converts one zerolog record and hands it to OpenTelemetry.
func (h *otelHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	var record otellog.Record
	record.SetSeverity(otelSeverity(level))
	record.SetSeverityText(level.String())
	record.SetBody(attribute.StringValue(msg))
	// The context is what ties the record to the trace that was active
	// when it was written.  This is the most valuable piece of data here.
	h.logger.Emit(e.GetCtx(), record)
}

// otelSeverity maps a zerolog level onto an OTel severity
func otelSeverity(level zerolog.Level) otellog.Severity {
	switch level {
	case zerolog.TraceLevel:
		return otellog.SeverityTrace1
	case zerolog.DebugLevel:
		return otellog.SeverityDebug1
	case zerolog.InfoLevel:
		return otellog.SeverityInfo1
	case zerolog.WarnLevel:
		return otellog.SeverityWarn1
	case zerolog.ErrorLevel:
		return otellog.SeverityError1
	case zerolog.FatalLevel:
		return otellog.SeverityFatal1
	case zerolog.PanicLevel:
		return otellog.SeverityFatal2
	default:
		return otellog.SeverityUndefined
	}
}
