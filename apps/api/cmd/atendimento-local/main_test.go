package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/llm/provedor"
)

// ---- .env ----

func TestParseEnv(t *testing.T) {
	src := "\xef\xbb\xbf#comentário\n\nOPENAI_API_KEY=abc123\nexport A=1\nB=\"com espaços # não é comentário\"\nC='aspas simples'\nD=valor # comentário final\nE=\nsem_igual\n  F  =  x  \n"
	got, err := parseEnv(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"OPENAI_API_KEY": "abc123", "A": "1", "B": "com espaços # não é comentário",
		"C": "aspas simples", "D": "valor", "E": "", "F": "x",
	}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, esperado %q", k, got[k], v)
		}
	}
}

func TestAplicarEnvSOTemPrioridade(t *testing.T) {
	so := map[string]string{"A": "do-so", "VAZIA": ""}
	setados := map[string]string{}
	aplicarEnv(map[string]string{"A": "do-arquivo", "B": "do-arquivo", "VAZIA": "do-arquivo"},
		func(k string) (string, bool) { v, ok := so[k]; return v, ok },
		func(k, v string) { setados[k] = v })
	if _, ok := setados["A"]; ok {
		t.Error("variável do SO foi sobrescrita")
	}
	if setados["B"] != "do-arquivo" || setados["VAZIA"] != "do-arquivo" {
		t.Errorf("setados = %v", setados)
	}
}

func TestCarregarArquivoEnv(t *testing.T) {
	if ok, err := carregarArquivoEnv(filepath.Join(t.TempDir(), "nao-existe"), false); ok || err != nil {
		t.Errorf("opcional ausente: ok=%v err=%v", ok, err)
	}
	if _, err := carregarArquivoEnv(filepath.Join(t.TempDir(), "nao-existe"), true); err == nil {
		t.Error("esperava erro para arquivo obrigatório ausente")
	}
	p := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(p, []byte("TESTE_SIM_LOCAL_X=\"arq\"\nTESTE_SIM_LOCAL_Y=arq\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TESTE_SIM_LOCAL_X", "so")
	t.Setenv("TESTE_SIM_LOCAL_Y", "")
	if ok, err := carregarArquivoEnv(p, true); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if v := os.Getenv("TESTE_SIM_LOCAL_X"); v != "so" {
		t.Errorf("X = %q, SO deveria ter prioridade", v)
	}
	if v := os.Getenv("TESTE_SIM_LOCAL_Y"); v != "arq" {
		t.Errorf("Y = %q", v)
	}
}

// ---- roteiro ----

func TestParseRoteiro(t *testing.T) {
	src := "# comentário\n\nOi\n+primeira\n  + segunda  \n/estado\n  # outro\nÚltima linha\n+\n"
	got, err := parseRoteiro(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []linhaRoteiro{
		{linhaMensagem, "Oi"}, {linhaEnfileira, "primeira"}, {linhaEnfileira, "segunda"},
		{linhaComando, "/estado"}, {linhaMensagem, "Última linha"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("linha %d = %v, esperado %v", i, got[i], want[i])
		}
	}
}

func TestRoteirosEmbutidosParseiam(t *testing.T) {
	nomes := nomesRoteiros()
	for _, n := range []string{"loop_passageiros", "sao_paulo", "videira_3_pessoas", "ida_e_volta", "doces"} {
		found := false
		for _, x := range nomes {
			found = found || x == n
		}
		if !found {
			t.Errorf("roteiro %s ausente", n)
			continue
		}
		l, _, err := abrirRoteiro(n)
		if err != nil || len(l) == 0 {
			t.Errorf("%s: %v (%d linhas)", n, err, len(l))
		}
	}
	l, _, _ := abrirRoteiro("loop_passageiros")
	if len(l) != 7 || l[0].Texto != "Passagem de Santa Catarina para o Maranhão" {
		t.Errorf("loop_passageiros: %v", l)
	}
}

// ---- ponta a ponta com modelo fake ----

type modeloFake struct {
	mu        sync.Mutex
	respostas []string
	pedidos   []llm.Pedido
}

func (m *modeloFake) Gerar(_ context.Context, p llm.Pedido) (llm.Resposta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pedidos = append(m.pedidos, p)
	txt := "Certo!"
	if len(m.respostas) > 0 {
		txt, m.respostas = m.respostas[0], m.respostas[1:]
	}
	return llm.Resposta{Texto: txt, TokensEntrada: 1200, TokensSaida: 30}, nil
}

func (m *modeloFake) textoPedido(i int) string {
	var b strings.Builder
	for _, x := range m.pedidos[i].Mensagens {
		b.WriteString(x.Texto + "\n")
	}
	return b.String()
}

func rodarRoteiro(t *testing.T, m llm.Modelo, roteiro string) (string, int, string) {
	t.Helper()
	old := criarModelo
	criarModelo = func(provedor.Config) (llm.Modelo, error) { return m, nil }
	t.Cleanup(func() { criarModelo = old })
	t.Setenv("ATENDIMENTO_V2_JUIZ", "off")
	t.Setenv("LLM_PROVEDOR", "nvidia")
	t.Setenv("NVIDIA_API_KEY", "chave-falsa")

	dir := t.TempDir()
	arq := filepath.Join(dir, "r.txt")
	if err := os.WriteFile(arq, []byte(roteiro), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"-roteiro", arq, "-saida", dir}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("código %d; stderr: %s; stdout: %s", code, errb.String(), out.String())
	}
	return out.String(), code, dir
}

func TestRoteiroPontaAPonta(t *testing.T) {
	m := &modeloFake{respostas: []string{"Oi! Para onde você vai?\nMe diga a cidade de origem."}}
	out, _, dir := rodarRoteiro(t, m, "# abre\nOlá, boa tarde\n/estado\n")

	if len(m.pedidos) != 1 {
		t.Fatalf("%d chamadas ao modelo, esperado 1", len(m.pedidos))
	}
	if !strings.Contains(m.textoPedido(0), "Olá, boa tarde") {
		t.Errorf("mensagem do cliente não chegou ao agente: %q", m.textoPedido(0))
	}
	if !strings.Contains(out, "Shabas> Oi! Para onde você vai?\n        Me diga a cidade de origem.") {
		t.Errorf("resposta do bot não impressa (multi-linha):\n%s", out)
	}
	if !strings.Contains(out, "[ferramentas: nenhuma ·") || !strings.Contains(out, "1.2k tokens]") {
		t.Errorf("linha de info do turno ausente:\n%s", out)
	}
	if !strings.Contains(out, "\"falhas\": 0") || !strings.Contains(out, "Pendências:") || !strings.Contains(out, "Status da conversa: BOT") {
		t.Errorf("/estado não imprimiu JSON/pendências/status:\n%s", out)
	}
	ents, _ := os.ReadDir(dir)
	achou := false
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "local-") {
			b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			achou = strings.Contains(string(b), "[CLIENTE] Olá, boa tarde") && strings.Contains(string(b), "[BOT] Oi! Para onde")
		}
	}
	if !achou {
		t.Error("transcrição não salva com a conversa")
	}
}

