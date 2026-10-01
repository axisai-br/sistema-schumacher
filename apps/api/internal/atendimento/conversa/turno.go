package conversa

import "time"

type Passo struct {
	Tipo      string `json:"tipo"` // "llm" | "ferramenta" | "checagem"
	Nome      string `json:"nome,omitempty"`
	Entrada   any    `json:"entrada,omitempty"`
	Saida     any    `json:"saida,omitempty"`
	Erro      string `json:"erro,omitempty"`
	DuracaoMS int64  `json:"duracao_ms"`
}

type Turno struct {
	ID, ConversaID string
	EntradaIDs     []string
	Passos         []Passo
	EstadoAntes    Estado
	EstadoDepois   Estado
	Resposta       string
	Resultado      string
	Modelo         string
	Erro           string
	TokensEntrada  int
	TokensSaida    int
	LatenciaMS     int64
	CriadoEm       time.Time
}

const (
	ResultadoEnviado           = "ENVIADO"
	ResultadoDescartadoMsgNova = "DESCARTADO_MSG_NOVA"
	ResultadoHumano            = "HUMANO"
	ResultadoErro              = "ERRO"
	ResultadoSemResposta       = "SEM_RESPOSTA"
)
