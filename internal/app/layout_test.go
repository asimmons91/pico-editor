package app

import "testing"

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
