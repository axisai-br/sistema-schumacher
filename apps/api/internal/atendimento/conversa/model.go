// Package conversa guarda o modelo de dominio e a persistencia do atendimento v2.
package conversa

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusBot       Status = "BOT"
	StatusHumano    Status = "HUMANO"
	StatusEncerrada Status = "ENCERRADA"
)

type Autor string

const (
	AutorCliente Autor = "CLIENTE"
	AutorBot     Autor = "BOT"
	AutorHumano  Autor = "HUMANO"
)

type Direcao string

const (
	DirecaoEntrada Direcao = "ENTRADA"
	DirecaoSaida   Direcao = "SAIDA"
)

type Tipo string

const (
	TipoTexto     Tipo = "TEXTO"
	TipoAudio     Tipo = "AUDIO"
	TipoImagem    Tipo = "IMAGEM"
	TipoDocumento Tipo = "DOCUMENTO"
	TipoOutro     Tipo = "OUTRO"
)

type Conversa struct {
	ID, Canal, Contato, Telefone, Nome string
	Status                             Status
	ResponsavelID                      string // vazio se nenhum
	Estado                             Estado
	Versao                             int
	PendenteDesde, UltimaEntradaEm     *time.Time
	HumanoAte                          *time.Time
	CriadoEm, AtualizadoEm             time.Time
}

type Mensagem struct {
	ID, ConversaID string
	Direcao        Direcao
	Autor          Autor
	Tipo           Tipo
	Texto          string
	Midia          map[string]any // pode ser nil
	ProvedorID     string         // vazio se nenhum
	TurnoID        string
	CriadoEm       time.Time
}

type Parada struct {
	StopID string `json:"stop_id"`
	Nome   string `json:"nome"`
	UF     string `json:"uf"`
}

type Opcao struct {
	Numero       int     `json:"numero"`
	TripID       string  `json:"trip_id"`
	BoardStopID  string  `json:"board_stop_id"`
	AlightStopID string  `json:"alight_stop_id"`
	Origem       string  `json:"origem"`
	Destino      string  `json:"destino"`
	Data         string  `json:"data"` // AAAA-MM-DD
	Horario      string  `json:"horario"`
	Preco        float64 `json:"preco"`
	Vagas        int     `json:"vagas"`
	Pacote       string  `json:"pacote,omitempty"`
}

type Passageiro struct {
	Nome          string `json:"nome"`
	Documento     string `json:"documento"`
	TipoDocumento string `json:"tipo_documento"` // CPF|RG|CNH
	CriancaAte5   bool   `json:"crianca_ate_5"`
}

// MaxTrechos e o limite de trechos (viagens) por conversa.
const MaxTrechos = 4

// Trecho e uma viagem escolhida na compra, com a reserva e o PIX dela. Ida e
// volta (ou varias viagens) sao trechos da mesma conversa: uma reserva e um PIX
// por trecho, porque bookings e payments trabalham com uma viagem por reserva.
type Trecho struct {
	Viagem      Opcao  `json:"viagem"`
	ReservaID   string `json:"reserva_id,omitempty"`
	PagamentoID string `json:"pagamento_id,omitempty"`
}

// Rota devolve "Origem para Destino" do trecho.
func (t Trecho) Rota() string { return t.Viagem.Origem + " para " + t.Viagem.Destino }

type Estado struct {
	Origem            *Parada      `json:"origem,omitempty"`
	Destino           *Parada      `json:"destino,omitempty"`
	Opcoes            []Opcao      `json:"opcoes,omitempty"`
	Trechos           []Trecho     `json:"trechos,omitempty"`
	PessoasInformadas int          `json:"pessoas_informadas,omitempty"` // quantidade dita pelo cliente antes dos nomes
	Passageiros       []Passageiro `json:"passageiros,omitempty"`        // os mesmos para todos os trechos
	Pagamento         string       `json:"pagamento,omitempty"`          // "integral" | "sinal"; vale para todos os trechos
	Falhas            int          `json:"falhas"`
	MotivoHumano      string       `json:"motivo_humano,omitempty"`
}

// UnmarshalJSON le tambem o formato antigo (uma unica viagem em "viagem",
// "reserva_id" e "pagamento_id" na raiz) e o converte para um Trecho.
func (e *Estado) UnmarshalJSON(b []byte) error {
	type alias Estado
	aux := struct {
		*alias
		Viagem      *Opcao `json:"viagem"`
		ReservaID   string `json:"reserva_id"`
		PagamentoID string `json:"pagamento_id"`
	}{alias: (*alias)(e)}
	*e = Estado{}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if len(e.Trechos) == 0 && (aux.Viagem != nil || aux.ReservaID != "" || aux.PagamentoID != "") {
		t := Trecho{ReservaID: aux.ReservaID, PagamentoID: aux.PagamentoID}
		if aux.Viagem != nil {
			t.Viagem = *aux.Viagem
		}
		e.Trechos = []Trecho{t}
	}
	return nil
}

// AlgumReservado diz se algum trecho ja tem reserva criada.
func (e Estado) AlgumReservado() bool {
	for _, t := range e.Trechos {
		if t.ReservaID != "" {
			return true
		}
	}
	return false
}

// TodosReservados diz se ha trechos e todos ja tem reserva criada.
func (e Estado) TodosReservados() bool {
	for _, t := range e.Trechos {
		if t.ReservaID == "" {
			return false
		}
	}
	return len(e.Trechos) > 0
}

