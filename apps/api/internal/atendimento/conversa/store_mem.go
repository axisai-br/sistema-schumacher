package conversa

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// StoreMem e uma implementacao em memoria de Store, com a mesma semantica da
// versao PostgreSQL (idempotencia, versao otimista e lease). Serve para testes.
type StoreMem struct {
	mu        sync.Mutex
	agora     func() time.Time
	conversas map[string]*registroMem
	porChave  map[string]string // canal\x00contato -> id
	mensagens []Mensagem
	porProv   map[string]int // provedor_id -> indice em mensagens
	turnos    []Turno
	expira    time.Duration // prazo sem mensagens para zerar o estado (0 desliga)
}

// DefinirExpiracao muda o prazo sem mensagens para zerar o estado (0 desliga).
func (s *StoreMem) DefinirExpiracao(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expira = d
}

type registroMem struct {
	c              Conversa
	processandoAte *time.Time
}

func NewStoreMem(agora func() time.Time) *StoreMem {
	if agora == nil {
		agora = time.Now
	}
	return &StoreMem{
		expira:    ExpiraEstadoPadrao,
		agora:     agora,
		conversas: map[string]*registroMem{},
		porChave:  map[string]string{},
		porProv:   map[string]int{},
	}
}

var _ Store = (*StoreMem)(nil)

