package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanSortsAndSkips(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "zeta.lua"), "z=1\n")
	writeFile(t, filepath.Join(root, "Alpha.p8"), "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\na=1\n")
	writeFile(t, filepath.Join(root, "notes.txt"), "ignored\n")
	writeFile(t, filepath.Join(root, ".hidden.lua"), "hidden=1\n")
	writeFile(t, filepath.Join(root, "node_modules", "x.lua"), "x=1\n")
	writeFile(t, filepath.Join(root, "src", "b.lua"), "b=1\n")
	writeFile(t, filepath.Join(root, "src", "sub", "c.lua"), "c=1\n")

	workspace, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Entries) != 3 {
		t.Fatalf("top entries = %+v, want Alpha.p8, src, zeta.lua", workspace.Entries)
	}
	if workspace.Entries[0].Name != "src" || !workspace.Entries[0].Dir {
		t.Fatalf("first entry = %+v, want src directory", workspace.Entries[0])
	}
	if workspace.Entries[1].Name != "Alpha.p8" || workspace.Entries[2].Name != "zeta.lua" {
		t.Fatalf("file order = %q, %q", workspace.Entries[1].Name, workspace.Entries[2].Name)
	}
	if len(workspace.Carts) == 0 {
		t.Fatal("cart was not indexed")
	}
}

func TestIncludeOwnership(t *testing.T) {
	root := t.TempDir()
	util := filepath.Join(root, "lib", "util.lua")
	writeFile(t, util, "function util() end\n")
	cart := filepath.Join(root, "game.p8")
	writeFile(t, cart, "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n#include lib/util.lua\nprint(1)\n")
	standalone := filepath.Join(root, "standalone.lua")
	writeFile(t, standalone, "x=1\n")

	workspace, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := workspace.ResolveInclude(util)
	if !ok || ref.Path != cart {
		t.Fatalf("ResolveInclude = %+v %t, want %s", ref, ok, cart)
	}
	if got := workspace.Owners[util]; got != cart {
		t.Fatalf("Owners[util] = %q, want %q", got, cart)
	}
	if _, ok := workspace.ResolveInclude(standalone); ok {
		t.Fatal("standalone .lua acquired an owner")
	}
	if workspace.Carts[util] == nil {
		t.Fatal("include path was not indexed in Carts")
	}
}

func TestNearestCartWins(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared", "lib.lua")
	writeFile(t, shared, "x=1\n")
	writeFile(t, filepath.Join(root, "outer.p8"), "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n#include shared/lib.lua\n")
	writeFile(t, filepath.Join(root, "shared", "inner.p8"), "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n#include lib.lua\n")

	workspace, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := workspace.ResolveInclude(shared)
	if !ok {
		t.Fatal("shared include was not resolved")
	}
	if ref.Path != filepath.Join(root, "shared", "inner.p8") {
		t.Fatalf("owner = %q, want nearest shared cart", ref.Path)
	}
}

func TestChildCartOwnsParentInclude(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib.lua")
	writeFile(t, lib, "x=1\n")
	cart := filepath.Join(root, "src", "game.p8")
	writeFile(t, cart, "pico-8 cartridge // pico-8.com\nversion 42\n__lua__\n#include ../lib.lua\n")

	workspace, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := workspace.ResolveInclude(lib)
	if !ok || ref.Path != cart {
		t.Fatalf("ResolveInclude = %+v %t, want child cart %s", ref, ok, cart)
	}
	if got := workspace.Owners[lib]; got != cart {
		t.Fatalf("Owners[lib] = %q, want %q", got, cart)
	}
}

func TestRefreshPicksUpNewFiles(t *testing.T) {
	root := t.TempDir()
	workspace, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Entries) != 0 {
		t.Fatalf("entries = %+v, want none", workspace.Entries)
	}
	writeFile(t, filepath.Join(root, "new.lua"), "x=1\n")
	if err := workspace.Refresh(); err != nil {
		t.Fatal(err)
	}
	if len(workspace.Entries) != 1 || workspace.Entries[0].Name != "new.lua" {
		t.Fatalf("entries after refresh = %+v", workspace.Entries)
	}
}

func TestDirectorySymlinkNotFollowed(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	writeFile(t, filepath.Join(target, "x.lua"), "x=1\n")
	link := filepath.Join(root, "loop")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	workspace, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range workspace.Entries {
		if entry.Name == "loop" {
			t.Fatal("directory symlink was followed")
		}
	}
}

func TestFileSymlinkIncluded(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.lua")
	writeFile(t, target, "x=1\n")
	link := filepath.Join(root, "alias.lua")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	workspace, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range workspace.Entries {
		names = append(names, entry.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "alias.lua") {
		t.Fatalf("file symlink missing from %v", names)
	}
}

func TestOversizedCartIsNotRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "big.p8")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, maxCartScanSize+1); err != nil {
		t.Fatal(err)
	}
	workspace, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Carts[path] != nil {
		t.Fatal("oversized cart was indexed")
	}
	found := false
	for _, warning := range workspace.Warnings {
		if strings.Contains(warning, "larger than") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("warnings = %q, want an oversized-cart warning", workspace.Warnings)
	}
}

func TestInvalidRoot(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("Open(missing) succeeded")
	}
}
