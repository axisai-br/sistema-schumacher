package agente

import (
	"strings"
	"testing"
	"time"

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
			lista, mudou, _, avisos, _ := mudancaPassageiros(c.atuais, c.texto, 0, nil)
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

func msgCli(t string) conversa.Mensagem {
	return conversa.Mensagem{Autor: conversa.AutorCliente, Texto: t}
}
func msgBot(t string) conversa.Mensagem { return conversa.Mensagem{Autor: conversa.AutorBot, Texto: t} }

func TestPendentesCompletadosDepois(t *testing.T) {
	hoje := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	rita := conversa.Passageiro{Nome: "Rita Lima", Documento: "52998224725", TipoDocumento: "CPF"}
	rogerio := conversa.Passageiro{Nome: "Rogerio Dias", Documento: "98765432100", TipoDocumento: "CPF"}
	casos := []struct {
		nome  string
		hist  []conversa.Mensagem
		lista []conversa.Passageiro
		quer  string
	}{
		{"cpf invalido e depois o certo", []conversa.Mensagem{msgCli("lucia ferreira cpf 123.456.789-00"), msgBot("O CPF de Lucia..."), msgCli("desculpa, o certo é 529.982.247-25")},
			nil, "Lucia Ferreira:52998224725"},
		{"certidao de 8 anos e depois o cpf dele", []conversa.Mensagem{msgCli("[foto de documento: certidão de nascimento, nome JOAO LIMA, nascimento 10/05/2018]"), msgBot("Recebi a certidão..."), msgCli("o cpf dele é 111.444.777-35")},
			[]conversa.Passageiro{rita}, "Rita Lima:52998224725|Joao Lima:11144477735"},
		{"cpf repetido e depois o da sandra", []conversa.Mensagem{msgCli("rogerio dias cpf 98765432100 e sandra dias cpf 98765432100"), msgBot("O documento ..."), msgCli("foi mal, o da sandra é 39053344705")},
			[]conversa.Passageiro{rogerio}, "Rogerio Dias:98765432100|Sandra Dias:39053344705"},
		{"mae sem documento e depois o cpf dela", []conversa.Mensagem{msgCli("eu, marcia alves cpf 52998224725, meu filho de 4 anos, Pedro Alves, e minha mãe rosa alves"), msgBot("Falta o CPF..."), msgCli("rosa alves cpf 11144477735")},
			[]conversa.Passageiro{{Nome: "Marcia Alves", Documento: "52998224725", TipoDocumento: "CPF"}, {Nome: "Pedro Alves", CriancaAte5: true}}, "Marcia Alves:52998224725|Pedro Alves:crianca|Rosa Alves:11144477735"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			pend := pendentesDoHistorico(c.hist, c.lista, hoje)
			lista, mudou, _, avisos, _ := mudancaPassageiros(c.lista, c.hist[len(c.hist)-1].Texto, 0, pend)
			if !mudou || nomesDocs(lista) != c.quer {
				t.Fatalf("lista=%q mudou=%v avisos=%v pend=%v", nomesDocs(lista), mudou, avisos, pend)
			}
		})
	}
}

func TestCitadosForaSeguramReserva(t *testing.T) {
	_, _, _, avisos, fora := mudancaPassageiros(nil, "eu, marcia alves cpf 52998224725, meu filho de 4 anos, Pedro Alves, e minha mãe rosa alves", 0, nil)
	if fora != 1 || len(avisos) != 1 || !strings.Contains(avisos[0], "Rosa Alves") {
		t.Fatalf("fora=%d avisos=%v", fora, avisos)
	}
	lista, _, _, _, _ := mudancaPassageiros(nil, "eu, marcia alves cpf 52998224725, meu filho de 4 anos, Pedro Alves, e minha mãe rosa alves", 0, nil)
	if nomesDocs(lista) != "Marcia Alves:52998224725|Pedro Alves:crianca" {
		t.Fatalf("lista=%q", nomesDocs(lista))
	}
	if _, _, _, _, fora := mudancaPassageiros(nil, "rogerio dias cpf 98765432100 e sandra dias cpf 98765432100", 0, nil); fora != 1 {
		t.Fatalf("cpf repetido: fora=%d", fora)
	}
}

func TestNomeDepoisDeFrase(t *testing.T) {
	lista, mudou, _, _, _ := mudancaPassageiros(nil, "o terceiro é o caio costa cpf 84434891030", 0, nil)
	if !mudou || nomesDocs(lista) != "Caio Costa:84434891030" {
		t.Fatalf("lista=%q", nomesDocs(lista))
	}
}
