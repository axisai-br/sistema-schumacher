package evals

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/availability"
	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/pricing"
)

var casosEsperados = []string{
	"audio_nao_compreendido", "cidade_nao_atendida", "cidade_nao_atendida_b", "destino_direto", "duas_mensagens",
	"fora_do_assunto", "irritado", "llm_fora", "muda_de_ideia", "pede_ajuda",
	"quantidade_livre_a", "quantidade_livre_b", "quantidade_livre_c", "quantidade_livre_d",
	"quantidade_no_inicio", "reserva_completa_sinal", "sc_para_ma",
}

func TestCasosParseiamEValidam(t *testing.T) {
	casos, err := CarregarCasos()
	if err != nil {
		t.Fatalf("CarregarCasos: %v", err)
	}
	got := map[string]Caso{}
	for _, c := range casos {
		got[c.Nome] = c
	}
	for _, n := range casosEsperados {
		if _, ok := got[n]; !ok {
			t.Errorf("caso %q ausente em casos/", n)
		}
	}
	if len(casos) != len(casosEsperados) {
		t.Errorf("%d casos carregados, esperado %d", len(casos), len(casosEsperados))
	}
	reCPF := regexp.MustCompile(`\d{3}\.\d{3}\.\d{3}-\d{2}`)
	for _, c := range casos {
		for _, cpf := range reCPF.FindAllString(c.ObjetivoCliente, -1) {
			if !ferramentas.ValidarCPF(cpf) {
				t.Errorf("%s: CPF %s do objetivo é inválido", c.Nome, cpf)
			}
		}
		if c.Grupo != "" && c.Grupo != "reserva" {
			t.Errorf("%s: grupo desconhecido %q", c.Nome, c.Grupo)
		}
	}
	for _, n := range []string{"pede_ajuda", "irritado", "llm_fora", "cidade_nao_atendida", "cidade_nao_atendida_b"} {
		if !got[n].Critico {
			t.Errorf("%s deveria ser crítico", n)
		}
	}
}

func TestCasoInvalidoRecusado(t *testing.T) {
	ok := Caso{Nome: "x", Descricao: "d", ObjetivoCliente: "o", PrimeiraMsg: "oi", MaxTurnos: 3, Espera: Espera{Status: "BOT"}}
	if err := ok.Validar(); err != nil {
		t.Fatalf("caso válido recusado: %v", err)
	}
	for nome, mut := range map[string]func(*Caso){
		"sem nome":       func(c *Caso) { c.Nome = "" },
		"sem objetivo":   func(c *Caso) { c.ObjetivoCliente = "" },
		"sem turnos":     func(c *Caso) { c.MaxTurnos = 0 },
		"status ruim":    func(c *Caso) { c.Espera.Status = "TALVEZ" },
		"sem status":     func(c *Caso) { c.Espera.Status = "" },
		"regex invalida": func(c *Caso) { c.Proibido = []string{"("} },
	} {
		c := ok
		mut(&c)
		if err := c.Validar(); err == nil {
			t.Errorf("%s: esperava erro", nome)
		}
	}
}

func hojeTeste() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }

func TestFixturesViagens(t *testing.T) {
	fx := NovasFixtures(hojeTeste())
	vs := fx.Viagens()
	if len(vs) < 12 {
		t.Fatalf("%d viagens, esperado >= 12", len(vs))
	}
	hoje := fx.Hoje.Format("2006-01-02")
	for _, v := range vs {
		if v.Data <= hoje {
			t.Errorf("viagem %s em %s não é futura", v.ID, v.Data)
		}
		if v.Vagas != VagasPorViagem {
			t.Errorf("viagem %s com %d vagas", v.ID, v.Vagas)
		}
	}
	cs := fx.Cidades()
	if len(cs) != 13 {
		t.Errorf("%d cidades, esperado 13", len(cs))
	}
	// A viagem do dia 12 existe.
	achou := false
	for _, v := range vs {
		if strings.HasSuffix(v.Data, "-12") {
			achou = true
		}
	}
	if !achou {
		t.Error("esperava viagem no dia 12")
	}
}