// Pendencias lista, em ordem, o que falta para fechar a compra. Vazio quando
// tudo esta feito. Nao ha campo de fase: e sempre calculado do estado. Reserva
// e PIX sao pendencias por trecho (sem sufixo quando ha um unico trecho).
func (e Estado) Pendencias() []string {
	var p []string
	if len(e.Trechos) == 0 {
		p = append(p, "escolher viagem")
	}
	if len(e.Passageiros) == 0 {
		p = append(p, "informar passageiros (nome e documento)")
	} else {
		faltam := 0
		for _, pax := range e.Passageiros {
			if strings.TrimSpace(pax.Nome) == "" || strings.TrimSpace(pax.Documento) == "" {
				faltam++
			}
		}
		if e.PessoasInformadas > len(e.Passageiros) {
			faltam += e.PessoasInformadas - len(e.Passageiros)
		}
		if faltam > 0 {
			p = append(p, fmt.Sprintf("completar dados de %d passageiro(s)", faltam))
		}
	}
	if e.Pagamento == "" {
		p = append(p, "escolher pagamento: integral ou sinal")
	}
	for i, t := range e.Trechos {
		suf := ""
		if len(e.Trechos) > 1 {
			suf = fmt.Sprintf(" do trecho %d (%s)", i+1, t.Rota())
		}
		if t.ReservaID == "" {
			p = append(p, "criar reserva"+suf)
		}
		if t.PagamentoID == "" {
			p = append(p, "gerar PIX"+suf)
		}
	}
	return p
}

// Pagantes conta os passageiros que nao sao criancas ate 5 anos.
func (e Estado) Pagantes() int {
	n := 0
	for _, p := range e.Passageiros {
		if !p.CriancaAte5 {
			n++
		}
	}
	return n
}

// Resumo devolve um texto em portugues para o atendente humano. Documentos
// aparecem mascarados (somente os 3 ultimos digitos).
func (e Estado) Resumo() string {
	var b strings.Builder
	switch {
	case e.Origem != nil && e.Destino != nil:
		fmt.Fprintf(&b, "Rota: %s para %s\n", nomeParada(e.Origem), nomeParada(e.Destino))
	case e.Origem != nil:
		fmt.Fprintf(&b, "Rota: origem %s, destino a definir\n", nomeParada(e.Origem))
	case e.Destino != nil:
		fmt.Fprintf(&b, "Rota: origem a definir, destino %s\n", nomeParada(e.Destino))
	default:
		b.WriteString("Rota: ainda nao definida\n")
	}
	if len(e.Trechos) == 0 {
		b.WriteString("Viagem: nenhuma escolhida\n")
	}
	for i, t := range e.Trechos {
		v := t.Viagem
		fmt.Fprintf(&b, "Trecho %d: %s para %s em %s as %s, R$ %.2f por pessoa; reserva: %s; PIX: %s\n",
			i+1, v.Origem, v.Destino, formatarData(v.Data), v.Horario, v.Preco,
			valorOu(t.ReservaID, "nao criada"), pixResumo(t.PagamentoID))
	}
	if len(e.Passageiros) > 0 {
		b.WriteString("Passageiros:\n")
		for i, p := range e.Passageiros {
			fmt.Fprintf(&b, "  %d. %s", i+1, valorOu(p.Nome, "(sem nome)"))
			if p.Documento != "" {
				tipo := p.TipoDocumento
				if tipo == "" {
					tipo = "doc"
				}
				fmt.Fprintf(&b, " - %s %s", tipo, MascararDocumento(p.Documento))
			}
			if p.CriancaAte5 {
				b.WriteString(" (crianca ate 5 anos)")
			}
			b.WriteString("\n")
		}
	} else if e.PessoasInformadas > 0 {
		fmt.Fprintf(&b, "Passageiros: %d informado(s), sem dados ainda\n", e.PessoasInformadas)
	} else {
		b.WriteString("Passageiros: nenhum informado\n")
	}
	fmt.Fprintf(&b, "Pagamento: %s\n", valorOu(e.Pagamento, "nao escolhido"))
	if pend := e.Pendencias(); len(pend) > 0 {
		fmt.Fprintf(&b, "Pendencias: %s\n", strings.Join(pend, "; "))
	} else {
		b.WriteString("Pendencias: nenhuma\n")
	}
	if e.MotivoHumano != "" {
		fmt.Fprintf(&b, "Motivo da transferencia: %s\n", e.MotivoHumano)
	}
	return strings.TrimRight(b.String(), "\n")
}

// MascararDocumento mostra somente os 3 ultimos digitos do documento.
func MascararDocumento(doc string) string {
	var digitos []rune
	for _, r := range doc {
		if r >= '0' && r <= '9' {
			digitos = append(digitos, r)
		}
	}
	if len(digitos) == 0 {
		return "***"
	}
	if len(digitos) <= 3 {
		return "***"
	}
	return "***" + string(digitos[len(digitos)-3:])
}

func nomeParada(p *Parada) string {
	if p.UF != "" {
		return p.Nome + "/" + p.UF
	}
	return p.Nome
}

func formatarData(d string) string {
	if t, err := time.Parse("2006-01-02", d); err == nil {
		return t.Format("02/01/2006")
	}
	return d
}

func valorOu(v, padrao string) string {
	if strings.TrimSpace(v) == "" {
		return padrao
	}
	return v
}

func pixResumo(id string) string {
	if id == "" {
		return "nao gerado"
	}
	return "gerado (" + id + ")"
}
