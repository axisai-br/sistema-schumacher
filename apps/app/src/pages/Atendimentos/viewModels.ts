import type { AtendimentoTab } from "./sessionFilters";
import type {
  ChatMessage,
  ChatSession,
  ConversationListItemVM,
  ConversationPresenceState,
  MessageBubbleVM,
} from "./types";

const RECENT_ACTIVITY_WINDOW_MS = 1000 * 60 * 30;

export function formatAtendimentoTime(value?: string | null) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";

  const now = new Date();
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();

  if (sameDay) {
    return date.toLocaleTimeString("pt-BR", {
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  return date.toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function getSessionDisplayName(session: ChatSession) {
  const name = session.customer_name?.trim();
  if (name) return name;
  const phone = session.customer_phone?.trim();
  if (phone) return phone;
  return session.contact_key;
}

export function getSessionSecondaryLabel(session: ChatSession) {
  const phone = session.customer_phone?.trim();
  if (phone) return phone;
  return session.contact_key;
}

export function getConversationPresenceState(session: ChatSession): ConversationPresenceState {
  if (session.status === "RESOLVED") {
    return "RESOLVED";
  }
  if (session.handoff_status === "HUMAN" || session.handoff_status === "HUMAN_REQUESTED") {
    return "IN_PROGRESS";
  }
  return "UNATTENDED";
}

export function isRecentActivity(lastMessageAt?: string | null) {
  if (!lastMessageAt) return false;
  const ms = Date.parse(lastMessageAt);
  if (Number.isNaN(ms)) return false;
  return Date.now() - ms <= RECENT_ACTIVITY_WINDOW_MS;
}

function getFallbackPreview(session: ChatSession) {
  const metadataPreview =
    typeof session.metadata?.last_message_preview === "string"
      ? session.metadata.last_message_preview.trim()
      : "";
  if (metadataPreview) {
    return metadataPreview;
  }
  if (session.status === "RESOLVED") {
    return "Atendimento finalizado aguardando nova mensagem.";
  }
  if (session.handoff_status === "HUMAN") {
    return "Em atendimento humano.";
  }
  if (session.handoff_status === "HUMAN_REQUESTED") {
    return "Solicitacao de operador pendente.";
  }
  return "Aguardando interacao do cliente.";
}

export function mapSessionToConversationListItemVM(session: ChatSession): ConversationListItemVM {
  const presence = getConversationPresenceState(session);
  return {
    id: session.id,
    title: getSessionDisplayName(session),
    subtitle: getSessionSecondaryLabel(session),
    preview: getFallbackPreview(session),
    timeLabel: formatAtendimentoTime(session.last_message_at),
    isRecentActivity: isRecentActivity(session.last_message_at),
    statusLabel: session.status,
    handoffLabel: session.handoff_status,
    statusTone: session.status === "RESOLVED" ? "neutral" : "info",
    handoffTone:
      session.handoff_status === "HUMAN"
        ? "success"
        : session.handoff_status === "HUMAN_REQUESTED"
          ? "neutral"
          : "warning",
    presence,
  };
}

export function mapMessageToBubbleVM(message: ChatMessage): MessageBubbleVM {
  const outbound = message.direction === "OUTBOUND";
  return {
    id: message.id,
    body: message.body?.trim() || "[sem texto]",
    timeLabel: formatAtendimentoTime(message.sent_at || message.received_at || message.created_at),
    outbound,
  };
}

export function canUserReplyToSession(session: ChatSession | null, currentUserId: string) {
  if (!session || currentUserId === "") return false;
  return (
    session.status === "ACTIVE" &&
    session.handoff_status === "HUMAN" &&
    session.current_owner_user_id === currentUserId
  );
}

export function getTabEmptyStateMessage(tab: AtendimentoTab) {
  if (tab === "NAO_ATENDIDO") {
    return "Nenhum atendimento aguardando operador neste momento.";
  }
  if (tab === "ABERTO") {
    return "Nenhum atendimento aberto em posse humana.";
  }
  return "Nenhum atendimento finalizado para este filtro.";
}
