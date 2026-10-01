package ferramentas

import (
	"testing"
	"time"
)

func TestResolverQuando(t *testing.T) {
	hoje := time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC) // quarta-feira
	casos := []struct {
		texto, de, ate string // vazio em de = nao reconhece
	}{
		{"quero viajar amanhã", "2026-10-01", "2026-10-01"},
		{"depois de amanha", "2026-10-02", "2026-10-02"},
		{"daqui 15 dias", "2026-10-12", "2026-10-18"},
		{"daqui a quinze dias", "2026-10-12", "2026-10-18"},
		{"dqui uns 10 dias", "2026-10-07", "2026-10-13"},
		{"daqui 2 semanas", "2026-10-11", "2026-10-17"},
		{"daqui um mês", "2026-10-23", "2026-11-06"},
		{"em 3 meses", "2026-12-23", "2027-01-06"},
		{"semana que vem", "2026-10-05", "2026-10-11"},
		{"próxima semana", "2026-10-05", "2026-10-11"},
		{"essa semana", "2026-09-30", "2026-10-04"},
		{"fim de semana", "2026-10-03", "2026-10-04"},
		{"mês que vem", "2026-10-01", "2026-10-31"},
		{"proximo mes", "2026-10-01", "2026-10-31"},
		{"fim do mês", "2026-10-21", "2026-10-31"}, // fim de setembro ja passou
		{"inicio do mes que vem", "2026-10-01", "2026-10-10"},
		{"começo de novembro", "2026-11-01", "2026-11-10"},
		{"meados de outubro", "2026-10-11", "2026-10-20"},
		{"final de dezembro", "2026-12-21", "2026-12-31"},
		{"em novembro", "2026-11-01", "2026-11-30"},
		{"pra janeiro", "2027-01-01", "2027-01-31"},
		{"quinta que vem", "2026-10-01", "2026-10-08"}, // ambiguo: esta ou a seguinte
		{"proxima sexta", "2026-10-02", "2026-10-09"},
		{"proxima terca", "2026-10-06", "2026-10-06"}, // ja e semana que vem
		{"na segunda", "2026-10-05", "2026-10-05"},
		{"segunda-feira", "2026-10-05", "2026-10-05"},
		{"terça da semana que vem", "2026-10-06", "2026-10-06"},
		{"quarta", "2026-10-07", "2026-10-07"}, // hoje e quarta: a proxima
		{"dia 15", "2026-10-15", "2026-10-15"},
		{"dia 20", "2026-10-20", "2026-10-20"},
		{"dia 31", "2026-10-31", "2026-10-31"},
		{"15/10", "2026-10-15", "2026-10-15"},
		{"10/09", "2027-09-10", "2027-09-10"}, // passado: ano seguinte
		{"12 de outubro", "2026-10-12", "2026-10-12"},
		{"dia 5 de novembro", "2026-11-05", "2026-11-05"},
		{"20 de setembro", "2027-09-20", "2027-09-20"},
		{"quero a ida dia 8 e volta dia 12", "2026-10-08", "2026-10-08"},
		{"hoje", "2026-09-30", "2026-09-30"},
		// nao sao datas
		{"a segunda opção", "", ""},
		{"somos 3 pessoas", "", ""},
		{"cpf 52998224725", "", ""},
		{"de chapeco para moncao", "", ""},
	}
	for _, c := range casos {
		p, ok := ResolverQuando(c.texto, hoje)
		if c.de == "" {
			if ok {
				t.Errorf("%q: nao deveria reconhecer, veio %s..%s", c.texto, p.De.Format("2006-01-02"), p.Ate.Format("2006-01-02"))
			}
			continue
		}
		if !ok {
			t.Errorf("%q: nao reconheceu", c.texto)
			continue
		}
		if got := p.De.Format("2006-01-02") + ".." + p.Ate.Format("2006-01-02"); got != c.de+".."+c.ate {
			t.Errorf("%q: got %s want %s..%s (%s)", c.texto, got, c.de, c.ate, p.Descricao)
		}
	}
}

func TestBuscarComQuando(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês","quando":"10/10"}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	d := dadosDe(m)
	if d["periodo_entendido"] == nil {
		t.Fatalf("sem periodo_entendido: %+v", d)
	}
	if s, _ := a.exec(t, "buscar_viagens", `{"destino":"Chapecó","quando":"quando der"}`); s.OK || s.Motivo != "quando_nao_entendido" {
		t.Fatalf("got %+v", s)
	}
}

func TestInterpretarData(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "interpretar_data", `{"expressao":"daqui 15 dias"}`)
	if !s.OK || dadosDe(m)["data_de"] == nil || dadosDe(m)["descricao"] == nil {
		t.Fatalf("%+v", m)
	}
}
