package ferramentas

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
)

const catalogoTTL = 5 * time.Minute

// apelidosFixos: alias normalizado -> nome normalizado da cidade.
var apelidosFixos = map[string]string{
	"moncao":       "moncao",
	"igarape":      "igarape do meio",
	"santa ines":   "santa ines",
	"sta ines":     "santa ines",
	"chapeco":      "chapeco",
	"concordia":    "concordia",
	"petrolandia":  "petrolandia",
	"campos novos": "campos novos",
	"monte carlo":  "monte carlo",
}

type indiceCatalogo struct {
	cidades []Cidade
	aliases map[string]Cidade // alias normalizado -> cidade
	chaves  []string          // aliases ordenados (determinismo do fuzzy)
}

// Catalogo guarda as cidades atendidas (cache de 5 min) e resolve nomes.
type Catalogo struct {
	fonte FonteCatalogo
	agora func() time.Time

	mu        sync.Mutex
	idx       *indiceCatalogo
	validoAte time.Time
}

func NovoCatalogo(f FonteCatalogo, agora func() time.Time) *Catalogo {
	if agora == nil {
		agora = time.Now
	}
	return &Catalogo{fonte: f, agora: agora}
}

func (c *Catalogo) carregar(ctx context.Context) (*indiceCatalogo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	agora := c.agora()
	if c.idx != nil && agora.Before(c.validoAte) {
		return c.idx, nil
	}
	cidades, err := c.fonte.Cidades(ctx)
	if err != nil {
		if c.idx != nil { // serve o cache vencido em vez de falhar
			return c.idx, nil
		}
		return nil, err
	}
	aliasesBD, err := c.fonte.Aliases(ctx)
	if err != nil {
		aliasesBD = nil // tabela de aliases e opcional
	}
	idx := &indiceCatalogo{cidades: cidades, aliases: map[string]Cidade{}}
	porNome := map[string]Cidade{}
	porID := map[string]Cidade{}
	for _, ci := range cidades {
		porNome[normalizar(ci.Nome)] = ci
		porID[ci.StopID] = ci
	}
	for alias, nome := range apelidosFixos {
		if ci, ok := porNome[nome]; ok {
			idx.aliases[alias] = ci
		}
	}
	for n, ci := range porNome {
		idx.aliases[n] = ci
	}
	for alias, id := range aliasesBD {
		if ci, ok := porID[id]; ok {
			if n := normalizar(alias); n != "" {
				idx.aliases[n] = ci
			}
		}
	}
	for k := range idx.aliases {
		idx.chaves = append(idx.chaves, k)
	}
	sort.Strings(idx.chaves)
	c.idx, c.validoAte = idx, agora.Add(catalogoTTL)
	return idx, nil
}

// Cidades devolve as cidades atendidas.
func (c *Catalogo) Cidades(ctx context.Context) ([]Cidade, error) {
	idx, err := c.carregar(ctx)
	if err != nil {
		return nil, err
	}
	return append([]Cidade(nil), idx.cidades...), nil
}

// CidadesDaUF devolve as cidades de um estado.
func (c *Catalogo) CidadesDaUF(ctx context.Context, uf string) ([]Cidade, error) {
	todas, err := c.Cidades(ctx)
	if err != nil {
		return nil, err
	}
	var out []Cidade
	for _, ci := range todas {
		if ci.UF == uf {
			out = append(out, ci)
		}
	}
	return out, nil
}

// EstadoDoTexto diz se o texto e apenas um estado ("santa catarina", "sc"...).
func (c *Catalogo) EstadoDoTexto(texto string) (uf string, ok bool) {
	uf, ok = estadosPorTexto[normalizar(texto)]
	return
}

// gruposPorUF agrupa cidades por UF: MA, SC, depois as demais em ordem alfabetica.
func gruposPorUF(cidades []Cidade) (ordem []string, grupos map[string][]Cidade) {
	grupos = map[string][]Cidade{}
	for _, ci := range cidades {
		grupos[ci.UF] = append(grupos[ci.UF], ci)
	}
	var outras []string
	for uf := range grupos {
		if uf != "MA" && uf != "SC" {
			outras = append(outras, uf)
		}
	}
	sort.Strings(outras)
	for _, uf := range append([]string{"MA", "SC"}, outras...) {
		if _, ok := grupos[uf]; ok {
			ordem = append(ordem, uf)
		}
	}
	return
}

// TextoCatalogo devolve o catalogo em texto compacto para o prompt do agente.
func (c *Catalogo) TextoCatalogo(ctx context.Context) (string, error) {
	cidades, err := c.Cidades(ctx)
	if err != nil {
		return "", err
	}
	ordem, grupos := gruposPorUF(cidades)
	partes := make([]string, 0, len(ordem))
	for _, uf := range ordem {
		itens := make([]string, 0, len(grupos[uf]))
		for _, ci := range grupos[uf] {
			item := ci.Nome
			if ci.PrecoBase > 0 {
				item += " (a partir de " + formatarReais(ci.PrecoBase) + ")"
			}
			itens = append(itens, item)
		}
		partes = append(partes, nomeUF(uf)+": "+strings.Join(itens, ", ")+".")
	}
	return strings.Join(partes, " "), nil
}

// ResolverCidade acha a cidade atendida que o texto indica. Nomes de estado
// nao resolvem cidade.
func (c *Catalogo) ResolverCidade(ctx context.Context, texto string) (conversa.Parada, bool) {
	idx, err := c.carregar(ctx)
	if err != nil {
		return conversa.Parada{}, false
	}
	n := normalizar(texto)
	if n == "" {
		return conversa.Parada{}, false
	}
	if _, ehEstado := estadosPorTexto[n]; ehEstado {
		return conversa.Parada{}, false
	}
	n = semSufixoUF(n)
	if ci, ok := idx.aliases[n]; ok {
		return paradaDe(ci), true
	}
	if len([]rune(n)) < 4 {
		return conversa.Parada{}, false
	}
	melhor := 3
	var achadas map[string]Cidade
	for _, k := range idx.chaves {
		d := distanciaLevenshtein(n, k)
		if d > 2 || d > melhor {
			continue
		}
		if d < melhor {
			melhor = d
			achadas = map[string]Cidade{}
		}
		ci := idx.aliases[k]
		achadas[ci.StopID] = ci
	}
	if len(achadas) != 1 {
		return conversa.Parada{}, false
	}
	for _, ci := range achadas {
		return paradaDe(ci), true
	}
	return conversa.Parada{}, false
}

func paradaDe(ci Cidade) conversa.Parada {
	return conversa.Parada{StopID: ci.StopID, Nome: ci.Nome, UF: ci.UF}
}
