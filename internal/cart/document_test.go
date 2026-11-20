package cart

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asimmons91/pico-editor/internal/editor"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, fixture(t, name), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenCodeAndTabs(t *testing.T) {
	doc, err := Open(copyFixture(t, "all_sections.p8"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.BaseLine != 3 {
		t.Fatalf("BaseLine = %d, want 3", doc.BaseLine)
	}
	if got := doc.Code.Text(); got != "-- main\nprint(1)\n-->8 second\n-- second\nfunction f() end\n" {
		t.Fatalf("code = %q", got)
	}
	want := []Tab{
		{Index: 0, Name: "Tab 1", StartLine: 0, EndLine: 2, MarkerLine: -1},
		{Index: 1, Name: "second", StartLine: 3, EndLine: 5, MarkerLine: 2},
	}
	if len(doc.Tabs) != len(want) {
		t.Fatalf("tabs = %+v", doc.Tabs)
	}
	for i := range want {
		if doc.Tabs[i] != want[i] {
			t.Fatalf("tab %d = %+v, want %+v", i, doc.Tabs[i], want[i])
		}
	}
}

func TestOpenLeadingMarkerTabs(t *testing.T) {
	doc, err := Open(copyFixture(t, "leading_marker.p8"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{
		{Index: 0, Name: "Tab 1", StartLine: 0, EndLine: 0, MarkerLine: -1},
		{Index: 1, Name: "first", StartLine: 1, EndLine: 2, MarkerLine: 0},
		{Index: 2, Name: "Tab 3", StartLine: 3, EndLine: 4, MarkerLine: 2},
	}
	for i := range want {
		if doc.Tabs[i] != want[i] {
			t.Fatalf("tab %d = %+v, want %+v", i, doc.Tabs[i], want[i])
		}
	}
}

func TestNoEditsRoundTripEveryFixture(t *testing.T) {
	fixtures := []string{
		"all_sections.p8", "blank_lines.p8", "bom.p8", "consecutive_markers.p8",
		"crlf.p8", "empty_body.p8", "indented_marker.p8", "leading_marker.p8",
		"lone_cr.p8", "no_trailing_newline.p8", "real_helloworld.p8",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			path := copyFixture(t, name)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := doc.Save(); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("save changed bytes\n got %q\nwant %q", after, before)
			}
		})
	}
}

