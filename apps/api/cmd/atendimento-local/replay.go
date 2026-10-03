package main

// Modo -replay: reenvia ao modelo um pedido gravado por ATD_DUMP_DIR (o
// contexto exato de um turno), n vezes, com o contexto original ou com uma
// variante. Serve para separar erro de capacidade do modelo de erro de
// contexto: mesmo turno, mesmo modelo, contexto diferente.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"schumacher-tur/api/internal/atendimento/agente"
	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
)

type registroDump struct {
	Pedido llm.Pedido `json:"pedido"`
}

func modoReplay(ctx context.Context, cfg config, modelo llm.Modelo, arquivos []string, n int, variante string, stdout, stderr io.Writer) int {
	for _, arq := range arquivos {
		b, err := os.ReadFile(arq)
		if err != nil {
			fmt.Fprintf(stderr, "erro: %v\n", err)
			return 2
		}
		var reg registroDump
		if err := json.Unmarshal(b, &reg); err != nil {
			fmt.Fprintf(stderr, "erro em %s: %v\n", arq, err)
			return 2
		}
		p := reg.Pedido
		p.Modelo = cfg.Modelo
		if variante == "v2" {
			p.Instrucoes = contextoV2(p.Instrucoes, cfg.Sinal)
		}
		ultimo := ""
		for i := len(p.Mensagens) - 1; i >= 0; i-- {
			if p.Mensagens[i].Papel == llm.PapelUsuario {
				ultimo = p.Mensagens[i].Texto
				break
			}
		}
		fmt.Fprintf(stdout, "##### %s [%s] cliente: %q\n", arq, variante, ultimo)
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, 3)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				t0 := time.Now()
				c, cancel := context.WithTimeout(ctx, cfg.Prov.Timeout+5*time.Second)
				defer cancel()
				r, err := modelo.Gerar(c, p)
				var out string
				switch {
				case err != nil:
					out = "ERRO " + err.Error()
				default:
					var ch []string
					for _, c := range r.Chamadas {
						ch = append(ch, c.Nome+" "+string(c.Argumentos))
					}
					out = strings.ReplaceAll(strings.TrimSpace(r.Texto), "\n", " | ")
					if len(ch) > 0 {
						out += " {CHAMA: " + strings.Join(ch, " ; ") + "}"
					}
				}
				mu.Lock()
				fmt.Fprintf(stdout, "  #%d (%.1fs) %s\n", i+1, time.Since(t0).Seconds(), out)
				mu.Unlock()
			}(i)
		}
		wg.Wait()
	}
	return 0
}

// contextoV2 reescreve o prompt de sistema de um pedido gravado: troca o
// ESTADO em JSON cru por uma situacao legivel (documentos mascarados, valores
// calculados, opcoes numeradas e proximo passo) e acrescenta regras de
// honestidade, persona e assuntos sem resposta na politica.
func contextoV2(instr string, sinal float64) string {
	const hEst, hCtx = "\n\n# ESTADO DA RESERVA\n", "\n\n# CONTEXTO\n"
	i, j := strings.Index(instr, hEst), strings.Index(instr, hCtx)
	if i < 0 || j < i {
		return instr
	}
	bloco := instr[i+len(hEst) : j]
	jsonEst := bloco
	if k := strings.Index(bloco, "\nPendências: "); k >= 0 {
		jsonEst = bloco[:k]
	}
	var e conversa.Estado
	if err := json.Unmarshal([]byte(jsonEst), &e); err != nil {
		return instr
	}
	pol := instr[:i]
	pol = strings.Replace(pol, "- Bagagem: consigo orientar sobre bagagens comuns do passageiro. Para itens especiais ou algo que não seja bagagem comum, o suporte atende: +55 49 9886-2222.",
		"- Bagagem, animais, duração da viagem, comodidades do ônibus, descontos, comprovante ou nota fiscal: NÃO temos essas informações aqui. Diga que o suporte confirma: +55 49 9886-2222.", 1)

	return pol + "\n\n" + agente.RegrasSempre + "\n\n# SITUAÇÃO DA COMPRA AGORA (fonte da verdade)\n" +
		agente.TextoSituacao(e, sinal) + instr[j:]
}
