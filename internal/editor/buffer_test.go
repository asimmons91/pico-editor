package editor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asimmons91/pico-editor/internal/fsutil"
)

func writeTestFile(t *testing.T, dir, name string, data []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	return path
}

func TestOpenSaveRoundTrips(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tests := []struct {
		name string
		data []byte
	}{
		{name: "lf trailing", data: []byte("print(1)\nprint(2)\n")},
		{name: "lf no trailing", data: []byte("print(1)\nprint(2)")},
		{name: "crlf trailing", data: []byte("print(1)\r\nprint(2)\r\n")},
		{name: "crlf no trailing", data: []byte("print(1)\r\nprint(2)")},
		{name: "empty", data: []byte{}},
		{name: "only newline", data: []byte("\n")},
		{name: "bom", data: append([]byte{0xef, 0xbb, 0xbf}, []byte("print(1)\r\n")...)},
		{name: "lone cr", data: []byte("a\rb\n")},
		{name: "crlf content cr", data: []byte("a\r\r\nb\r\n")},
		{name: "p8scii", data: []byte("spr(♥,⬅️,😐️)\n")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := writeTestFile(t, dir, test.name+".lua", test.data, 0o640)
			buffer, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if buffer.Dirty() {
				t.Error("newly opened buffer is dirty")
			}
			if err := buffer.Save(); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if !bytes.Equal(got, test.data) {
				t.Errorf("round-trip bytes = %q, want %q", got, test.data)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if got := info.Mode().Perm(); got != 0o640 {
				t.Errorf("mode = %o, want 640", got)
			}
		})
	}
}

func TestOpenRejectsInvalidFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	t.Run("invalid utf8", func(t *testing.T) {
		path := writeTestFile(t, dir, "bad.lua", []byte{0xff, 0xfe}, 0o644)
		_, err := Open(path)
		if !errors.Is(err, ErrNotUTF8) {
			t.Errorf("Open error = %v, want ErrNotUTF8", err)
		}
	})
	t.Run("mixed newlines", func(t *testing.T) {
		path := writeTestFile(t, dir, "mixed.lua", []byte("a\r\nb\n"), 0o644)
		_, err := Open(path)
		if !errors.Is(err, ErrMixedNewlines) {
			t.Errorf("Open error = %v, want ErrMixedNewlines", err)
		}
	})
	t.Run("too large", func(t *testing.T) {
		data := bytes.Repeat([]byte("a"), maxFileSize+1)
		path := writeTestFile(t, dir, "large.lua", data, 0o644)
		_, err := Open(path)
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("Open error = %v, want ErrTooLarge", err)
		}
	})
	t.Run("directory", func(t *testing.T) {
		if _, err := Open(dir); err == nil {
			t.Error("Open(directory) succeeded, want an error")
		}
	})
	t.Run("cart", func(t *testing.T) {
		path := writeTestFile(t, dir, "cart.p8", []byte("__lua__\nprint(1)\n"), 0o644)
		_, err := Open(path)
		if !errors.Is(err, ErrUnsupportedFile) {
			t.Errorf("Open(.p8) error = %v, want ErrUnsupportedFile", err)
		}
	})
}

func TestEditedCRLFAndBOMRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tests := []struct {
		name string
		data []byte
		want []byte
	}{
		{
			name: "bom crlf trailing",
			data: append([]byte{0xef, 0xbb, 0xbf}, []byte("one\r\ntwo\r\n")...),
			want: append([]byte{0xef, 0xbb, 0xbf}, []byte("one\r\ntwo!\r\n")...),
		},
		{
			name: "crlf no trailing newline",
			data: []byte("one\r\ntwo"),
			want: []byte("one\r\ntwo!"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := writeTestFile(t, dir, test.name+".lua", test.data, 0o644)
			buffer, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			editor := NewEditor(buffer)
			editor.MoveBufferEnd()
			if _, err := editor.Insert("!"); err != nil {
				t.Fatalf("Insert: %v", err)
			}
			if err := buffer.Save(); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if !bytes.Equal(got, test.want) {
				t.Errorf("edited round-trip = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDeletedWritableFileKeepsMode(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "private.lua", []byte("print(1)\n"), 0o600)
	buffer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	snapshot, err := buffer.PrepareSaveWithOptions(SaveOptions{AllowExternalChange: true})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode.Perm() != 0o600 {
		t.Fatalf("recreate mode = %o, want preserved 0600", snapshot.Mode.Perm())
	}
}

func TestConflictOverwriteUsesCurrentFileMode(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "mode.lua", []byte("print(1)\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.PrepareSaveWithOptions(SaveOptions{AllowExternalChange: true}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("read-only overwrite err = %v, want ErrReadOnly", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := buffer.PrepareSaveWithOptions(SaveOptions{AllowExternalChange: true})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode.Perm() != 0o600 {
		t.Fatalf("snapshot mode = %o, want current 0600", snapshot.Mode.Perm())
	}
}

func TestDeletedReadOnlyFileCanBeRecreated(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "readonly.lua", []byte("print(1)\n"), 0o444)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	snapshot, err := buffer.PrepareSaveWithOptions(SaveOptions{AllowExternalChange: true})
	if err != nil {
		t.Fatalf("PrepareSaveWithOptions: %v", err)
	}
	if snapshot.Mode.Perm() != 0o644 {
		t.Fatalf("recreate mode = %o, want 0644", snapshot.Mode.Perm())
	}
	if err := snapshot.Write(); err != nil {
		t.Fatal(err)
	}
	if err := buffer.CommitSave(snapshot, nil); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("recreated file = %v %v", info, err)
	}
}

func TestSaveRejectsReadOnlyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "readonly.lua", []byte("print(1)\n"), 0o444)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := buffer.Save(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Save error = %v, want ErrReadOnly", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "print(1)\n" {
		t.Errorf("read-only file changed to %q", data)
	}
	if !buffer.Dirty() {
		t.Error("failed save cleared the dirty flag")
	}
}

func TestSaveDetectsExternalChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "external.lua", []byte("print(1)\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := os.WriteFile(path, []byte("external change\n"), 0o644); err != nil {
		t.Fatalf("external write: %v", err)
	}
	if err := buffer.Save(); !errors.Is(err, ErrExternalChange) {
		t.Fatalf("Save error = %v, want ErrExternalChange", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "external change\n" {
		t.Errorf("external contents were overwritten: %q", data)
	}
}

func TestSaveDetectsExternalModeChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "mode.lua", []byte("print(1)\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := buffer.Save(); !errors.Is(err, ErrExternalChange) {
		t.Fatalf("Save error = %v, want ErrExternalChange after chmod", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "print(1)\n" {
		t.Errorf("read-only file was overwritten: %q", data)
	}
}

func TestSaveDetectsExternalTouch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "touch.lua", []byte("print(1)\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	touched := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(path, touched, touched); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := buffer.Save(); !errors.Is(err, ErrExternalChange) {
		t.Fatalf("Save error = %v, want ErrExternalChange after touch", err)
	}
}

func TestSaveDetectsExternalSameSizeRewrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "rewrite.lua", []byte("aaaa\n"), 0o644)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := os.WriteFile(path, []byte("bbbb\n"), 0o644); err != nil {
		t.Fatalf("external rewrite: %v", err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("restore mtime: %v", err)
	}
	if err := buffer.Save(); !errors.Is(err, ErrExternalChange) {
		t.Fatalf("Save error = %v, want ErrExternalChange from the hash check", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "bbbb\n" {
		t.Errorf("external rewrite was overwritten: %q", data)
	}
}

func TestSaveDetectsDeletedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "deleted.lua", []byte("print(1)\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := buffer.Save(); !errors.Is(err, ErrExternalChange) {
		t.Fatalf("Save error = %v, want ErrExternalChange after deletion", err)
	}
}

func TestSaveRejectsAmbiguousCR(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "cr.lua", []byte("a\rb\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveLineEnd()
	if _, err := editor.DeleteClusterBefore(); err != nil {
		t.Fatalf("DeleteClusterBefore: %v", err)
	}
	if got := buffer.Lines[0]; got != "a\r" {
		t.Fatalf("line = %q, want a trailing CR", got)
	}
	if err := buffer.Save(); !errors.Is(err, ErrAmbiguousCR) {
		t.Fatalf("Save error = %v, want ErrAmbiguousCR", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "a\rb\n" {
		t.Errorf("ambiguous save changed the file to %q", data)
	}

	editor.MoveBufferEnd()
	if _, err := editor.Insert("x"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := buffer.Save(); err != nil {
		t.Fatalf("Save after resolving ambiguity: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "a\rx\n" {
		t.Errorf("resolved file = %q, want a\\rx\\n", data)
	}
}

func TestOpenRejectsSymlinkToCart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cart := writeTestFile(t, dir, "cart.p8", []byte("__lua__\nprint(1)\n"), 0o644)
	link := filepath.Join(dir, "link.lua")
	if err := os.Symlink(cart, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := Open(link); !errors.Is(err, ErrUnsupportedFile) {
		t.Fatalf("Open(.lua -> cart.p8) error = %v, want ErrUnsupportedFile", err)
	}
}

func TestSaveRejectsCartPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "new.p8")
	buffer := New(path)
	editor := NewEditor(buffer)
	if _, err := editor.Insert("print(1)\n"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := buffer.Save(); !errors.Is(err, ErrUnsupportedFile) {
		t.Fatalf("Save(.p8) error = %v, want ErrUnsupportedFile", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cart file was created: %v", err)
	}
}

func TestOpenRejectsNonRegularFile(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("/dev/null"); err != nil {
		t.Skip("/dev/null is unavailable")
	}
	if _, err := Open("/dev/null"); !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("Open(/dev/null) error = %v, want ErrNotRegularFile", err)
	}
}

func TestSaveThroughSymlinkedParent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	realPath := writeTestFile(t, realDir, "file.lua", []byte("print(1)\n"), 0o644)
	linkDir := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	path := filepath.Join(linkDir, "file.lua")
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open through symlinked parent: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := buffer.Save(); err != nil {
		t.Fatalf("Save through symlinked parent: %v", err)
	}
	data, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("read real file: %v", err)
	}
	if string(data) != "print(1)!\n" {
		t.Errorf("real file = %q, want the saved edit", data)
	}
}

func TestFinishWriteAfterCommitError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "commit.lua", []byte("one\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("two"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	data := buffer.encode()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("simulate committed write: %v", err)
	}
	err = buffer.finishWrite(path, data, &fsutil.CommitError{Err: errors.New("sync failed")})
	if err == nil {
		t.Fatal("finishWrite accepted a commit error without reporting it")
	}
	if buffer.Dirty() {
		t.Error("committed bytes left the buffer dirty")
	}
	if !buffer.disk.known {
		t.Error("committed bytes left the disk state unknown")
	}
	if err := buffer.Save(); err != nil {
		t.Fatalf("retry after commit error: %v", err)
	}
}

func TestMarkSavedAfterDirectLinesAssignment(t *testing.T) {
	t.Parallel()

	buffer := New("")
	buffer.Lines = []string{"one", "two"}
	buffer.TrailingNewline = true
	buffer.MarkSaved()
	if buffer.Dirty() {
		t.Fatal("MarkSaved did not adopt directly assigned lines")
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !buffer.Dirty() {
		t.Fatal("buffer is not dirty after an edit")
	}
	editor.Undo()
	if buffer.Dirty() {
		t.Error("undo to the adopted save point left the buffer dirty")
	}
}

func TestEditorReloadClearsHistoryAndClampsCursor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "reload_editor.lua", []byte("one\ntwo\nthree\nfour\nfive\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.SetCursor(Pos{Line: 4, Rune: 2})
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := os.WriteFile(path, []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("external write: %v", err)
	}
	if err := editor.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if editor.CanUndo() || editor.CanRedo() {
		t.Error("Reload kept undo history")
	}
	if got := buffer.Text(); got != "alpha\n" {
		t.Errorf("reloaded text = %q", got)
	}
	editor.MoveLineEnd()
	if editor.Cursor.Pos.Line != 0 {
		t.Errorf("cursor line = %d, want 0", editor.Cursor.Pos.Line)
	}
	if buffer.Dirty() {
		t.Error("Reload left the buffer dirty")
	}
}

func TestEditorReloadDiscardsStaleUndo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "reload_undo.lua", []byte("aaaa\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("b"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := os.WriteFile(path, []byte("abcdef\n"), 0o644); err != nil {
		t.Fatalf("external write: %v", err)
	}
	if err := editor.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if _, ok := editor.Undo(); ok {
		t.Fatal("Undo after Reload applied a pre-reload change")
	}
	if got := buffer.Text(); got != "abcdef\n" {
		t.Errorf("text after rejected undo = %q", got)
	}
}

func TestSaveThroughSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	realPath := writeTestFile(t, dir, "real.lua", []byte("print(1)\n"), 0o644)
	linkPath := filepath.Join(dir, "link.lua")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	buffer, err := Open(linkPath)
	if err != nil {
		t.Fatalf("Open symlink: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := buffer.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("save replaced the symlink with a regular file")
	}
	data, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "print(1)!\n" {
		t.Errorf("target contents = %q, want the saved edit", data)
	}
}

func TestSaveRefusesSymlinkOutsideRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	realPath := writeTestFile(t, dir, "real.lua", []byte("print(1)\n"), 0o644)
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	linkPath := filepath.Join(subdir, "link.lua")
	if err := os.Symlink(filepath.Join("..", "real.lua"), linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	buffer, err := Open(linkPath)
	if err != nil {
		t.Fatalf("Open symlink: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("!"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := buffer.Save(); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("Save error = %v, want ErrOutsideRoot", err)
	}
	data, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "print(1)\n" {
		t.Errorf("outside-root target was modified: %q", data)
	}
}

func TestNewSavesUnnamedPath(t *testing.T) {
	t.Parallel()

	buffer := New("")
	if err := buffer.Save(); !errors.Is(err, ErrNoPath) {
		t.Fatalf("Save without path = %v, want ErrNoPath", err)
	}

	path := filepath.Join(t.TempDir(), "new.lua")
	buffer = New(path)
	editor := NewEditor(buffer)
	if _, err := editor.Insert("print(1)\n"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := buffer.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "print(1)\n" {
		t.Errorf("new file = %q, want print(1)\\n", data)
	}
}

func TestReloadDiscardsChanges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "reload.lua", []byte("one\n"), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	editor := NewEditor(buffer)
	editor.MoveBufferEnd()
	if _, err := editor.Insert("two"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !buffer.Dirty() {
		t.Fatal("buffer is not dirty after an edit")
	}
	if err := buffer.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if buffer.Dirty() {
		t.Error("Reload left the buffer dirty")
	}
	if got := buffer.Text(); got != "one\n" {
		t.Errorf("Reload text = %q, want one\\n", got)
	}
}

func TestLargeFileSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "large.lua", []byte(strings.Repeat("print(1)\n", 8000)), 0o644)
	buffer, err := Open(path)
	if err != nil {
		t.Fatalf("Open large file: %v", err)
	}
	editor := NewEditor(buffer)
	original := buffer.Text()
	for i := 0; i < 400; i++ {
		line := (i * 37) % len(buffer.Lines)
		editor.SetCursor(Pos{Line: line, Rune: i % 5})
		if _, err := editor.Insert("x"); err != nil {
			t.Fatalf("Insert in large file: %v", err)
		}
		if _, err := editor.DeleteClusterBefore(); err != nil {
			t.Fatalf("Delete in large file: %v", err)
		}
	}
	if got := buffer.Text(); got != original {
		t.Fatal("large-file edit session corrupted the text")
	}
	editor.MoveBufferStart()
	if _, err := editor.Insert("-- end\n"); err != nil {
		t.Fatalf("final insert: %v", err)
	}
	if err := editor.Save(); err != nil {
		t.Fatalf("Save large file: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen large file: %v", err)
	}
	if got := reopened.Text(); got != "-- end\n"+original {
		t.Error("large-file save/reopen did not preserve the session")
	}
}

func FuzzSaveRoundTrip(f *testing.F) {
	f.Add([]byte("print(1)\n"))
	f.Add([]byte("a\r\nb\r\n"))
	f.Add([]byte("a\rb\n"))
	f.Add([]byte{0xef, 0xbb, 0xbf, 'a', '\n'})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "fuzz.lua")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		buffer, err := Open(path)
		if errors.Is(err, ErrMixedNewlines) || errors.Is(err, ErrTooLarge) || errors.Is(err, ErrNotUTF8) {
			t.Skip()
		}
		if err != nil {
			t.Fatalf("Open valid input: %v", err)
		}
		if err := buffer.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("round-trip bytes = %q, want %q", got, data)
		}
	})
}
