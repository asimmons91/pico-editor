package editor

import (
	"errors"
	"testing"
)

func TestSearchLiteralSmartCase(t *testing.T) {
	t.Parallel()

	text := "Hello hello HELLO\n"
	search := Search{Pattern: "hello", SmartCase: true, Forward: true}
	match, ok, err := search.Find(text, Pos{})
	if err != nil || !ok {
		t.Fatalf("Find = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 0}) {
		t.Errorf("first match = %v, want 0,0", match.Start)
	}
	match, ok, err = search.Find(text, Pos{Line: 0, Rune: 1})
	if err != nil || !ok {
		t.Fatalf("Find from 1 = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 0, Rune: 6}) {
		t.Errorf("second match = %v, want 0,6", match.Start)
	}

	search.Pattern = "Hello"
	match, ok, err = search.Find(text, Pos{Line: 0, Rune: 1})
	if err != nil || !ok {
		t.Fatalf("Find uppercase = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{}) || !match.Wrapped {
		t.Errorf("uppercase wrap = %v wrapped=%t, want origin and wrapped", match.Start, match.Wrapped)
	}

	search.Pattern = "hello"
	search.SmartCase = false
	match, ok, err = search.Find(text, Pos{})
	if err != nil || !ok {
		t.Fatalf("case-sensitive Find = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 0, Rune: 6}) {
		t.Errorf("case-sensitive match = %v, want the lowercase hello at 0,6", match.Start)
	}
}

func TestSearchRegex(t *testing.T) {
	t.Parallel()

	text := "spr(1)\nspr(22)\n"
	search := Search{Pattern: `spr\((\d+)\)`, Regex: true, Forward: true}
	match, ok, err := search.Find(text, Pos{Line: 1})
	if err != nil || !ok {
		t.Fatalf("Find = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 1}) || match.End != (Pos{Line: 1, Rune: 7}) {
		t.Errorf("regex match = %v..%v, want 1,0..1,7", match.Start, match.End)
	}
}

func TestSearchWrapsBothDirections(t *testing.T) {
	t.Parallel()

	text := "one two one\n"
	search := Search{Pattern: "one", SmartCase: true, Forward: false}
	match, ok, err := search.Find(text, Pos{})
	if err != nil || !ok {
		t.Fatalf("backward Find = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 0, Rune: 8}) || !match.Wrapped {
		t.Errorf("backward wrap = %v wrapped=%t, want 0,8 wrapped", match.Start, match.Wrapped)
	}

	search.Forward = true
	match, ok, err = search.Find(text, Pos{Line: 0, Rune: 9})
	if err != nil || !ok {
		t.Fatalf("forward Find = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{}) || !match.Wrapped {
		t.Errorf("forward wrap = %v wrapped=%t, want origin wrapped", match.Start, match.Wrapped)
	}
}

func TestSearchExpandsClusterBoundaries(t *testing.T) {
	t.Parallel()

	text := "a⬅️b\n"
	search := Search{Pattern: "⬅", SmartCase: true, Forward: true}
	match, ok, err := search.Find(text, Pos{})
	if err != nil || !ok {
		t.Fatalf("Find base rune = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 0, Rune: 1}) || match.End != (Pos{Line: 0, Rune: 3}) {
		t.Errorf("base rune match = %v..%v, want the whole arrow cluster", match.Start, match.End)
	}

	search.Pattern = "\ufe0f"
	match, ok, err = search.Find(text, Pos{})
	if err != nil || !ok {
		t.Fatalf("Find VS16 = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 0, Rune: 1}) || match.End != (Pos{Line: 0, Rune: 3}) {
		t.Errorf("VS16 match = %v..%v, want the whole arrow cluster", match.Start, match.End)
	}
}

func TestSearchInvalidPatternPreservesSearch(t *testing.T) {
	t.Parallel()

	search := Search{Pattern: "[", Regex: true, Forward: true}
	_, _, err := search.Find("text", Pos{})
	if !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("invalid regex error = %v, want ErrInvalidPattern", err)
	}
	if search.Pattern != "[" {
		t.Errorf("Find changed the pattern to %q", search.Pattern)
	}

	search.Pattern = "text"
	if _, ok, err := search.Find("text", Pos{}); err != nil || !ok {
		t.Errorf("previous pattern no longer works: ok=%t err=%v", ok, err)
	}
}

func TestSearchEmptyPatternAndMultiline(t *testing.T) {
	t.Parallel()

	search := Search{Pattern: "", Forward: true}
	if _, ok, err := search.Find("abc", Pos{}); ok || err != nil {
		t.Errorf("empty pattern = ok=%t err=%v, want no match", ok, err)
	}

	search.Pattern = "a\nb"
	match, ok, err := search.Find("a\nb\n", Pos{})
	if err != nil || !ok {
		t.Fatalf("multiline Find = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{}) || match.End != (Pos{Line: 1, Rune: 1}) {
		t.Errorf("multiline match = %v..%v", match.Start, match.End)
	}
}

func TestSearchClampsTrailingNewline(t *testing.T) {
	t.Parallel()

	search := Search{Pattern: "\n", SmartCase: true, Forward: true}
	match, ok, err := search.Find("a\n", Pos{})
	if err != nil || !ok {
		t.Fatalf("Find newline = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 0, Rune: 1}) || match.End != (Pos{Line: 0, Rune: 1}) {
		t.Errorf("trailing newline match = %v..%v, want the last line's end", match.Start, match.End)
	}
}

func TestSearchFindIn(t *testing.T) {
	t.Parallel()

	buffer := New("")
	buffer.Lines = []string{"one", "two"}
	search := Search{Pattern: "two", SmartCase: true, Forward: true}
	match, ok, err := search.FindIn(buffer, Pos{Line: 0, Rune: 2})
	if err != nil || !ok {
		t.Fatalf("FindIn = %v, %t, %v", match, ok, err)
	}
	if match.Start != (Pos{Line: 1}) {
		t.Errorf("FindIn match = %v, want 1,0", match.Start)
	}
}