func TestBuscadorSemanticaDoRepositorio(t *testing.T) {
	ctx := context.Background()
	fx := NovasFixtures(hojeTeste())
	b := fx.Buscador()

	// Só destino: todos os resultados terminam em Chapecó, ordenados por data.
	res, err := b.Search(ctx, availability.SearchFilter{DestinationStopID: "sc-chapeco", OnlyActive: true, Limit: 50})
	if err != nil || len(res) == 0 {
		t.Fatalf("destino só: err=%v n=%d", err, len(res))
	}
	for i, r := range res {
		if r.DestinationStopID != "sc-chapeco" {
			t.Errorf("destino %s", r.DestinationStopID)
		}
		if r.Price != 1100 {
			t.Errorf("preço Chapecó = %v", r.Price)
		}
		if i > 0 && res[i-1].TripDate > r.TripDate {
			t.Errorf("fora de ordem por data")
		}
	}
	// Só origem SC: só existem viagens SC>MA a partir dela (sequência crescente).
	res, _ = b.Search(ctx, availability.SearchFilter{OriginStopID: "sc-videira", Limit: 50})
	if len(res) == 0 {
		t.Fatal("Videira sem resultados")
	}
	for _, r := range res {
		if !strings.HasSuffix(r.DestinationDisplayName, "/MA") {
			t.Errorf("Videira -> %s", r.DestinationDisplayName)
		}
	}
	// Sequência crescente: Monção -> Santa Inês nunca existe na ida.
	res, _ = b.Search(ctx, availability.SearchFilter{OriginStopID: "ma-moncao", DestinationStopID: "ma-santa-ines", Limit: 50})
	if len(res) != 0 {
		t.Errorf("trecho intra-UF/inverso devolveu %d resultados", len(res))
	}
	// Videira -> Monção: preço 950, só nas viagens SC>MA.
	res, _ = b.Search(ctx, availability.SearchFilter{OriginStopID: "sc-videira", DestinationStopID: "ma-moncao", Limit: 50})
	if len(res) == 0 || res[0].Price != 950 {
		t.Fatalf("Videira->Monção: %+v", res)
	}
	// Filtro de data e Limit.
	d := res[1].TripDate
	dt, _ := time.Parse("2006-01-02", d)
	one, _ := b.Search(ctx, availability.SearchFilter{OriginStopID: "sc-videira", DestinationStopID: "ma-moncao", TripDate: &dt})
	if len(one) != 1 || one[0].TripDate != d {
		t.Errorf("TripDate: %+v", one)
	}
	lim, _ := b.Search(ctx, availability.SearchFilter{OriginStopID: "sc-videira"})
	if len(lim) != 10 {
		t.Errorf("limite padrão = %d", len(lim))
	}
	de, ate := dt, dt
	faixa, _ := b.Search(ctx, availability.SearchFilter{OriginStopID: "sc-videira", DestinationStopID: "ma-moncao", DateFrom: &de, DateTo: &ate})
	if len(faixa) != 1 {
		t.Errorf("faixa de data = %d", len(faixa))
	}
	// Passado só com IncludePast.
	ontem := fx.Hoje.AddDate(0, 0, -1)
	if r, _ := b.Search(ctx, availability.SearchFilter{DateTo: &ontem}); len(r) != 0 {
		t.Errorf("passado devolveu %d", len(r))
	}
	// Nome como texto também filtra (sem acento, com /UF).
	if r, _ := b.Search(ctx, availability.SearchFilter{Destination: "chapeco/sc", Limit: 5}); len(r) == 0 {
		t.Error("filtro por nome de destino não casou")
	}
}

func TestBuscadorSemVaga(t *testing.T) {
	ctx := context.Background()
	fx := NovasFixtures(hojeTeste())
	b := fx.Buscador()
	res, _ := b.Search(ctx, availability.SearchFilter{OriginStopID: "sc-videira", DestinationStopID: "ma-moncao", Limit: 50})
	trip := res[0].TripID
	fx.Ocupar(trip, VagasPorViagem-2) // sobram 2
	res, _ = b.Search(ctx, availability.SearchFilter{OriginStopID: "sc-videira", DestinationStopID: "ma-moncao", Qty: 3, Limit: 50})
	for _, r := range res {
		if r.TripID == trip {
			t.Errorf("viagem com 2 vagas apareceu para Qty=3")
		}
	}
	res, _ = b.Search(ctx, availability.SearchFilter{OriginStopID: "sc-videira", DestinationStopID: "ma-moncao", Limit: 50})
	if res[0].TripID != trip || res[0].SeatsAvailable != 2 {
		t.Errorf("sem Qty deveria mostrar %s com 2 vagas: %+v", trip, res[0])
	}
}

