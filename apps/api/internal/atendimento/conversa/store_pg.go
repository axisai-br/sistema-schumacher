package conversa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type storePG struct {
	pool   *pgxpool.Pool
	expira time.Duration
}

// NewStorePG cria o Store sobre PostgreSQL (tabelas atd_*, migracao 0023),
// com o estado expirando apos ExpiraEstadoPadrao sem mensagens.
func NewStorePG(pool *pgxpool.Pool) Store {
	return &storePG{pool: pool, expira: ExpiraEstadoPadrao}
}

// NewStorePGExpira e NewStorePG com outro prazo de expiracao do estado
// (0 desliga: o estado so zera quando a equipe encerra).
func NewStorePGExpira(pool *pgxpool.Pool, expira time.Duration) Store {
	return &storePG{pool: pool, expira: expira}
}

const colunasConversa = `
	id::text, canal, contato, coalesce(telefone, ''), coalesce(nome, ''), status,
	coalesce(responsavel_id::text, ''), estado, versao,
	pendente_desde, ultima_entrada_em, humano_ate, criado_em, atualizado_em`

const colunasMensagem = `
	id::text, conversa_id::text, direcao, autor, tipo, coalesce(texto, ''), midia,
	coalesce(provedor_id, ''), coalesce(turno_id::text, ''), criado_em`

func scanConversa(row pgx.Row) (Conversa, error) {
	var c Conversa
	var status string
	var estado []byte
	err := row.Scan(&c.ID, &c.Canal, &c.Contato, &c.Telefone, &c.Nome, &status,
		&c.ResponsavelID, &estado, &c.Versao,
		&c.PendenteDesde, &c.UltimaEntradaEm, &c.HumanoAte, &c.CriadoEm, &c.AtualizadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Conversa{}, ErrNaoEncontrada
		}
		return Conversa{}, err
	}
	c.Status = Status(status)
	if len(estado) > 0 {
		if err := json.Unmarshal(estado, &c.Estado); err != nil {
			return Conversa{}, fmt.Errorf("conversa: estado inválido: %w", err)
		}
	}
	return c, nil
}

func scanMensagem(row pgx.Row) (Mensagem, error) {
	var m Mensagem
	var direcao, autor, tipo string
	var midia []byte
	err := row.Scan(&m.ID, &m.ConversaID, &direcao, &autor, &tipo, &m.Texto, &midia,
		&m.ProvedorID, &m.TurnoID, &m.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Mensagem{}, ErrNaoEncontrada
		}
		return Mensagem{}, err
	}
	m.Direcao, m.Autor, m.Tipo = Direcao(direcao), Autor(autor), Tipo(tipo)
	if len(midia) > 0 && string(midia) != "null" {
		if err := json.Unmarshal(midia, &m.Midia); err != nil {
			return Mensagem{}, fmt.Errorf("conversa: mídia inválida: %w", err)
		}
	}
	return m, nil
}

func jsonOuNil(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *storePG) mensagemPorProvedor(ctx context.Context, q pgx.Tx, provedorID string) (Mensagem, bool, error) {
	row := q.QueryRow(ctx, `select `+colunasMensagem+` from atd_mensagens where provedor_id = $1`, provedorID)
	m, err := scanMensagem(row)
	if errors.Is(err, ErrNaoEncontrada) {
		return Mensagem{}, false, nil
	}
	return m, err == nil, err
}

