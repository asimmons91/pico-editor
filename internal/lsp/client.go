package lsp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	picolsp "github.com/asimmons91/pico-lsp/pkg/lsp"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

const (
	requestTimeoutFeature = 2 * time.Second
	requestTimeoutSlow    = 5 * time.Second
	shutdownTimeout       = 2 * time.Second
)

type client struct {
	conn       jsonrpc2.Conn
	serverConn jsonrpc2.Conn
	server     protocol.Server
	cancel     context.CancelFunc
	cancelSrv  context.CancelFunc
}

func startClient(ctx context.Context, opts Options, handler protocol.Client) (*client, *protocol.InitializeResult, error) {
	serverPipe, clientPipe := net.Pipe()
	logger := opts.Log
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	serverCtx, cancelSrv := context.WithCancel(ctx)
	srv := picolsp.New(picolsp.Options{Logger: logger.With("component", "pico-lsp")})
	serverConn := srv.Serve(serverCtx, jsonrpc2.NewStream(serverPipe))

	clientCtx, cancelClient := context.WithCancel(ctx)
	_, conn, server := protocol.NewClient(clientCtx, handler, jsonrpc2.NewStream(clientPipe))
	result, err := server.Initialize(clientCtx, initializeParams(opts))
	if err != nil {
		cancelClient()
		_ = conn.Close()
		_ = serverConn.Close()
		cancelSrv()
		return nil, nil, fmt.Errorf("lsp initialize: %w", err)
	}
	if err := server.Initialized(clientCtx, &protocol.InitializedParams{}); err != nil {
		cancelClient()
		_ = conn.Close()
		_ = serverConn.Close()
		cancelSrv()
		return nil, nil, fmt.Errorf("lsp initialized: %w", err)
	}
	return &client{
		conn:       conn,
		serverConn: serverConn,
		server:     server,
		cancel:     cancelClient,
		cancelSrv:  cancelSrv,
	}, result, nil
}

func initializeParams(opts Options) *protocol.InitializeParams {
	root := opts.WorkspaceRoot
	params := &protocol.InitializeParams{
		ProcessID:             processID(),
		ClientInfo:            protocol.ClientInfo{Name: "pico-editor", Version: protocol.NewOptional(opts.Version)},
		Capabilities:          clientCapabilities(),
		InitializationOptions: opts.Settings.initializationOptions(),
	}
	if root != "" {
		rootURI := uri.File(root)
		//nolint:staticcheck // rootUri remains part of the M5 initialize contract.
		params.RootURI = &rootURI
		params.WorkspaceFolders = protocol.NewNullable([]protocol.WorkspaceFolder{{
			URI:  rootURI,
			Name: filepath.Base(root),
		}})
	}
	return params
}

func processID() *int32 {
	pid := int32(os.Getpid())
	return &pid
}

func clientCapabilities() protocol.ClientCapabilities {
	const raw = `{
		"workspace": {
			"workspaceFolders": true,
			"configuration": true,
			"semanticTokens": {"refreshSupport": true}
		},
		"textDocument": {
			"synchronization": {
				"dynamicRegistration": false,
				"willSave": false,
				"willSaveWaitUntil": false,
				"didSave": true
			},
			"publishDiagnostics": {
				"relatedInformation": true,
				"versionSupport": true,
				"tagSupport": {"valueSet": [1, 2]}
			},
			"semanticTokens": {
				"dynamicRegistration": false,
				"requests": {"full": true},
				"tokenTypes": [],
				"tokenModifiers": [],
				"formats": ["relative"],
				"overlappingTokenSupport": false,
				"multilineTokenSupport": false,
				"serverCancelSupport": false,
				"augmentsSyntaxTokens": false
			},
			"completion": {
				"dynamicRegistration": false,
				"completionItem": {
					"snippetSupport": false,
					"documentationFormat": ["plaintext"],
					"insertReplaceSupport": true
				},
				"contextSupport": true
			},
			"hover": {
				"dynamicRegistration": false,
				"contentFormat": ["plaintext"]
			},
			"signatureHelp": {
				"dynamicRegistration": false,
				"signatureInformation": {
					"documentationFormat": ["plaintext"]
				}
			},
			"definition": {
				"dynamicRegistration": false,
				"linkSupport": false
			}
		},
		"general": {"positionEncodings": ["utf-16"]}
	}`
	var caps protocol.ClientCapabilities
	if err := protocol.Unmarshal([]byte(raw), &caps); err != nil {
		return protocol.ClientCapabilities{}
	}
	return caps
}

func (c *client) stop(ctx context.Context) {
	if c == nil {
		return
	}
	stopCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()
	_ = c.server.Shutdown(stopCtx)
	_ = c.server.Exit(stopCtx)
	_ = c.conn.Close()
	_ = c.serverConn.Close()
	select {
	case <-c.serverConn.Done():
	case <-stopCtx.Done():
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.cancelSrv != nil {
		c.cancelSrv()
	}
}