func primeiraOpcao(t *testing.T, fx *Fixtures, origem, destino string) availability.SearchResult {
	t.Helper()
	res, _ := fx.Buscador().Search(context.Background(), availability.SearchFilter{OriginStopID: origem, DestinationStopID: destino, Limit: 5})
	if len(res) == 0 {
		t.Fatal("sem viagem")
	}
	return res[0]
}

func TestReservasEPix(t *testing.T) {
	ctx := context.Background()
	fx := NovasFixtures(hojeTeste())
	rs, ps := fx.Reservas(), NovosPagamentos()
	op := primeiraOpcao(t, fx, "sc-videira", "ma-moncao")
	src := "WHATSAPP_V2"
	in := bookings.CreateBookingInput{
		TripID: op.TripID, BoardStopID: op.BoardStopID, AlightStopID: op.AlightStopID,
		Passengers: []bookings.PassengerInput{
			{Name: "Maria Silva", Document: "12345678909", DocumentType: "CPF"},
			{Name: "Pedro Silva", IsLapChild: true},
		},
		IdempotencyKey: "k1", Source: &src, TotalAmount: 950, DepositAmount: 250, RemainderAmount: 700,
	}

	// Validações.
	bad := in
	bad.Passengers = []bookings.PassengerInput{{Name: " ", Document: "1", DocumentType: "CPF"}}
	if _, err := rs.Create(ctx, bad); err != bookings.ErrPassengerNameRequired {
		t.Errorf("nome vazio: %v", err)
	}
	bad = in
	bad.Passengers = []bookings.PassengerInput{{Name: "Fulano de Tal"}}
	if _, err := rs.Create(ctx, bad); err != bookings.ErrPassengerDocumentType {
		t.Errorf("sem documento: %v", err)
	}
	bad = in
	bad.Passengers = []bookings.PassengerInput{{Name: "Fulano de Tal", Document: "123", DocumentType: "XYZ"}}
	if _, err := rs.Create(ctx, bad); err != bookings.ErrPassengerDocumentType {
		t.Errorf("tipo inválido: %v", err)
	}
	bad = in
	bad.DepositAmount = 100
	if _, err := rs.Create(ctx, bad); err != bookings.ErrInvalidAmounts {
		t.Errorf("valores: %v", err)
	}
	bad = in
	bad.TotalAmount, bad.DepositAmount, bad.RemainderAmount = 1900, 250, 1650 // preço x 2 (criança de colo não paga)
	if _, err := rs.Create(ctx, bad); err != bookings.ErrInvalidAmounts {
		t.Errorf("total errado: %v", err)
	}
	bad = in
	bad.TripID = ""
	if _, err := rs.Create(ctx, bad); err != bookings.ErrMissingFields {
		t.Errorf("sem viagem: %v", err)
	}
	if len(rs.Reservas()) != 0 {
		t.Fatal("reserva inválida foi gravada")
	}

	// Criação válida, idempotente, e vagas decrementam só pelo adulto.
	d, err := rs.Create(ctx, in)
	if err != nil || d.Booking.ID == "" || d.Booking.ReservationCode == "" {
		t.Fatalf("create: %v %+v", err, d)
	}
	d2, _ := rs.Create(ctx, in)
	if d2.Booking.ID != d.Booking.ID || len(rs.Reservas()) != 1 {
		t.Errorf("idempotência falhou")
	}
	if v := fx.Viagens(); func() int {
		for _, x := range v {
			if x.ID == op.TripID {
				return x.Vagas
			}
		}
		return -1
	}() != VagasPorViagem-1 {
		t.Errorf("vagas não decrementaram em 1")
	}
	if got, err := rs.Get(ctx, d.Booking.ID); err != nil || len(got.Passengers) != 2 {
		t.Errorf("get: %v %+v", err, got)
	}
	if l, _ := rs.List(ctx, bookings.ListFilter{ReservationCode: d.Booking.ReservationCode}); len(l) != 1 {
		t.Errorf("list: %d", len(l))
	}

	// Sem vagas.
	fx.Ocupar(op.TripID, VagasPorViagem) // zera
	lotada := in
	lotada.IdempotencyKey = "k2"
	if _, err := rs.Create(ctx, lotada); err != bookings.ErrNoSeatsAvailable {
		t.Errorf("lotada: %v", err)
	}

	// PIX.
	pay, raw, err := ps.Create(ctx, payments.CreatePaymentInput{
		BookingID: d.Booking.ID, Amount: 250, Method: "PIX",
		Customer: &payments.CustomerInput{Name: "Maria Silva", Document: "12345678909", Phone: "5549991000001"},
	})
	if err != nil || pay.ID == "" {
		t.Fatalf("pix: %v", err)
	}
	_, codigo := payments.ExtractCheckoutAndPix(raw)
	if !strings.HasPrefix(codigo, "00020126") || codigo != ps.CodigoPix(pay.ID) {
		t.Errorf("copia-e-cola inesperado: %q", codigo)
	}
	st, err := ps.GetStatus(ctx, pay.ID)
	if err != nil || st.Status != "PENDING" || st.Amount != 250 {
		t.Errorf("status: %v %+v", err, st)
	}
	if _, _, err := ps.Create(ctx, payments.CreatePaymentInput{BookingID: "x", Amount: 0, Method: "PIX"}); err == nil {
		t.Error("valor zero deveria falhar")
	}
}

