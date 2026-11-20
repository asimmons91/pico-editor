// Command pico-editor is a native PICO-8 code editor.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/app"
	"github.com/asimmons91/pico-editor/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main without the process exit so that flag and version handling can
// be tested.
func run(args []string, stdout, stderr io.Writer) (exitCode int) {
	// Install the panic net before flag parsing so startup panics are logged
	// too. The fallback logger is replaced once the configured one is open.
	logger := stderrLogger(slog.LevelInfo, stderr)
	closeLog := func() {}

	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("panic", "panic", recovered, "stack", string(debug.Stack()))
			fmt.Fprintf(stderr, "pico-editor: panic: %v\n", recovered)
			exitCode = 1
		}
		closeLog()
	}()

	options, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if options.showVersion {
		fmt.Fprintln(stdout, version.String())
		return 0
	}

	logger, closeLog = setupLogger(options.level, options.logFile, stderr)

	logger.Info("starting",
		"version", version.Version,
		"commit", version.Commit,
		"build_date", version.BuildDate,
	)

	// M0 and M1 use the default 4x scale. M7 feeds the configured scale here;
	// 0 selects the automatic 80%-of-monitor path.
	scale := app.DefaultScale
	if monitor := ebiten.Monitor(); monitor != nil {
		width, height := monitor.Size()
		scale = app.ResolveScale(app.DefaultScale, width, height)
	}
	logger.Info("window",
		"scale", scale,
		"width", app.CanvasWidth*scale,
		"height", app.CanvasHeight*scale,
	)

	ebiten.SetWindowTitle("pico-editor " + version.Version)
	ebiten.SetWindowSize(app.CanvasWidth*scale, app.CanvasHeight*scale)
	ebiten.SetWindowSizeLimits(app.CanvasWidth, app.CanvasHeight, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	game := app.New(app.Config{
		Logger:       logger,
		DebugCharset: options.debugCharset,
		File:         options.file,
	})
	if err := game.StartupError(); err != nil {
		logger.Error("startup", "error", err)
		fmt.Fprintf(stderr, "pico-editor: %v\n", err)
		return 1
	}

	if err := ebiten.RunGame(game); err != nil {
		logger.Error("run", "error", err)
		fmt.Fprintf(stderr, "pico-editor: %v\n", err)
		return 1
	}

	logger.Info("stopped")
	return 0
}

type options struct {
	showVersion  bool
	debugCharset bool
	file         string
	level        slog.Level
	logFile      string
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var (
		opts      options
		levelText string
	)

	flags := flag.NewFlagSet("pico-editor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.showVersion, "version", false, "print version information and exit")
	flags.BoolVar(&opts.debugCharset, "debug-charset", false, "show the P8SCII full-charset debug screen")
	flags.StringVar(&levelText, "log-level", "info", "log level: debug, info, warn, or error")
	flags.StringVar(&opts.logFile, "log-file", "", "log file path (default: $XDG_STATE_HOME/pico-editor/pico-editor.log)")
	flags.Usage = func() {
		printUsage(stderr, flags)
	}

	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() > 1 {
		fmt.Fprintf(stderr, "pico-editor: expected at most one file, got %s\n", strings.Join(flags.Args(), " "))
		flags.Usage()
		return options{}, errors.New("too many file arguments")
	}
	if flags.NArg() == 1 {
		opts.file = flags.Arg(0)
	}
	if opts.file != "" && opts.debugCharset {
		fmt.Fprintln(stderr, "pico-editor: cannot combine --debug-charset with a file argument")
		flags.Usage()
		return options{}, errors.New("conflicting debug screens")
	}

	level, err := parseLevel(levelText)
	if err != nil {
		fmt.Fprintf(stderr, "pico-editor: %v\n", err)
		flags.Usage()
		return options{}, err
	}
	opts.level = level
	return opts, nil
}

func printUsage(w io.Writer, flags *flag.FlagSet) {
	fmt.Fprintln(w, "usage: pico-editor [flags] [file]")
	fmt.Fprintln(w)
	flags.PrintDefaults()
}

func parseLevel(text string) (slog.Level, error) {
	switch text {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q (want debug, info, warn, or error)", text)
	}
}

// setupLogger opens the log file and falls back to stderr when it cannot.
func setupLogger(level slog.Level, path string, stderr io.Writer) (*slog.Logger, func()) {
	if path == "" {
		path = defaultLogPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(stderr, "pico-editor: warning: cannot create log directory %s: %v; logging to stderr\n", dir, err)
		return stderrLogger(level, stderr), func() {}
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(stderr, "pico-editor: warning: cannot open log file %s: %v; logging to stderr\n", path, err)
		return stderrLogger(level, stderr), func() {}
	}
	_ = file.Chmod(0o600)

	return slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: level})), func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(stderr, "pico-editor: warning: closing log file: %v\n", err)
		}
	}
}

func stderrLogger(level slog.Level, stderr io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
}

func defaultLogPath() string {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" && filepath.IsAbs(stateHome) {
		return filepath.Join(stateHome, "pico-editor", "pico-editor.log")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "state", "pico-editor", "pico-editor.log")
	}
	return filepath.Join(os.TempDir(), "pico-editor", "pico-editor.log")
}
