package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"schumacher-tur/api/internal/atendimento/llm"
)

// modeloDump grava cada pedido ao modelo (e a resposta) em ATD_DUMP_DIR, para
// inspecionar o contexto exato que o LLM recebeu.
type modeloDump struct {
	m   llm.Modelo
	dir string
	n   atomic.Int64
}

func comDump(m llm.Modelo, dir string) llm.Modelo {
	if dir == "" || m == nil {
		return m
	}
	_ = os.MkdirAll(dir, 0o755)
	return &modeloDump{m: m, dir: dir}
}

func (d *modeloDump) Gerar(ctx context.Context, p llm.Pedido) (llm.Resposta, error) {
	r, err := d.m.Gerar(ctx, p)
	reg := map[string]any{"pedido": p, "resposta": r}
	if err != nil {
		reg["erro"] = err.Error()
	}
	if b, e := json.MarshalIndent(reg, "", "  "); e == nil {
		_ = os.WriteFile(filepath.Join(d.dir, fmt.Sprintf("%04d.json", d.n.Add(1))), b, 0o644)
	}
	return r, err
}
