package version

import "testing"

func TestStringDevelopmentBuild(t *testing.T) {
	t.Parallel()

	got := String()
	want := "pico-editor dev (unknown, unknown)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestFormatInjectedBuild(t *testing.T) {
	t.Parallel()

	got := format("1.2.3", "abc1234", "2026-11-19T12:00:00Z")
	want := "pico-editor 1.2.3 (abc1234, 2026-11-19T12:00:00Z)"
	if got != want {
		t.Errorf("format() = %q, want %q", got, want)
	}
}
