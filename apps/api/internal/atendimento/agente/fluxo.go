package agente

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
)

// Motor por comandos (ATENDIMENTO_V2_MOTOR=comandos): o LLM nunca responde
// livremente nem chama ferramenta. A cada turno:
//  1. entender: Jev + parsers + extrator JSON (em paralelo, dentro de rotear);
//  2. decidir/executar: rotear em modo template (codigo chama as ferramentas);
//  3. falar: so templates e respostas fixas (faq.md); o que nao tem resposta
//     vira "nao sei, o suporte confirma" ou o proximo passo da compra.
// Como o texto sai do que foi executado, nao ha como afirmar o que nao houve.

const (
	MotorAgente   = "agente"
	MotorComandos = "comandos"
)

const (
	textoAudio        = "Não consegui entender o áudio. 🙏 Pode me escrever?"
	textoImagem       = "Recebi a imagem, mas ela não parece ser um documento. Se for de um passageiro, me manda o nome completo e o CPF por escrito?"
	textoCorrigirDado = "Pra corrigir, me manda o nome do passageiro e o documento certo. Por exemplo: Maria Pereira CPF 987.654.321-00."
	textoSemData      = "Não tem viagem nessa data. Estas são as próximas:\n\n"
	textoSemExata     = "Não achei exatamente o que você pediu. Estas são as opções disponíveis:\n\n"
	limiteNaoEntendi  = 2
)

// turnoComandos devolve o texto do turno ou uma transferencia.
func (a *Agente) turnoComandos(ctx context.Context, tc *turno, hist []conversa.Mensagem) (string, *transf) {
	tc.comandos = true
	texto := textoRecenteCliente(hist)
	baixo := semAcento(strings.ToLower(texto))
	semMidia := strings.TrimSpace(reMarcadorMidia.ReplaceAllString(texto, ""))

	// Midia sem texto: audio nao entendido ou imagem que nao e documento.
	if semMidia == "" && !reFotoDocumento.MatchString(texto) {
		switch {
		case reMidiaAudio.MatchString(baixo):
			return a.registrarAto(tc, "audio", textoAudio), nil
		case reMidiaImagem.MatchString(baixo):
			return a.registrarAto(tc, "imagem", textoImagem), nil
		}
	}
	// Desistencia antes de reservar: encerra sem insistir.
	if reDesistir.MatchString(baixo) && !tc.estado.AlgumReservado() {
		return a.registrarAto(tc, "desistir", TextoDesistir), nil
	}

	res := a.rotear(ctx, tc, hist)
	if res.transf != nil {
		return "", res.transf
	}
	resp := res.resposta
	ato := "template"
	if resp == "" {
		var tr *transf
		resp, ato, tr = a.respostaSemTemplate(ctx, tc, hist, texto)
		if tr != nil {
			return "", tr
		}
	}
	// Pergunta solta junto com o fluxo ("e aceita cartão?"): responde antes.
	if ass := a.assuntoTurno(tc, texto); ass != "" && ato != "faq" {
		resp = respostaFAQ(ass, tc.estado, a.cfg.SinalPorPagante) + "\n\n" + resp
		ato += "+faq:" + ass
	}
	if reMidiaImagem.MatchString(baixo) && !reFotoDocumento.MatchString(texto) {
		resp = textoImagem + "\n\n" + resp
	}

	// "Nao entendi" repetido: a mesma resposta duas vezes sem a compra andar.
	if mudouNoTurno(tc) {
		tc.estado.Falhas = 0
	} else if resp == ultimaRespostaBot(hist) {
		tc.estado.Falhas++
		if tc.estado.Falhas >= limiteNaoEntendi {
			return "", &transf{motivo: "nao entendeu o cliente duas vezes (motor comandos)"}
		}
	}
	return a.registrarAto(tc, ato, resp), nil
}

var reMarcadorMidia = reMidiaGeral

// registrarAto grava o ato escolhido nos passos do turno (auditoria).
func (a *Agente) registrarAto(tc *turno, ato, texto string) string {
	tc.passos = append(tc.passos, conversa.Passo{Tipo: "ato", Nome: ato})
	return texto
}

// assuntoTurno: pergunta solta do cliente (palavra-chave primeiro, extrator
// depois). "outro" vira NaoSei so quando nada mais respondeu.
func (a *Agente) assuntoTurno(tc *turno, texto string) string {
	if ass := assuntoDoTexto(texto); ass != "" {
		return ass
	}
	if tc.ext != nil && tc.ext.Assunto != "" && tc.ext.Assunto != "nenhum" && tc.ext.Assunto != "outro" && strings.Contains(texto, "?") {
		return tc.ext.Assunto
	}
	return ""
}

