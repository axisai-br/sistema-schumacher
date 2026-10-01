import { apiGet, apiPost } from "./api";
import { buildListQuery } from "../hooks/buildListQuery";

export type ConversaStatus = "BOT" | "HUMANO" | "ENCERRADA";
export type MensagemAutor = "CLIENTE" | "BOT" | "HUMANO";

export type Mensagem = {
  id: string;
  direcao: string;
  autor: MensagemAutor;
  tipo: string;
  texto: string;
  midia?: Record<string, unknown>;
  provedor_id?: string;
  turno_id?: string;
  criado_em: string;
};

export type Conversa = {
  id: string;
  canal: string;
  telefone: string;
  nome: string;
  status: ConversaStatus;
  responsavel_id: string;
  atualizado_em: string;
  pendente_desde: string | null;
  pendente: boolean;
  humano_ate: string | null;
  resumo: string;
  motivo_humano: string;
};

export type ConversaListItem = Conversa & {
  ultima_mensagem?: Mensagem;
};

export type OpcaoViagemV2 = {
  numero?: number;
  trip_id?: string;
  origem: string;
  destino: string;
  data: string;
  horario: string;
  preco: number;
  vagas?: number;
};

/** Um trecho da compra (ida, volta...): uma reserva e um PIX por trecho. */
export type TrechoV2 = {
  viagem: OpcaoViagemV2;
  reserva_id?: string;
  pagamento_id?: string;
};

export type EstadoV2 = {
  trechos?: TrechoV2[];
  passageiros?: Array<{ nome: string; documento?: string; tipo_documento?: string; crianca_ate_5?: boolean }>;
  pagamento?: "integral" | "sinal" | "";
  motivo_humano?: string;
  [chave: string]: unknown;
};

export type ConversaDetalhe = {
  conversa: Conversa;
  estado: EstadoV2;
  mensagens: Mensagem[];
  turnos: Array<Record<string, unknown>>;
  resumo: string;
};

export type ListarConversasParams = {
  status?: ConversaStatus | "";
  q?: string;
  limite?: number;
};

export function listarConversas(params: ListarConversasParams = {}) {
  return apiGet<{ conversas: ConversaListItem[] }>(
    buildListQuery("/atendimento/conversas", {
      status: params.status,
      q: params.q?.trim(),
      limite: params.limite,
    })
  );
}

export function obterConversa(id: string) {
  return apiGet<ConversaDetalhe>(`/atendimento/conversas/${encodeURIComponent(id)}`);
}

export function assumirConversa(id: string) {
  return apiPost<Conversa>(`/atendimento/conversas/${encodeURIComponent(id)}/assumir`, {});
}

export function devolverConversa(id: string) {
  return apiPost<Conversa>(`/atendimento/conversas/${encodeURIComponent(id)}/devolver`, {});
}

export function encerrarConversa(id: string) {
  return apiPost<Conversa>(`/atendimento/conversas/${encodeURIComponent(id)}/encerrar`, {});
}

export function responderConversa(id: string, texto: string) {
  return apiPost<Mensagem>(`/atendimento/conversas/${encodeURIComponent(id)}/responder`, { texto });
}
