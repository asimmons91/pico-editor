package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryCommandHasDocumentation(t *testing.T) {
	game := New(Config{})
	keymapDoc, err := os.ReadFile(filepath.Join("..", "..", "docs", "KEYMAP.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(keymapDoc)
	for _, command := range game.commands {
		if !strings.Contains(text, "`"+command.ID+"`") {
			t.Errorf("command %q has no KEYMAP.md entry", command.ID)
		}
	}
	for _, name := range []string{
		"README.md", "docs/USAGE.md", "docs/CONFIG.md", "docs/KEYMAP.md",
		"docs/TROUBLESHOOTING.md", "docs/INPUT.md", "docs/VIM.md",
		"docs/images/cart-view.png", "docs/images/completion.png",
		"docs/images/picker.png", "docs/images/palette.png",
		"docs/images/error-dialog.png",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", name)); err != nil {
			t.Errorf("missing documentation %s: %v", name, err)
		}
	}
}
