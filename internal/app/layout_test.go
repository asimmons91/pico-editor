package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/render"
	"github.com/asimmons91/pico-editor/internal/vim"
)

func TestSidebarResizesEditorAndGutter(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	session := game.session
	ed := session.CurrentEditor()

	if got := session.codeWidth(); got != GridColumns-gutterWidth {
		t.Fatalf("hidden-tree code width = %d, want %d", got, GridColumns-gutterWidth)
	}
	if got := ed.Viewport.Width; got != session.codeWidth() {
		t.Fatalf("hidden-tree viewport width = %d, want %d", got, session.codeWidth())
	}

	session.setTreeVisible(true)
	if got := session.editorLeft(); got != treeColumns {
		t.Fatalf("editor left = %d, want %d", got, treeColumns)
	}
	if got := ed.Viewport.Width; got != GridColumns-treeColumns-gutterWidth {
		t.Fatalf("sidebar viewport width = %d, want %d", got, GridColumns-treeColumns-gutterWidth)
	}
	if session.ClickFileTabs(0, 0) {
		t.Fatal("file tabs accepted a click inside the sidebar")
	}

	session.setTreeVisible(false)
	if got := ed.Viewport.Width; got != GridColumns-gutterWidth {
		t.Fatalf("restored viewport width = %d, want %d", got, GridColumns-gutterWidth)
	}
}

func TestFocusTreeOpensAndResizesSidebar(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	session := game.session
	session.Tree.Visible = false
	session.Tree.Focused = false

	if !session.FocusTree() {
		t.Fatal("FocusTree did not handle the request")
	}
	if !session.Tree.Visible || !session.Tree.Focused {
		t.Fatalf("tree state = visible %t focused %t, want both true", session.Tree.Visible, session.Tree.Focused)
	}
	if got := session.CurrentEditor().Viewport.Width; got != GridColumns-treeColumns-gutterWidth {
		t.Fatalf("viewport width = %d, want %d", got, GridColumns-treeColumns-gutterWidth)
	}
}

func TestStatusLayoutSplitsLeftAndRight(t *testing.T) {
	data := fullStatusData()
	data.file = "game.p8:1"
	data.percent = 42
	left, right := layoutStatusSections(64, data)
	if len(left) == 0 || left[0].text != "N" {
		t.Fatalf("left segments = %+v, want mode first", left)
	}
	if !statusHasSegment(left, "game.p8:1*") {
		t.Fatal("left segments missing file")
	}
	if !statusHasSegment(right, "42%") {
		t.Fatal("right segments missing percentage")
	}
	if !statusHasSegment(right, "12:7") {
		t.Fatal("right segments missing position")
	}
}

func statusHasSegment(segments []statusSegment, want string) bool {
	for _, segment := range segments {
		if segment.text == want {
			return true
		}
	}
	return false
}

func TestToggleTreeReflowsViewport(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	session := game.session
	if session.Tree.Visible {
		t.Fatal("file-based startup unexpectedly opened the sidebar")
	}

	if !session.ToggleTree() {
		t.Fatal("ToggleTree did not handle the open request")
	}
	if !session.Tree.Visible || !session.Tree.Focused {
		t.Fatalf("tree state = visible %t focused %t, want both true", session.Tree.Visible, session.Tree.Focused)
	}
	if got := session.CurrentEditor().Viewport.Width; got != GridColumns-treeColumns-gutterWidth {
		t.Fatalf("opened viewport width = %d, want %d", got, GridColumns-treeColumns-gutterWidth)
	}

	if !session.ToggleTree() {
		t.Fatal("ToggleTree did not handle the close request")
	}
	if session.Tree.Visible {
		t.Fatal("tree stayed visible after the close request")
	}
	if got := session.CurrentEditor().Viewport.Width; got != GridColumns-gutterWidth {
		t.Fatalf("closed viewport width = %d, want %d", got, GridColumns-gutterWidth)
	}
}

