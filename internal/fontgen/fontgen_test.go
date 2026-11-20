package fontgen

import (
	"bytes"
	"image/png"
	"strconv"
	"strings"
	"testing"
)

func fullData(t *testing.T) []byte {
	t.Helper()
	var out strings.Builder
	out.WriteString("# comment\n")
	for b := 0; b < 256; b++ {
		width := 4
		if b >= 0x80 {
			width = 8
		}
		rows := []string{"00", "00", "00", "00", "00", "00"}
		if b == 0x41 {
			rows = []string{"00", "06", "05", "07", "05", "00"}
		}
		out.WriteString(strings.ToLower(strings.TrimSpace(
			formatRecord(b, width, rows),
		)) + "\n")
	}
	return []byte(out.String())
}

func formatRecord(b, width int, rows []string) string {
	return strings.TrimSpace(
		strings.Join([]string{
			twoHex(b), strings.TrimSpace(strconv.Itoa(width)),
			rows[0], rows[1], rows[2], rows[3], rows[4], rows[5],
		}, " "),
	)
}

func twoHex(v int) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[(v>>4)&0xf], digits[v&0xf]})
}

func TestParseValidData(t *testing.T) {
	records, err := Parse(fullData(t))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := records[0x41]; got.Width != 4 || got.Rows[3] != 0x07 {
		t.Errorf("records[0x41] = %+v, want width 4 and row 3 = 0x07", got)
	}
	if got := records[0x80]; got.Width != 8 {
		t.Errorf("records[0x80].Width = %d, want 8", got.Width)
	}
}

func TestParseRejectsBadData(t *testing.T) {
	base := string(fullData(t))
	cases := []struct {
		name string
		data string
		want string
	}{
		{"missing", strings.Replace(base, "41 4 00 06 05 07 05 00\n", "", 1), "missing byte 0x41"},
		{"duplicate", base + "41 4 00 06 05 07 05 00\n", "duplicate byte 0x41"},
		{"field count", base + "42 4 00 00\n", "got 4 fields"},
		{"bad hex", strings.Replace(base, "42 4 00 00 00 00 00 00", "42 4 00 00 00 00 zz 00", 1), "not a 8-bit hexadecimal"},
		{"bad width", strings.Replace(base, "41 4 00 06 05 07 05 00", "41 5 00 06 05 07 05 00", 1), "width = 5"},
		{"wide in narrow range", strings.Replace(base, "41 4 00 06 05 07 05 00", "41 8 00 06 05 07 05 00", 1), "width 8 is invalid"},
		{"ink outside", strings.Replace(base, "41 4 00 06 05 07 05 00", "41 4 00 16 05 07 05 00", 1), "ink outside"},
		{"control ink", strings.Replace(base, "00 4 00 00 00 00 00 00", "00 4 01 00 00 00 00 00", 1), "must be blank"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.data)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse() error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestGenerateGoDeterministic(t *testing.T) {
	records, err := Parse(fullData(t))
	if err != nil {
		t.Fatal(err)
	}
	first := GenerateGo(records)
	second := GenerateGo(records)
	if !bytes.Equal(first, second) {
		t.Fatal("GenerateGo is not deterministic")
	}
	for _, want := range []string{
		"package font",
		"var glyphs = [256]Glyph{",
		"{Width: 4, Rows: [6]uint8{0x00, 0x06, 0x05, 0x07, 0x05, 0x00}},",
	} {
		if !bytes.Contains(first, []byte(want)) {
			t.Errorf("GenerateGo output does not contain %q", want)
		}
	}
}

func TestGeneratePNG(t *testing.T) {
	records, err := Parse(fullData(t))
	if err != nil {
		t.Fatal(err)
	}
	data, err := GeneratePNG(records)
	if err != nil {
		t.Fatalf("GeneratePNG() error = %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	if got := img.Bounds().Size(); got.X != 128 || got.Y != 128 {
		t.Errorf("atlas size = %v, want 128x128", got)
	}
	if second, _ := GeneratePNG(records); !bytes.Equal(data, second) {
		t.Error("GeneratePNG is not deterministic")
	}
}
