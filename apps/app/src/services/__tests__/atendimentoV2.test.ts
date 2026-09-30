import { afterEach, describe, expect, it, vi } from "vitest";
import { apiGet, apiPost } from "../api";
import {
  assumirConversa,
  devolverConversa,
  encerrarConversa,
  listarConversas,
  obterConversa,
  responderConversa,
} from "../atendimentoV2";

vi.mock("../api", () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
}));

describe("atendimentoV2 service", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("lista conversas sem filtros", async () => {
    vi.mocked(apiGet).mockResolvedValueOnce({ conversas: [] });
    await listarConversas();
    expect(apiGet).toHaveBeenCalledWith("/atendimento/conversas");
  });

  it("lista conversas com status, busca e limite", async () => {
    vi.mocked(apiGet).mockResolvedValueOnce({ conversas: [] });
    await listarConversas({ status: "HUMANO", q: " joao ", limite: 50 });
    expect(apiGet).toHaveBeenCalledWith("/atendimento/conversas?status=HUMANO&q=joao&limite=50");
  });

  it("obtem detalhe", async () => {
    vi.mocked(apiGet).mockResolvedValueOnce({ conversa: { id: "c1" }, mensagens: [] });
    const res = await obterConversa("c1");
    expect(apiGet).toHaveBeenCalledWith("/atendimento/conversas/c1");
    expect(res.conversa.id).toBe("c1");
  });

  it("dispara assumir, devolver e encerrar", async () => {
    vi.mocked(apiPost).mockResolvedValue({});
    await assumirConversa("c1");
    await devolverConversa("c1");
    await encerrarConversa("c1");
    expect(apiPost).toHaveBeenNthCalledWith(1, "/atendimento/conversas/c1/assumir", {});
    expect(apiPost).toHaveBeenNthCalledWith(2, "/atendimento/conversas/c1/devolver", {});
    expect(apiPost).toHaveBeenNthCalledWith(3, "/atendimento/conversas/c1/encerrar", {});
  });

  it("responde com texto e propaga erro", async () => {
    vi.mocked(apiPost).mockResolvedValueOnce({ id: "m1" });
    await responderConversa("c1", "ola");
    expect(apiPost).toHaveBeenCalledWith("/atendimento/conversas/c1/responder", { texto: "ola" });

    vi.mocked(apiPost).mockRejectedValueOnce(new Error("conflict"));
    await expect(responderConversa("c1", "x")).rejects.toThrow("conflict");
  });
});
