export type AtendimentoTab = "NAO_ATENDIDO" | "ABERTO" | "FINALIZADO";

export type AtendimentoSession = {
  id: string;
  status: string;
  handoff_status: string;
  last_message_at?: string | null;
};

export function isSessionInTab(session: AtendimentoSession, tab: AtendimentoTab) {
  if (tab === "NAO_ATENDIDO") {
    return session.status === "ACTIVE" && session.handoff_status === "BOT";
  }
  if (tab === "ABERTO") {
    return session.status === "ACTIVE" && (session.handoff_status === "HUMAN" || session.handoff_status === "HUMAN_REQUESTED");
  }
  return session.status === "RESOLVED";
}

export function sortByLastMessageDesc(left: AtendimentoSession, right: AtendimentoSession) {
  const leftMs = left.last_message_at ? Date.parse(left.last_message_at) : 0;
  const rightMs = right.last_message_at ? Date.parse(right.last_message_at) : 0;
  return rightMs - leftMs;
}
