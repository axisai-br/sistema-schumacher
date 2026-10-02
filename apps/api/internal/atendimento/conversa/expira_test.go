package conversa

import (
	"context"
	"testing"
	"time"
)

func TestEstadoExpiraSemMensagens(t *testing.T) {
	ctx := context.Background()
	agora := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s := NewStoreMem(func() time.Time { return agora })
	entrada := func(id string) Conversa {
		c, _, _, err := s.RegistrarEntrada(ctx, NovaEntrada{Contato: "5511@s", Telefone: "5511", Texto: "oi", ProvedorID: id})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c := entrada("m1")
	if _, err := s.SalvarEstado(ctx, c.ID, Estado{PessoasInformadas: 2}, c.Versao); err != nil {
		t.Fatal(err)
	}
	agora = agora.Add(71 * time.Hour)
	if c = entrada("m2"); c.Estado.PessoasInformadas != 2 {
		t.Fatalf("antes do prazo o estado fica: %+v", c.Estado)
	}
	agora = agora.Add(73 * time.Hour)
	if c = entrada("m3"); c.Estado.PessoasInformadas != 0 {
		t.Fatalf("depois do prazo o estado zera: %+v", c.Estado)
	}
	s.DefinirExpiracao(0)
	if _, err := s.SalvarEstado(ctx, c.ID, Estado{PessoasInformadas: 1}, c.Versao); err != nil {
		t.Fatal(err)
	}
	agora = agora.Add(500 * time.Hour)
	if c = entrada("m4"); c.Estado.PessoasInformadas != 1 {
		t.Fatalf("expiracao desligada: %+v", c.Estado)
	}
}
