package editor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
