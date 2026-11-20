package vim

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
)

// matrixRows is the frozen command matrix from docs/VIM.md. Every row must
// appear in a testdata fixture; adding a row means updating both files.
var matrixRows = []string{
	"2.1-insert", "2.1-insert-line", "2.1-open-line", "2.1-visual",
	"2.1-command-line", "2.1-escape", "2.1-dot", "2.1-undo-redo",
	"2.1-gv", "2.1-jumps", "2.1-scroll", "2.1-scroll-bottom",
	"2.1-ctrl-bracket",
	"2.1-hover", "2.1-definition", "2.1-definition-ctrl",
	"2.1-scroll-center",
	"2.2-hjkl", "2.2-words", "2.2-words-big", "2.2-line", "2.2-gg-G", "2.2-find",
	"2.2-percent", "2.2-paragraph", "2.2-screen", "2.2-half-page",
	"2.2-find-F", "2.2-find-t", "2.2-find-T", "2.2-enter-motion",
	"2.2-vertical-desired", "2.2-find-t-count", "2.2-find-t-repeat",
	"2.2-find-T-count",
	"2.2-full-page", "2.2-line-motion", "2.2-gg-count",
	"2.2-percent-string", "2.2-space-bs",
	"2.2-space-bs-wrap", "2.2-space-bs-back",
	"2.2-search-next", "2.2-star-hash",
	"2.3-d-c-y", "2.3-d-c-change", "2.3-d-c-yank", "2.3-dd-cc-yy",
	"2.3-cc", "2.3-yy", "2.3-D-C-Y", "2.3-C", "2.3-Y", "2.3-x-X",
	"2.3-s-S", "2.3-S", "2.3-r", "2.3-r-count", "2.3-J", "2.3-J-count",
	"2.3-tilde", "2.3-tilde-count", "2.3-shift", "2.3-shift-motion",
	"2.3-p-P", "2.3-p-before", "2.3-dgg", "2.3-dG",
	"2.3-shift-textobject", "2.3-d-halfpage", "2.3-d-repeat-find",
	"dot-find-operator", "dot-append", "dot-append-eol",
	"dot-first-nonblank", "dot-open-below", "dot-open-above",
	"dot-change-count", "2.3-cw-word-end", "2.3-cw-one-letter",
	"2.3-cw-count", "2.3-invalid-g", "2.3-C-empty",
	"2.3-cw-empty", "2.3-cw-empty-middle", "2.3-s-empty",
	"2.3-undo-redo",
	"2.4-iw-aw", "2.4-aw", "2.4-iW-aW", "2.4-aW", "2.4-paren",
	"2.4-paren-close-alias", "2.4-brace", "2.4-bracket",
	"2.4-dquote", "2.4-squote", "2.4-paragraph", "2.4-aparagraph",
	"2.5-visual", "2.5-v-line", "2.5-v-block", "2.5-o", "2.5-gv",
	"2.5-extend", "2.5-d-c-y", "2.5-change", "2.5-yank",
	"visual-clipboard-yank", "visual-named-yank", "2.5-shift",
	"2.5-shift-left", "2.5-case", "2.5-case-upper", "2.5-case-lower",
	"2.5-J", "2.5-paste", "2.5-block-IA", "2.5-block-A",
	"2.5-block-delete", "2.5-block-yank", "2.5-block-change",
	"2.5-block-shift", "2.5-block-change-undo", "2.5-line-change",
	"2.5-line-case", "2.5-block-case", "2.5-block-wide-append",
	"2.5-gv-after-delete",
	"dot-block-delete",
	"dot-block-change", "dot-visual-wide", "dot-visual-paste", "2.5-colon", "2.5-esc",
	"2.6-text", "2.6-enter", "2.6-backspace", "2.6-delete", "2.6-tab",
	"2.6-ctrl-w-u", "2.6-ctrl-u", "2.6-esc", "2.6-arrows",
	"2.6-p8-modes", "2.6-completion", "2.6-count", "2.6-home-end",
	"2.6-ctrl-bracket",
	"2.7-search", "2.7-cancel", "2.7-n-N", "2.7-star-hash", "2.7-noh",
	"2.7-highlight", "2.7-history", "2.7-smartcase", "2.7-magic",
	"2.7-backward", "2.7-hash",
	"2.8-w", "2.8-q", "2.8-q-bang", "2.8-wq-x", "2.8-x", "2.8-e",
	"2.8-e-bang", "2.8-e-path", "2.8-bn-bp", "2.8-b", "2.8-ls",
	"2.8-tabn-tabp", "2.8-help", "2.8-help-topic", "2.8-diag",
	"2.8-lsp-restart",
	"2.8-messages", "2.8-reload", "2.8-substitute",
	"2.8-commandline-edit", "2.8-commandline-clear",
	"2.8-commandline-word", "2.8-commandline-history",
	"2.8-commandline-prefix", "2.8-commandline-ambiguous",
	"2.8-errors", "2.8-error-substitute", "2.8-error-name",
	"2.8-error-write-path",
	"2.9-current", "2.9-whole", "2.9-visual", "2.9-lines", "2.9-flags",
	"2.9-replacement-amp", "2.9-replacement-groups",
	"2.9-replacement-slash", "2.9-undo", "2.9-no-match",
	"2.9-dot-range", "2.9-invalid-regex", "2.9-missing-delimiter",
	"2.9-empty-pattern", "2.9-cluster-replace", "2.9-cluster-dot",
	"2.10-unnamed", "2.10-zero", "2.10-numbered", "2.10-named",
	"2.10-append", "2.10-blackhole", "2.10-clipboard-write",
	"2.10-clipboard-read", "2.10-types", "2.10-block-paste",
	"2.10-block-replace", "dot-clipboard-paste",
	"counts-multiply", "counts-lines", "counts-goto", "counts-zero",
	"dot-insert", "dot-operator", "dot-cw-s", "undo-insert-group",
	"undo-substitute", "undo-visual",
}

