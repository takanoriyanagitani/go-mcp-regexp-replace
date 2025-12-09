package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	replace "github.com/takanoriyanagitani/go-mcp-regexp-replace"
	"github.com/takanoriyanagitani/go-mcp-regexp-replace/replacer/wasi"
	wa0 "github.com/takanoriyanagitani/go-mcp-regexp-replace/replacer/wasi/wa0"
)

const (
	defaultPort         = 12040
	readTimeoutSeconds  = 10
	writeTimeoutSeconds = 10
	maxHeaderExponent   = 20
	maxBodyBytes        = 1 * 1024 * 1024 // 1 MiB
)

var (
	port       = flag.Int("port", defaultPort, "port to listen")
	enginePath = flag.String(
		"path2engine",
		"./engine/rs/rs-regexp-replace-wasi/rs-regexp-replace-wasi.wasm",
		"path to the WASM regex engine",
	)
	mem     = flag.Uint("mem", 64, "WASM memory limit in MiB")
	timeout = flag.Uint("timeout", 100, "WASM execution timeout in milliseconds")
)

const wasmPageSizeKiB = 64
const kiBytesInMiByte = 1024
const wasmPagesInMiB = kiBytesInMiByte / wasmPageSizeKiB

// withMaxBodyBytes is a middleware to limit the size of request bodies.
func withMaxBodyBytes(h http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		h.ServeHTTP(w, r)
	})
}

// toClientError converts an internal error to a client-friendly error string.
func toClientError(err error) string {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline exceeded") {
		return "Text replacement timed out"
	}
	if errors.Is(err, replace.ErrInvalidPattern) {
		return "Invalid regular expression pattern"
	}
	if errors.Is(err, wa0.ErrUuid) {
		return "Engine configuration error"
	}
	if errors.Is(err, wa0.ErrInput) {
		return "Invalid pattern, text, or replacement input format"
	}
	if errors.Is(err, wa0.ErrOutputJson) {
		return "Engine output error"
	}
	if errors.Is(err, wa0.ErrInstantiate) {
		return "Engine instantiation failed"
	}

	return "Internal server error"
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memoryLimitPages := uint32(*mem) * wasmPagesInMiB
	replacer, cleanup, err := wasi.NewWasiReplacer(ctx, *enginePath, memoryLimitPages)
	if err != nil {
		log.Printf("failed to create WASI replacer: %v\n", err)
		return
	}
	defer func() {
		if err := cleanup(); err != nil {
			log.Printf("failed to cleanup WASI replacer: %v\n", err)
		}
	}()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "regexp-replace",
		Version: "v0.1.0",
		Title:   "Regular Expression Replacer",
	}, nil)

	regexpReplaceTool := func(ctx context.Context, req *mcp.CallToolRequest, input replace.ReplaceInput) (
		*mcp.CallToolResult,
		replace.ReplaceResultDto,
		error,
	) {
		timeoutCtx, cancelTimeout := context.WithTimeout(ctx, time.Duration(*timeout)*time.Millisecond)
		defer cancelTimeout()

		result := replacer(timeoutCtx, input)
		if result.Error != nil {
			log.Printf("Error processing pattern='%s' on text='%s': %v", input.Pattern, input.Text, result.Error)
			clientError := toClientError(result.Error)
			return nil, replace.ReplaceResultDto{
				Error: &replace.ErrorDto{
					Code:    -1, // Generic client error
					Message: clientError,
				},
			}, nil
		}
		return nil, replace.ReplaceResultDto{
			ReplacedText: result.Replaced,
			Error:        nil,
		}, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         "regexp-replace",
		Title:        "Regular Expression Replacer",
		Description:  "Tool to replace text using a regular expression.",
		InputSchema:  nil, // Inferred by AddTool
		OutputSchema: nil, // Inferred by AddTool
	}, regexpReplaceTool)

	address := fmt.Sprintf(":%d", *port)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(req *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)

	httpServer := &http.Server{
		Addr:           address,
		Handler:        withMaxBodyBytes(mcpHandler, maxBodyBytes),
		ReadTimeout:    readTimeoutSeconds * time.Second,
		WriteTimeout:   writeTimeoutSeconds * time.Second,
		MaxHeaderBytes: 1 << maxHeaderExponent,
	}

	log.Printf("Ready to start HTTP MCP server. Listening on %s\n", address)
	err = httpServer.ListenAndServe()
	if err != nil {
		log.Printf("Failed to listen and serve: %v\n", err)
		return
	}
}
