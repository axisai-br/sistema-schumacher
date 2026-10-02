package agente

import (
	"strings"
	"testing"

	"schumacher-tur/api/internal/atendimento/conversa"
)

func nomesDocs(ps []conversa.Passageiro) string {
	var b []string
	for _, p := range ps {
		s := p.Nome
		if p.Documento != "" {
			s += ":" + p.Documento
		}
		if p.CriancaAte5 {
			s += ":crianca"
		}
		b = append(b, s)
	}
	return strings.Join(b, "|")
}

func TestMudancaPassageiros(t *testing.T) {
	joao := conversa.Passageiro{Nome: "João Pereira", Documento: "11144477735", TipoDocumento: "CPF"}
	bruno := conversa.Passageiro{Nome: "Bruno Teixeira", Documento: "11144477735", TipoDocumento: "CPF"}
	ana := conversa.Passageiro{Nome: "Ana Beatriz", Documento: "52998224725", TipoDocumento: "CPF"}
	casos := []struct {
		nome    string
		atuais  []conversa.Passageiro
		texto   string
		quer    string
		avisos  int
		naoMuda bool
	}{
		{"cpf sem a palavra", nil, "tiago prado 12345678909", "Tiago Prado:12345678909", 0, false},
		{"numero que nao e cpf sem a palavra", nil, "joana prado 12345678900", "", 0, true},
		{"rg escrito", nil, "e a sogra, helena prado, RG 4.512.887 SSP/SC", "Helena Prado:4512887", 0, false},
		{"lista em mensagens juntas", nil, "joana prado cpf 111.444.777-35\ntiago prado 12345678909\ne a sogra, helena prado, RG 4.512.887 SSP/SC\npronto, são esses 3",
			"Joana Prado:11144477735|Tiago Prado:12345678909|Helena Prado:4512887", 0, false},
		{"nome longo", nil, "maria das graças de oliveira albuquerque neto, cpf 390.533.447-05", "Maria das Graças de Oliveira Albuquerque Neto:39053344705", 0, false},
		{"se chama com idade e cpf", nil, "ele se chama lucas martins, tem 10 anos, cpf 123.456.789-09", "Lucas Martins:12345678909", 0, false},
		{"correcao pelo nome", []conversa.Passageiro{joao}, "maria pereira cpf 39053344705\nerrei o cpf da maria, o certo é 987.654.321-00",
			"João Pereira:11144477735|Maria Pereira:98765432100", 0, false},
		{"correcao sem nome com um adulto", []conversa.Passageiro{bruno}, "opa, o cpf tá errado, o certo é 529.982.247-25", "Bruno Teixeira:52998224725", 0, false},
		{"troca de passageiro", []conversa.Passageiro{ana, bruno}, "na verdade o bruno não vai mais, no lugar dele vai o carlos eduardo mendes cpf 84434891030",
			"Ana Beatriz:52998224725|Carlos Eduardo Mendes:84434891030", 0, false},
		{"cpf repetido vira aviso", []conversa.Passageiro{ana}, "sandra lima cpf 529.982.247-25", "Ana Beatriz:52998224725", 1, true},
		{"cpf invalido vira aviso", nil, "lucia ferreira cpf 123.456.789-00", "", 1, true},
		{"bebe sem documento", nil, "bebê sofia reis", "Sofia Reis:crianca", 0, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			lista, mudou, _, avisos := mudancaPassageiros(c.atuais, c.texto, 0)
			if mudou == c.naoMuda {
				t.Errorf("mudou=%v", mudou)
			}
			if got := nomesDocs(lista); got != c.quer {
				t.Errorf("lista=%q, quer %q", got, c.quer)
			}
			if len(avisos) != c.avisos {
				t.Errorf("avisos=%v", avisos)
			}
		})
	}
}

func TestQuantidadeDoTexto(t *testing.T) {
	casos := map[string]int{
		"somos 3":                          3,
		"sair de videira, são 3 pessoas":   3,
		"só eu":                            1,
		"eu e minha esposa":                2,
		"eu e meu filho":                   2,
		"eu e mais 2":                      3,
		"vamos em duas":                    2,
		"quero ir de monção pra fraiburgo": 0,
	}
	for txt, quer := range casos {
		n, ok := quantidadeDoTexto(txt)
		if (quer == 0 && ok) || (quer > 0 && n != quer) {
			t.Errorf("%q: n=%d ok=%v, quer %d", txt, n, ok, quer)
		}
	}
}

func TestTrocarTrechosPelaRota(t *testing.T) {
	e := conversa.Estado{
		Origem:  &conversa.Parada{Nome: "Igarapé do Meio"},
		Destino: &conversa.Parada{Nome: "Ituporanga"},
		Trechos: []conversa.Trecho{{Viagem: conversa.Opcao{TripID: "trip-01", Origem: "Santa Inês", Destino: "Videira"}}},
	}
	if !trocarTrechosPelaRota(&e) || len(e.Trechos) != 0 {
		t.Fatalf("rota nova deveria tirar o trecho sem reserva: %+v", e.Trechos)
	}
	volta := conversa.Estado{
		Origem:  &conversa.Parada{Nome: "Videira"},
		Destino: &conversa.Parada{Nome: "Santa Inês"},
		Trechos: []conversa.Trecho{{Viagem: conversa.Opcao{Origem: "Santa Inês", Destino: "Videira"}}},
	}
	if trocarTrechosPelaRota(&volta) || len(volta.Trechos) != 1 {
		t.Fatal("a volta nao troca o trecho de ida")
	}
}

func TestOpcaoClaraMesmoOnibusOutraRota(t *testing.T) {
	a := &Agente{}
	est := conversa.Estado{
		Opcoes:  []conversa.Opcao{{Numero: 1, TripID: "trip-01", BoardStopID: "ts-igarape", AlightStopID: "ts-ituporanga"}},
		Trechos: []conversa.Trecho{{Viagem: conversa.Opcao{TripID: "trip-01", BoardStopID: "ts-santa-ines", AlightStopID: "ts-videira"}}},
	}
	rt := Rota{Intencao: IntencaoEscolherOpcao, ConfIntencao: 0.99, Opcao: "1", ConfOpcao: 0.99}
	if n, ok := a.opcaoClara(rt, est); !ok || n != 1 {
		t.Fatalf("mesmo onibus com outras paradas e outra viagem: n=%d ok=%v", n, ok)
	}
}