func TestDirectoryStartupSizesEditorForSidebar(t *testing.T) {
	session, err := NewSessionAt(t.TempDir(), "", slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if !session.Tree.Visible || !session.Tree.Focused {
		t.Fatalf("startup tree = visible %t focused %t, want both true", session.Tree.Visible, session.Tree.Focused)
	}
	if got := session.CurrentEditor().Viewport.Width; got != GridColumns-treeColumns-gutterWidth {
		t.Fatalf("startup viewport width = %d, want %d", got, GridColumns-treeColumns-gutterWidth)
	}
}

func TestSidebarTabClicksUseEditorOffset(t *testing.T) {
	game, _ := newCartGame(t, fullCart)
	session := game.session
	session.setTreeVisible(true)
	buffer := session.CurrentBuffer()

	cartHits := fitTabs(session.cartTabLabels(buffer), buffer.ActiveTab, session.editorWidth())
	if len(cartHits) < 2 {
		t.Fatalf("cart hits = %+v", cartHits)
	}
	cartX := session.editorLeftPx() + (cartHits[1].StartCell+1)*CellWidth
	if !session.ClickCartTabs(cartX, cartTabRow*CellHeight) {
		t.Fatal("sidebar-offset cart tab click was not handled")
	}
	if buffer.ActiveTab != 1 {
		t.Fatalf("active cart tab = %d, want 1", buffer.ActiveTab)
	}

	other := filepath.Join(filepath.Dir(buffer.Editor.Buffer.Path), "other.lua")
	if err := os.WriteFile(other, []byte("y=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !session.OpenPath(other, false) {
		t.Fatal("OpenPath did not open the second buffer")
	}
	fileHits := fitTabs(session.fileTabLabels(), session.Current, session.editorWidth())
	if len(fileHits) < 2 {
		t.Fatalf("file hits = %+v", fileHits)
	}
	fileX := session.editorLeftPx() + fileHits[0].StartCell*CellWidth
	if !session.ClickFileTabs(fileX, fileTabRow*CellHeight) {
		t.Fatal("sidebar-offset file tab click was not handled")
	}
	if session.Current != 0 {
		t.Fatalf("current buffer = %d, want 0", session.Current)
	}
}

func TestPopupGeometryRespectsSidebar(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	session := game.session
	session.setTreeVisible(true)

	width := session.popupWidth(56)
	if width != session.editorWidth()-2 {
		t.Fatalf("sidebar popup width = %d, want %d", width, session.editorWidth()-2)
	}
	pixels := width * font.NarrowWidth
	if x := session.popupLeft(pixels); x < session.editorLeftPx() || x+pixels > session.editorRightPx() {
		t.Fatalf("popupLeft = %d, want within [%d, %d]", x, session.editorLeftPx(), session.editorRightPx())
	}
	if x := session.clampPopupX(0, pixels); x != session.editorLeftPx() {
		t.Fatalf("clampPopupX = %d, want %d", x, session.editorLeftPx())
	}

	session.setTreeVisible(false)
	if width := session.popupWidth(56); width != 56 {
		t.Fatalf("full-width popup width = %d, want 56", width)
	}
}

func TestGutterSignsPreferErrors(t *testing.T) {
	game, _ := newEditorGame(t, "one\ntwo\n", &recordingClipboard{})
	session := game.session
	documentURI := session.activeURI()
	session.LSPDiagnostics[documentURI] = []lsp.Diagnostic{
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 0}, End: lsp.Position{Line: 0, Character: 1}},
			Severity: lsp.SeverityWarning,
		},
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 0}, End: lsp.Position{Line: 0, Character: 1}},
			Severity: lsp.SeverityError,
		},
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 1}, End: lsp.Position{Line: 1, Character: 1}},
			Severity: lsp.SeverityWarning,
		},
	}

	signs := session.diagnosticSigns(session.CurrentEditor(), documentURI, 0)
	if sign := signs[0]; sign.letter != 'E' || sign.color != gutterError {
		t.Fatalf("line 0 sign = %+v, want an error sign", sign)
	}
	if sign := signs[1]; sign.letter != 'W' || sign.color != gutterWarning {
		t.Fatalf("line 1 sign = %+v, want a warning sign", sign)
	}
}

func TestStatusBarDrawsModeBackgrounds(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	session := game.session
	for _, test := range []struct {
		mode       vim.Mode
		background uint8
	}{
		{vim.ModeNormal, modeNormalBackground},
		{vim.ModeInsert, modeInsertBackground},
		{vim.ModeVisualChar, modeVisualBackground},
	} {
		session.Vim.Mode = test.mode
		surface := render.NewSurface(CanvasWidth, CanvasHeight)
		session.Draw(surface, false)
		if countPaletteInRows(surface, test.background, statusRow, GridRows-1) == 0 {
			t.Errorf("mode %v did not paint background %d", test.mode, test.background)
		}
	}
}

func TestSidebarDoesNotCoverModalOverlays(t *testing.T) {
	game, _ := newEditorGame(t, "abc\n", &recordingClipboard{})
	session := game.session
	session.setTreeVisible(true)
	session.DiagList = &DiagnosticList{Items: []DiagnosticItem{{
		Range:    lsp.Range{Start: lsp.Position{Line: 0}, End: lsp.Position{Line: 0, Character: 1}},
		Severity: lsp.SeverityWarning,
		Message:  "unused",
	}}}

	surface := render.NewSurface(CanvasWidth, CanvasHeight)
	session.Draw(surface, false)
	found := false
	for y := 0; y < CanvasHeight; y++ {
		if surface.At(8, y) == panelBorder {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("sidebar painted over the centered diagnostic modal")
	}
}

func TestStatusPercent(t *testing.T) {
	for _, test := range []struct {
		line, lines, want int
	}{
		{0, 0, 0},
		{0, 1, 100},
		{0, 3, 33},
		{2, 3, 100},
	} {
		if got := statusPercent(test.line, test.lines); got != test.want {
			t.Errorf("statusPercent(%d, %d) = %d, want %d", test.line, test.lines, got, test.want)
		}
	}
}
