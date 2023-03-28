# `tint`: 🌈 **slog.Handler** that writes tinted logs

[![Go Reference](https://pkg.go.dev/badge/github.com/lmittmann/tint.svg)](https://pkg.go.dev/github.com/lmittmann/tint)
[![Go Report Card](https://goreportcard.com/badge/github.com/lmittmann/tint)](https://goreportcard.com/report/github.com/lmittmann/tint)

<picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://user-images.githubusercontent.com/3458786/227801217-5f2d6966-8112-4753-8cbd-409fb1a43141.png">
    <source media="(prefers-color-scheme: light)" srcset="https://user-images.githubusercontent.com/3458786/227801217-5f2d6966-8112-4753-8cbd-409fb1a43141.png">
    <img src="https://user-images.githubusercontent.com/3458786/227801217-5f2d6966-8112-4753-8cbd-409fb1a43141.png">
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