func copiarTempo(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

func clonarEstado(e Estado) Estado {
	b, err := json.Marshal(e)
	if err != nil {
		return e
	}
	var out Estado
	if err := json.Unmarshal(b, &out); err != nil {
		return e
	}
	return out
}

func (r *registroMem) copia() Conversa {
	c := r.c
	c.Estado = clonarEstado(c.Estado)
	c.PendenteDesde = copiarTempo(c.PendenteDesde)
	c.UltimaEntradaEm = copiarTempo(c.UltimaEntradaEm)
	c.HumanoAte = copiarTempo(c.HumanoAte)
	return c
}

func copiarMensagem(m Mensagem) Mensagem {
	if m.Midia != nil {
		cp := make(map[string]any, len(m.Midia))
		for k, v := range m.Midia {
			cp[k] = v
		}
		m.Midia = cp
	}
	return m
}

func (s *StoreMem) RegistrarEntrada(_ context.Context, in NovaEntrada) (Conversa, Mensagem, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if in.ProvedorID != "" {
		if i, ok := s.porProv[in.ProvedorID]; ok {
			m := s.mensagens[i]
			return s.conversas[m.ConversaID].copia(), copiarMensagem(m), true, nil
		}
	}
	// Relogio do servidor: RecebidaEm (relogio do provedor) nao e usado para
	// pendente_desde, ultima_entrada_em, humano_ate nem criado_em.
	recebida := s.agora()
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

	chave := canal + "\x00" + in.Contato
	id, existe := s.porChave[chave]
	var reg *registroMem
	if existe {
		reg = s.conversas[id]
	} else {
		agora := s.agora()
		reg = &registroMem{c: Conversa{
			ID: uuid.NewString(), Canal: canal, Contato: in.Contato,
			Telefone: in.Telefone, Nome: in.Nome, Status: StatusBot,
			CriadoEm: agora, AtualizadoEm: agora,
		}}
		s.conversas[reg.c.ID] = reg
		s.porChave[chave] = reg.c.ID
	}

	direcao := DirecaoEntrada
	switch autor {
	case AutorCliente:
		if s.expira > 0 && reg.c.Status == StatusBot && reg.c.UltimaEntradaEm != nil && recebida.Sub(*reg.c.UltimaEntradaEm) > s.expira {
			// Bot parado ha mais que o prazo: compra nova.
			reg.c.Estado = Estado{}
			reg.c.Versao++
		}
		if reg.c.Status == StatusEncerrada {
			// Nova mensagem reabre: novo atendimento, estado zerado.
			reg.c.Status = StatusBot
			reg.c.Estado = Estado{}
			reg.c.Versao++
			reg.c.ResponsavelID = ""
			reg.c.HumanoAte = nil
		}
		if in.Telefone != "" {
			reg.c.Telefone = in.Telefone
		}
		if in.Nome != "" {
			reg.c.Nome = in.Nome
		}
		if reg.c.PendenteDesde == nil {
			reg.c.PendenteDesde = copiarTempo(&recebida)
		}
		reg.c.UltimaEntradaEm = copiarTempo(&recebida)
	case AutorHumano:
		direcao = DirecaoSaida
		reg.c.Status = StatusHumano
		ate := recebida.Add(PausaHumanoPadrao)
		reg.c.HumanoAte = &ate
		reg.c.PendenteDesde = nil
	default:
		direcao = DirecaoSaida
	}
	reg.c.AtualizadoEm = s.agora()

	m := Mensagem{
		ID: uuid.NewString(), ConversaID: reg.c.ID, Direcao: direcao, Autor: autor,
		Tipo: tipo, Texto: in.Texto, Midia: in.Midia, ProvedorID: in.ProvedorID,
		CriadoEm: recebida,
	}
	s.anexar(m)
	return reg.copia(), copiarMensagem(m), false, nil
}

func (s *StoreMem) anexar(m Mensagem) {
	s.mensagens = append(s.mensagens, copiarMensagem(m))
	if m.ProvedorID != "" {
		s.porProv[m.ProvedorID] = len(s.mensagens) - 1
	}
}

func (s *StoreMem) RegistrarSaida(_ context.Context, conversaID string, autor Autor, texto, provedorID, turnoID string) (Mensagem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reg, ok := s.conversas[conversaID]
	if !ok {
		return Mensagem{}, ErrNaoEncontrada
	}
	if provedorID != "" {
		if i, ok := s.porProv[provedorID]; ok {
			return copiarMensagem(s.mensagens[i]), nil
		}
	}
	agora := s.agora()
	m := Mensagem{
		ID: uuid.NewString(), ConversaID: conversaID, Direcao: DirecaoSaida, Autor: autor,
		Tipo: TipoTexto, Texto: texto, ProvedorID: provedorID, TurnoID: turnoID, CriadoEm: agora,
	}
	s.anexar(m)
	reg.c.AtualizadoEm = agora
	return m, nil
}

func (s *StoreMem) ProvedorIDConhecido(_ context.Context, provedorID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if provedorID == "" {
		return false, nil
	}
	i, ok := s.porProv[provedorID]
	return ok && s.mensagens[i].Direcao == DirecaoSaida, nil
}

func (s *StoreMem) Obter(_ context.Context, id string) (Conversa, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reg, ok := s.conversas[id]
	if !ok {
		return Conversa{}, ErrNaoEncontrada
	}
	return reg.copia(), nil
}

func (s *StoreMem) Historico(_ context.Context, conversaID string, limite int) ([]Mensagem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var todas []Mensagem
	for _, m := range s.mensagens {
		if m.ConversaID == conversaID {
			todas = append(todas, m)
		}
	}
	// Mesma ordem do PG: (criado_em, seq); sort estavel preserva a insercao.
	sort.SliceStable(todas, func(i, j int) bool { return todas[i].CriadoEm.Before(todas[j].CriadoEm) })
	if limite > 0 && len(todas) > limite {
		todas = todas[len(todas)-limite:]
	}
	out := make([]Mensagem, len(todas))
	for i, m := range todas {
		out[i] = copiarMensagem(m)
	}
	return out, nil
}

func (s *StoreMem) EntradasPendentes(_ context.Context, conversaID string, desde time.Time) ([]Mensagem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Mensagem
	for _, m := range s.mensagens {
		if m.ConversaID == conversaID && m.Autor == AutorCliente && !m.CriadoEm.Before(desde) {
			out = append(out, copiarMensagem(m))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CriadoEm.Before(out[j].CriadoEm) })
	return out, nil
}

type lockMem struct {
	s    *StoreMem
	c    Conversa
	once sync.Once
}

func (l *lockMem) Conversa() Conversa { return l.c }

func (l *lockMem) Liberar(_ context.Context) error {
	l.once.Do(func() {
		l.s.mu.Lock()
		defer l.s.mu.Unlock()
		if reg, ok := l.s.conversas[l.c.ID]; ok {
			reg.processandoAte = nil
		}
	})
	return nil
}

func (s *StoreMem) ReivindicarDevida(_ context.Context, debounce time.Duration) (Lock, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agora := s.agora()
	limite := agora.Add(-debounce)
	var escolhida *registroMem
	for _, reg := range s.conversas {
		c := reg.c
		if c.Status != StatusBot || c.PendenteDesde == nil || c.UltimaEntradaEm == nil {
			continue
		}
		if c.UltimaEntradaEm.After(limite) {
			continue
		}
		if reg.processandoAte != nil && !reg.processandoAte.Before(agora) {
			continue
		}
		if escolhida == nil || c.PendenteDesde.Before(*escolhida.c.PendenteDesde) {
			escolhida = reg
		}
	}
	if escolhida == nil {
		return nil, false, nil
	}
	ate := agora.Add(LeaseDuracao)
	escolhida.processandoAte = &ate
	return &lockMem{s: s, c: escolhida.copia()}, true, nil
}

func (s *StoreMem) ChegouEntradaDepois(_ context.Context, conversaID string, t time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reg, ok := s.conversas[conversaID]
	if !ok {
		return false, ErrNaoEncontrada
	}
	return reg.c.UltimaEntradaEm != nil && reg.c.UltimaEntradaEm.After(t), nil
}

func (s *StoreMem) SalvarEstado(_ context.Context, conversaID string, estado Estado, versaoEsperada int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reg, ok := s.conversas[conversaID]
	if !ok {
		return 0, ErrNaoEncontrada
	}
	if reg.c.Versao != versaoEsperada {
		return 0, ErrVersaoConflito
	}
	reg.c.Estado = clonarEstado(estado)
	reg.c.Versao++
	reg.c.AtualizadoEm = s.agora()
	return reg.c.Versao, nil
}

func (s *StoreMem) ConcluirPendencia(_ context.Context, conversaID string, ate time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	reg, ok := s.conversas[conversaID]
	if !ok {
		return ErrNaoEncontrada
	}
	if reg.c.UltimaEntradaEm == nil || !reg.c.UltimaEntradaEm.After(ate) {
		reg.c.PendenteDesde = nil
	}
	return nil
}

func (s *StoreMem) MudarStatus(_ context.Context, conversaID string, status Status, responsavelID, motivo string) (Conversa, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reg, ok := s.conversas[conversaID]
	if !ok {
		return Conversa{}, ErrNaoEncontrada
	}
	reg.c.Status = status
	reg.c.ResponsavelID = responsavelID
	switch status {
	case StatusHumano:
		if motivo != "" {
			reg.c.Estado.MotivoHumano = motivo
		}
	case StatusBot:
		reg.c.ResponsavelID = ""
		reg.c.HumanoAte = nil
		reg.c.Estado.MotivoHumano = ""
	}
	if status == StatusHumano {
		reg.c.HumanoAte = nil
	}
	reg.c.AtualizadoEm = s.agora()
	return reg.copia(), nil
}

func (s *StoreMem) RegistrarTurno(_ context.Context, t Turno) (Turno, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.conversas[t.ConversaID]; !ok {
		return Turno{}, ErrNaoEncontrada
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	t.CriadoEm = s.agora()
	t.EstadoAntes = clonarEstado(t.EstadoAntes)
	t.EstadoDepois = clonarEstado(t.EstadoDepois)
	s.turnos = append(s.turnos, t)
	return t, nil
}

func (s *StoreMem) Turnos(_ context.Context, conversaID string, limite int) ([]Turno, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Turno
	for _, t := range s.turnos {
		if t.ConversaID == conversaID {
			out = append(out, t)
		}
	}
	if limite > 0 && len(out) > limite {
		out = out[len(out)-limite:]
	}
	return out, nil
}

func (s *StoreMem) Listar(_ context.Context, f FiltroLista) ([]Conversa, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	busca := strings.ToLower(strings.TrimSpace(f.Busca))
	var out []Conversa
	for _, reg := range s.conversas {
		if f.Status != "" && reg.c.Status != f.Status {
			continue
		}
		if busca != "" && !strings.Contains(strings.ToLower(reg.c.Nome), busca) &&
			!strings.Contains(strings.ToLower(reg.c.Telefone), busca) {
			continue
		}
		out = append(out, reg.copia())
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AtualizadoEm.After(out[j].AtualizadoEm) })
	limite := f.Limite
	if limite <= 0 {
		limite = 50
	}
	if len(out) > limite {
		out = out[:limite]
	}
	return out, nil
}

func (s *StoreMem) ReativarPausadas(_ context.Context, agora time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, reg := range s.conversas {
		c := &reg.c
		if c.Status == StatusHumano && c.ResponsavelID == "" && c.HumanoAte != nil && c.HumanoAte.Before(agora) {
			c.Status = StatusBot
			c.HumanoAte = nil
			c.Estado.MotivoHumano = ""
			c.AtualizadoEm = s.agora()
			n++
		}
	}
	return n, nil
}

func (s *StoreMem) SaidaBotRecente(_ context.Context, contato, texto string, janela time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	limite := s.agora().Add(-janela)
	alvo := strings.TrimSpace(texto)
	for _, m := range s.mensagens {
		if m.Direcao != DirecaoSaida || m.Autor != AutorBot || !m.CriadoEm.After(limite) || m.Midia["envio_status"] == "FALHOU" {
			continue
		}
		reg, ok := s.conversas[m.ConversaID]
		if !ok || reg.c.Canal != "WHATSAPP" || reg.c.Contato != contato {
			continue
		}
		if strings.TrimSpace(m.Texto) == alvo {
			return true, nil
		}
	}
	return false, nil
}

func (s *StoreMem) AtualizarTexto(_ context.Context, mensagemID, texto string, midiaExtra map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.mensagens {
		if s.mensagens[i].ID != mensagemID {
			continue
		}
		m := &s.mensagens[i]
		m.Texto = texto
		if len(midiaExtra) > 0 {
			novo := make(map[string]any, len(m.Midia)+len(midiaExtra))
			for k, v := range m.Midia {
				novo[k] = v
			}
			for k, v := range midiaExtra {
				novo[k] = v
			}
			m.Midia = novo
		}
		return nil
	}
	return ErrNaoEncontrada
}

func (s *StoreMem) ConfirmarEnvio(_ context.Context, mensagemID, provedorID, erroEnvio string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.mensagens {
		m := &s.mensagens[i]
		if m.ID != mensagemID {
			continue
		}
		if erroEnvio == "" {
			if provedorID != "" {
				m.ProvedorID = provedorID
				s.porProv[provedorID] = i
			}
			return nil
		}
		novo := make(map[string]any, len(m.Midia)+2)
		for k, v := range m.Midia {
			novo[k] = v
		}
		novo["envio_status"] = "FALHOU"
		novo["envio_erro"] = erroEnvio
		m.Midia = novo
		return nil
	}
	return ErrNaoEncontrada
}
