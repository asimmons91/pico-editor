// Command fontgen validates internal/font/glyphs.txt and regenerates the
// checked-in Go atlas and preview PNG.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/asimmons91/pico-editor/internal/fontgen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fontgen: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dataPath = flag.String("data", "glyphs.txt", "glyph data file")
		goPath   = flag.String("out", "atlas_gen.go", "generated Go atlas")
		pngPath  = flag.String("png", "atlas.png", "generated preview PNG")
		check    = flag.Bool("check", false, "verify outputs are current instead of writing them")
	)
	flag.Parse()

	records, err := fontgen.ParseFile(*dataPath)
	if err != nil {
		return err
	}
	generatedGo := fontgen.GenerateGo(records)
	generatedPNG, err := fontgen.GeneratePNG(records)
	if err != nil {
		return err
	}

	if *check {
		if err := checkFile(*goPath, generatedGo); err != nil {
			return err
		}
		return checkFile(*pngPath, generatedPNG)
	}
	if err := os.WriteFile(*goPath, generatedGo, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", *goPath, err)
	}
	if err := os.WriteFile(*pngPath, generatedPNG, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", *pngPath, err)
	}
	return nil
}

func checkFile(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("check %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s is stale; run go generate ./internal/font", path)
	}
	return nil
}