// respostaSemTemplate cobre o que rotear deixou para o LLM no motor antigo.
func (a *Agente) respostaSemTemplate(ctx context.Context, tc *turno, hist []conversa.Mensagem, texto string) (string, string, *transf) {
	baixo := semAcento(strings.ToLower(texto))
	rt := Rota{}
	if tc.rota != nil {
		rt = *tc.rota
	}
	// Cidade nao atendida.
	if (rt.Origem == CidadeNaoAtendida && rt.ConfOrigem >= 0.8) || (rt.Destino == CidadeNaoAtendida && rt.ConfDestino >= 0.8) {
		return a.textoCidadeNaoAtendida(ctx), "cidade_nao_atendida", nil
	}
	// Busca feita neste turno com aviso (sem viagem na data, sem vaga, rota
	// sem viagens).
	if r, ok := a.respostaBusca(ctx, tc); ok {
		return r, "busca_aviso", nil
	}
	// Pergunta solta.
	if ass := a.assuntoTurno(tc, texto); ass != "" {
		return respostaFAQ(ass, tc.estado, a.cfg.SinalPorPagante) + "\n\n" + textoProximoPasso(tc.estado), "faq", nil
	}
	// Quem e voce / tentativa de mudar as regras.
	if rePersonaPergunta.MatchString(baixo) || reInjecao.MatchString(baixo) {
		return TextoPersona + " Posso te ajudar com a sua viagem.\n\n" + textoProximoPasso(tc.estado), "persona", nil
	}
	if tc.ext != nil && tc.ext.Assunto == "outro" && strings.Contains(texto, "?") {
		return TextoNaoSei + "\n\n" + textoProximoPasso(tc.estado), "faq_outro", nil
	}
	// Correcao que o codigo nao entendeu.
	if rt.CorrigePassageiro >= 0.8 && !tc.estado.AlgumReservado() {
		return textoCorrigirDado, "pedir_correcao", nil
	}
	if tc.ext != nil && tc.ext.Desistir && !tc.estado.AlgumReservado() {
		return TextoDesistir, "desistir", nil
	}
	return textoProximoPasso(tc.estado), "proximo_passo", nil
}

func (a *Agente) textoCidadeNaoAtendida(ctx context.Context) string {
	var cidades []ferramentas.Cidade
	if a.d.Cidades != nil {
		cidades, _ = a.d.Cidades.Cidades(ctx)
	}
	t := "Essa cidade a gente não atende. 😕"
	if len(cidades) > 0 {
		t += "\n\n" + textoCidadesAtendidas(cidades)
	}
	return t + "\n\nPara outro destino, o suporte pode ajudar: +55 49 9886-2222."
}

// respostaBusca le o ultimo resultado de buscar_viagens deste turno e monta a
// resposta para os casos com aviso.
func (a *Agente) respostaBusca(ctx context.Context, tc *turno) (string, bool) {
	if len(tc.resultados) == 0 {
		return "", false
	}
	var s struct {
		OK    bool `json:"ok"`
		Dados struct {
			Opcoes      []conversa.Opcao `json:"opcoes"`
			Proxima     map[string]any   `json:"proxima_data_disponivel"`
			RotaSem     bool             `json:"rota_sem_viagens"`
			SemVagaPara bool             `json:"sem_vaga_para_pessoas"`
		} `json:"dados"`
	}
	if json.Unmarshal([]byte(tc.resultados[len(tc.resultados)-1]), &s) != nil || !s.OK {
		return "", false
	}
	d := s.Dados
	switch {
	case len(d.Opcoes) > 0:
		return textoSemExata + textoOpcoes(d.Opcoes), true
	case d.RotaSem:
		return a.textoCidadeNaoAtendida(ctx), true
	case d.Proxima != nil && tc.estado.Origem != nil && tc.estado.Destino != nil:
		args := map[string]any{"origem": tc.estado.Origem.Nome, "destino": tc.estado.Destino.Nome}
		if _, ok := a.preExecutar(ctx, tc, "buscar_viagens", args); ok {
			if ops, _ := opcoesSemAviso(tc.resultados); len(ops) > 0 {
				return textoSemData + textoOpcoes(ops), true
			}
		}
		return fmt.Sprintf("Não tem viagem nessa data. A próxima é %v %v às %v.", d.Proxima["dia_semana"], dataBR(fmt.Sprint(d.Proxima["data"])), d.Proxima["horario"]), true
	case d.SemVagaPara:
		return "Nessas datas não há vagas para todos. Quer ver outra data? Se preferir, o suporte ajuda: +55 49 9886-2222.", true
	}
	return "", false
}

func dataBR(iso string) string {
	if len(iso) == 10 {
		return iso[8:10] + "/" + iso[5:7]
	}
	return iso
}

func ultimaRespostaBot(hist []conversa.Mensagem) string {
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].Autor == conversa.AutorBot {
			return strings.TrimSpace(hist[i].Texto)
		}
	}
	return ""
}

// aplicarExtracaoPassageiros junta a lista com o que so o extrator achou:
// passageiros novos validos, correcoes e remocoes por primeiro nome.
func aplicarExtracaoPassageiros(lista []conversa.Passageiro, ex *Extracao) ([]conversa.Passageiro, bool) {
	mudou := false
	for _, nome := range ex.Remover {
		if i := acharPassageiro(lista, primeiroNome(nome)); i >= 0 {
			lista = append(lista[:i], lista[i+1:]...)
			mudou = true
		}
	}
	for _, p := range passageirosExtraidos(ex) {
		var entrou bool
		lista, entrou, _ = juntarPassageiro(lista, p)
		mudou = mudou || entrou
	}
	for _, c := range ex.Corrigir {
		if i := acharPassageiro(lista, primeiroNome(c.Nome)); i >= 0 {
			var av []string
			mudou = corrigirDocumento(&lista[i], c.Documento, &av) || mudou
		}
	}
	return lista, mudou
}

func primeiroNome(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}
