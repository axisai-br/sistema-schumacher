import { ArrowLeft } from "lucide-react";
import StatusBadge from "../../components/StatusBadge";
import type { ChatSession } from "./types";
import { formatAtendimentoTime, getSessionDisplayName, getSessionSecondaryLabel } from "./viewModels";

type ConversationThreadHeaderProps = {
  activeSession: ChatSession;
  canReply: boolean;
  canAssume: boolean;
  statusHint: string | null;
  assumePending: boolean;
  resolvePending: boolean;
  onAssume: () => void;
  onResolve: () => void;
  onBackToList: () => void;
};

function getInitials(name: string) {
  const chunks = name
    .split(" ")
    .map((part) => part.trim())
    .filter(Boolean)
    .slice(0, 2);
  if (chunks.length === 0) return "?";
  return chunks.map((chunk) => chunk[0]?.toUpperCase() ?? "").join("");
}

export default function ConversationThreadHeader({
  activeSession,
  canReply,
  canAssume,
  statusHint,
  assumePending,
  resolvePending,
  onAssume,
  onResolve,
  onBackToList,
}: ConversationThreadHeaderProps) {
  const title = getSessionDisplayName(activeSession);
  const subtitle = getSessionSecondaryLabel(activeSession);

  return (
    <div className="atendimento-chat-header">
      <div className="atendimento-chat-header-main">
        <div className="atendimento-chat-identity">
          <button
            type="button"
            className="button ghost sm atendimento-back-button"
            onClick={onBackToList}
            aria-label="Voltar para lista"
          >
            <ArrowLeft size={16} />
          </button>
          <div className="atendimento-avatar lg">{getInitials(title)}</div>
          <div className="atendimento-chat-title-group">
            <div className="section-title">{title}</div>
            <div className="page-subtitle">{subtitle}</div>
            <div className="atendimento-chat-meta-line">
              <StatusBadge tone={activeSession.status === "RESOLVED" ? "neutral" : "info"}>
                {activeSession.status}
              </StatusBadge>
              <StatusBadge
                tone={
                  activeSession.handoff_status === "HUMAN"
                    ? "success"
                    : activeSession.handoff_status === "HUMAN_REQUESTED"
                      ? "neutral"
                      : "warning"
                }
              >
                {activeSession.handoff_status}
              </StatusBadge>
              <span className="atendimento-last-time">
                Ultima mensagem: {formatAtendimentoTime(activeSession.last_message_at)}
              </span>
            </div>
          </div>
        </div>
        <div className="atendimento-chat-actions">
          {canAssume ? (
            <button className="button secondary sm" type="button" disabled={assumePending} onClick={onAssume}>
              {assumePending ? "Assumindo..." : "Assumir"}
            </button>
          ) : null}
          {canReply ? (
            <button className="button ghost sm" type="button" disabled={resolvePending} onClick={onResolve}>
              {resolvePending ? "Finalizando..." : "Finalizar atendimento"}
            </button>
          ) : null}
        </div>
      </div>
      {statusHint ? <div className="atendimento-thread-hint">{statusHint}</div> : null}
    </div>
  );
}
