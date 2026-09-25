package internal

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rs/zerolog"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// capture keeps exported records in memory so a test can look at them.
type capture struct {
	records []sdklog.Record
}

func (c *capture) Export(_ context.Context, records []sdklog.Record) error {
	for _, r := range records {
		// The SDK pools what it hands over, so take a copy.
		c.records = append(c.records, r.Clone())
	}
	return nil
}

func (c *capture) Shutdown(context.Context) error   { return nil }
func (c *capture) ForceFlush(context.Context) error { return nil }

// installCapture registers a global LoggerProvider that records into a
// capture, and returns it.
func installCapture(t *testing.T) *capture {
	t.Helper()
	c := &capture{}
	global.SetLoggerProvider(sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(c)),
	))
	return c
}

// A log record is only worth sending to OpenTelemetry if it is tied to the
// trace that was active when it was written. That correlation is the whole
// point, so it is the thing worth pinning down.
func TestOtelHookCarriesTraceContext(t *testing.T) {
	c := installCapture(t)

	tracer := sdktrace.NewTracerProvider().Tracer("test")
	ctx, span := tracer.Start(context.Background(), "operation")
	defer span.End()

	logger := zerolog.New(io.Discard).Hook(newOtelHook("test"))
	logger.Info().Ctx(ctx).Str("server", "api").Msg("listening")

	if len(c.records) != 1 {
		t.Fatalf("got %d records, want 1", len(c.records))
	}
	got := c.records[0]

	if want := "listening"; got.Body().AsString() != want {
		t.Errorf("body = %q, want %q", got.Body().AsString(), want)
	}
	if got.Severity() != otellog.SeverityInfo1 {
		t.Errorf("severity = %v, want %v", got.Severity(), otellog.SeverityInfo1)
	}
	if want := "info"; got.SeverityText() != want {
		t.Errorf("severity text = %q, want %q", got.SeverityText(), want)
	}
	if !got.TraceID().IsValid() || got.TraceID() != span.SpanContext().TraceID() {
		t.Errorf("trace id = %v, want %v", got.TraceID(), span.SpanContext().TraceID())
	}
	if !got.SpanID().IsValid() || got.SpanID() != span.SpanContext().SpanID() {
		t.Errorf("span id = %v, want %v", got.SpanID(), span.SpanContext().SpanID())
	}
}

func TestOtelHookLevels(t *testing.T) {
	c := installCapture(t)
	logger := zerolog.New(io.Discard).Hook(newOtelHook("test"))

	logger.Trace().Msg("t")
	logger.Debug().Msg("d")
	logger.Info().Msg("i")
	logger.Warn().Msg("w")
	logger.Error().Err(errors.New("boom")).Msg("e")

	want := []otellog.Severity{
		otellog.SeverityTrace1,
		otellog.SeverityDebug1,
		otellog.SeverityInfo1,
		otellog.SeverityWarn1,
		otellog.SeverityError1,
	}
	if len(c.records) != len(want) {
		t.Fatalf("got %d records, want %d", len(c.records), len(want))
	}
	for i, w := range want {
		if c.records[i].Severity() != w {
			t.Errorf("record %d severity = %v, want %v", i, c.records[i].Severity(), w)
		}
	}
}

// The global level is applied before the hook runs, so a level that is turned
// off must not reach OpenTelemetry either.
func TestOtelHookRespectsGlobalLevel(t *testing.T) {
	c := installCapture(t)
	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(zerolog.TraceLevel) })

	logger := zerolog.New(io.Discard).Hook(newOtelHook("test"))
	logger.Debug().Msg("dropped")
	logger.Error().Msg("kept")

	if len(c.records) != 1 {
		t.Fatalf("got %d records, want 1", len(c.records))
	}
	if want := "kept"; c.records[0].Body().AsString() != want {
		t.Errorf("body = %q, want %q", c.records[0].Body().AsString(), want)
	}
}
