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
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/asimmons91/pico-editor/internal/app"
	"github.com/asimmons91/pico-editor/internal/config"
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

	game := app.New(app.Config{
		Logger:       logger,
		DebugCharset: options.debugCharset,
		File:         options.file,
		EnableLSP:    true,
		Version:      version.Version,
	})
	if err := game.StartupError(); err != nil {
		logger.Error("startup", "error", err)
		fmt.Fprintf(stderr, "pico-editor: %v\n", err)
		return 1
	}

	scale := game.InitialScale()
	logger.Info("window",
		"scale", scale,
		"width", app.CanvasWidth*scale,
		"height", app.CanvasHeight*scale,
	)

	ebiten.SetWindowTitle("pico-editor " + version.Version)
	ebiten.SetWindowSize(app.CanvasWidth*scale, app.CanvasHeight*scale)
	ebiten.SetWindowSizeLimits(app.CanvasWidth, app.CanvasHeight, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	// The game decides whether a close request can proceed so unsaved buffers
	// are not discarded silently.
	ebiten.SetWindowClosingHandled(true)

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
	flags.StringVar(&opts.logFile, "log-file", "", "log file path, or - for stderr (default: $XDG_STATE_HOME/pico-editor/pico-editor.log)")
	flags.Usage = func() {
		printUsage(stderr, flags)
	}

	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() > 1 {
		fmt.Fprintf(stderr, "pico-editor: expected at most one file or directory, got %s\n", strings.Join(flags.Args(), " "))
		flags.Usage()
		return options{}, errors.New("too many file arguments")
	}
	if flags.NArg() == 1 {
		opts.file = flags.Arg(0)
		if opts.file == "" {
			fmt.Fprintln(stderr, "pico-editor: path argument must not be empty")
			flags.Usage()
			return options{}, errors.New("empty file argument")
		}
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
	fmt.Fprintln(w, "usage: pico-editor [flags] [file|directory]")
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

// setupLogger opens the rotating log file and falls back to stderr when it
// cannot. "--log-file -" selects stderr directly.
func setupLogger(level slog.Level, path string, stderr io.Writer) (*slog.Logger, func()) {
	if path == "-" {
		return stderrLogger(level, stderr), func() {}
	}
	if path == "" {
		path = defaultLogPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(stderr, "pico-editor: warning: cannot create log directory %s: %v; logging to stderr\n", dir, err)
		return stderrLogger(level, stderr), func() {}
	}

	logFile, err := openRotatingFile(path, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "pico-editor: warning: cannot open log file %s: %v; logging to stderr\n", path, err)
		return stderrLogger(level, stderr), func() {}
	}

	return slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: level})), func() {
		if err := logFile.Close(); err != nil {
			fmt.Fprintf(stderr, "pico-editor: warning: closing log file: %v\n", err)
		}
	}
}

// logRotateBytes is the M7 rotation threshold. Three numbered backups are kept.
const logRotateBytes int64 = 1 << 20

type rotatingFile struct {
	path   string
	file   *os.File
	mu     sync.Mutex
	max    int64
	stderr io.Writer
	retry  int64
}

func openRotatingFile(path string, stderr io.Writer) (*rotatingFile, error) {
	writer := &rotatingFile{path: path, max: logRotateBytes, stderr: stderr}
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", path)
		}
		if info.Size() >= writer.max {
			if err := writer.rotateFiles(); err != nil {
				fmt.Fprintf(stderr, "pico-editor: warning: cannot rotate log %s: %v\n", path, err)
			}
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	_ = file.Chmod(0o600)
	writer.file = file
	return writer, nil
}

func (r *rotatingFile) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return 0, os.ErrClosed
	}
	n, err := r.file.Write(data)
	info, statErr := r.file.Stat()
	if err == nil && statErr == nil && info.Size() >= r.max && info.Size() >= r.retry {
		r.rotate()
	}
	return n, err
}

func (r *rotatingFile) rotate() {
	old := r.file
	if err := r.rotateFiles(); err != nil {
		r.retry = r.max * 2
		fmt.Fprintf(r.stderr, "pico-editor: warning: cannot rotate log %s: %v; keeping the current file\n", r.path, err)
		return
	}
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		// The renamed file descriptor is still open; keep writing to it rather
		// than losing records.
		fmt.Fprintf(r.stderr, "pico-editor: warning: reopening log %s: %v\n", r.path, err)
		r.retry = r.max * 2
		return
	}
	_ = file.Chmod(0o600)
	if err := old.Close(); err != nil {
		fmt.Fprintf(r.stderr, "pico-editor: warning: closing rotated log: %v\n", err)
	}
	r.file = file
	r.retry = 0
}

func (r *rotatingFile) rotateFiles() error {
	_ = os.Remove(r.path + ".3")
	_ = os.Rename(r.path+".2", r.path+".3")
	_ = os.Rename(r.path+".1", r.path+".2")
	if err := os.Rename(r.path, r.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

func stderrLogger(level slog.Level, stderr io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
}

func defaultLogPath() string {
	return config.LogPath()
}
