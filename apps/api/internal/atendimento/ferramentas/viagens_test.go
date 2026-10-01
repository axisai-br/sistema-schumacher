package ferramentas

import (
	"testing"

	"schumacher-tur/api/internal/atendimento/conversa"
)

func TestListarRotas(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "listar_rotas", `{}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	estados := dadosDe(m)["estados"].([]any)
	if len(estados) != 2 {
		t.Fatalf("esperava 2 estados: %+v", estados)
	}
	sc := estados[1].(map[string]any)
	if sc["uf"] != "SC" || len(sc["cidades"].([]any)) != 10 {
		t.Fatalf("SC inesperado: %+v", sc)
	}
}

func TestPadraoRegistraNoveNaOrdem(t *testing.T) {
	a := novoAmbiente(t)
	quer := []string{"listar_rotas", "buscar_viagens", "escolher_viagem", "remover_trecho", "registrar_passageiros", "criar_reserva", "gerar_pix", "consultar_reserva", "transferir_para_humano"}
	defs := a.reg.Defs()
	if len(defs) != len(quer) {
		t.Fatalf("got %d defs", len(defs))
	}
	for i, d := range defs {
		if d.Nome != quer[i] || d.Descricao == "" || len(d.Parametros) == 0 {
			t.Errorf("def %d inesperada: %+v", i, d.Nome)
		}
	}
}

func TestBuscarSomenteDestino(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "buscar_viagens", `{"destino":"Chapeco"}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	d := dadosDe(m)
	ops := d["opcoes"].([]any)
	if len(ops) != 1 { // so t4 tem destino Chapeco
		t.Fatalf("opcoes: %+v", ops)
	}
	f := a.b.filtros[0]
	if f.OriginStopID != "" || f.DestinationStopID != "sc-ch" || !f.OnlyActive || f.Limit != 10 || f.Qty != 0 {
		t.Fatalf("filtro inesperado: %+v", f)
	}
	if f.DateFrom.Format("2006-01-02") != "2026-09-30" || f.DateTo.Format("2006-01-02") != "2026-11-29" {
		t.Fatalf("janela padrao: %v - %v", f.DateFrom, f.DateTo)
	}
	e := a.ctx.Estado
	if e.Origem != nil || e.Destino == nil || e.Destino.StopID != "sc-ch" {
		t.Fatalf("estado: %+v", e)
	}
	if len(e.Opcoes) != 1 || e.Opcoes[0].Numero != 1 || e.Opcoes[0].Origem != "Santa Inês" || e.Opcoes[0].Horario != "07:00" {
		t.Fatalf("opcoes no estado: %+v", e.Opcoes)
	}
}

func TestBuscarOrigemEDestinoNumeraOpcoes(t *testing.T) {
	a := novoAmbiente(t)
	s, _ := a.exec(t, "buscar_viagens", `{"origem":"chapecó - sc","destino":"sta ines"}`)
	if !s.OK {
		t.Fatal("deveria ok")
	}
	e := a.ctx.Estado
	if len(e.Opcoes) != 2 || e.Opcoes[0].Numero != 1 || e.Opcoes[1].Numero != 2 {
		t.Fatalf("%+v", e.Opcoes)
	}
	o := e.Opcoes[0]
	if o.TripID != "t1" || o.Data != "2026-10-10" || o.Preco != 1100 || o.Vagas != 12 || o.BoardStopID == "" || o.AlightStopID == "" {
		t.Fatalf("opcao: %+v", o)
	}
}

func TestBuscarCidadeNaoAtendida(t *testing.T) {
	for _, cidade := range []string{"São Paulo", "Pomerode"} {
		a := novoAmbiente(t)
		s, m := a.exec(t, "buscar_viagens", `{"destino":"`+cidade+`"}`)
		if s.OK || s.Motivo != "cidade_nao_atendida" {
			t.Fatalf("%s: %+v", cidade, m)
		}
		d := dadosDe(m)
		if d["cidade_informada"] != cidade || len(d["cidades_atendidas"].([]any)) != 13 {
			t.Fatalf("dados: %+v", d)
		}
		if len(a.b.filtros) != 0 {
			t.Fatal("nao deveria buscar")
		}
	}
}

