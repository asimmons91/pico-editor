package app

import (
	"testing"

	"github.com/asimmons91/pico-editor/internal/editor"
	"github.com/asimmons91/pico-editor/internal/input"
	"github.com/asimmons91/pico-editor/internal/lsp"
	"github.com/asimmons91/pico-editor/internal/vim"
)

func completionSession(t *testing.T, text string) (*Session, *editor.Editor) {
	t.Helper()
	buffer := editor.New("")
	buffer.Lines = []string{text}
	ed := editor.NewEditor(buffer)
	session := &Session{
		Buffers: []*SessionBuffer{{Editor: ed}},
		Current: 0,
		Vim:     vim.New(),
	}
	return session, ed
}

func TestAcceptCompletionAppliesMainAndAdditionalEdits(t *testing.T) {
	session, ed := completionSession(t, "abcdef")
	ed.SetCursor(editor.Pos{Line: 0, Rune: 4})
	session.Completion = &CompletionPopup{Items: []lsp.CompletionItem{{
		Label: "X",
		TextEdit: &lsp.TextEdit{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 1}, End: lsp.Position{Line: 0, Character: 4}},
			NewText: "X",
		},
		Additional: []lsp.TextEdit{{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 5}, End: lsp.Position{Line: 0, Character: 6}},
			NewText: "Y",
		}},
	}}}
	session.acceptCompletion()
	if got := ed.Buffer.Lines[0]; got != "aXeY" {
		t.Fatalf("line = %q, want aXeY", got)
	}
}

func TestAcceptCompletionAppliesMultipleAdditionalEditsDescending(t *testing.T) {
	session, ed := completionSession(t, "abcdef")
	ed.SetCursor(editor.Pos{Line: 0, Rune: 4})
	session.Completion = &CompletionPopup{Items: []lsp.CompletionItem{{
		Label: "M",
		TextEdit: &lsp.TextEdit{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 4}, End: lsp.Position{Line: 0, Character: 5}},
			NewText: "M",
		},
		Additional: []lsp.TextEdit{
			{
				Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 1}},
				NewText: "XX",
			},
			{
				Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 2}, End: lsp.Position{Line: 0, Character: 3}},
				NewText: "Y",
			},
		},
	}}}
	session.acceptCompletion()
	if got := ed.Buffer.Lines[0]; got != "XXbYdMf" {
		t.Fatalf("line = %q, want XXbYdMf", got)
	}
}

func TestCompletionDismissesOnCtrlW(t *testing.T) {
	session, ed := completionSession(t, "foo bar")
	ed.SetCursor(editor.Pos{Line: 0, Rune: 7})
	session.Vim.Mode = vim.ModeInsert
	session.Completion = &CompletionPopup{Items: []lsp.CompletionItem{{Label: "baz", TextEdit: &lsp.TextEdit{}}}}
	handled := session.handleLSPInput(input.Event{Kind: input.KindKeyDown, Key: input.KeyW, Mods: input.ModCtrl})
	if handled {
		t.Fatal("Ctrl+W was consumed instead of reaching Vim")
	}
	if session.Completion != nil {
		t.Fatal("completion popup survived Ctrl+W")
	}
}

func TestAcceptCompletionDropsOverlappingAdditionalEdit(t *testing.T) {
	session, ed := completionSession(t, "abcdef")
	ed.SetCursor(editor.Pos{Line: 0, Rune: 4})
	session.Completion = &CompletionPopup{Items: []lsp.CompletionItem{{
		Label: "X",
		TextEdit: &lsp.TextEdit{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 1}, End: lsp.Position{Line: 0, Character: 4}},
			NewText: "X",
		},
		Additional: []lsp.TextEdit{{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 3}, End: lsp.Position{Line: 0, Character: 5}},
			NewText: "Z",
		}},
	}}}
	session.acceptCompletion()
	if got := ed.Buffer.Lines[0]; got != "aXef" {
		t.Fatalf("line = %q, want aXef", got)
	}
	if session.Status == "" {
		t.Fatal("overlapping edit did not notify")
	}
}