type fixture struct {
	file      string
	matrix    []string
	name      string
	mode      Mode
	buffer    string
	cursor    editor.Pos
	keys      string
	clipboard string
	path      string
	height    int

	wantBuffer     *string
	wantCursor     *editor.Pos
	wantMode       *Mode
	wantRegister   *fixtureRegister
	wantNotify     string
	wantBell       bool
	wantAction     []string
	wantClipboard  *string
	wantSelection  *fixtureSelection
	wantTop        *int
	wantLeft       *int
	wantHighlights *int
}

type fixtureRegister struct {
	selector byte
	kind     registerKind
	text     string
}

type fixtureSelection struct {
	kind   editor.SelectionKind
	anchor editor.Pos
	cursor editor.Pos
}

func TestMatrixFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	known := make(map[string]bool, len(matrixRows))
	for _, id := range matrixRows {
		known[id] = true
	}
	covered := make(map[string]bool)
	for _, fix := range fixtures {
		for _, id := range fix.matrix {
			if !known[id] {
				t.Errorf("%s: fixture uses unknown matrix row %s", fix.file, id)
			}
			if covered[id] {
				t.Errorf("%s: matrix row %s covered more than once", fix.file, id)
			}
			covered[id] = true
		}
	}
	for _, id := range matrixRows {
		if !covered[id] {
			t.Errorf("matrix row %s has no fixture", id)
		}
	}
	for _, fix := range fixtures {
		fix := fix
		t.Run(fix.name, func(t *testing.T) {
			runFixture(t, fix)
		})
	}
}

func loadFixtures(t *testing.T) []fixture {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "matrix", "*.vimtest"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	var fixtures []fixture
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, block := range splitFixtureBlocks(string(data)) {
			fix := parseFixture(t, path, block)
			if fix.name != "" {
				fixtures = append(fixtures, fix)
			}
		}
	}
	return fixtures
}

func splitFixtureBlocks(text string) []string {
	lines := strings.Split(text, "\n")
	var blocks []string
	var current strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			blocks = append(blocks, current.String())
			current.Reset()
			continue
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if strings.TrimSpace(current.String()) != "" {
		blocks = append(blocks, current.String())
	}
	return blocks
}

