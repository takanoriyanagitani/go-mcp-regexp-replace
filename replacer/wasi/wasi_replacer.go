package wasi

import (
	"context"
	"fmt"
	"log"
	"os"

	rt "github.com/takanoriyanagitani/go-mcp-regexp-replace"
	wa0 "github.com/takanoriyanagitani/go-mcp-regexp-replace/replacer/wasi/wa0"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// NewWasiReplacer creates a new Replacer that uses the WASI WebAssembly module.
// It loads the WASM binary from the given wasmFilePath.
func NewWasiReplacer(
	ctx context.Context,
	wasmFilePath string,
	memoryLimitPages uint32,
) (rt.Replacer, func() error, error) {
	wasmBinary, err := os.ReadFile(wasmFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read WASM file from %s: %w", wasmFilePath, err)
	}

	rcfg := wa0.RuntimeConfigNewDefault().
		WithPageLimit(memoryLimitPages)
	// The wazero.Runtime is created, and a cleanup function to close it is
	// returned. This cleanup function is deferred in main() to ensure
	// resources are released explicitly on server shutdown, which is a best practice.
	r := rcfg.ToRuntime(ctx)
	// Use context.Background() for the cleanup function to ensure that the Close()
	// operation runs to completion, even if the main application context is already cancelled.
	cleanup := func() error { return r.Close(context.Background()) }

	// Instantiate WASI snapshot preview 1, which is required for many WASI modules.
	_, err = wasi_snapshot_preview1.Instantiate(ctx, r.Runtime)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to instantiate wasi_snapshot_preview1: %w", err)
	}

	compiled, err := r.Compile(ctx, wasmBinary)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile WASM module from %s: %w", wasmFilePath, err)
	}

	log.Printf("WASM module from %s compiled successfully.", wasmFilePath)

	// Create a base WasmConfig with default settings.
	// This module configures stdin/stdout for the WASM module.
	config := wa0.WasmConfig{
		ModuleConfig: wazero.NewModuleConfig().
			WithSysWalltime().
			WithSysNanotime().
			WithSysNanosleep().
			WithStderr(log.Writer()), // Direct stderr to Go's log.
	}

	return r.ToReplacer(ctx, compiled, config), cleanup, nil
}
