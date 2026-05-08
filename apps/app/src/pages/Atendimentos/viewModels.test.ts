import { describe, expect, it } from "vitest";
import type { ChatSession } from "./types";
import {
  canUserReplyToSession,
  getConversationPresenceState,
  getSessionDisplayName,
  mapSessionToConversationListItemVM,
} from "./viewModels";

function buildSession(overrides: Partial<ChatSession> = {}): ChatSession {
  return {
    id: "s1",
    channel: "WHATSAPP",
    contact_key: "5511999999999@whatsapp.net",
    status: "ACTIVE",
    handoff_status: "BOT",
    ...overrides,
  };
}

describe("atendimentos view models", () => {
  it("usa fallback de nome para telefone e contact key", () => {
    const withPhone = buildSession({ customer_phone: "5511999999999" });
    const withoutPhone = buildSession({ customer_phone: undefined });

    expect(getSessionDisplayName(withPhone)).toBe("5511999999999");
    expect(getSessionDisplayName(withoutPhone)).toBe("5511999999999@whatsapp.net");
  });

  it("deriva estado de presenca e badges da sessao", () => {
    const unattended = mapSessionToConversationListItemVM(buildSession({ handoff_status: "BOT" }));
    const inProgress = mapSessionToConversationListItemVM(buildSession({ handoff_status: "HUMAN" }));
    const resolved = mapSessionToConversationListItemVM(buildSession({ status: "RESOLVED", handoff_status: "BOT" }));

    expect(unattended.presence).toBe("UNATTENDED");
    expect(inProgress.presence).toBe("IN_PROGRESS");
    expect(resolved.presence).toBe("RESOLVED");
    expect(getConversationPresenceState(buildSession({ handoff_status: "HUMAN_REQUESTED" }))).toBe("IN_PROGRESS");
  });

  it("libera resposta apenas para operador dono da posse humana", () => {
    const session = buildSession({
      handoff_status: "HUMAN",
      current_owner_user_id: "user-1",
    });

    expect(canUserReplyToSession(session, "user-1")).toBe(true);
    expect(canUserReplyToSession(session, "user-2")).toBe(false);
    expect(canUserReplyToSession(buildSession({ handoff_status: "BOT" }), "user-1")).toBe(false);
  });
});
