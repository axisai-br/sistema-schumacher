import type { AtendimentoSession } from "./sessionFilters";

export type ChatSession = AtendimentoSession & {
  channel: string;
  contact_key: string;
  customer_phone?: string;
  customer_name?: string;
  current_owner_user_id?: string;
  metadata?: Record<string, unknown> | null;
};

export type ChatMessage = {
  id: string;
  direction: string;
  body?: string;
  sender_name?: string;
  processing_status: "AUTOMATION_PENDING" | "AUTOMATION_SENT";
  received_at: string;
  sent_at?: string | null;
  created_at: string;
};

export type ConversationPresenceState = "UNATTENDED" | "IN_PROGRESS" | "RESOLVED";

export type ConversationListItemVM = {
  id: string;
  title: string;
  subtitle: string;
  preview: string;
  timeLabel: string;
  isRecentActivity: boolean;
  statusLabel: string;
  handoffLabel: string;
  statusTone: "neutral" | "info" | "success";
  handoffTone: "warning" | "success" | "neutral";
  presence: ConversationPresenceState;
};

export type MessageBubbleVM = {
  id: string;
  body: string;
  timeLabel: string;
  statusSymbol: string;
  statusLabel: string;
  outbound: boolean;
};