func TestCotadorIgualBusca(t *testing.T) {
	fx := NovasFixtures(hojeTeste())
	op := primeiraOpcao(t, fx, "ma-santa-ines", "sc-chapeco")
	q, err := fx.Cotador().Quote(context.Background(), quoteIn(op))
	if err != nil || q.FinalAmount != op.Price {
		t.Errorf("cotação %v != busca %v (%v)", q.FinalAmount, op.Price, err)
	}
}

func TestCanalFake(t *testing.T) {
	c := &CanalFake{}
	id, err := c.Enviar(context.Background(), "x", "olá")
	if err != nil || id == "" || c.Total() != 1 || c.Textos(0)[0] != "olá" {
		t.Errorf("canal: %v %q", err, id)
	}
}

func TestRegrasGlobais(t *testing.T) {
	permitido := func(v float64) bool { return v == 950 || v == 1100 }
	if f := RegrasGlobais([]string{"a", "b", "a"}, permitido); len(f) != 0 {
		t.Errorf("falso positivo: %v", f)
	}
	if f := RegrasGlobais([]string{"Oi! Tudo bem?", "oi tudo bem", "OI, tudo bem??"}, permitido); len(f) != 1 {
		t.Errorf("3 iguais (normalizadas): %v", f)
	}
	if f := RegrasGlobais([]string{"x", "Oi", "oi", "x", "oi"}, permitido); len(f) != 0 {
		t.Errorf("só 2 seguidas: %v", f)
	}
	if f := RegrasGlobais([]string{"Fica R$ 950,00 por pessoa e R$ 1.100 a outra"}, permitido); len(f) != 0 {
		t.Errorf("valores permitidos recusados: %v", f)
	}
	if f := RegrasGlobais([]string{"Fica R$ 999 por pessoa"}, permitido); len(f) != 1 {
		t.Errorf("valor estranho aceito: %v", f)
	}
	if v := ValoresReais("de R$ 1.100,50 e R$950 e R$ 3.300"); len(v) != 3 || v[0] != 1100.5 || v[1] != 950 || v[2] != 3300 {
		t.Errorf("ValoresReais = %v", v)
	}
}

// ---- harness ponta a ponta, sem rede ----

// modeloFunc adapta uma funcao a llm.Modelo.
type modeloFunc func(llm.Pedido) (llm.Resposta, error)

func (f modeloFunc) Gerar(_ context.Context, p llm.Pedido) (llm.Resposta, error) { return f(p) }

func casoPorNome(t *testing.T, nome string) Caso {
	t.Helper()
	cs, err := CarregarCasos()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Nome == nome {
			return c
		}
	}
	t.Fatalf("caso %s não existe", nome)
	return Caso{}
}