func TestEditOnlyChangesCodeBody(t *testing.T) {
	path := copyFixture(t, "all_sections.p8")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	before := fixture(t, "all_sections.p8")
	if _, err := doc.Code.Apply(editor.Edit{
		Start: editor.Pos{Line: 0},
		End:   editor.Pos{Line: 0, Rune: 6},
		Text:  "-- changed",
	}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	prefix := before[:bytes.Index(before, []byte("-- main"))]
	if !bytes.HasPrefix(after, prefix) {
		t.Fatal("header bytes changed")
	}
	suffix := before[bytes.Index(before, []byte("__gfx__")):]
	if !bytes.HasSuffix(after, suffix) {
		t.Fatal("non-code sections changed")
	}
	if !bytes.Contains(after, []byte("-- changed")) {
		t.Fatal("edit was not saved")
	}
}

func TestCRLFRoundTrip(t *testing.T) {
	path := copyFixture(t, "crlf.p8")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Code.Newline != "\n" || doc.Code.Lines[0] != "-- main" {
		t.Fatalf("buffer = %q newline %q", doc.Code.Lines, doc.Code.Newline)
	}
	if _, err := doc.Code.Apply(editor.Edit{
		Start: editor.Pos{Line: 1},
		End:   editor.Pos{Line: 1, Rune: 8},
		Text:  "print(2)",
	}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(after, []byte("__lua__\r\n-- main\r\nprint(2)\r\n-->8 second\r\n")) {
		t.Fatalf("body did not keep CRLF: %q", after)
	}
	if !bytes.Contains(after, []byte("__gfx__\r\n0000\r\n")) {
		t.Fatal("suffix newline style changed")
	}
}

func TestNoCodeSectionRejected(t *testing.T) {
	_, err := Open(copyFixture(t, "no_lua.p8"))
	if !errors.Is(err, ErrNoCode) {
		t.Fatalf("err = %v, want ErrNoCode", err)
	}
}

func TestUnterminatedHeaderSaveFails(t *testing.T) {
	path := copyFixture(t, "unterminated_header.p8")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Code.Apply(editor.Edit{
		Start: editor.Pos{},
		End:   editor.Pos{},
		Text:  "x = 1",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("failed save changed the file")
	}
}

func TestPrepareSaveDetectsConflictAndBackup(t *testing.T) {
	path := copyFixture(t, "all_sections.p8")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Code.Apply(editor.Edit{Start: editor.Pos{}, End: editor.Pos{}, Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.PrepareSave(SaveOptions{}); !errors.Is(err, ErrExternalChange) {
		t.Fatalf("PrepareSave err = %v, want ErrExternalChange", err)
	}
	snapshot, err := doc.PrepareSave(SaveOptions{AllowExternalChange: true})
	if err != nil {
		t.Fatal(err)
	}
	backup, err := snapshot.Backup()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "external" {
		t.Fatalf("backup = %q, want external bytes", data)
	}
	if err := snapshot.Write(); err != nil {
		t.Fatal(err)
	}
	if err := doc.CommitSave(snapshot, nil); err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() {
		t.Fatal("cart stayed dirty after save")
	}
}

func TestDeletedCartReportsDeleted(t *testing.T) {
	path := copyFixture(t, "all_sections.p8")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.PrepareSave(SaveOptions{}); !errors.Is(err, ErrDeleted) {
		t.Fatalf("PrepareSave err = %v, want ErrDeleted", err)
	}
	snapshot, err := doc.PrepareSave(SaveOptions{AllowExternalChange: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Write(); err != nil {
		t.Fatal(err)
	}
	if err := doc.CommitSave(snapshot, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("recreated file: %v", err)
	}
}

func TestCartSymlinkOutsideRootNeedsPermission(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "game.p8")
	if err := os.WriteFile(target, fixture(t, "all_sections.p8"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.p8")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	doc, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	doc.SetWorkspaceRoot(root)
	if _, err := doc.PrepareSave(SaveOptions{}); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("PrepareSave err = %v, want ErrOutsideRoot", err)
	}
	if _, err := doc.PrepareSave(SaveOptions{AllowOutsideRoot: true}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was replaced: %v %v", info, err)
	}
}

func TestReadOnlyCartRefusesSave(t *testing.T) {
	path := copyFixture(t, "all_sections.p8")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.PrepareSave(SaveOptions{}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("PrepareSave err = %v, want ErrReadOnly", err)
	}
}

func TestSyncTabsAfterEdits(t *testing.T) {
	path := copyFixture(t, "all_sections.p8")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Code.Apply(editor.Edit{
		Start: editor.Pos{Line: 0},
		End:   editor.Pos{Line: 0},
		Text:  "new\nline\n",
	}); err != nil {
		t.Fatal(err)
	}
	if err := doc.SyncTabs(); err != nil {
		t.Fatal(err)
	}
	if doc.Tabs[0].EndLine != 4 {
		t.Fatalf("tab 0 end = %d, want 4", doc.Tabs[0].EndLine)
	}
	if doc.Tabs[1].StartLine != 5 || doc.Tabs[1].MarkerLine != 4 {
		t.Fatalf("tab 1 = %+v", doc.Tabs[1])
	}
}

func TestLoneCRInCodePreserved(t *testing.T) {
	path := copyFixture(t, "lone_cr.p8")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Code.Lines[0], "\r") {
		t.Fatalf("line = %q, want a data CR", doc.Code.Lines[0])
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(after, []byte("a\rb")) {
		t.Fatalf("saved cart lost the data CR: %q", after)
	}
}

func TestEmptyTabAtEOF(t *testing.T) {
	const original = "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=1\n-->8\n"
	path := filepath.Join(t.TempDir(), "trailing.p8")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Tabs) != 2 {
		t.Fatalf("tabs = %+v", doc.Tabs)
	}
	last := doc.Tabs[1]
	if last.StartLine != 2 || last.EndLine != 3 {
		t.Fatalf("last tab = %+v, want the synthetic blank line included", last)
	}
	if doc.Dirty() {
		t.Fatal("untouched cart opened dirty")
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Fatalf("untouched save = %q, want %q", after, original)
	}
	if _, err := doc.Code.Apply(editor.Edit{
		Start: editor.Pos{Line: 2},
		End:   editor.Pos{Line: 2},
		Text:  "hello",
	}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(after, []byte("-->8\nhello\n")) {
		t.Fatalf("edit in trailing tab = %q", after)
	}
}