func TestBuscarSoEstado(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "buscar_viagens", `{"destino":"santa catarina"}`)
	if s.OK || s.Motivo != "informe_a_cidade" {
		t.Fatalf("%+v", m)
	}
	if cs := dadosDe(m)["cidades"].([]any); len(cs) != 10 || cs[0] != "Fraiburgo" {
		t.Fatalf("cidades: %+v", cs)
	}
	s, m = a.exec(t, "buscar_viagens", `{"origem":"MA"}`)
	if s.Motivo != "informe_a_cidade" || len(dadosDe(m)["cidades"].([]any)) != 3 {
		t.Fatalf("MA: %+v", m)
	}
}

func TestBuscarSemOrigemNemDestino(t *testing.T) {
	a := novoAmbiente(t)
	if s, _ := a.exec(t, "buscar_viagens", `{"pessoas":2}`); s.OK || s.Motivo != "informe_origem_ou_destino" {
		t.Fatalf("%+v", s)
	}
}

func TestBuscarPessoasGravaEstadoEFiltraVagas(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês","pessoas":3}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	if a.ctx.Estado.PessoasInformadas != 3 {
		t.Fatalf("pessoas: %d", a.ctx.Estado.PessoasInformadas)
	}
	// t2 tem 1 vaga: fica de fora
	if len(a.ctx.Estado.Opcoes) != 1 || a.ctx.Estado.Opcoes[0].TripID != "t1" {
		t.Fatalf("opcoes: %+v", a.ctx.Estado.Opcoes)
	}
	if dadosDe(m)["sem_vaga_para_pessoas_count"] != float64(1) {
		t.Fatalf("dados: %+v", dadosDe(m))
	}
}

func TestBuscarTodasLotadasDiferenciaSemViagem(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "buscar_viagens", `{"origem":"Videira","destino":"Monção","pessoas":5}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	d := dadosDe(m)
	if d["sem_vaga_para_pessoas"] != true || len(d["opcoes"].([]any)) != 0 || len(d["viagens_lotadas"].([]any)) != 1 {
		t.Fatalf("dados: %+v", d)
	}
	if len(a.ctx.Estado.Opcoes) != 0 {
		t.Fatal("estado deveria ficar sem opcoes")
	}
}

func TestBuscarSemResultadosSugereProximaData(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "buscar_viagens", `{"origem":"Videira","destino":"Monção","data_de":"2026-10-15"}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	d := dadosDe(m)
	if len(d["opcoes"].([]any)) != 0 || d["mensagem"] != "sem viagens nesse período" {
		t.Fatalf("dados: %+v", d)
	}
	prox := d["proxima_data_disponivel"].(map[string]any)
	if prox["data"] != "2026-10-12" {
		t.Fatalf("proxima: %+v", prox)
	}
	if len(a.b.filtros) != 2 || a.b.filtros[0].DateFrom.Format("2006-01-02") != "2026-10-15" || a.b.filtros[0].DateTo.Format("2006-01-02") != "2026-10-15" {
		t.Fatalf("filtros: %+v", a.b.filtros)
	}
	if a.b.filtros[1].DateTo != nil {
		t.Fatal("re-busca deveria ser sem data final")
	}
}

func TestBuscarSemViagemNenhuma(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "buscar_viagens", `{"origem":"Seara","destino":"Monção"}`)
	if !s.OK || dadosDe(m)["proxima_data_disponivel"] != nil || len(dadosDe(m)["opcoes"].([]any)) != 0 {
		t.Fatalf("%+v", m)
	}
}

func TestBuscarDatasInvalidas(t *testing.T) {
	a := novoAmbiente(t)
	if s, _ := a.exec(t, "buscar_viagens", `{"destino":"Chapecó","data_de":"amanhã"}`); s.Motivo != "data_invalida" {
		t.Fatalf("%+v", s)
	}
	if s, _ := a.exec(t, "buscar_viagens", `{"destino":"Chapecó","data_de":"2026-10-10","data_ate":"2026-10-01"}`); s.Motivo != "periodo_invalido" {
		t.Fatalf("%+v", s)
	}
	if s, _ := a.exec(t, "buscar_viagens", `{"destino":"Chapecó","data_ate":"2026-09-01"}`); s.Motivo != "data_no_passado" {
		t.Fatalf("%+v", s)
	}
}

func TestBuscarPessoasComoString(t *testing.T) {
	a := novoAmbiente(t)
	if s, _ := a.exec(t, "buscar_viagens", `{"destino":"Chapecó","pessoas":"2"}`); !s.OK || a.ctx.Estado.PessoasInformadas != 2 {
		t.Fatalf("%+v", s)
	}
}