func (s *storePG) RegistrarEntrada(ctx context.Context, in NovaEntrada) (Conversa, Mensagem, bool, error) {
	// Relogio do servidor (now() do banco) em pendente_desde, ultima_entrada_em,
	// humano_ate e criado_em; o horario do provedor fica so em recebida_em.
	var recebida any
	if !in.RecebidaEm.IsZero() {
		recebida = in.RecebidaEm
	}
	canal := in.Canal
	if canal == "" {
		canal = "WHATSAPP"
	}
	tipo := in.Tipo
	if tipo == "" {
		tipo = TipoTexto
	}
	autor := in.Autor
	if autor == "" {
		autor = AutorCliente
	}
	midia, err := jsonOuNil(in.Midia)
	if err != nil {
		return Conversa{}, Mensagem{}, false, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Conversa{}, Mensagem{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if in.ProvedorID != "" {
		m, ok, err := s.mensagemPorProvedor(ctx, tx, in.ProvedorID)
		if err != nil {
			return Conversa{}, Mensagem{}, false, err
		}
		if ok {
			c, err := scanConversa(tx.QueryRow(ctx, `select `+colunasConversa+` from atd_conversas where id = $1::uuid`, m.ConversaID))
			return c, m, true, err
		}
	}

	var c Conversa
	direcao := DirecaoEntrada
	switch autor {
	case AutorCliente:
		c, err = scanConversa(tx.QueryRow(ctx, `
			insert into atd_conversas (canal, contato, telefone, nome, pendente_desde, ultima_entrada_em)
			values ($1, $2, nullif($3, ''), nullif($4, ''), now(), now())
			on conflict (canal, contato) do update set
				telefone = coalesce(nullif(excluded.telefone, ''), atd_conversas.telefone),
				nome = coalesce(nullif(excluded.nome, ''), atd_conversas.nome),
				pendente_desde = coalesce(atd_conversas.pendente_desde, excluded.pendente_desde),
				ultima_entrada_em = excluded.ultima_entrada_em,
				status = case when atd_conversas.status = 'ENCERRADA' then 'BOT' else atd_conversas.status end,
				-- Compra nova: atendimento encerrado, ou bot parado ha mais que o prazo ($5 s).
				estado = case when atd_conversas.status = 'ENCERRADA'
					or ($5::float8 > 0 and atd_conversas.status = 'BOT' and atd_conversas.ultima_entrada_em < now() - $5::float8 * interval '1 second')
					then '{}'::jsonb else atd_conversas.estado end,
				versao = case when atd_conversas.status = 'ENCERRADA'
					or ($5::float8 > 0 and atd_conversas.status = 'BOT' and atd_conversas.ultima_entrada_em < now() - $5::float8 * interval '1 second')
					then atd_conversas.versao + 1 else atd_conversas.versao end,
				responsavel_id = case when atd_conversas.status = 'ENCERRADA' then null else atd_conversas.responsavel_id end,
				humano_ate = case when atd_conversas.status = 'ENCERRADA' then null else atd_conversas.humano_ate end,
				atualizado_em = now()
			returning `+colunasConversa,
			canal, in.Contato, in.Telefone, in.Nome, s.expira.Seconds()))
	case AutorHumano:
		direcao = DirecaoSaida
		c, err = scanConversa(tx.QueryRow(ctx, `
			insert into atd_conversas (canal, contato, telefone, nome, status, humano_ate)
			values ($1, $2, nullif($3, ''), nullif($4, ''), 'HUMANO', now() + $5::float8 * interval '1 second')
			on conflict (canal, contato) do update set
				status = 'HUMANO',
				humano_ate = excluded.humano_ate,
				pendente_desde = null,
				atualizado_em = now()
			returning `+colunasConversa,
			canal, in.Contato, in.Telefone, in.Nome, PausaHumanoPadrao.Seconds()))
	default:
		direcao = DirecaoSaida
		c, err = scanConversa(tx.QueryRow(ctx, `
			insert into atd_conversas (canal, contato, telefone, nome)
			values ($1, $2, nullif($3, ''), nullif($4, ''))
			on conflict (canal, contato) do update set atualizado_em = now()
			returning `+colunasConversa,
			canal, in.Contato, in.Telefone, in.Nome))
	}
	if err != nil {
		return Conversa{}, Mensagem{}, false, err
	}

	m, err := scanMensagem(tx.QueryRow(ctx, `
		insert into atd_mensagens (conversa_id, direcao, autor, tipo, texto, midia, provedor_id, recebida_em)
		values ($1::uuid, $2, $3, $4, $5, $6::jsonb, nullif($7, ''), $8::timestamptz)
		on conflict (provedor_id) do nothing
		returning `+colunasMensagem,
		c.ID, string(direcao), string(autor), string(tipo), in.Texto, midia, in.ProvedorID, recebida))
	if errors.Is(err, ErrNaoEncontrada) {
		// Corrida: outro webhook inseriu o mesmo provedor_id entre o select e o insert.
		// Desfaz o upsert da conversa e devolve a mensagem existente.
		_ = tx.Rollback(ctx)
		return s.duplicadaFora(ctx, in.ProvedorID)
	}
	if err != nil {
		return Conversa{}, Mensagem{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Conversa{}, Mensagem{}, false, err
	}
	return c, m, false, nil
}

func (s *storePG) duplicadaFora(ctx context.Context, provedorID string) (Conversa, Mensagem, bool, error) {
	m, err := scanMensagem(s.pool.QueryRow(ctx, `select `+colunasMensagem+` from atd_mensagens where provedor_id = $1`, provedorID))
	if err != nil {
		return Conversa{}, Mensagem{}, false, err
	}
	c, err := s.Obter(ctx, m.ConversaID)
	return c, m, true, err
}

func (s *storePG) RegistrarSaida(ctx context.Context, conversaID string, autor Autor, texto, provedorID, turnoID string) (Mensagem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Mensagem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	m, err := scanMensagem(tx.QueryRow(ctx, `
		insert into atd_mensagens (conversa_id, direcao, autor, tipo, texto, provedor_id, turno_id)
		values ($1::uuid, 'SAIDA', $2, 'TEXTO', $3, nullif($4, ''), nullif($5, '')::uuid)
		on conflict (provedor_id) do update set provedor_id = excluded.provedor_id
		returning `+colunasMensagem,
		conversaID, string(autor), texto, provedorID, turnoID))
	if err != nil {
		return Mensagem{}, err
	}
	if _, err := tx.Exec(ctx, `update atd_conversas set atualizado_em = now() where id = $1::uuid`, conversaID); err != nil {
		return Mensagem{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Mensagem{}, err
	}
	return m, nil
}

func (s *storePG) ProvedorIDConhecido(ctx context.Context, provedorID string) (bool, error) {
	if provedorID == "" {
		return false, nil
	}
	var ok bool
	err := s.pool.QueryRow(ctx,
		`select exists(select 1 from atd_mensagens where provedor_id = $1 and direcao = 'SAIDA')`, provedorID).Scan(&ok)
	return ok, err
}

func (s *storePG) Obter(ctx context.Context, id string) (Conversa, error) {
	return scanConversa(s.pool.QueryRow(ctx, `select `+colunasConversa+` from atd_conversas where id = $1::uuid`, id))
}

func (s *storePG) Historico(ctx context.Context, conversaID string, limite int) ([]Mensagem, error) {
	if limite <= 0 {
		limite = 20
	}
	rows, err := s.pool.Query(ctx, `
		select `+colunasMensagem+` from (
			select * from atd_mensagens where conversa_id = $1::uuid
			order by criado_em desc, seq desc limit $2
		) t order by criado_em asc, seq asc`, conversaID, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return coletarMensagens(rows)
}

func (s *storePG) EntradasPendentes(ctx context.Context, conversaID string, desde time.Time) ([]Mensagem, error) {
	rows, err := s.pool.Query(ctx, `
		select `+colunasMensagem+` from atd_mensagens
		where conversa_id = $1::uuid and autor = 'CLIENTE' and criado_em >= $2
		order by criado_em asc, seq asc`, conversaID, desde)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return coletarMensagens(rows)
}

func coletarMensagens(rows pgx.Rows) ([]Mensagem, error) {
	var out []Mensagem
	for rows.Next() {
		m, err := scanMensagem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type lockPG struct {
	pool *pgxpool.Pool
	c    Conversa
	once sync.Once
	err  error
}

func (l *lockPG) Conversa() Conversa { return l.c }

func (l *lockPG) Liberar(ctx context.Context) error {
	l.once.Do(func() {
		_, l.err = l.pool.Exec(ctx, `update atd_conversas set processando_ate = null where id = $1::uuid`, l.c.ID)
	})
	return l.err
}

func (s *storePG) ReivindicarDevida(ctx context.Context, debounce time.Duration) (Lock, bool, error) {
	c, err := scanConversa(s.pool.QueryRow(ctx, `
		update atd_conversas set processando_ate = now() + $2::float8 * interval '1 second'
		where id = (
			select id from atd_conversas
			where status = 'BOT'
			  and pendente_desde is not null
			  and ultima_entrada_em <= now() - $1::float8 * interval '1 second'
			  and (processando_ate is null or processando_ate < now())
			order by pendente_desde
			for update skip locked
			limit 1
		)
		returning `+colunasConversa, debounce.Seconds(), LeaseDuracao.Seconds()))
	if errors.Is(err, ErrNaoEncontrada) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &lockPG{pool: s.pool, c: c}, true, nil
}

func (s *storePG) ChegouEntradaDepois(ctx context.Context, conversaID string, t time.Time) (bool, error) {
	var chegou bool
	err := s.pool.QueryRow(ctx,
		`select coalesce(ultima_entrada_em > $2, false) from atd_conversas where id = $1::uuid`,
		conversaID, t).Scan(&chegou)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNaoEncontrada
	}
	return chegou, err
}

func (s *storePG) SalvarEstado(ctx context.Context, conversaID string, estado Estado, versaoEsperada int) (int, error) {
	b, err := json.Marshal(estado)
	if err != nil {
		return 0, err
	}
	var nova int
	err = s.pool.QueryRow(ctx, `
		update atd_conversas set estado = $2::jsonb, versao = versao + 1, atualizado_em = now()
		where id = $1::uuid and versao = $3
		returning versao`, conversaID, string(b), versaoEsperada).Scan(&nova)
	if errors.Is(err, pgx.ErrNoRows) {
		var existe bool
		if qerr := s.pool.QueryRow(ctx, `select exists(select 1 from atd_conversas where id = $1::uuid)`, conversaID).Scan(&existe); qerr != nil {
			return 0, qerr
		}
		if !existe {
			return 0, ErrNaoEncontrada
		}
		return 0, ErrVersaoConflito
	}
	return nova, err
}

func (s *storePG) ConcluirPendencia(ctx context.Context, conversaID string, ate time.Time) error {
	_, err := s.pool.Exec(ctx, `
		update atd_conversas set pendente_desde = null
		where id = $1::uuid and (ultima_entrada_em is null or ultima_entrada_em <= $2)`, conversaID, ate)
	return err
}

func (s *storePG) MudarStatus(ctx context.Context, conversaID string, status Status, responsavelID, motivo string) (Conversa, error) {
	switch status {
	case StatusHumano:
		return scanConversa(s.pool.QueryRow(ctx, `
			update atd_conversas set status = $2, responsavel_id = nullif($3::text, '')::uuid, humano_ate = null,
				estado = case when $4::text <> '' then jsonb_set(estado, '{motivo_humano}', to_jsonb($4::text)) else estado end,
				atualizado_em = now()
			where id = $1::uuid returning `+colunasConversa, conversaID, string(status), responsavelID, motivo))
	case StatusBot:
		return scanConversa(s.pool.QueryRow(ctx, `
			update atd_conversas set status = $2, responsavel_id = null, humano_ate = null,
				estado = estado - 'motivo_humano', atualizado_em = now()
			where id = $1::uuid returning `+colunasConversa, conversaID, string(status)))
	default:
		return scanConversa(s.pool.QueryRow(ctx, `
			update atd_conversas set status = $2, responsavel_id = nullif($3::text, '')::uuid, atualizado_em = now()
			where id = $1::uuid returning `+colunasConversa, conversaID, string(status), responsavelID))
	}
}

func (s *storePG) RegistrarTurno(ctx context.Context, t Turno) (Turno, error) {
	if t.EntradaIDs == nil {
		t.EntradaIDs = []string{}
	}
	if t.Passos == nil {
		t.Passos = []Passo{}
	}
	passos, err := json.Marshal(t.Passos)
	if err != nil {
		return Turno{}, err
	}
	antes, err := json.Marshal(t.EstadoAntes)
	if err != nil {
		return Turno{}, err
	}
	depois, err := json.Marshal(t.EstadoDepois)
	if err != nil {
		return Turno{}, err
	}
	err = s.pool.QueryRow(ctx, `
		insert into atd_turnos (id, conversa_id, entrada_ids, passos, estado_antes, estado_depois,
			resposta, resultado, modelo, tokens_entrada, tokens_saida, latencia_ms, erro)
		values (coalesce(nullif($1, '')::uuid, gen_random_uuid()), $2::uuid, $3::text[]::uuid[], $4::jsonb, $5::jsonb, $6::jsonb,
			$7, $8, $9, $10, $11, $12, nullif($13, ''))
		returning id::text, criado_em`,
		t.ID, t.ConversaID, t.EntradaIDs, string(passos), string(antes), string(depois),
		t.Resposta, t.Resultado, t.Modelo, t.TokensEntrada, t.TokensSaida, t.LatenciaMS, t.Erro).Scan(&t.ID, &t.CriadoEm)
	if err != nil {
		return Turno{}, err
	}
	return t, nil
}

func (s *storePG) Turnos(ctx context.Context, conversaID string, limite int) ([]Turno, error) {
	if limite <= 0 {
		limite = 20
	}
	rows, err := s.pool.Query(ctx, `
		select id::text, conversa_id::text, entrada_ids::text[], passos, estado_antes, estado_depois,
			coalesce(resposta, ''), coalesce(resultado, ''), coalesce(modelo, ''), coalesce(erro, ''),
			tokens_entrada, tokens_saida, latencia_ms, criado_em
		from (
			select * from atd_turnos where conversa_id = $1::uuid order by criado_em desc limit $2
		) t order by criado_em asc`, conversaID, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Turno
	for rows.Next() {
		var t Turno
		var passos, antes, depois []byte
		var latencia int32
		if err := rows.Scan(&t.ID, &t.ConversaID, &t.EntradaIDs, &passos, &antes, &depois,
			&t.Resposta, &t.Resultado, &t.Modelo, &t.Erro,
			&t.TokensEntrada, &t.TokensSaida, &latencia, &t.CriadoEm); err != nil {
			return nil, err
		}
		t.LatenciaMS = int64(latencia)
		if len(passos) > 0 {
			if err := json.Unmarshal(passos, &t.Passos); err != nil {
				return nil, fmt.Errorf("conversa: passos inválidos: %w", err)
			}
		}
		if len(antes) > 0 {
			_ = json.Unmarshal(antes, &t.EstadoAntes)
		}
		if len(depois) > 0 {
			_ = json.Unmarshal(depois, &t.EstadoDepois)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *storePG) Listar(ctx context.Context, f FiltroLista) ([]Conversa, error) {
	limite := f.Limite
	if limite <= 0 {
		limite = 50
	}
	busca := strings.TrimSpace(f.Busca)
	if busca != "" {
		r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
		busca = "%" + r.Replace(busca) + "%"
	}
	rows, err := s.pool.Query(ctx, `
		select `+colunasConversa+` from atd_conversas
		where ($1::text = '' or status = $1::text)
		  and ($2::text = '' or nome ilike $2::text or telefone ilike $2::text)
		order by atualizado_em desc
		limit $3`, string(f.Status), busca, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Conversa
	for rows.Next() {
		c, err := scanConversa(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *storePG) ReativarPausadas(ctx context.Context, agora time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		update atd_conversas set status = 'BOT', humano_ate = null, estado = estado - 'motivo_humano', atualizado_em = now()
		where status = 'HUMANO' and responsavel_id is null and humano_ate < $1`, agora)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *storePG) SaidaBotRecente(ctx context.Context, contato, texto string, janela time.Duration) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		select exists(
			select 1 from atd_mensagens m
			join atd_conversas c on c.id = m.conversa_id
			where c.canal = 'WHATSAPP' and c.contato = $1
			  and m.direcao = 'SAIDA' and m.autor = 'BOT'
			  and coalesce(m.midia->>'envio_status', '') <> 'FALHOU'
			  and btrim(coalesce(m.texto, '')) = btrim($2::text)
			  and m.criado_em > now() - $3::float8 * interval '1 second'
		)`, contato, texto, janela.Seconds()).Scan(&ok)
	return ok, err
}

func (s *storePG) AtualizarTexto(ctx context.Context, mensagemID, texto string, midiaExtra map[string]any) error {
	extra := "{}"
	if len(midiaExtra) > 0 {
		b, err := json.Marshal(midiaExtra)
		if err != nil {
			return err
		}
		extra = string(b)
	}
	tag, err := s.pool.Exec(ctx, `
		update atd_mensagens set texto = $2, midia = coalesce(midia, '{}'::jsonb) || $3::jsonb
		where id = $1::uuid`, mensagemID, texto, extra)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrada
	}
	return nil
}

func (s *storePG) ConfirmarEnvio(ctx context.Context, mensagemID, provedorID, erroEnvio string) error {
	var tag pgconn.CommandTag
	var err error
	if erroEnvio == "" {
		tag, err = s.pool.Exec(ctx, `
			update atd_mensagens set provedor_id = coalesce(nullif($2::text, ''), provedor_id)
			where id = $1::uuid`, mensagemID, provedorID)
	} else {
		b, merr := json.Marshal(map[string]any{"envio_status": "FALHOU", "envio_erro": erroEnvio})
		if merr != nil {
			return merr
		}
		tag, err = s.pool.Exec(ctx, `
			update atd_mensagens set midia = coalesce(midia, '{}'::jsonb) || $2::jsonb
			where id = $1::uuid`, mensagemID, string(b))
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNaoEncontrada
	}
	return nil
}
