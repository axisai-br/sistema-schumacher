// Comando atendimento-local: simulador local de conversas do atendimento v2.
//
// Roda o agente REAL com um LLM real, mas com dados de teste em memoria: sem
// WhatsApp, sem banco e sem o resto do sistema. Uso (de dentro de apps/api):
//
//	go run ./cmd/atendimento-local                    # interativo
//	go run ./cmd/atendimento-local -roteiro sao_paulo # roteiro
//	go run ./cmd/atendimento-local -caso pede_ajuda   # caso de avaliacao
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/evals"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/llm/provedor"
)

const (
	arquivoEnvPadrao = ".env.atendimento-local"
	dirSaidaPadrao   = "internal/atendimento/evals/saida"
)

// criarModelo monta o llm.Modelo (agente, juiz e cliente simulado usam o
// mesmo). E variavel para os testes injetarem um modelo fake.
var criarModelo = func(c provedor.Config) (llm.Modelo, error) { return provedor.Novo(c) }

// config reune as variaveis do simulador.
type config struct {
	Prov          provedor.Config
	Modelo        string // modelo do agente e do juiz
	ModeloCliente string // modelo do cliente simulado (modo -caso)
	Juiz          string // llm | jev | off
	TypesafeKey   string
	Sinal         float64
	Orcamento     time.Duration // ATD_ORCAMENTO_S: orcamento do turno (0 = padrao do agente)
	Motor         string        // ATENDIMENTO_V2_MOTOR: agente (padrao) ou comandos
	ExtratorRes   string        // LLM_MODELO_EXTRATOR_RESERVA
	Descricao     string        // "provedor · modelo" (sem chave)
}

func lerConfig(get func(string) string) (config, error) {
	prov := provedor.ConfigDoAmbiente(get)
	c := config{Prov: prov, Modelo: prov.Modelo, Descricao: prov.Descricao()}
	c.Motor = strings.ToLower(strings.TrimSpace(get("ATENDIMENTO_V2_MOTOR")))
	c.ExtratorRes = strings.TrimSpace(get("LLM_MODELO_EXTRATOR_RESERVA"))
	if n, err := strconv.Atoi(strings.TrimSpace(get("ATD_ORCAMENTO_S"))); err == nil && n > 0 {
		c.Orcamento = time.Duration(n) * time.Second
	}
	if n, err := strconv.Atoi(strings.TrimSpace(get("LLM_TIMEOUT_S"))); err == nil && n > 0 {
		c.Prov.Timeout = time.Duration(n) * time.Second
	}
	c.Juiz = strings.ToLower(strings.TrimSpace(get("ATENDIMENTO_V2_JUIZ")))
	if c.Juiz == "" {
		c.Juiz = "llm"
	}
	if c.Juiz != "llm" && c.Juiz != "jev" && c.Juiz != "off" {
		return c, fmt.Errorf("ATENDIMENTO_V2_JUIZ=%q inválido (use llm, jev ou off)", c.Juiz)
	}
	c.TypesafeKey = strings.TrimSpace(get("TYPESAFE_API_KEY"))
	c.Sinal = 250
	if v := strings.TrimSpace(get("ATENDIMENTO_V2_SINAL_POR_PAGANTE")); v != "" {
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		if err != nil || f <= 0 {
			return c, fmt.Errorf("ATENDIMENTO_V2_SINAL_POR_PAGANTE=%q inválido", v)
		}
		c.Sinal = f
	}
	c.ModeloCliente = strings.TrimSpace(get("EVAL_MODELO_CLIENTE"))
	if c.ModeloCliente == "" {
		c.ModeloCliente = prov.Modelo
	}
	return c, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run e o main testavel. Devolve o codigo de saida (0 ok, 1 falha de caso/erro,
// 2 uso ou configuracao).
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("atendimento-local", flag.ContinueOnError)
	fs.SetOutput(stderr)
	envPath := fs.String("env", "", "arquivo .env (padrão: ./"+arquivoEnvPadrao+")")
	roteiro := fs.String("roteiro", "", "executa um roteiro (caminho de arquivo ou nome de -lista)")
	caso := fs.String("caso", "", "roda caso(s) de avaliação com cliente simulado por LLM (nome ou \"todos\")")
	k := fs.Int("k", 1, "repetições de cada caso (modo -caso)")
	lista := fs.Bool("lista", false, "lista os casos e roteiros disponíveis")
	saida := fs.String("saida", dirSaidaPadrao, "pasta das transcrições")
	replay := fs.String("replay", "", "reenvia pedidos gravados por ATD_DUMP_DIR (arquivos .json separados por vírgula)")
	nReplay := fs.Int("n", 5, "repetições de cada pedido (modo -replay)")
	variante := fs.String("variante", "orig", "contexto do replay: orig ou v2")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *lista {
		return cmdLista(stdout, stderr)
	}

	if *envPath != "" {
		if _, err := carregarArquivoEnv(*envPath, true); err != nil {
			fmt.Fprintf(stderr, "erro: %v\n", err)
			return 2
		}
	} else if _, err := carregarArquivoEnv(arquivoEnvPadrao, false); err != nil {
		fmt.Fprintf(stderr, "erro: %v\n", err)
		return 2
	}

	cfg, err := lerConfig(os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "erro de configuração: %v\n", err)
		return 2
	}
	modelo, err := criarModelo(cfg.Prov)
	modelo = comDump(modelo, os.Getenv("ATD_DUMP_DIR"))
	if err != nil {
		fmt.Fprintf(stderr, "erro: %v\n", err)
		fmt.Fprintf(stderr, "Coloque %s em apps/api/%s (copie de %s.example) ou exporte a variável no ambiente.\n",
			cfg.Prov.VariavelChave(), arquivoEnvPadrao, arquivoEnvPadrao)
		return 2
	}
	if _, aviso := criarJuiz(cfg, modelo); aviso != "" {
		fmt.Fprintf(stderr, "aviso: %s\n", aviso)
	}

	switch {
	case *replay != "":
		return modoReplay(ctx, cfg, modelo, strings.Split(*replay, ","), *nReplay, *variante, stdout, stderr)
	case *caso != "":
		return modoCaso(ctx, cfg, modelo, *caso, *k, *saida, stdout, stderr)
	case *roteiro != "":
		return modoRoteiro(ctx, cfg, modelo, *roteiro, *saida, stdout, stderr)
	default:
		return modoInterativo(ctx, cfg, modelo, stdin, stdout, *saida)
	}
}

