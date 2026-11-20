package app

import (
	"fmt"
	"strings"

	"github.com/asimmons91/pico-editor/internal/font"
	"github.com/asimmons91/pico-editor/internal/lsp"
)

// statusData is the pure input to the status bar layout. Keeping it separate
// from Session makes truncation testable at 20, 40, and 64 cells.
type statusData struct {
	mode         string
	inputMode    string
	file         string
	dirty        bool
	line         int
	column       int
	errors       int
	warnings     int
	deprecated   bool
	sizes        string
	sizeColor    uint8
	zColor       uint8
	lspStatus    lsp.Status
	message      string
	messageError bool
}

type statusPart struct {
	segment statusSegment
	drop    int // larger drops first; 0 never
	file    bool
}

func buildStatusParts(data statusData) []statusPart {
	parts := make([]statusPart, 0, 10)
	mode := data.mode
	if data.inputMode != "" {
		mode += " " + data.inputMode
	}
	parts = append(parts, statusPart{segment: statusSegment{mode, 10}, drop: 0})
	if data.file != "" {
		text := data.file
		if data.dirty {
			text += "*"
		}
		parts = append(parts, statusPart{segment: statusSegment{text, 7}, file: true})
	}
	parts = append(parts, statusPart{segment: statusSegment{fmt.Sprintf("%d:%d", data.line, data.column), 7}, drop: 0})
	if data.deprecated {
		parts = append(parts, statusPart{segment: statusSegment{"!", 8}, drop: 3})
	}
	if data.errors > 0 {
		parts = append(parts, statusPart{segment: statusSegment{fmt.Sprintf("E%d", data.errors), 8}, drop: 3})
	}
	if data.warnings > 0 {
		parts = append(parts, statusPart{segment: statusSegment{fmt.Sprintf("W%d", data.warnings), 9}, drop: 3})
	}
	if data.sizes != "" {
		if index := strings.LastIndex(data.sizes, " Z"); index > 0 {
			parts = append(parts, statusPart{segment: statusSegment{data.sizes[:index], 7}, drop: 4})
			parts = append(parts, statusPart{segment: statusSegment{data.sizes[index+1:], data.zColor}, drop: 4})
		} else {
			parts = append(parts, statusPart{segment: statusSegment{data.sizes, data.sizeColor}, drop: 4})
		}
	}
	switch data.lspStatus {
	case lsp.StatusFailed:
		parts = append(parts, statusPart{segment: statusSegment{"LSP!", 8}, drop: 0})
	case lsp.StatusReady:
		parts = append(parts, statusPart{segment: statusSegment{"LSP", 11}, drop: 2})
	case lsp.StatusStarting:
		parts = append(parts, statusPart{segment: statusSegment{"LSP...", 10}, drop: 2})
	}
	if data.message != "" {
		color := uint8(7)
		if data.messageError {
			color = 8
		}
		parts = append(parts, statusPart{segment: statusSegment{data.message, color}, drop: 0})
	}
	return parts
}

func statusPartsCells(parts []statusPart) int {
	if len(parts) == 0 {
		return 0
	}
	cells := 0
	for index, part := range parts {
		if index > 0 {
			cells += 2
		}
		cells += font.CellWidth(part.segment.text)
	}
	return cells
}

// layoutStatus applies the documented drop order: sizes, diagnostics, LSP
// ready, then file path. Mode, position, and LSP failure are never dropped. A
// notification temporarily replaces sizes/diagnostics/LSP-ready so it can be
// read in the rightmost cells.
func layoutStatus(width int, data statusData) []statusSegment {
	parts := buildStatusParts(data)
	if data.message != "" {
		kept := parts[:0]
		for _, part := range parts {
			if part.drop >= 2 {
				continue
			}
			kept = append(kept, part)
		}
		parts = kept
	}
	for statusPartsCells(parts) > width {
		drop := 0
		for _, part := range parts {
			if part.drop > drop {
				drop = part.drop
			}
		}
		if drop == 0 {
			break
		}
		kept := parts[:0]
		for _, part := range parts {
			if part.drop == drop {
				continue
			}
			kept = append(kept, part)
		}
		parts = kept
	}
	if statusPartsCells(parts) > width {
		fileIndex := -1
		for index, part := range parts {
			if part.file {
				fileIndex = index
				break
			}
		}
		if fileIndex >= 0 {
			without := make([]statusPart, 0, len(parts)-1)
			without = append(without, parts[:fileIndex]...)
			without = append(without, parts[fileIndex+1:]...)
			remaining := width - statusPartsCells(without) - 2
			if remaining >= 6 {
				parts[fileIndex].segment.text = clipMiddle(parts[fileIndex].segment.text, remaining)
			} else {
				parts = without
			}
		}
	}
	if statusPartsCells(parts) > width && data.message != "" {
		// The message is always the last part; clip it to what remains.
		messageIndex := len(parts) - 1
		used := statusPartsCells(parts[:messageIndex])
		remaining := width - used
		if used > 0 {
			remaining -= 2
		}
		parts[messageIndex].segment.text = clipMiddle(parts[messageIndex].segment.text, remaining)
	}
	segments := make([]statusSegment, 0, len(parts))
	for _, part := range parts {
		if part.segment.text == "" {
			continue
		}
		segments = append(segments, part.segment)
	}
	return segments
}

// statusText joins segments with the two-cell inter-segment gap used by draw.
func statusText(segments []statusSegment) string {
	texts := make([]string, 0, len(segments))
	for _, segment := range segments {
		texts = append(texts, segment.text)
	}
	return strings.Join(texts, "  ")
}

// clipMiddle shortens text to limit cells, inserting an ellipsis in the middle.
// It never splits a P8SCII cluster.
func clipMiddle(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if font.CellWidth(text) <= limit {
		return text
	}
	ellipsis := "…"
	ellipsisCells := font.CellWidth(ellipsis)
	if limit < ellipsisCells {
		return clipCells(ellipsis, limit)
	}
	runes := []rune(text)
	left := (limit - ellipsisCells) / 2
	right := limit - ellipsisCells - left
	head := clipCells(string(runes), left)
	tailText := string(runes[max(0, len(runes)-right):])
	tail := clipCells(tailText, right)
	if head == "" && tail == "" {
		return clipCells(ellipsis, limit)
	}
	return head + ellipsis + tail
}
