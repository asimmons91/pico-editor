package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type clientHandler struct {
	protocol.UnimplementedClient
	service *Service
}

func (h *clientHandler) PublishDiagnostics(_ context.Context, params *protocol.PublishDiagnosticsParams) error {
	if h == nil || h.service == nil || params == nil {
		return nil
	}
	h.service.emit(convertDiagnostics(params))
	return nil
}

func (h *clientHandler) Configuration(_ context.Context, params *protocol.ConfigurationParams) ([]protocol.LSPAny, error) {
	if h == nil || h.service == nil || params == nil {
		return nil, nil
	}
	sections := make([]string, 0, len(params.Items))
	for _, item := range params.Items {
		if item.Section != nil {
			sections = append(sections, *item.Section)
		} else {
			sections = append(sections, "")
		}
	}
	return h.service.settings.configuration(sections), nil
}

func (h *clientHandler) WorkspaceFolders(_ context.Context) ([]protocol.WorkspaceFolder, error) {
	if h == nil || h.service == nil || h.service.opts.WorkspaceRoot == "" {
		return nil, nil
	}
	root := uri.File(h.service.opts.WorkspaceRoot)
	return []protocol.WorkspaceFolder{{URI: root, Name: baseName(h.service.opts.WorkspaceRoot)}}, nil
}

func (h *clientHandler) SemanticTokensRefresh(_ context.Context) error {
	if h == nil || h.service == nil {
		return nil
	}
	h.service.emit(Event{Kind: EventSemanticRefresh})
	return nil
}

func (h *clientHandler) ShowMessage(_ context.Context, params *protocol.ShowMessageParams) error {
	if h == nil || h.service == nil || params == nil {
		return nil
	}
	h.service.log.Info("lsp message", "type", params.Type, "message", params.Message)
	h.service.emit(Event{Kind: EventMessage, Message: params.Message})
	return nil
}

func (h *clientHandler) LogMessage(_ context.Context, params *protocol.LogMessageParams) error {
	if h == nil || h.service == nil || params == nil {
		return nil
	}
	level := slogLevel(params.Type)
	h.service.log.Log(context.Background(), level, "lsp log", "message", params.Message)
	return nil
}
