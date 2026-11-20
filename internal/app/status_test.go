package app

import (
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/lsp"
)

func fullStatusData() statusData {
	return statusData{
		mode:      "N",
		file:      "game.p8:1*",
		dirty:     true,
		line:      12,
		column:    7,
		errors:    2,
		warnings:  1,
		sizes:     "T120 C300 Z80",
		sizeColor: 7,
		zColor:    7,
		lspStatus: lsp.StatusReady,
	}
}

func TestStatusLayout64CellsKeepsEverything(t *testing.T) {
	segments := layoutStatus(64, fullStatusData())
	text := statusText(segments)
	for _, want := range []string{"N", "game.p8:1*", "12:7", "E2", "W1", "T120 C300", "Z80", "LSP"} {
		if !strings.Contains(text, want) {
			t.Errorf("status %q missing %q", text, want)
		}
	}
	if fontCells(text) > 64 {
		t.Errorf("status %q is %d cells, want <= 64", text, fontCells(text))
	}
}

func TestStatusLayoutDropsSizesThenDiagnostics(t *testing.T) {
	data := fullStatusData()
	data.file = "game.p8:1*"
	segments := layoutStatus(40, data)
	text := statusText(segments)
	if strings.Contains(text, "T120") {
		t.Errorf("status %q kept sizes at 40 cells", text)
	}
	if !strings.Contains(text, "E2") {
		t.Errorf("status %q dropped diagnostics before sizes", text)
	}
	if fontCells(text) > 40 {
		t.Errorf("status %q is %d cells, want <= 40", text, fontCells(text))
	}
}

func TestStatusLayoutDropsDiagnosticsWhenSpaceIsTight(t *testing.T) {
	data := fullStatusData()
	data.file = strings.Repeat("very-long-name", 4) + ".p8"
	segments := layoutStatus(40, data)
	text := statusText(segments)
	if strings.Contains(text, "T120") || strings.Contains(text, "E2") {
		t.Errorf("status %q kept sizes/diagnostics at 40 cells with a long file", text)
	}
	if !strings.Contains(text, "…") {
		t.Errorf("status %q did not truncate the file middle", text)
	}
	if fontCells(text) > 40 {
		t.Errorf("status %q is %d cells, want <= 40", text, fontCells(text))
	}
}

func TestStatusLayoutKeepsModePositionAndLSPFailureAt20(t *testing.T) {
	data := fullStatusData()
	data.file = "a-very-long-file-name.p8"
	data.lspStatus = lsp.StatusFailed
	segments := layoutStatus(20, data)
	text := statusText(segments)
	for _, want := range []string{"N", "12:7", "LSP!"} {
		if !strings.Contains(text, want) {
			t.Errorf("status %q missing required %q", text, want)
		}
	}
	if fontCells(text) > 20 {
		t.Errorf("status %q is %d cells, want <= 20", text, fontCells(text))
	}
}

func TestStatusLayoutMessageReplacesSizesAndDiagnostics(t *testing.T) {
	data := fullStatusData()
	data.message = "written"
	segments := layoutStatus(64, data)
	text := statusText(segments)
	if !strings.Contains(text, "written") {
		t.Fatalf("status %q missing the notification", text)
	}
	if strings.Contains(text, "T120") || strings.Contains(text, "E2") {
		t.Errorf("status %q kept sizes/diagnostics behind a notification", text)
	}
}

func TestStatusLayoutTruncatesFileMiddle(t *testing.T) {
	data := statusData{mode: "N", file: strings.Repeat("abcdefghij", 4), line: 1, column: 1}
	segments := layoutStatus(24, data)
	text := statusText(segments)
	if !strings.Contains(text, "…") {
		t.Errorf("status %q did not truncate the middle of the file name", text)
	}
	if fontCells(text) > 24 {
		t.Errorf("status %q is %d cells, want <= 24", text, fontCells(text))
	}
}

func TestStatusModeLabels(t *testing.T) {
	for _, test := range []struct {
		mode string
		want string
	}{
		{"N", "N"}, {"I", "I"}, {"V", "V"}, {"VL", "VL"}, {"VB", "VB"}, {":", ":"},
	} {
		segments := layoutStatus(64, statusData{mode: test.mode, line: 1, column: 1})
		if got := segments[0].text; got != test.want {
			t.Errorf("mode %q rendered as %q", test.mode, got)
		}
	}
}

func TestStatusDeprecatedMarker(t *testing.T) {
	segments := layoutStatus(64, statusData{mode: "N", line: 1, column: 1, deprecated: true})
	if !strings.Contains(statusText(segments), "!") {
		t.Fatal("deprecated marker missing")
	}
}

func fontCells(text string) int {
	return font.CellWidth(text)
}
