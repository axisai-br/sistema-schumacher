package ferramentas

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/payments"
)

// TipoDocumento e o tipo de documento aceito na reserva.
type TipoDocumento string

const (
	DocCPF TipoDocumento = "CPF"
	DocRG  TipoDocumento = "RG"
	DocCNH TipoDocumento = "CNH"
)

// ValidarCPF confere 11 digitos e digitos verificadores (validador de payments).
func ValidarCPF(s string) bool {
	d := apenasDigitos(s)
	return len(d) == 11 && payments.IsSupportedDocument(d)
}

// errDocumento descreve por que um documento foi recusado.
type errDocumento struct{ codigo, detalhe string }

// ClassificarDocumento identifica e normaliza um documento. tipoInformado
// (opcional) desfaz a ambiguidade CPF x CNH, ambos com 11 digitos.
// Devolve o tipo, o documento normalizado e, em caso de erro, (codigo, detalhe).
func ClassificarDocumento(doc, tipoInformado string) (TipoDocumento, string, string, string) {
	t, norm, err := classificarDocumento(doc, tipoInformado)
	if err != nil {
		return "", "", err.codigo, err.detalhe
	}
	return t, norm, "", ""
}

func classificarDocumento(doc, tipoInformado string) (TipoDocumento, string, *errDocumento) {
	limpo := strings.ToUpper(strings.NewReplacer(".", "", "-", "", "/", "", " ", "").Replace(strings.TrimSpace(doc)))
	for _, r := range limpo {
		if !unicode.IsDigit(r) && !(r >= 'A' && r <= 'Z') {
			return "", "", &errDocumento{"documento_invalido", "documento com caracteres invalidos"}
		}
	}
	digitos := apenasDigitos(limpo)
	somenteDigitos := len(digitos) == len(limpo)
	tipo := TipoDocumento(strings.ToUpper(strings.TrimSpace(tipoInformado)))
	switch tipo {
	case DocCPF:
		if !ValidarCPF(limpo) {
			return "", "", &errDocumento{"cpf_invalido", "CPF invalido (confira os 11 digitos)"}
		}
		return DocCPF, digitos, nil
	case DocCNH:
		if !somenteDigitos || len(digitos) != 11 {
			return "", "", &errDocumento{"cnh_invalida", "CNH deve ter 11 digitos"}
		}
		return DocCNH, digitos, nil
	case DocRG:
		if len(limpo) < 5 || len(limpo) > 14 {
			return "", "", &errDocumento{"rg_invalido", "RG deve ter de 5 a 14 caracteres"}
		}
		return DocRG, limpo, nil
	case "":
	default:
		return "", "", &errDocumento{"tipo_documento_invalido", "tipo_documento deve ser CPF, RG ou CNH"}
	}
	if somenteDigitos && len(digitos) == 11 {
		if ValidarCPF(digitos) {
			return DocCPF, digitos, nil
		}
		return "", "", &errDocumento{"cpf_invalido", "CPF invalido (confira os 11 digitos); se for CNH, envie tipo_documento=CNH"}
	}
	if len(limpo) >= 5 && len(limpo) <= 14 {
		return DocRG, limpo, nil
	}
	return "", "", &errDocumento{"documento_invalido", "documento deve ser CPF (11 digitos), RG (5 a 14 caracteres) ou CNH (11 digitos)"}
}

func nomeValido(nome string) bool {
	palavras := strings.Fields(nome)
	if len(palavras) < 2 {
		return false
	}
	for _, r := range nome {
		if unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// pendenciasDados e Estado.Pendencias() ajustada: crianca ate 5 anos pode
// ficar sem documento, entao so adultos sem nome/documento contam.
func pendenciasDados(e conversa.Estado) []string {
	faltam := 0
	for _, p := range e.Passageiros {
		if strings.TrimSpace(p.Nome) == "" || (!p.CriancaAte5 && strings.TrimSpace(p.Documento) == "") {
			faltam++
		}
	}
	if e.PessoasInformadas > len(e.Passageiros) && len(e.Passageiros) > 0 {
		faltam += e.PessoasInformadas - len(e.Passageiros)
	}
	out := []string{}
	for _, p := range e.Pendencias() {
		if strings.HasPrefix(p, "completar dados de") {
			if faltam > 0 {
				out = append(out, fmt.Sprintf("completar dados de %d passageiro(s)", faltam))
			}
			continue
		}
		out = append(out, p)
	}
	return out
}

// pendenciasParaReserva: o que falta antes de escolher pagamento/criar reserva.
func pendenciasParaReserva(e conversa.Estado) []string {
	out := []string{}
	for _, p := range pendenciasDados(e) {
		if strings.HasPrefix(p, "escolher pagamento") || strings.HasPrefix(p, "criar reserva") || strings.HasPrefix(p, "gerar PIX") {
			continue
		}
		out = append(out, p)
	}
	if len(e.Passageiros) > 0 && e.Pagantes() == 0 {
		out = append(out, "incluir ao menos um passageiro adulto pagante")
	}
	return out
}

// mesmaListaPassageiros compara duas listas (nome sem diferenciar caixa,
// documento, tipo e crianca), na mesma ordem.
func mesmaListaPassageiros(a, b []conversa.Passageiro) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i].Nome, b[i].Nome) || a[i].Documento != b[i].Documento ||
			a[i].TipoDocumento != b[i].TipoDocumento || a[i].CriancaAte5 != b[i].CriancaAte5 {
			return false
		}
	}
	return true
}

type registrarPassageiros struct{}