func parseFixture(t *testing.T, path, block string) fixture {
	t.Helper()
	fix := fixture{file: path, mode: ModeNormal}
	for _, line := range strings.Split(block, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("%s: malformed line %q", path, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "matrix":
			for _, id := range strings.Split(value, ",") {
				fix.matrix = append(fix.matrix, strings.TrimSpace(id))
			}
		case "name":
			fix.name = value
		case "mode":
			fix.mode = parseMode(t, value)
		case "buffer":
			fix.buffer = unescape(value)
		case "cursor":
			fix.cursor = parsePos(t, value)
		case "keys":
			fix.keys = value
		case "clipboard":
			fix.clipboard = unescape(value)
		case "path":
			fix.path = value
		case "height":
			number, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("%s: bad height %q", path, value)
			}
			fix.height = number
		case "want.buffer":
			text := unescape(value)
			fix.wantBuffer = &text
		case "want.cursor":
			position := parsePos(t, value)
			fix.wantCursor = &position
		case "want.mode":
			mode := parseMode(t, value)
			fix.wantMode = &mode
		case "want.register":
			fix.wantRegister = parseRegister(t, value)
		case "want.notify":
			fix.wantNotify = value
		case "want.bell":
			fix.wantBell = value == "true"
		case "want.action":
			for _, action := range strings.Split(value, ",") {
				fix.wantAction = append(fix.wantAction, strings.TrimSpace(action))
			}
		case "want.clipboard":
			text := unescape(value)
			fix.wantClipboard = &text
		case "want.selection":
			fix.wantSelection = parseSelection(t, value)
		case "want.top":
			number, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("%s: bad want.top %q", path, value)
			}
			fix.wantTop = &number
		case "want.highlights":
			number, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("%s: bad want.highlights %q", path, value)
			}
			fix.wantHighlights = &number
		case "want.left":
			number, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("%s: bad want.left %q", path, value)
			}
			fix.wantLeft = &number
		default:
			t.Fatalf("%s: unknown fixture key %q", path, key)
		}
	}
	if fix.name == "" {
		t.Fatalf("%s: fixture has no name", path)
	}
	return fix
}

func runFixture(t *testing.T, fix fixture) {
	t.Helper()
	ed := testEditor(t, fix.buffer, fix.cursor.Line, fix.cursor.Rune)
	if fix.path == "none" {
		ed.Buffer.Path = ""
	}
	if fix.height > 0 {
		ed.Viewport.Height = fix.height
		ed.EnsureVisible()
	}
	v := New()
	v.Mode = fix.mode
	if fix.mode.Visual() {
		kind := editor.SelectionChar
		switch fix.mode {
		case ModeVisualLine:
			kind = editor.SelectionLine
		case ModeVisualBlock:
			kind = editor.SelectionBlock
		}
		ed.Selection = editor.Selection{Anchor: fix.cursor, Cursor: fix.cursor, Kind: kind}
	}
	fakeClipboard := fix.clipboard
	var gotActions []string
	var notifications []string
	bells := false
	for _, event := range parseKeys(t, fix.keys) {
		result := v.Handle(event, ed)
		notifications = append(notifications, result.Notify)
		bells = bells || result.Bell
		for _, action := range result.Actions {
			gotActions = append(gotActions, action.Kind.String())
			switch action.Kind {
			case ActionClipboardWrite:
				fakeClipboard = action.Text
			case ActionClipboardRead:
				result := v.CompleteClipboardRead(fakeClipboard, nil, ed)
				notifications = append(notifications, result.Notify)
				bells = bells || result.Bell
			}
		}
	}
	if fix.wantBuffer != nil {
		got := strings.Join(ed.Buffer.Lines, "\n")
		if got != *fix.wantBuffer {
			t.Errorf("buffer = %q, want %q", got, *fix.wantBuffer)
		}
	}
	if fix.wantCursor != nil && ed.Cursor.Pos != *fix.wantCursor {
		t.Errorf("cursor = %+v, want %+v", ed.Cursor.Pos, *fix.wantCursor)
	}
	if fix.wantMode != nil && v.Mode != *fix.wantMode {
		t.Errorf("mode = %s, want %s", v.Mode, *fix.wantMode)
	}
	if fix.wantRegister != nil {
		value, ok := v.registers.get(fix.wantRegister.selector)
		if !ok {
			t.Errorf("register %q is empty", fix.wantRegister.selector)
		} else if value.Kind != fix.wantRegister.kind || value.Text != fix.wantRegister.text {
			t.Errorf("register %q = %v %q, want %v %q", fix.wantRegister.selector, value.Kind, value.Text, fix.wantRegister.kind, fix.wantRegister.text)
		}
	}
	if fix.wantNotify != "" {
		joined := strings.Join(notifications, "\n")
		if !strings.Contains(joined, fix.wantNotify) {
			t.Errorf("notifications = %q, want substring %q", joined, fix.wantNotify)
		}
	}
	if fix.wantBell && !bells {
		t.Error("expected a bell")
	}
	if len(fix.wantAction) > 0 {
		if strings.Join(gotActions, ",") != strings.Join(fix.wantAction, ",") {
			t.Errorf("actions = %v, want %v", gotActions, fix.wantAction)
		}
	}
	if fix.wantClipboard != nil && fakeClipboard != *fix.wantClipboard {
		t.Errorf("clipboard = %q, want %q", fakeClipboard, *fix.wantClipboard)
	}
	if fix.wantTop != nil && ed.Viewport.TopLine != *fix.wantTop {
		t.Errorf("viewport top = %d, want %d", ed.Viewport.TopLine, *fix.wantTop)
	}
	if fix.wantLeft != nil && ed.Viewport.LeftCell != *fix.wantLeft {
		t.Errorf("viewport left = %d, want %d", ed.Viewport.LeftCell, *fix.wantLeft)
	}
	if fix.wantHighlights != nil {
		matches := v.SearchHighlights(ed)
		if len(matches) != *fix.wantHighlights {
			t.Errorf("search highlights = %d, want %d", len(matches), *fix.wantHighlights)
		}
	}
	if fix.wantSelection != nil {
		if ed.Selection.Kind != fix.wantSelection.kind ||
			ed.Selection.Anchor != fix.wantSelection.anchor ||
			ed.Selection.Cursor != fix.wantSelection.cursor {
			t.Errorf("selection = %+v, want %+v", ed.Selection, *fix.wantSelection)
		}
	}
}