func TestRoteiroMaisAgrupaMensagensNumUnicoTurno(t *testing.T) {
	m := &modeloFake{respostas: []string{"Entendi tudo."}}
	out, _, _ := rodarRoteiro(t, m, "+primeira mensagem\n+segunda mensagem\nterceira mensagem\n")

	if len(m.pedidos) != 1 {
		t.Fatalf("%d chamadas ao modelo, esperado 1 turno só", len(m.pedidos))
	}
	p := m.textoPedido(0)
	for _, s := range []string{"primeira mensagem", "segunda mensagem", "terceira mensagem"} {
		if !strings.Contains(p, s) {
			t.Errorf("%q não chegou ao agente: %q", s, p)
		}
	}
	if n := strings.Count(out, "Shabas> "); n != 1 {
		t.Errorf("%d respostas do bot, esperado 1:\n%s", n, out)
	}
	if !strings.Contains(out, "[na fila: 1 mensagem(ns)") || !strings.Contains(out, "[na fila: 2 mensagem(ns)") {
		t.Errorf("aviso de fila ausente:\n%s", out)
	}
}

func TestRoteiroTransferenciaEBot(t *testing.T) {
	m := &modeloFake{respostas: []string{"Claro, como posso ajudar?"}}
	out, _, _ := rodarRoteiro(t, m, "quero falar com um atendente\nmais uma mensagem\n/bot\noi de novo\n")
	if !strings.Contains(out, "*** conversa transferida para atendente humano — motivo:") {
		t.Errorf("aviso de transferência ausente:\n%s", out)
	}
	if !strings.Contains(out, "NÃO processada") {
		t.Errorf("aviso de mensagem não processada ausente:\n%s", out)
	}
	if !strings.Contains(out, "[conversa devolvida ao bot]") || !strings.Contains(out, "Shabas> Claro, como posso ajudar?") {
		t.Errorf("/bot não devolveu a conversa ao bot:\n%s", out)
	}
}

func TestSemChaveSaiComCodigo2(t *testing.T) {
	old := criarModelo
	criarModelo = func(c provedor.Config) (llm.Modelo, error) { return provedor.Novo(c) }
	t.Cleanup(func() { criarModelo = old })
	t.Setenv("LLM_PROVEDOR", "nvidia")
	t.Setenv("NVIDIA_API_KEY", "")
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"-env", writeTemp(t, "# vazio\n")}, strings.NewReader(""), &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), ".env.atendimento-local") || !strings.Contains(errb.String(), "NVIDIA_API_KEY") {
		t.Errorf("code=%d stderr=%q", code, errb.String())
	}
}

func TestListaSemChave(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"-lista"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("code=%d %s", code, errb.String())
	}
	for _, s := range []string{"pede_ajuda", "sao_paulo", "doces"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("-lista sem %s:\n%s", s, out.String())
		}
	}
}

func writeTemp(t *testing.T, conteudo string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