// Caso 12: LLM fora do ar -> mensagem técnica + humano, sem rede.
func TestCasoLLMForaOffline(t *testing.T) {
	c := casoPorNome(t, "llm_fora")
	if !c.LLMFora {
		t.Fatal("caso llm_fora sem a flag llm_fora")
	}
	amb := NovoAmbiente(ModeloQueFalha{}, ConfigAmbiente{})
	ex := Executar(context.Background(), c, amb, &ClienteRoteiro{})
	if !ex.OK() {
		t.Fatalf("falhas: erro=%v falhas=%v\n%s", ex.Erro, ex.Falhas, FormatarTranscricao(ex))
	}
	if ex.Status != conversa.StatusHumano {
		t.Errorf("status = %s", ex.Status)
	}
	if len(ex.Primeira) != 1 || !strings.Contains(ex.Primeira[0], "problema técnico") {
		t.Errorf("mensagem técnica ausente: %v", ex.Primeira)
	}
}

// modeloRoteirizado simula um agente correto para destino_direto: busca e lista.
func modeloRoteirizado() llm.Modelo {
	return modeloFunc(func(p llm.Pedido) (llm.Resposta, error) {
		n := len(p.Mensagens)
		if n > 0 && p.Mensagens[n-1].Papel == llm.PapelFerramenta {
			var out struct {
				Dados struct {
					Opcoes []struct {
						Data, Horario string
						Preco         float64
					} `json:"opcoes"`
				} `json:"dados"`
			}
			_ = json.Unmarshal([]byte(p.Mensagens[n-1].Texto), &out)
			if len(out.Dados.Opcoes) == 0 {
				return llm.Resposta{Texto: "Não encontrei viagens agora."}, nil
			}
			o := out.Dados.Opcoes[0]
			dt, _ := time.Parse("2006-01-02", o.Data)
			return llm.Resposta{Texto: "Tem sim! A próxima sai em " + dt.Format("02/01") + " às " + o.Horario + ", por R$ " + "1.100 por pessoa."}, nil
		}
		return llm.Resposta{Chamadas: []llm.ChamadaFerramenta{{ID: "c1", Nome: "buscar_viagens", Argumentos: json.RawMessage(`{"destino":"Chapecó"}`)}}}, nil
	})
}

func TestHarnessComAgenteRoteirizado(t *testing.T) {
	c := casoPorNome(t, "destino_direto")
	amb := NovoAmbiente(modeloRoteirizado(), ConfigAmbiente{})
	ex := Executar(context.Background(), c, amb, &ClienteRoteiro{Falas: []string{"valeu, vou ver com a familia"}})
	if ex.Erro != nil {
		t.Fatalf("erro: %v\n%s", ex.Erro, FormatarTranscricao(ex))
	}
	if ex.MaxOpcoes < 1 || ex.Status != conversa.StatusBot {
		t.Errorf("opcoes=%d status=%s", ex.MaxOpcoes, ex.Status)
	}
	if !ex.OK() {
		t.Errorf("falhas: %v\n%s", ex.Falhas, FormatarTranscricao(ex))
	}
}

func TestAvaliarDetectaViolacoes(t *testing.T) {
	c := casoPorNome(t, "destino_direto")
	amb := NovoAmbiente(modeloRoteirizado(), ConfigAmbiente{})
	ex := &Execucao{
		Caso: c.Nome, Amb: amb, Status: conversa.StatusBot,
		Primeira:    []string{"Quantas pessoas vão?"},
		Transcricao: []Msg{{"CLIENTE", "Tem viagem pra Chapecó?"}, {"BOT", "Quantas pessoas vão?"}, {"BOT", "Fica R$ 77"}},
	}
	f := Avaliar(c, ex)
	joined := strings.Join(f, "\n")
	for _, want := range []string{"proibido na primeira resposta", "exige_primeira_resposta", "destino", "opções mostradas", "fora das fixtures"} {
		if !strings.Contains(joined, want) {
			t.Errorf("faltou falha %q em:\n%s", want, joined)
		}
	}
}

func quoteIn(o availability.SearchResult) pricing.QuoteInput {
	return pricing.QuoteInput{TripID: o.TripID, BoardStopID: o.BoardStopID, AlightStopID: o.AlightStopID}
}
