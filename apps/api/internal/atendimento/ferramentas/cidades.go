package ferramentas

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cidade e uma cidade atendida (uma parada em stops).
type Cidade struct {
	StopID    string
	Nome      string
	UF        string
	PrecoBase float64 // menor preco ativo de/para a cidade; 0 se desconhecido
}

// FonteCatalogo fornece cidades e apelidos ao Catalogo.
type FonteCatalogo interface {
	Cidades(ctx context.Context) ([]Cidade, error)
	// Aliases devolve alias -> stop_id (pode ser vazio).
	Aliases(ctx context.Context) (map[string]string, error)
}

type fontePG struct{ pool *pgxpool.Pool }

// NovaFontePG le cidades e aliases do Postgres.
func NovaFontePG(pool *pgxpool.Pool) FonteCatalogo { return &fontePG{pool: pool} }

const sqlCidades = `
with ativos as (
  select origin_stop_id as stop_id, price from route_segment_prices
   where upper(coalesce(status, 'ACTIVE')) = 'ACTIVE'
  union all
  select destination_stop_id as stop_id, price from route_segment_prices
   where upper(coalesce(status, 'ACTIVE')) = 'ACTIVE'
), precos as (
  select stop_id, min(price)::float8 as preco from ativos group by stop_id
), em_viagens as (
  select distinct ts.stop_id
    from trip_stops ts
    join trips t on t.trip_id = ts.trip_id
   where ts.is_active = true
     and t.trip_date >= current_date
     and upper(coalesce(t.status, '')) in ('SCHEDULED', 'IN_PROGRESS', 'ATIVO', 'ACTIVE')
)
select s.stop_id, s.display_name, coalesce(p.preco, 0)::float8
  from stops s
  left join precos p on p.stop_id = s.stop_id
 where (p.stop_id is not null or s.stop_id in (select stop_id from em_viagens))
   and lower(s.display_name) not like '%teste%'
 order by s.display_name`

func (f *fontePG) Cidades(ctx context.Context) ([]Cidade, error) {
	rows, err := f.pool.Query(ctx, sqlCidades)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cidade
	for rows.Next() {
		var id, display string
		var preco float64
		if err := rows.Scan(&id, &display, &preco); err != nil {
			return nil, err
		}
		nome, uf := separarNomeUF(display)
		out = append(out, Cidade{StopID: id, Nome: nome, UF: uf, PrecoBase: preco})
	}
	return out, rows.Err()
}

func (f *fontePG) Aliases(ctx context.Context) (map[string]string, error) {
	rows, err := f.pool.Query(ctx, `select alias, stop_id from atd_cidades_alias`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, err
		}
		out[alias] = id
	}
	return out, rows.Err()
}

// separarNomeUF quebra "Fraiburgo/SC" em ("Fraiburgo", "SC").
func separarNomeUF(display string) (string, string) {
	display = strings.TrimSpace(display)
	if i := strings.LastIndex(display, "/"); i >= 0 {
		return strings.TrimSpace(display[:i]), strings.ToUpper(strings.TrimSpace(display[i+1:]))
	}
	return display, ""
}

var substAcentos = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// normalizar: minusculas, sem acento, pontuacao vira espaco, espacos colapsados.
func normalizar(s string) string {
	s = substAcentos.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// semSufixoUF remove "sc"/"ma" soltos no fim do texto ja normalizado
// ("chapeco sc" -> "chapeco"). Nunca esvazia o texto.
func semSufixoUF(n string) string {
	tokens := strings.Fields(n)
	for len(tokens) > 1 {
		ult := tokens[len(tokens)-1]
		if ult != "sc" && ult != "ma" {
			break
		}
		tokens = tokens[:len(tokens)-1]
	}
	return strings.Join(tokens, " ")
}

var estadosPorTexto = map[string]string{
	"sc": "SC", "santa catarina": "SC", "estado de santa catarina": "SC",
	"ma": "MA", "maranhao": "MA", "estado do maranhao": "MA", "estado de maranhao": "MA",
}

var nomesUF = map[string]string{"MA": "Maranhão", "SC": "Santa Catarina"}

func nomeUF(uf string) string {
	if n, ok := nomesUF[uf]; ok {
		return n
	}
	if uf == "" {
		return "Outras"
	}
	return uf
}

// distanciaLevenshtein entre duas strings (por runas).
func distanciaLevenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			custo := 1
			if ra[i-1] == rb[j-1] {
				custo = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+custo)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
