import type { MessageBubbleVM } from "./types";

type MessageBubbleProps = {
  message: MessageBubbleVM;
};

export default function MessageBubble({ message }: MessageBubbleProps) {
  return (
    <div className={`atendimento-bubble-row ${message.outbound ? "outbound" : "inbound"}`}>
      <div className={`atendimento-bubble ${message.outbound ? "outbound" : "inbound"}`}>
        <div>{message.body}</div>
        <small>
          {message.timeLabel}
          {message.statusLabel ? <span>{message.statusLabel}</span> : null}
        </small>
      </div>
    </div>
  );
}
