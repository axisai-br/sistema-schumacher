package agente

import (
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
)

func TestOpcaoDoTexto(t *testing.T) {
	hoje := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC) // sexta
	ops := []conversa.Opcao{
		{Numero: 1, Data: "2026-10-05", Horario: "07:30", Preco: 1100}, // seg
		{Numero: 2, Data: "2026-10-12", Horario: "07:30", Preco: 1100}, // seg
		{Numero: 3, Data: "2026-10-15", Horario: "06:00", Preco: 950},  // qui
	}
	est := conversa.Estado{Opcoes: ops}
	casos := map[string]int{
		"a segunda opção 👍":            2,
		"a primeira":                   1,
		"2":                            2,
		"opção 3 por favor":            3,
		"dia 12":                       2,
		"15/10":                        3,
		"a de quinta":                  3,
		"o mais cedo possível":         1,
		"a mais barata":                3,
		"segunda q vem":                1,
		"quero ir de chapeco":          0,
		"somos 2":                      0,
		"joao lima cpf 529.982.247-25": 0,
	}
	for txt, quer := range casos {
		if n, _ := opcaoDoTexto(txt, est, hoje); n != quer {
			t.Errorf("%q: opcao %d, quer %d", txt, n, quer)
		}
	}
	so := conversa.Estado{Opcoes: ops[:1]}
	if n, _ := opcaoDoTexto("pode ser essa", so, hoje); n != 1 {
		t.Errorf("com uma opcao, 'essa' escolhe: %d", n)
	}
	if n, _ := opcaoDoTexto("pode ser essa", est, hoje); n != 0 {
		t.Errorf("com varias opcoes, 'essa' e ambiguo: %d", n)
	}
}