func cmdLista(stdout, stderr io.Writer) int {
	casos, err := evals.CarregarCasos()
	if err != nil {
		fmt.Fprintf(stderr, "erro ao carregar casos: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "casos de avaliação (-caso <nome> | -caso todos):")
	for _, c := range casos {
		fmt.Fprintf(stdout, "  %-26s %s\n", c.Nome, c.Descricao)
	}
	fmt.Fprintln(stdout, "\nroteiros (-roteiro <nome ou caminho>):")
	for _, n := range append(nomesRoteiros(), nomesStress()...) {
		fmt.Fprintf(stdout, "  %s\n", n)
	}
	fmt.Fprintln(stdout, "  (grupos: todos = raiz, stress, tudo = os dois, rapido = amostra de ~15 min)")
	return 0
}

// ---- modo interativo ----

func modoInterativo(ctx context.Context, cfg config, modelo llm.Modelo, stdin io.Reader, stdout io.Writer, dirSaida string) int {
	s := novaSessao(cfg, modelo, stdout, nil, dirSaida)
	s.cabecalho()

	linhas := make(chan string)
	go func() {
		defer close(linhas)
		sc := bufio.NewScanner(stdin)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			linhas <- sc.Text()
		}
	}()

loop:
	for {
		fmt.Fprint(stdout, "você> ")
		select {
		case <-ctx.Done():
			fmt.Fprintln(stdout)
			break loop
		case l, ok := <-linhas:
			if !ok {
				fmt.Fprintln(stdout)
				break loop
			}
			if s.Linha(ctx, l) {
				break loop
			}
		}
	}
	return encerrar(s, stdout)
}

func encerrar(s *sessao, stdout io.Writer) int {
	p, err := s.salvar()
	if err != nil {
		fmt.Fprintf(stdout, "erro ao salvar a transcrição: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Transcrição salva em %s\n", p)
	return 0
}

// ---- modo roteiro ----

func modoRoteiro(ctx context.Context, cfg config, modelo llm.Modelo, ref, dirSaida string, stdout, stderr io.Writer) int {
	if nomes, ok := grupoRoteiros(ref); ok {
		var ps []placar
		cod := 0
		for _, n := range nomes {
			p, c := executarRoteiro(ctx, cfg, modelo, n, dirSaida, stdout, stderr)
			ps = append(ps, p)
			cod = max(cod, c)
		}
		imprimirPlacares(stdout, ps)
		return cod
	}
	p, cod := executarRoteiro(ctx, cfg, modelo, ref, dirSaida, stdout, stderr)
	if cod == 0 {
		fmt.Fprintf(stdout, "PLACAR %s\n", p.linha())
		if len(p.Falhas) > 0 {
			fmt.Fprintf(stdout, "FALHAS: %s\n", strings.Join(p.Falhas, "; "))
		}
	}
	return cod
}

// executarRoteiro executa um roteiro numa sessao nova e devolve o placar.
func executarRoteiro(ctx context.Context, cfg config, modelo llm.Modelo, ref, dirSaida string, stdout, stderr io.Writer) (placar, int) {
	linhas, nome, err := abrirRoteiro(ref)
	if err != nil {
		fmt.Fprintf(stderr, "erro: %v\n", err)
		return placar{Roteiro: ref}, 2
	}
	s := novaSessao(cfg, modelo, stdout, nil, dirSaida)
	s.cabecalho()
	fmt.Fprintf(stdout, "Roteiro: %s (%d linhas)\n\n", nome, len(linhas))
	for _, l := range linhas {
		if ctx.Err() != nil {
			break
		}
		txt := l.Texto
		if l.Tipo == linhaEnfileira {
			txt = "+" + txt
		}
		fmt.Fprintf(stdout, "você> %s\n", txt)
		if s.Linha(ctx, txt) {
			break
		}
	}
	fmt.Fprintln(stdout)
	p := s.placar
	p.Roteiro = nome
	p.Reservas = len(s.amb.Reservas.Reservas())
	for _, pg := range s.amb.Pagamentos.Pagamentos() {
		if pg.Status != "CANCELLED" {
			p.Pix++
		}
	}
	esp, err := lerEspera(ref)
	if err != nil {
		fmt.Fprintf(stderr, "erro: %v\n", err)
		return p, 2
	}
	r := resultadoRoteiro{Reservas: p.Reservas, Pix: p.Pix, Transferiu: p.Transferiu, Respostas: s.amb.Canal.Textos(0)}
	if conv, err := s.amb.ConversaAtual(context.Background()); err == nil {
		r.Estado = conv.Estado
	}
	p.ComEspera = !esp.vazia()
	p.Falhas = esp.avaliar(r)
	return p, encerrar(s, stdout)
}

// ---- modo caso ----

func modoCaso(ctx context.Context, cfg config, modelo llm.Modelo, nome string, k int, dirSaida string, stdout, stderr io.Writer) int {
	casos, err := evals.CarregarCasos()
	if err != nil {
		fmt.Fprintf(stderr, "erro ao carregar casos: %v\n", err)
		return 1
	}
	var sel []evals.Caso
	if strings.EqualFold(nome, "todos") {
		sel = casos
	} else {
		for _, c := range casos {
			if c.Nome == nome {
				sel = append(sel, c)
			}
		}
		if len(sel) == 0 {
			fmt.Fprintf(stderr, "erro: caso %q não existe (veja -lista)\n", nome)
			return 2
		}
	}
	if k < 1 {
		k = 1
	}
	type res struct {
		caso string
		ok   int
	}
	var resumo []res
	reprovou := false
	for _, c := range sel {
		r := res{caso: c.Nome}
		for i := 1; i <= k; i++ {
			if ctx.Err() != nil {
				return 1
			}
			var m llm.Modelo = modelo
			if c.LLMFora {
				m = evals.ModeloQueFalha{}
			}
			juiz, _ := criarJuiz(cfg, m)
			amb := evals.NovoAmbiente(m, evals.ConfigAmbiente{ModeloNome: cfg.Modelo, Juiz: juiz, SinalPorPagante: cfg.Sinal})
			cli := &evals.ClienteLLM{Modelo: modelo, ModeloNome: cfg.ModeloCliente}
			t0 := time.Now()
			ex := evals.Executar(ctx, c, amb, cli)
			fmt.Fprintf(stdout, "==== %s (execução %d/%d, %s) ====\n", c.Nome, i, k, fmtDur(time.Since(t0)))
			fmt.Fprintln(stdout, evals.FormatarTranscricao(ex))
			if ex.OK() {
				r.ok++
				fmt.Fprintln(stdout, "APROVADO")
			} else {
				reprovou = true
				fmt.Fprintln(stdout, "REPROVADO")
				if ex.Erro != nil {
					fmt.Fprintf(stdout, "  - erro: %v\n", ex.Erro)
				}
				for _, f := range ex.Falhas {
					fmt.Fprintf(stdout, "  - %s\n", f)
				}
			}
			fmt.Fprintln(stdout)
			p := filepath.Join(dirSaida, fmt.Sprintf("local-caso-%s-%d.txt", c.Nome, i))
			if err := os.MkdirAll(dirSaida, 0o755); err == nil {
				_ = os.WriteFile(p, []byte(evals.FormatarTranscricao(ex)), 0o644)
			}
		}
		resumo = append(resumo, r)
	}
	if len(resumo) > 1 || k > 1 {
		fmt.Fprintln(stdout, "resumo:")
		for _, r := range resumo {
			fmt.Fprintf(stdout, "  %-26s %d/%d\n", r.caso, r.ok, k)
		}
	}
	fmt.Fprintf(stdout, "Transcrições em %s\n", dirSaida)
	if reprovou {
		return 1
	}
	return 0
}
