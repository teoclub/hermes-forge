# Logger

## Usage

### slog

This package is built on the standard library `log/slog`. You can still use
plain slog handlers directly.

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
	Level: logger.LevelInfo,
}))

logger.InfoContext(ctx, "agent started",
	"provider", "anthropic",
	"model", model,
)
```

### Global logger

Common global helpers are available for gradual migration. Their signatures
mirror slog: the first argument is the message, followed by key/value pairs or
`slog.Attr` values.

```go
logger.SetDefault(logger.New(logger.WithLevel(logger.LevelDebug)))

logger.Info("started")
logger.Info("provider initialized", "provider", "anthropic", "model", model)
logger.InfoContext(ctx, "agent run started", "work_dir", workDir)
logger.Error("agent run failed", "err", err)
```

### Builder

`logger.NewHandler` builds a default handler. `logger.WrapHandler` wraps an
existing handler with hermes-forge logger decorators. `logger.NewLogger` is a
convenience wrapper around `slog.New(logger.WrapHandler(...))`.

```go
l := logger.NewLogger(
	slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logger.LevelInfo,
	}),
).With(
	slog.String("service.name", "hermes-forge"),
	slog.String("service.version", version),
)

logger.SetDefault(l)
```

For the common case, use `logger.New`:

```go
l := logger.New(
	logger.WithWriter(os.Stdout),
	logger.WithFormat(logger.FormatJSON),
	logger.WithLevel(logger.LevelInfo),
	logger.WithAddSource(true),
)
```

Available options:

Handler options for `New` and `NewHandler`:

- `WithWriter(io.Writer)` sets the output destination. Default: `os.Stderr`.
- `WithFormat(logger.FormatText | logger.FormatJSON)` selects text or JSON.
- `WithLevel(logger.Leveler)` sets the minimum enabled level.
- `WithAddSource(bool)` includes caller source information.
- `WithReplaceAttr(func(groups []string, attr slog.Attr) slog.Attr)` customizes attrs.

Wrapper options for `WrapHandler` and `NewLogger`:

- `WithExtractor(...logger.Extractor)` adds context attr extractors.

### Context attrs

Attach attributes to a `context.Context` and they will flow through any
context-aware log call automatically.

```go
ctx = logger.ContextWithAttrs(ctx,
	slog.String("trace_id", traceID),
	slog.String("chat_id", chatID),
)

logger.InfoContext(ctx, "handling message")
```

`logger.New`, `logger.NewHandler`, `logger.WrapHandler`, and `logger.NewLogger`
include `logger.AttrsFromContext` by default. Add custom extractors when attrs
live somewhere else in the context.

```go
l := logger.NewLogger(logger.NewHandler(), logger.WithExtractor(func(ctx context.Context) []slog.Attr {
	runID, _ := ctx.Value(runIDKey{}).(string)
	if runID == "" {
		return nil
	}
	return []slog.Attr{slog.String("run_id", runID)}
}))
```

### Replace attrs

Use `WithReplaceAttr` for redaction or output normalization. It is passed
directly to `slog.HandlerOptions.ReplaceAttr`.

```go
l := logger.New(logger.WithReplaceAttr(func(groups []string, attr slog.Attr) slog.Attr {
	if attr.Key == "password" || attr.Key == "token" {
		return slog.String(attr.Key, "***")
	}
	return attr
}))
```

### Levels

The package aliases slog levels and adds `LevelFatal`.

```go
logger.LevelDebug
logger.LevelInfo
logger.LevelWarn
logger.LevelError
logger.LevelFatal
```

```go
level := logger.ParseLevel("debug")
```

Unknown strings fall back to `LevelInfo`.

### Tests

```sh
go test ./logger
go test ./...
```
