# `tint`: 🌈 **slog.Handler** that writes tinted logs

[![Go Reference](https://pkg.go.dev/badge/github.com/lmittmann/tint.svg)](https://pkg.go.dev/github.com/lmittmann/tint)
[![Go Report Card](https://goreportcard.com/badge/github.com/lmittmann/tint)](https://goreportcard.com/report/github.com/lmittmann/tint)

<picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://user-images.githubusercontent.com/3458786/230217128-1ccce237-bf6c-42f5-b026-a86720541584.png">
    <source media="(prefers-color-scheme: light)" srcset="https://user-images.githubusercontent.com/3458786/230217128-1ccce237-bf6c-42f5-b026-a86720541584.png">
    <img src="https://user-images.githubusercontent.com/3458786/230217128-1ccce237-bf6c-42f5-b026-a86720541584.png">
</picture>
<br>
<br>

Package `tint` provides a [`slog.Handler`](https://pkg.go.dev/golang.org/x/exp/slog#Handler) that writes tinted logs. The output format is inspired by the `zerolog.ConsoleWriter`.

```
go get github.com/lmittmann/tint
```

> **Note**
>
> [`slog`](https://pkg.go.dev/golang.org/x/exp/slog) is an experimental structured logging package, that will be included in the **standard library** in **Go 1.21**. See [#56345](https://github.com/golang/go/issues/56345) for tracking the progress.


## Usage

```go
// create a new logger
logger := slog.New(tint.NewHandler(os.Stderr))

// set global logger with custom options
slog.SetDefault(slog.New(tint.Options{
	Level:      slog.LevelDebug,
	TimeFormat: time.Kitchen,
}.NewHandler(os.Stderr)))
```

### Windows

ANSI color support in the terminal on Windows can be enabled by using e.g. [`go-colorable`](https://github.com/mattn/go-colorable).

```go
logger := slog.New(tint.NewHandler(colorable.NewColorableStderr()))
```

### Customize

tint implements `ReplaceAttr` pattern.
`ReplaceAttr` can remove or format an attribute.
Non string attributes are rendered as-is.
Time and Level attributes are formatted using zerolog.ConsoleWriter style.

Example:

``` go
var levelStrings = map[slog.Level]string{
    slog.LevelDebug: "\033[2mDEBUG",
    slog.LevelInfo:  "\033[1mINFO ",
    slog.LevelWarn:  "\033[1;38;5;185mWARN ",
    slog.LevelError: "\033[1;31mERROR",
}

slog.SetDefault(slog.New(tint.Options{
    TimeFormat: "15:04:05",
    Level:      slog.LevelDebug,
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        if a.Key == slog.LevelKey {
            a.Value = slog.StringValue(levelStrings[slog.Level(a.Value.Int64())])
        }
        return a
    },
}.NewHandler(os.Stderr)))

slog.Info("Starting server", "addr", ":8080", "env", "production")
slog.Debug("Connected to DB", "db", "myapp", "host", "localhost:5432")
slog.Warn("Slow request", "method", "GET", "path", "/users", "duration", 497*time.Millisecond)
slog.Error("DB connection lost", tint.Err(errors.New("connection reset")), "db", "myapp")
```