func (t *registrarPassageiros) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "registrar_passageiros",
		Descricao: "Registra a lista COMPLETA de passageiros (substitui a anterior): nome completo e documento (CPF, RG ou CNH) de cada um. " +
			"Use quando o cliente informar nomes e documentos, reenviando todos os passageiros a cada chamada. " +
			"Crianca ate 5 anos (nao paga) pode ficar sem documento: marque crianca_ate_5=true. " +
			"Se o documento tiver 11 digitos e for CNH (nao CPF), informe tipo_documento='CNH'. Devolve erros por passageiro e o que ainda falta. " +
			"Os mesmos passageiros valem para TODOS os trechos (ida e volta). A quantidade e conferida contra as vagas de todos os trechos escolhidos. " +
			"Depois que algum trecho ja tem reserva a lista NAO pode mais mudar (so reenviar a mesma lista e aceito): para alterar passageiros transfira para um atendente.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{
    "passageiros":{
      "type":"array","minItems":1,
      "items":{
        "type":"object",
        "properties":{
          "nome":{"type":"string","description":"Nome completo (nome e sobrenome)."},
          "documento":{"type":"string","description":"CPF, RG ou CNH. Opcional so para crianca ate 5 anos."},
          "tipo_documento":{"type":"string","enum":["CPF","RG","CNH"],"description":"Opcional; necessario so para CNH de 11 digitos ou RG ambiguo."},
          "crianca_ate_5":{"type":"boolean","description":"true se tem ate 5 anos (nao paga passagem)."}
        },
        "required":["nome"],
        "additionalProperties":false
      }
    }
  },
  "required":["passageiros"],
  "additionalProperties":false
}`),
	}
}

func (t *registrarPassageiros) Executar(_ context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a struct {
		Passageiros []struct {
			Nome          string `json:"nome"`
			Documento     string `json:"documento"`
			TipoDocumento string `json:"tipo_documento"`
			CriancaAte5   bool   `json:"crianca_ate_5"`
		} `json:"passageiros"`
	}
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	e := c.Estado
	// Regra: com qualquer trecho ja reservado, a lista de passageiros fica
	// travada (as reservas existentes guardam os passageiros originais); so a
	// mesma lista e aceita (idempotente), conferido apos a validacao abaixo.
	reservado := e.AlgumReservado()
	if len(a.Passageiros) == 0 {
		return falha("lista_vazia", "Envie ao menos um passageiro.")
	}
	if !reservado {
		// Vagas: o menor numero de vagas entre os trechos ainda sem reserva.
		for i, tr := range e.Trechos {
			if tr.ReservaID == "" && len(a.Passageiros) > tr.Viagem.Vagas {
				return falhaDados("passageiros_acima_das_vagas", map[string]any{
					"trecho":     i + 1,
					"rota":       tr.Rota(),
					"vagas":      tr.Viagem.Vagas,
					"informados": len(a.Passageiros),
					"mensagem":   "Ha mais passageiros do que vagas nessa viagem. Informe o cliente e ofereca outra opcao.",
				})
			}
		}
	}

	type erroPax struct {
		Passageiro int    `json:"passageiro"`
		Nome       string `json:"nome,omitempty"`
		Motivo     string `json:"motivo"`
		Detalhe    string `json:"detalhe"`
	}
	var erros []erroPax
	lista := make([]conversa.Passageiro, 0, len(a.Passageiros))
	vistos := map[string]int{}
	for i, p := range a.Passageiros {
		n := i + 1
		nome := strings.Join(strings.Fields(p.Nome), " ")
		pax := conversa.Passageiro{Nome: nome, CriancaAte5: p.CriancaAte5}
		bad := false
		if !nomeValido(nome) {
			erros = append(erros, erroPax{n, nome, "nome_invalido", "informe nome e sobrenome (2 palavras ou mais, sem numeros)"})
			bad = true
		}
		if strings.TrimSpace(p.Documento) == "" {
			if !p.CriancaAte5 {
				erros = append(erros, erroPax{n, nome, "documento_obrigatorio", "peca CPF, RG ou CNH deste passageiro"})
				bad = true
			}
		} else if tipo, norm, err := classificarDocumento(p.Documento, p.TipoDocumento); err != nil {
			erros = append(erros, erroPax{n, nome, err.codigo, err.detalhe})
			bad = true
		} else {
			pax.Documento, pax.TipoDocumento = norm, string(tipo)
			if prev, dup := vistos[norm]; dup {
				erros = append(erros, erroPax{n, nome, "documento_duplicado", fmt.Sprintf("mesmo documento do passageiro %d", prev)})
				bad = true
			}
			vistos[norm] = n
		}
		if !bad {
			lista = append(lista, pax)
		}
	}
	if len(erros) > 0 {
		return falhaDados("dados_invalidos", map[string]any{
			"erros":    erros,
			"mensagem": "Nada foi salvo. Corrija os passageiros com erro e reenvie a lista completa.",
		})
	}

	if reservado {
		if !mesmaListaPassageiros(lista, e.Passageiros) {
			return falha("reserva_ja_criada", "Ja existe reserva criada (os trechos reservados guardam os passageiros originais); para alterar passageiros transfira para um atendente.")
		}
	} else {
		e.Passageiros = lista
	}
	view := make([]map[string]any, len(lista))
	for i, p := range lista {
		m := map[string]any{"numero": i + 1, "nome": p.Nome, "crianca_ate_5": p.CriancaAte5}
		if p.Documento != "" {
			m["tipo_documento"] = p.TipoDocumento
			m["documento"] = conversa.MascararDocumento(p.Documento)
		}
		view[i] = m
	}
	return sucesso(map[string]any{
		"passageiros": view,
		"total":       len(lista),
		"pagantes":    e.Pagantes(),
		"pendencias":  pendenciasDados(*e),
	})
}
