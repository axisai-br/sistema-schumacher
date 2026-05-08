import StatusBadge from "../../components/StatusBadge";
import type { ConversationListItemVM } from "./types";

type ConversationListItemProps = {
  item: ConversationListItemVM;
  active: boolean;
  onSelect: () => void;
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

export default function ConversationListItem({ item, active, onSelect }: ConversationListItemProps) {
  return (
    <button
      type="button"
      className={`atendimento-session-item ${active ? "active" : ""}`}
      onClick={onSelect}
      aria-pressed={active}
    >
      <div className="atendimento-session-leading">
        <div className="atendimento-avatar">{getInitials(item.title)}</div>
        <span
          className={`atendimento-activity-dot ${item.isRecentActivity ? "active" : ""}`}
          aria-hidden="true"
        />
      </div>
      <div className="atendimento-session-content">
        <div className="atendimento-session-top">
          <strong>{item.title}</strong>
          <span>{item.timeLabel}</span>
        </div>
        <div className="atendimento-session-subtitle">{item.subtitle}</div>
        <div className="atendimento-session-preview">{item.preview}</div>
        <div className="atendimento-session-meta">
          <StatusBadge tone={item.statusTone}>{item.statusLabel}</StatusBadge>
          <StatusBadge tone={item.handoffTone}>{item.handoffLabel}</StatusBadge>
        </div>
      </div>
    </button>
  );
}
