package wa0replex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/google/uuid"
	rt "github.com/takanoriyanagitani/go-mcp-regexp-replace"
	"github.com/tetratelabs/wazero"
	wa "github.com/tetratelabs/wazero/api"
)

var (
	ErrInstantiate error = errors.New("internal error: invalid replacement engine")
	ErrUuid        error = errors.New("internal error: unable to configure engine")
	ErrInput       error = errors.New("input error: invalid json")
	ErrOutputJson  error = errors.New("output error: invalid json")
)

type RuntimeConfig struct{ wazero.RuntimeConfig }

func RuntimeConfigNewDefault() RuntimeConfig {
	return RuntimeConfig{
		RuntimeConfig: wazero.NewRuntimeConfig().
			WithCloseOnContextDone(true),
	}
}

func (c RuntimeConfig) WithPageLimit(memoryLimitPages uint32) RuntimeConfig {
	return RuntimeConfig{
		//nolint:staticcheck
		RuntimeConfig: c.RuntimeConfig.WithMemoryLimitPages(memoryLimitPages),
	}
}

func (c RuntimeConfig) ToRuntime(ctx context.Context) WasmRuntime {
	rtm := wazero.NewRuntimeWithConfig(ctx, c.RuntimeConfig)
	return WasmRuntime{Runtime: rtm}
}

type WasmRuntime struct{ wazero.Runtime }

type UUID struct{ uuid.UUID }

func (u UUID) String() string { return u.UUID.String() }

func (u UUID) ToInstanceName() string { return "instance-" + u.String() }

func UUID7() (UUID, error) {
	uid, e := uuid.NewV7()
	if e != nil {
		return UUID{}, fmt.Errorf("failed to create V7 UUID: %w", e)
	}
	return UUID{UUID: uid}, nil
}

func (r WasmRuntime) Close(ctx context.Context) error {
	err := r.Runtime.Close(ctx)
	if err != nil {
		return fmt.Errorf("failed to close runtime: %w", err)
	}
	return nil
}

func (r WasmRuntime) Compile(ctx context.Context, wasm []byte) (Compiled, error) {
	//nolint:staticcheck
	cmod, e := r.Runtime.CompileModule(ctx, wasm)
	if e != nil {
		return Compiled{}, fmt.Errorf("failed to compile module: %w", e)
	}
	return Compiled{CompiledModule: cmod}, nil
}

func (r WasmRuntime) Instantiate(ctx context.Context, cmod Compiled, cfg WasmConfig) (WasmInstance, error) {
	//nolint:staticcheck
	ins, e := r.Runtime.InstantiateModule(ctx, cmod.CompiledModule, cfg.ModuleConfig)
	if e != nil {
		return WasmInstance{}, fmt.Errorf("failed to instantiate module: %w", e)
	}
	return WasmInstance{Module: ins}, nil
}

func (r WasmRuntime) ToReplacer(ctx context.Context, cmod Compiled, cfg WasmConfig) rt.Replacer {
	return func(ctx context.Context, input rt.ReplaceInput) rt.ReplaceResult {
		newCfg, e := cfg.WithAutoID()
		if nil != e {
			log.Printf("Failed to generate UUID for WASM instance: %v", e)
			return rt.ReplaceResult{
				Error: ErrUuid,
			}
		}

		inputJson, e := input.ToJson()
		if nil != e {
			log.Printf("Failed to serialize input to JSON: %v", e)
			return rt.ReplaceResult{
				Error: ErrInput,
			}
		}

		var output bytes.Buffer

		cfgWithStdio := newCfg.
			WithReader(bytes.NewReader(inputJson)).
			WithWriter(&output)

		ins, e := r.Instantiate(ctx, cmod, cfgWithStdio)
		if nil != e {
			if errors.Is(e, context.DeadlineExceeded) {
				log.Printf("WASM module execution timed out: %v", e)
			} else {
				log.Printf("Failed to instantiate WASM module: %v", e)
			}
			return rt.ReplaceResult{
				Error: fmt.Errorf("%w: %w", ErrInstantiate, e),
			}
		}
		defer func() { _ = ins.Close(ctx) }()

		outputJson := output.Bytes()
		parsed, e := rt.ReplaceResultDtoFromJson(outputJson)
		if nil != e {
			log.Printf("Failed to parse JSON output from WASM: %v. Raw output: %s", e, string(outputJson))
			return rt.ReplaceResult{
				Error: ErrOutputJson,
			}
		}

		return parsed.ToResult()
	}
}

type Compiled struct{ wazero.CompiledModule }

type WasmConfig struct{ wazero.ModuleConfig }

func (c WasmConfig) WithName(name string) WasmConfig {
	return WasmConfig{
		ModuleConfig: c.ModuleConfig.WithName(name),
	}
}

func (c WasmConfig) WithID(id UUID) WasmConfig {
	name := id.ToInstanceName()
	return c.WithName(name)
}

func (c WasmConfig) WithAutoID() (WasmConfig, error) {
	id, e := UUID7()
	if e != nil {
		return WasmConfig{}, fmt.Errorf("failed to get auto ID: %w", e)
	}
	neo := c.WithID(id)
	return neo, nil
}

func (c WasmConfig) WithReader(rdr io.Reader) WasmConfig {
	return WasmConfig{
		//nolint:staticcheck
		ModuleConfig: c.ModuleConfig.WithStdin(rdr),
	}
}

func (c WasmConfig) WithWriter(wtr io.Writer) WasmConfig {
	return WasmConfig{
		//nolint:staticcheck
		ModuleConfig: c.ModuleConfig.WithStdout(wtr),
	}
}

type WasmInstance struct{ wa.Module }

func (i WasmInstance) Close(ctx context.Context) error {
	err := i.Module.Close(ctx)
	if err != nil {
		return fmt.Errorf("failed to close module: %w", err)
	}
	return nil
}
