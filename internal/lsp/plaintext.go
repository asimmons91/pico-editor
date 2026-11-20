package lsp

import (
	"regexp"
	"strings"

	"go.lsp.dev/protocol"
)

var (
	markdownLink   = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	markdownBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	markdownUnder  = regexp.MustCompile(`__([^_]+)__`)
	markdownItalic = regexp.MustCompile(`\*([^*\n]+)\*`)
)

func hoverText(contents protocol.HoverContents) string {
	switch value := contents.(type) {
	case protocol.String:
		return stripMarkdown(string(value))
	case *protocol.MarkupContent:
		if value == nil {
			return ""
		}
		return stripMarkdown(value.Value)
	case *protocol.MarkedStringWithLanguage: //nolint:staticcheck // legacy LSP union arm
		if value == nil {
			return ""
		}
		return stripMarkdown(value.Value)
	case protocol.MarkedStringSlice:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			var text string
			switch entry := item.(type) {
			case protocol.String:
				text = stripMarkdown(string(entry))
			case *protocol.MarkedStringWithLanguage: //nolint:staticcheck // legacy LSP union arm
				if entry != nil {
					text = stripMarkdown(entry.Value)
				}
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	default:
		return ""
	}
}

func stripMarkdown(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			line = strings.TrimLeft(trimmed, "# ")
		}
		line = markdownLink.ReplaceAllString(line, "$1")
		line = markdownBold.ReplaceAllString(line, "$1")
		line = markdownUnder.ReplaceAllString(line, "$1")
		line = markdownItalic.ReplaceAllString(line, "$1")
		line = strings.ReplaceAll(line, "`", "")
		out = append(out, strings.TrimRight(line, " "))
	}
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
