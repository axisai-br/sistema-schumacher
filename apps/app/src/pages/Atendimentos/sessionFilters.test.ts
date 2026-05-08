import { describe, expect, it } from "vitest";
import { isSessionInTab, sortByLastMessageDesc, type AtendimentoSession } from "./sessionFilters";

describe("atendimento session filters", () => {
  it("classifica sessoes nas tres abas operacionais", () => {
    const naoAtendido: AtendimentoSession = {
      id: "s1",
      status: "ACTIVE",
      handoff_status: "BOT",
    };
    const aberto: AtendimentoSession = {
      id: "s2",
      status: "ACTIVE",
      handoff_status: "HUMAN",
    };
    const finalizado: AtendimentoSession = {
      id: "s3",
      status: "RESOLVED",
      handoff_status: "BOT",
    };

    expect(isSessionInTab(naoAtendido, "NAO_ATENDIDO")).toBe(true);
    expect(isSessionInTab(aberto, "ABERTO")).toBe(true);
    expect(isSessionInTab(finalizado, "FINALIZADO")).toBe(true);
    expect(isSessionInTab(naoAtendido, "FINALIZADO")).toBe(false);
  });

  it("ordena sessoes pela ultima mensagem em ordem decrescente", () => {
    const older: AtendimentoSession = {
      id: "older",
      status: "ACTIVE",
      handoff_status: "BOT",
      last_message_at: "2026-05-06T08:00:00Z",
    };
    const newer: AtendimentoSession = {
      id: "newer",
      status: "ACTIVE",
      handoff_status: "BOT",
      last_message_at: "2026-05-06T09:00:00Z",
    };

    expect([older, newer].sort(sortByLastMessageDesc).map((item) => item.id)).toEqual(["newer", "older"]);
  });
});
