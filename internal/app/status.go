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
	percent      int
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
	right   bool
	block   bool
}

func buildStatusParts(data statusData) []statusPart {
	parts := make([]statusPart, 0, 11)
	mode := data.mode
	if data.inputMode != "" {
		mode += " " + data.inputMode
	}
	parts = append(parts, statusPart{segment: statusSegment{mode, statusForeground}, drop: 0, block: true})
	if data.file != "" {
		text := data.file
		if data.dirty {
			text += "*"
		}
		parts = append(parts, statusPart{segment: statusSegment{text, statusSecondary}, file: true})
	}
	if data.deprecated {
		parts = append(parts, statusPart{segment: statusSegment{"!", statusErrorForeground}, drop: 3})
	}
	if data.errors > 0 {
		parts = append(parts, statusPart{segment: statusSegment{fmt.Sprintf("E%d", data.errors), statusErrorForeground}, drop: 3})
	}
	if data.warnings > 0 {
		parts = append(parts, statusPart{segment: statusSegment{fmt.Sprintf("W%d", data.warnings), statusWarningForeground}, drop: 3})
	}
	parts = append(parts, statusPart{segment: statusSegment{fmt.Sprintf("%d:%d", data.line, data.column), statusForeground}, drop: 0, right: true})
	if data.sizes != "" {
		if index := strings.LastIndex(data.sizes, " Z"); index > 0 {
			parts = append(parts, statusPart{segment: statusSegment{data.sizes[:index], statusForeground}, drop: 4, right: true})
			parts = append(parts, statusPart{segment: statusSegment{data.sizes[index+1:], data.zColor}, drop: 4, right: true})
		} else {
			parts = append(parts, statusPart{segment: statusSegment{data.sizes, data.sizeColor}, drop: 4, right: true})
		}
	}
	switch data.lspStatus {
	case lsp.StatusFailed:
		parts = append(parts, statusPart{segment: statusSegment{"LSP!", statusErrorForeground}, drop: 0, right: true})
	case lsp.StatusReady:
		parts = append(parts, statusPart{segment: statusSegment{"LSP", statusSecondary}, drop: 2, right: true})
	case lsp.StatusStarting:
		parts = append(parts, statusPart{segment: statusSegment{"LSP...", statusWarningForeground}, drop: 2, right: true})
	}
	if data.percent > 0 {
		parts = append(parts, statusPart{segment: statusSegment{fmt.Sprintf("%d%%", data.percent), statusForeground}, drop: 4, right: true})
	}
	if data.message != "" {
		color := uint8(statusForeground)
		if data.messageError {
			color = statusErrorForeground
		}
		parts = append(parts, statusPart{segment: statusSegment{data.message, color}, drop: 0, right: true})
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
		if part.block {
			cells += 2
		}
	}
	return cells
}

// layoutStatus applies the documented drop order: sizes, diagnostics, LSP
// ready, then file path. Mode, position, and LSP failure are never dropped. A
// notification temporarily replaces sizes/diagnostics/LSP-ready so it can be
// read in the rightmost cells.
func layoutStatus(width int, data statusData) []statusSegment {
	left, right := layoutStatusSections(width, data)
	return append(left, right...)
}

// layoutStatusSections returns the left and right-aligned status segments.
func layoutStatusSections(width int, data statusData) (left, right []statusSegment) {
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
	for _, part := range parts {
		if part.segment.text == "" {
			continue
		}
		if part.right {
			right = append(right, part.segment)
		} else {
			left = append(left, part.segment)
		}
	}
	return left, right
}

func statusPercent(line, lines int) int {
	if lines <= 0 {
		return 0
	}
	return (line + 1) * 100 / lines
}

func statusSegmentsCells(segments []statusSegment) int {
	if len(segments) == 0 {
		return 0
	}
	cells := 0
	for index, segment := range segments {
		if index > 0 {
			cells += 2
		}
		cells += font.CellWidth(segment.text)
	}
	return cells
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
	left := (limit - ellipsisCells) / 2
	right := limit - ellipsisCells - left
	head := clipCells(text, left)
	tail := tailCells(text, right)
	if head == "" && tail == "" {
		return clipCells(ellipsis, limit)
	}
	return head + ellipsis + tail
}

// tailCells returns the last clusters that fit in limit display cells. It walks
// P8SCII clusters from the end so wide glyphs and variation selectors are never
// split.
func tailCells(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	clusters := make([]font.Cluster, 0, len(text))
	for cluster := range font.Clusters(text) {
		clusters = append(clusters, cluster)
	}
	used := 0
	start := len(clusters)
	for index := len(clusters) - 1; index >= 0; index-- {
		width := clusters[index].Width / font.NarrowWidth
		if used+width > limit {
			break
		}
		used += width
		start = index
	}
	var out strings.Builder
	for _, cluster := range clusters[start:] {
		out.WriteString(cluster.Text)
	}
	return out.String()
}
