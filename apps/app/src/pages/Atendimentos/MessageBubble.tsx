import type { MessageBubbleVM } from "./types";

type MessageBubbleProps = {
  message: MessageBubbleVM;
};

export default function MessageBubble({ message }: MessageBubbleProps) {
  return (
    <div className={`atendimento-bubble-row ${message.outbound ? "outbound" : "inbound"}`}>
      <div className={`atendimento-bubble ${message.outbound ? "outbound" : "inbound"}`}>
        <div>{message.body}</div>
        <small title={message.statusLabel}>
          {message.timeLabel} {message.statusSymbol}
        </small>
      </div>
    </div>
  );
}
