package lsp

import (
	"go.lsp.dev/protocol"
)

func convertDiagnostics(params *protocol.PublishDiagnosticsParams) Event {
	event := Event{Kind: EventDiagnostics, URI: params.URI.String()}
	if version, ok := params.Version.Get(); ok {
		event.Version = int(version)
	}
	event.Diagnostics = make([]Diagnostic, 0, len(params.Diagnostics))
	for _, item := range params.Diagnostics {
		event.Diagnostics = append(event.Diagnostics, Diagnostic{
			URI:      params.URI.String(),
			Version:  event.Version,
			Range:    convertRange(item.Range),
			Severity: Severity(item.Severity),
			Message:  plainText(item.Message),
			Source:   optionalString(item.Source),
		})
	}
	return event
}

func convertRange(r protocol.Range) Range {
	return Range{
		Start: Position{Line: int(r.Start.Line), Character: int(r.Start.Character)},
		End:   Position{Line: int(r.End.Line), Character: int(r.End.Character)},
	}
}

func optionalString(value protocol.Optional[string]) string {
	text, _ := value.Get()
	return text
}

// plainText strips Markdown fences, emphasis markers, and link syntax from a
// tooltip so the 1-bit renderer can draw it verbatim.
func plainText(value protocol.InlayHintTooltip) string {
	switch v := value.(type) {
	case protocol.String:
		return stripMarkdown(string(v))
	case *protocol.MarkupContent:
		if v == nil {
			return ""
		}
		return stripMarkdown(v.Value)
	default:
		return ""
	}
}