func parseMode(t *testing.T, text string) Mode {
	t.Helper()
	switch strings.ToLower(text) {
	case "normal":
		return ModeNormal
	case "insert":
		return ModeInsert
	case "visual":
		return ModeVisualChar
	case "v-line":
		return ModeVisualLine
	case "v-block":
		return ModeVisualBlock
	case "command":
		return ModeCommandLine
	default:
		t.Fatalf("unknown mode %q", text)
		return ModeNormal
	}
}

func parsePos(t *testing.T, text string) editor.Pos {
	t.Helper()
	parts := strings.Split(text, ",")
	if len(parts) != 2 {
		t.Fatalf("bad cursor %q", text)
	}
	line, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		t.Fatalf("bad cursor %q: %v", text, err)
	}
	runeIndex, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		t.Fatalf("bad cursor %q: %v", text, err)
	}
	return editor.Pos{Line: line, Rune: runeIndex}
}

func parseRegister(t *testing.T, text string) *fixtureRegister {
	t.Helper()
	parts := strings.SplitN(text, ":", 3)
	if len(parts) != 3 {
		t.Fatalf("bad register expectation %q", text)
	}
	selector := byte(0)
	if parts[0] != "unnamed" && parts[0] != "" {
		selector = parts[0][0]
	}
	kind := registerChar
	switch parts[1] {
	case "char":
		kind = registerChar
	case "line":
		kind = registerLine
	case "block":
		kind = registerBlock
	default:
		t.Fatalf("bad register kind %q", parts[1])
	}
	return &fixtureRegister{selector: selector, kind: kind, text: unescape(parts[2])}
}

func parseSelection(t *testing.T, text string) *fixtureSelection {
	t.Helper()
	parts := strings.Split(text, ",")
	if len(parts) != 5 {
		t.Fatalf("bad selection %q", text)
	}
	kind := editor.SelectionChar
	switch parts[0] {
	case "char":
		kind = editor.SelectionChar
	case "line":
		kind = editor.SelectionLine
	case "block":
		kind = editor.SelectionBlock
	default:
		t.Fatalf("bad selection kind %q", parts[0])
	}
	return &fixtureSelection{
		kind:   kind,
		anchor: parsePos(t, parts[1]+","+parts[2]),
		cursor: parsePos(t, parts[3]+","+parts[4]),
	}
}

func unescape(text string) string {
	text = strings.ReplaceAll(text, `\n`, "\n")
	text = strings.ReplaceAll(text, `\t`, "\t")
	text = strings.ReplaceAll(text, `\s`, " ")
	text = strings.ReplaceAll(text, `\\`, "\\")
	return text
}

var _ = input.KindKeyDown
