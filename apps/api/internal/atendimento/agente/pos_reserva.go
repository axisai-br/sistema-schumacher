package agente

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"schumacher-tur/api/internal/atendimento/ferramentas"
)

// limiarPosReserva: confianca minima para agir sobre o pedido pos-reserva.
const limiarPosReserva = 0.85

// posReserva trata, sem LLM quando possivel, o que o cliente pede depois da
// reserva criada. ok=false: segue o fluxo normal.
func (a *Agente) posReserva(ctx context.Context, tc *turno, rt Rota) (resultadoRota, string, bool) {
	est := tc.estado
	if !est.AlgumReservado() || rt.ConfPosReserva < limiarPosReserva {
		return resultadoRota{}, "", false
	}
	switch rt.PosReserva {
	case PosTrocarPagamento:
		nova := ""
		if (rt.Pagamento == PagamentoIntegral || rt.Pagamento == PagamentoSinal) && rt.ConfPagamento >= limiarPosReserva {
			nova = rt.Pagamento
		} else if est.Pagamento == PagamentoIntegral {
			nova = PagamentoSinal
		} else if est.Pagamento == PagamentoSinal {
			nova = PagamentoIntegral
		}
		if nova == "" || nova == est.Pagamento {
			return resultadoRota{}, "", false
		}
		pre, ok := a.preExecutar(ctx, tc, "trocar_pagamento", map[string]any{"pagamento": nova})
		if !ok {
			if ultimoMotivo(tc) == "ja_pago" {
				return resultadoRota{transf: &transf{motivo: "cliente quer trocar a forma de pagamento, mas o PIX ja foi pago"}}, "pos_reserva_ja_pago", true
			}
			return resultadoRota{}, "pos_reserva_troca_falhou", true
		}
		if px := pixDoTurno(tc.resultados); len(px) > 0 {
			nome := map[string]string{PagamentoIntegral: "valor integral", PagamentoSinal: "sinal"}[nova]
			txt := fmt.Sprintf("Pronto, troquei para %s ✅ O PIX anterior foi cancelado; use este novo:\n\n", nome) +
				strings.TrimPrefix(textoFechamento(px, tc.resultados, nova), "Reserva feita! ✅ Seguem os PIX (copia e cola):\n")
			return resultadoRota{resposta: txt}, "pos_reserva_troca_pagamento", true
		}
		return resultadoRota{pre: pre}, "pos_reserva_troca_pagamento", true
	case PosTrocarViagem:
		return resultadoRota{transf: &transf{motivo: "cliente quer trocar data ou viagem de reserva ja criada"}}, "pos_reserva_transfere", true
	case PosTrocarPassageiro:
		return resultadoRota{transf: &transf{motivo: "cliente quer trocar passageiro de reserva ja criada"}}, "pos_reserva_transfere", true
	case PosCancelar:
		return resultadoRota{transf: &transf{motivo: "cliente quer cancelar a reserva"}}, "pos_reserva_transfere", true
	case PosJaPaguei:
		if _, ok := a.preExecutar(ctx, tc, "consultar_reserva", map[string]any{}); ok {
			return resultadoRota{resposta: textoStatusPagamento(tc.resultados)}, "pos_reserva_ja_paguei", true
		}
		return resultadoRota{}, "pos_reserva_consulta_falhou", true
	case PosPixDeNovo:
		pre, ok := a.preExecutar(ctx, tc, "gerar_pix", map[string]any{})
		if !ok {
			return resultadoRota{}, "pos_reserva_pix_falhou", true
		}
		if px := pixDoTurno(tc.resultados); len(px) > 0 {
			txt := strings.Replace(textoPix(px), "Reserva feita! ✅ Seguem os PIX", "Aqui está o PIX de novo", 1)
			return resultadoRota{resposta: txt}, "pos_reserva_pix", true
		}
		return resultadoRota{pre: pre}, "pos_reserva_pix", true
	}
	return resultadoRota{}, "", false
}

// ultimoMotivo e o motivo de falha da ultima ferramenta executada no turno.
func ultimoMotivo(tc *turno) string {
	for i := len(tc.passos) - 1; i >= 0; i-- {
		if tc.passos[i].Tipo == "ferramenta" {
			if s, ok := tc.passos[i].Saida.(ferramentas.Saida); ok {
				return s.Motivo
			}
			return ""
		}
	}
	return ""
}

// textoStatusPagamento responde "ja paguei" a partir de consultar_reserva.
func textoStatusPagamento(resultados []string) string {
	var s struct {
		Dados struct {
			Reservas []struct {
				Codigo    string `json:"codigo_reserva"`
				Pagamento struct {
					Status string `json:"status"`
				} `json:"pagamento"`
			} `json:"reservas"`
		} `json:"dados"`
	}
	if len(resultados) == 0 || json.Unmarshal([]byte(resultados[len(resultados)-1]), &s) != nil || len(s.Dados.Reservas) == 0 {
		return textoPagamentoPendente
	}
	var pagas, pendentes []string
	for _, r := range s.Dados.Reservas {
		if st := strings.ToUpper(r.Pagamento.Status); st == "PAID" || st == "CONFIRMED" || st == "CAPTURED" {
			pagas = append(pagas, r.Codigo)
		} else {
			pendentes = append(pendentes, r.Codigo)
		}
	}
	if len(pendentes) == 0 {
		return "Pagamento confirmado ✅ Sua reserva " + strings.Join(pagas, ", ") + " está garantida. Boa viagem! 😊"
	}
	if len(pagas) > 0 {
		return fmt.Sprintf("O pagamento da reserva %s já foi confirmado ✅ O da reserva %s ainda não apareceu aqui. %s",
			strings.Join(pagas, ", "), strings.Join(pendentes, ", "), textoPagamentoPendente)
	}
	return textoPagamentoPendente
}

const textoPagamentoPendente = "Ainda não apareceu a confirmação do PIX aqui. Normalmente cai em alguns minutos; assim que confirmar, a reserva fica garantida. Se já faz mais de 30 minutos, me manda o comprovante que a equipe confere. 😊"
