package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
)

// placar resume um roteiro: chegou a reserva/PIX, transferiu, quantos turnos
// usaram o LLM e a latencia.
type placar struct {
	Roteiro    string
	Turnos     int
	ComLLM     int // turnos com ao menos uma chamada ao LLM
	ChamadasLL int // chamadas ao LLM somadas
	Reservas   int
	Pix        int
	Transferiu bool
	Duracoes   []time.Duration
}

// registrar soma um turno ao placar.
func (p *placar) registrar(t conversa.Turno, dur time.Duration) {
	p.Turnos++
	p.Duracoes = append(p.Duracoes, dur)
	n := 0
	for _, ps := range t.Passos {
		if ps.Tipo == "llm" {
			n++
		}
	}
	p.ChamadasLL += n
	if n > 0 {
		p.ComLLM++
	}
}

func (p placar) mediana() time.Duration {
	if len(p.Duracoes) == 0 {
		return 0
	}
	d := append([]time.Duration(nil), p.Duracoes...)
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[len(d)/2]
}

func (p placar) maxima() time.Duration {
	var m time.Duration
	for _, d := range p.Duracoes {
		m = max(m, d)
	}
	return m
}

// linha formata o placar numa linha da tabela.
func (p placar) linha() string {
	sim := func(b bool) string {
		if b {
			return "sim"
		}
		return "nao"
	}
	return fmt.Sprintf("%-32s reservas=%d pix=%d transferiu=%-3s turnos=%-2d sem_llm=%-2d chamadas_llm=%-2d lat_mediana=%s lat_max=%s",
		p.Roteiro, p.Reservas, p.Pix, sim(p.Transferiu), p.Turnos, p.Turnos-p.ComLLM, p.ChamadasLL, fmtDur(p.mediana()), fmtDur(p.maxima()))
}

// imprimirPlacares escreve a tabela final de -roteiro todos.
func imprimirPlacares(w io.Writer, ps []placar) {
	fmt.Fprintln(w, "\n=== PLACAR ===")
	turnos, semLLM := 0, 0
	var durs []time.Duration
	for _, p := range ps {
		fmt.Fprintln(w, p.linha())
		turnos += p.Turnos
		semLLM += p.Turnos - p.ComLLM
		durs = append(durs, p.Duracoes...)
	}
	tot := placar{Roteiro: "TOTAL", Duracoes: durs}
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 40))
	fmt.Fprintf(w, "turnos=%d sem_llm=%d (%.0f%%) lat_mediana=%s lat_max=%s\n", turnos, semLLM,
		100*float64(semLLM)/float64(max(turnos, 1)), fmtDur(tot.mediana()), fmtDur(tot.maxima()))
}