func TestBuscarNaoApagaTrechosEscolhidos(t *testing.T) {
	a := novoAmbiente(t)
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	a.exec(t, "escolher_viagem", `{"opcao":1}`)
	if len(a.ctx.Estado.Trechos) != 1 {
		t.Fatal("trecho deveria estar escolhido")
	}
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`) // mesma busca mantem
	if len(a.ctx.Estado.Trechos) != 1 {
		t.Fatal("trecho deveria ser mantido")
	}
	// busca da volta (e ate de outra rota): a ida continua la
	a.exec(t, "buscar_viagens", `{"origem":"Santa Inês","destino":"Chapecó"}`)
	a.exec(t, "buscar_viagens", `{"origem":"Videira","destino":"Monção"}`)
	if len(a.ctx.Estado.Trechos) != 1 || a.ctx.Estado.Trechos[0].Viagem.TripID != "t1" {
		t.Fatalf("a ida nao pode ser apagada pela busca: %+v", a.ctx.Estado.Trechos)
	}
}

func TestBuscarErroDoServico(t *testing.T) {
	a := novoAmbiente(t)
	a.b.err = errBoom
	if s, _ := a.exec(t, "buscar_viagens", `{"destino":"Chapecó"}`); s.OK || s.Motivo != "erro_busca" {
		t.Fatalf("%+v", s)
	}
}

func TestEscolherViagem(t *testing.T) {
	a := novoAmbiente(t)
	if s, m := a.exec(t, "escolher_viagem", `{"opcao":1}`); s.OK || s.Motivo != "opcao_inexistente" {
		t.Fatalf("sem busca: %+v", m)
	}
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	s, m := a.exec(t, "escolher_viagem", `{"opcao":9}`)
	if s.OK || s.Motivo != "opcao_inexistente" || len(dadosDe(m)["opcoes_validas"].([]any)) != 2 {
		t.Fatalf("opcao 9: %+v", m)
	}
	if len(a.ctx.Estado.Trechos) != 0 {
		t.Fatal("nao deveria gravar viagem")
	}
	s, m = a.exec(t, "escolher_viagem", `{"opcao":"1"}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	var v *conversa.Opcao
	if len(a.ctx.Estado.Trechos) == 1 {
		v = &a.ctx.Estado.Trechos[0].Viagem
	}
	if v == nil || v.TripID != "t1" || v.Vagas != 12 {
		t.Fatalf("viagem: %+v", v)
	}
	// a rebusca usou os stop ids corretos e a data
	ult := a.b.filtros[len(a.b.filtros)-1]
	if ult.OriginStopID != "sc-ch" || ult.DestinationStopID != "ma-si" || ult.DateFrom.Format("2006-01-02") != "2026-10-10" {
		t.Fatalf("revalidacao: %+v", ult)
	}
}

func TestEscolherViagemSemVagasNaRevalidacao(t *testing.T) {
	a := novoAmbiente(t)
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	// vagas acabam entre a busca e a escolha
	base := viagensPadrao()
	base[0].SeatsAvailable = 0
	a.b.resultados = buscadorDe(base...).resultados
	s, m := a.exec(t, "escolher_viagem", `{"opcao":1}`)
	if s.OK || s.Motivo != "sem_vagas_suficientes" {
		t.Fatalf("%+v", m)
	}
	if len(a.ctx.Estado.Trechos) != 0 {
		t.Fatal("nao deveria gravar")
	}
	// viagem sumiu
	a.b.resultados = buscadorDe().resultados
	if s, _ := a.exec(t, "escolher_viagem", `{"opcao":1}`); s.Motivo != "viagem_indisponivel" {
		t.Fatalf("%+v", s)
	}
}

func TestEscolherViagemRespeitaPessoasInformadas(t *testing.T) {
	a := novoAmbiente(t)
	a.exec(t, "buscar_viagens", `{"origem":"Videira","destino":"Monção"}`) // t3 tem 3 vagas
	a.ctx.Estado.PessoasInformadas = 4
	if s, _ := a.exec(t, "escolher_viagem", `{"opcao":1}`); s.Motivo != "sem_vagas_suficientes" {
		t.Fatalf("%+v", s)
	}
}

var _ = conversa.Estado{}
