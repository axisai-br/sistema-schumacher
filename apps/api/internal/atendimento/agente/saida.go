package agente

import (
	"regexp"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
)

// Filtro de saida: ultima barreira antes do WhatsApp, vale para todo texto
// (LLM ou template). Corrige o que da para corrigir (CPF completo; o markdown
// vira formato do WhatsApp em canal.PartesWhatsApp, no envio) e
// troca o que nao da (texto degenerado, vazamento de persona ou de ferramenta)
// por uma resposta montada em codigo.

// TextoPersona responde perguntas sobre "quem e voce / qual modelo".
const TextoPersona = "Sou o Shabas, atendente virtual da Schumacher Tur. 😊"

var (
	rePersona       = regexp.MustCompile(`(?i)nvidia|nemotron|modelo de linguagem|language model|\bLLM\b|\bGPT\b|openai|intelig[eê]ncia artificial treinad|prompt( de sistema)?\b|minhas instru[cç][oõ]es`)
	reCPFSaida      = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)
	nomesFerramenta = []string{"buscar_viagens", "escolher_viagem", "registrar_passageiros", "criar_reserva", "gerar_pix",
		"trocar_pagamento", "consultar_reserva", "transferir_para_humano", "remover_trecho", "interpretar_data", "listar_rotas"}
)

// filtrarSaida devolve o texto a enviar e o motivo de cada ajuste (vazio =
// nada mudou).
func filtrarSaida(texto string, est conversa.Estado) (string, []string) {
	var motivos []string
	if p := textoQuebrado(texto); p != "" {
		return textoProximoPasso(est), []string{"quebrado: " + p}
	}
	if degenerado(texto) {
		return textoProximoPasso(est), []string{"degenerado"}
	}
	baixo := strings.ToLower(texto)
	for _, f := range nomesFerramenta {
		if strings.Contains(baixo, f) {
			return TextoPersona + "\n\n" + textoProximoPasso(est), []string{"ferramenta: " + f}
		}
	}
	if m := rePersona.FindString(texto); m != "" {
		return TextoPersona + "\n\n" + textoProximoPasso(est), []string{"persona: " + m}
	}
	// CPF completo nunca sai; o copia-e-cola do PIX (linha 000201...) fica.
	linhas := strings.Split(texto, "\n")
	mascarou := false
	for i, l := range linhas {
		if strings.Contains(l, "000201") {
			continue
		}
		if novo := reCPFSaida.ReplaceAllStringFunc(l, conversa.MascararDocumento); novo != l {
			linhas[i], mascarou = novo, true
		}
	}
	if mascarou {
		texto, motivos = strings.Join(linhas, "\n"), append(motivos, "cpf")
	}
	return texto, motivos
}

// degenerado: o mesmo pedaco curto (1-4 caracteres, sem espaco) repetido 8+
// vezes seguidas ("-0-0-0-0...", "0000000000"), sinal de geracao quebrada.
func degenerado(texto string) bool {
	r := []rune(texto)
	for l := 1; l <= 4; l++ {
		for i := 0; i+l*8 <= len(r); i++ {
			seg := string(r[i : i+l])
			if strings.TrimSpace(seg) != seg || seg == "." || seg == "-" || seg == "=" || seg == "_" {
				continue
			}
			rep := 1
			for j := i + l; j+l <= len(r) && string(r[j:j+l]) == seg; j += l {
				rep++
			}
			if rep >= 8 {
				return true
			}
		}
	}
	return false
}
