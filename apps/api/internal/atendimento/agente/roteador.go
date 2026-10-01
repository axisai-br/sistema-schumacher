package agente

import (
	"context"
	"sort"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
)

// Intencoes devolvidas pelo Roteador (valores da pergunta "intencao").
const (
	IntencaoSaudacao           = "saudacao_apenas"
	IntencaoCidadesAtendidas   = "perguntar_cidades_atendidas"
	IntencaoBuscarViagens      = "buscar_viagens"
	IntencaoEscolherOpcao      = "escolher_opcao"
	IntencaoInformarPassageiro = "informar_passageiros"
	IntencaoFormaPagamento     = "forma_pagamento"
	IntencaoConsultarReserva   = "consultar_reserva"
	IntencaoDuvidaInformativa  = "duvida_informativa"
	IntencaoForaDoAssunto      = "fora_do_assunto"
	IntencaoOutro              = "outro"
)

// Respostas especiais das perguntas "origem" e "destino" (alem dos nomes das
// cidades) e da pergunta "opcao_escolhida".
const (
	CidadeNaoInformada = "nao_informado"
	CidadeNaoAtendida  = "cidade_nao_atendida"
	OpcaoNenhuma       = "nenhuma"
)

// EntradaRota e o que o Roteador recebe para classificar a mensagem do cliente.
type EntradaRota struct {
	Mensagens []conversa.Mensagem  // ultimas mensagens (o roteador usa ate 6)
	Estado    conversa.Estado      // estado da reserva ate aqui
	Cidades   []ferramentas.Cidade // cidades atendidas (catalogo)
}

// Rota e o veredito do Roteador sobre a ultima mensagem do cliente. Todas as
// confiancas e probabilidades ficam em 0..1.
type Rota struct {
	PedeHumano float64 // probabilidade de o cliente pedir um humano
	Irritacao  float64 // 0 (calmo) .. 1 (irritado)

	Intencao     string // uma das Intencao*; vazio se nao respondida
	ConfIntencao float64

	// Origem e Destino: nome de uma cidade atendida, CidadeNaoInformada ou
	// CidadeNaoAtendida; vazio se a pergunta nao foi feita.
	Origem, Destino         string
	ConfOrigem, ConfDestino float64

	// Opcao: "1".."n" ou OpcaoNenhuma; vazio se nao havia opcoes no estado.
	Opcao     string
	ConfOpcao float64

	// DetalhesExtras e a probabilidade de a conversa ja trazer data, periodo ou
	// numero de pessoas (a busca previa sem esses filtros nao seria fiel).
	DetalhesExtras float64

	// PedeVolta e a probabilidade de o cliente perguntar pela volta (sentido
	// oposto da rota ja buscada). So e perguntado quando o estado tem a rota.
	PedeVolta float64
}

// Roteador classifica a ultima mensagem do cliente (intencao, cidades,
// humano/irritacao) para o codigo decidir o caminho do turno.
type Roteador interface {
	Rotear(ctx context.Context, e EntradaRota) (Rota, error)
}

// FonteCidades fornece as cidades atendidas (satisfeita por *ferramentas.Catalogo).
type FonteCidades interface {
	Cidades(ctx context.Context) ([]ferramentas.Cidade, error)
}

type juizDeRoteador struct{ r Roteador }

// JuizDeRoteador adapta um Roteador a Juiz (so pede_humano e irritacao), util
// para comparar roteador e juizes nas avaliacoes.
func JuizDeRoteador(r Roteador) Juiz { return juizDeRoteador{r} }

func (j juizDeRoteador) Avaliar(ctx context.Context, ultimas []conversa.Mensagem) (Avaliacao, error) {
	rt, err := j.r.Rotear(ctx, EntradaRota{Mensagens: ultimas})
	if err != nil {
		return Avaliacao{}, err
	}
	return Avaliacao{PedeHumano: rt.PedeHumano, Irritacao: rt.Irritacao}, nil
}

var nomesEstado = map[string]string{"MA": "Maranhão", "SC": "Santa Catarina"}

// textoCidadesAtendidas monta, so com codigo, a resposta para "quais cidades
// vocês atendem?" a partir do catalogo (mesmos grupos de listar_rotas: MA, SC e
// depois as demais UFs).
func textoCidadesAtendidas(cidades []ferramentas.Cidade) string {
	grupos := map[string][]string{}
	for _, c := range cidades {
		grupos[c.UF] = append(grupos[c.UF], c.Nome)
	}
	var outras []string
	for uf := range grupos {
		if uf != "MA" && uf != "SC" {
			outras = append(outras, uf)
		}
	}
	sort.Strings(outras)
	var linhas []string
	for _, uf := range append([]string{"MA", "SC"}, outras...) {
		nomes, ok := grupos[uf]
		if !ok {
			continue
		}
		rot := nomesEstado[uf]
		if rot == "" {
			rot = uf
			if rot == "" {
				rot = "Outras"
			}
		}
		linhas = append(linhas, rot+": "+juntarNomes(nomes)+".")
	}
	return "Atendemos viagens nos dois sentidos entre estas cidades:\n\n" + strings.Join(linhas, "\n") +
		"\n\nDe qual cidade você sai ou pra onde quer ir?"
}

func juntarNomes(nomes []string) string {
	switch len(nomes) {
	case 0:
		return ""
	case 1:
		return nomes[0]
	}
	return strings.Join(nomes[:len(nomes)-1], ", ") + " e " + nomes[len(nomes)-1]
}
