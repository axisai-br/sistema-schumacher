package ferramentas

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestResolverCidade(t *testing.T) {
	cat := novoCatalogoFake()
	ctx := context.Background()
	casos := []struct {
		texto string
		nome  string // vazio = nao resolve
	}{
		{"fraiburgo", "Fraiburgo"},
		{"Chapeco", "Chapecó"},
		{"sta ines", "Santa Inês"},
		{"Moncao/MA", "Monção"},
		{"chapecó - sc", "Chapecó"},
		{"Santa Inês, MA", "Santa Inês"},
		{"igarape", "Igarapé do Meio"},
		{"Campos Novos SC", "Campos Novos"},
		{"monte carlo", "Monte Carlo"},
		{"Fraiburgu", "Fraiburgo"}, // typo, distancia 1
		{"chapeko", "Chapecó"},     // typo
		{"santa catarina", ""},
		{"SC", ""},
		{"maranhão", ""},
		{"ma", ""},
		{"São Paulo", ""},
		{"Pomerode", ""},
		{"", ""},
	}
	for _, c := range casos {
		p, ok := cat.ResolverCidade(ctx, c.texto)
		if c.nome == "" {
			if ok {
				t.Errorf("%q nao deveria resolver, got %+v", c.texto, p)
			}
			continue
		}
		if !ok || p.Nome != c.nome {
			t.Errorf("%q: got %+v ok=%v, want %s", c.texto, p, ok, c.nome)
		}
	}
	p, _ := cat.ResolverCidade(ctx, "fraiburgo")
	if p.StopID != "sc-fr" || p.UF != "SC" {
		t.Errorf("parada inesperada: %+v", p)
	}
}

func TestResolverCidadeAliasDoBancoEEmpateFuzzy(t *testing.T) {
	f := &fonteFake{cidades: cidadesReais(), aliases: map[string]string{"floripa": "sc-fr", "desconhecida": "x"}}
	cat := NovoCatalogo(f, func() time.Time { return agoraFixa })
	if p, ok := cat.ResolverCidade(context.Background(), "Floripa"); !ok || p.StopID != "sc-fr" {
		t.Fatalf("alias do banco: %+v %v", p, ok)
	}
	// "seara" vs "sera": unico candidato; mas "videir" vs nada ambiguo.
	f2 := &fonteFake{cidades: []Cidade{{"a", "Campo Alto", "SC", 0}, {"b", "Campo Belo", "SC", 0}}}
	cat2 := NovoCatalogo(f2, nil)
	if _, ok := cat2.ResolverCidade(context.Background(), "campo alno"); !ok {
		t.Fatal("campo alno deveria resolver para Campo Alto (distancia 1)")
	}
	if _, ok := cat2.ResolverCidade(context.Background(), "campo aero"); ok {
		t.Fatal("empate Alto/Belo (distancia 2 para ambos) nao deveria resolver")
	}
}

func TestTextoCatalogo(t *testing.T) {
	txt, err := novoCatalogoFake().TextoCatalogo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, quer := range []string{
		"Maranhão: Santa Inês (a partir de R$ 950), Monção",
		"Santa Catarina: Fraiburgo (a partir de R$ 950)",
		"Campos Novos (a partir de R$ 1.000)",
		"Chapecó (a partir de R$ 1.100)",
	} {
		if !strings.Contains(txt, quer) {
			t.Errorf("faltou %q em %q", quer, txt)
		}
	}
	if strings.Index(txt, "Maranhão") > strings.Index(txt, "Santa Catarina") {
		t.Error("MA deve vir antes de SC")
	}
}

func TestCatalogoCache(t *testing.T) {
	f := &fonteFake{cidades: cidadesReais()}
	agora := agoraFixa
	cat := NovoCatalogo(f, func() time.Time { return agora })
	ctx := context.Background()
	_, _ = cat.TextoCatalogo(ctx)
	_, _ = cat.TextoCatalogo(ctx)
	agora = agora.Add(4 * time.Minute)
	_, _ = cat.ResolverCidade(ctx, "videira")
	if f.chamadas != 1 {
		t.Fatalf("cache deveria evitar recarga, chamadas=%d", f.chamadas)
	}
	agora = agora.Add(2 * time.Minute)
	_, _ = cat.TextoCatalogo(ctx)
	if f.chamadas != 2 {
		t.Fatalf("cache vencido deveria recarregar, chamadas=%d", f.chamadas)
	}
}

func TestFormatarReais(t *testing.T) {
	for in, want := range map[float64]string{950: "R$ 950", 1000: "R$ 1.000", 1234.5: "R$ 1.234,50", 12345678: "R$ 12.345.678"} {
		if got := formatarReais(in); got != want {
			t.Errorf("%v: got %q want %q", in, got, want)
		}
	}
}
