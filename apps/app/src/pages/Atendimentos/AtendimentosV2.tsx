import { useEffect, useMemo, useState, type FormEvent } from "react";
import { ArrowLeft } from "lucide-react";
import InlineAlert from "../../components/InlineAlert";
import LoadingState from "../../components/LoadingState";
import StatusBadge from "../../components/StatusBadge";
import SearchToolbar from "../../components/input/SearchToolbar";
import useDebouncedValue from "../../hooks/useDebouncedValue";
import useToast from "../../hooks/useToast";
import {
  useAssumirConversaV2,
  useConversaV2,
  useConversasV2,
  useDevolverConversaV2,
  useEncerrarConversaV2,
  useResponderConversaV2,
} from "../../hooks/useAtendimentoV2";
import { APIRequestError } from "../../services/api";
import type { Conversa, ConversaStatus, TrechoV2 } from "../../services/atendimentoV2";
import ReplyComposer from "./ReplyComposer";
import useMessageAutoScroll from "./useMessageAutoScroll";

type StatusFilter = "" | ConversaStatus;

const FILTERS: Array<{ id: StatusFilter; label: string }> = [
  { id: "", label: "Todas" },
  { id: "BOT", label: "Com o bot" },
  { id: "HUMANO", label: "Com humano" },
  { id: "ENCERRADA", label: "Encerradas" },
];

const STATUS_LABEL: Record<ConversaStatus, string> = {
  BOT: "Com o bot",
  HUMANO: "Com humano",
  ENCERRADA: "Encerrada",
};

const AUTOR_LABEL: Record<string, string> = {
  CLIENTE: "Cliente",
  BOT: "Bot",
  HUMANO: "Atendente",
};

function formatTime(value?: string | null) {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--";
  return date.toLocaleString("pt-BR", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}

function displayName(c: Pick<Conversa, "nome" | "telefone">) {
  return c.nome?.trim() || c.telefone || "Sem nome";
}

function initials(name: string) {
  const chunks = name
    .split(" ")
    .map((p) => p.trim())
    .filter(Boolean)
    .slice(0, 2);
  return chunks.length === 0 ? "?" : chunks.map((c) => c[0]?.toUpperCase() ?? "").join("");
}

function statusTone(status: ConversaStatus): "neutral" | "info" | "success" {
  if (status === "HUMANO") return "success";
  if (status === "BOT") return "info";
  return "neutral";
}

function isAwaiting(c: Conversa) {
  return c.status === "HUMANO" && !c.responsavel_id;
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function isConflict(error: unknown) {
  return error instanceof APIRequestError && error.code === "CONFLICT";
}

export default function AtendimentosV2() {
  const toast = useToast();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("");
  const [search, setSearch] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [mobileThreadOpen, setMobileThreadOpen] = useState(false);
  const [replyBody, setReplyBody] = useState("");
  const [replyError, setReplyError] = useState<string | null>(null);
  const debouncedSearch = useDebouncedValue(search, 250);

  const listQuery = useConversasV2({ status: statusFilter, q: debouncedSearch, limite: 100 });
  const conversas = useMemo(() => listQuery.data ?? [], [listQuery.data]);

  useEffect(() => {
    if (!selectedId && conversas.length > 0) setSelectedId(conversas[0].id);
  }, [conversas, selectedId]);

  const detailQuery = useConversaV2(selectedId);
  const detail = detailQuery.data && detailQuery.data.conversa.id === selectedId ? detailQuery.data : null;
  const active: Conversa | null = detail?.conversa ?? conversas.find((c) => c.id === selectedId) ?? null;
  const mensagens = useMemo(() => detail?.mensagens ?? [], [detail]);

  const { containerRef, showNewMessagesHint, scrollToLatest, handleContainerScroll } = useMessageAutoScroll(
    mensagens.length
  );

  const assumir = useAssumirConversaV2();
  const devolver = useDevolverConversaV2();
  const encerrar = useEncerrarConversaV2();
  const responder = useResponderConversaV2();

  const canReply = active?.status === "HUMANO";
  const resumo = detail?.resumo ?? active?.resumo ?? "";
  const trechos: TrechoV2[] = detail?.estado?.trechos ?? [];

  const run = (mutate: () => Promise<unknown>, ok: string, fail: string) => {
    setReplyError(null);
    mutate().then(
      () => toast.success(ok),
      (error) => toast.error(errorMessage(error, fail))
    );
  };

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    const texto = replyBody.trim();
    if (!active || !canReply || !texto) return;
    setReplyError(null);
    try {
      await responder.mutateAsync({ id: active.id, texto });
      setReplyBody("");
    } catch (error) {
      setReplyError(
        isConflict(error)
          ? "A conversa nao esta mais com atendimento humano. Assuma a conversa para responder."
          : errorMessage(error, "Nao foi possivel enviar a resposta.")
      );
    }
  };

  return (
    <section className="atendimentos-page">
      <div className="atendimentos-layout">
        <div className={`atendimentos-list-panel ${mobileThreadOpen ? "mobile-hidden" : ""}`}>
          <div className="atendimentos-panel">
            <div className="atendimentos-mini-head">
              <strong>Atendimentos</strong>
              <span className="badge">Atualizacao a cada 5s</span>
            </div>
            <div className="atendimentos-sticky-top">
              <div className="atendimentos-rail" role="group" aria-label="Filtrar por status">
                {FILTERS.map((f) => (
                  <button
                    key={f.id || "TODAS"}
                    type="button"
                    className={`atendimentos-rail-item ${statusFilter === f.id ? "active" : ""}`}
                    aria-pressed={statusFilter === f.id}
                    onClick={() => setStatusFilter(f.id)}
                  >
                    <span className="atendimentos-rail-label">{f.label}</span>
                  </button>
                ))}
              </div>
              <SearchToolbar
                value={search}
                onChange={setSearch}
                placeholder="Buscar por nome ou telefone"
                inputLabel="Buscar atendimentos"
                resultCount={conversas.length}
              />
            </div>
            <div className="atendimentos-list-body">
              {listQuery.isLoading ? <LoadingState label="Carregando atendimentos..." /> : null}
              {listQuery.error ? (
                <InlineAlert tone="error">
                  {errorMessage(listQuery.error, "Nao foi possivel carregar os atendimentos.")}
                </InlineAlert>
              ) : null}
              {!listQuery.isLoading && !listQuery.error ? (
                <div className="atendimentos-session-list">
                  {conversas.length === 0 ? (
                    <div className="empty-state">Nenhuma conversa encontrada.</div>
                  ) : (
                    conversas.map((c) => {
                      const name = displayName(c);
                      return (
                        <button
                          key={c.id}
                          type="button"
                          className={`atendimento-session-item ${selectedId === c.id ? "active" : ""}`}
                          aria-pressed={selectedId === c.id}
                          onClick={() => {
                            setSelectedId(c.id);
                            setReplyError(null);
                            setMobileThreadOpen(true);
                          }}
                        >
                          <div className="atendimento-session-leading">
                            <div className="atendimento-avatar">{initials(name)}</div>
                          </div>
                          <div className="atendimento-session-content">
                            <div className="atendimento-session-top">
                              <strong>{name}</strong>
                              <span>{formatTime(c.ultima_mensagem?.criado_em ?? c.atualizado_em)}</span>
                            </div>
                            <div className="atendimento-session-subtitle">{c.telefone}</div>
                            <div className="atendimento-session-preview">
                              {c.ultima_mensagem?.texto || "Sem mensagens"}
                            </div>
                            {c.motivo_humano ? (
                              <div className="atendimento-session-preview">
                                <strong>Motivo: {c.motivo_humano}</strong>
                              </div>
                            ) : null}
                            <div className="atendimento-session-meta">
                              <StatusBadge tone={statusTone(c.status)}>{STATUS_LABEL[c.status] ?? c.status}</StatusBadge>
                              {isAwaiting(c) ? <StatusBadge tone="warning">Aguardando atendente</StatusBadge> : null}
                            </div>
                          </div>
                        </button>
                      );
                    })
                  )}
                </div>
              ) : null}
            </div>
          </div>
        </div>

        <div className={`atendimentos-chat-panel ${mobileThreadOpen ? "mobile-open" : ""}`}>
          {!active ? (
            <div className="empty-state">
              <button
                type="button"
                className="button ghost sm atendimento-back-button-only-mobile"
                onClick={() => setMobileThreadOpen(false)}
              >
                <ArrowLeft size={16} /> Voltar para fila
              </button>
              Selecione um atendimento para visualizar as mensagens.
            </div>
          ) : (
            <>
              <div className="atendimento-chat-header">
                <div className="atendimento-chat-header-main">
                  <div className="atendimento-chat-identity">
                    <button
                      type="button"
                      className="button ghost sm atendimento-back-button"
                      onClick={() => setMobileThreadOpen(false)}
                      aria-label="Voltar para lista"
                    >
                      <ArrowLeft size={16} />
                    </button>
                    <div className="atendimento-avatar lg">{initials(displayName(active))}</div>
                    <div className="atendimento-chat-title-group">
                      <div className="section-title">{displayName(active)}</div>
                      <div className="page-subtitle">{active.telefone}</div>
                      <div className="atendimento-chat-meta-line">
                        <StatusBadge tone={statusTone(active.status)}>
                          {STATUS_LABEL[active.status] ?? active.status}
                        </StatusBadge>
                        {isAwaiting(active) ? <StatusBadge tone="warning">Aguardando atendente</StatusBadge> : null}
                        <span className="atendimento-last-time">Atualizada: {formatTime(active.atualizado_em)}</span>
                      </div>
                    </div>
                  </div>
                  <div className="atendimento-chat-actions">
                    {active.status !== "HUMANO" ? (
                      <button
                        className="button secondary sm"
                        type="button"
                        disabled={assumir.isPending}
                        onClick={() =>
                          run(
                            () => assumir.mutateAsync(active.id),
                            "Atendimento assumido.",
                            "Nao foi possivel assumir o atendimento."
                          )
                        }
                      >
                        {assumir.isPending ? "Assumindo..." : "Assumir"}
                      </button>
                    ) : null}
                    {active.status === "HUMANO" ? (
                      <button
                        className="button ghost sm"
                        type="button"
                        disabled={devolver.isPending}
                        onClick={() =>
                          run(
                            () => devolver.mutateAsync(active.id),
                            "Conversa devolvida ao bot.",
                            "Nao foi possivel devolver ao bot."
                          )
                        }
                      >
                        {devolver.isPending ? "Devolvendo..." : "Devolver ao bot"}
                      </button>
                    ) : null}
                    {active.status !== "ENCERRADA" ? (
                      <button
                        className="button ghost sm"
                        type="button"
                        disabled={encerrar.isPending}
                        onClick={() =>
                          run(
                            () => encerrar.mutateAsync(active.id),
                            "Atendimento encerrado.",
                            "Nao foi possivel encerrar o atendimento."
                          )
                        }
                      >
                        {encerrar.isPending ? "Encerrando..." : "Encerrar"}
                      </button>
                    ) : null}
                  </div>
                </div>
                {active.motivo_humano ? (
                  <div className="atendimento-thread-hint">
                    <strong>Motivo do atendimento humano:</strong> {active.motivo_humano}
                  </div>
                ) : null}
                {active.status === "BOT" ? (
                  <div className="atendimento-thread-hint">Assuma a conversa para responder.</div>
                ) : null}
                {active.status === "ENCERRADA" ? (
                  <div className="atendimento-thread-hint">Conversa encerrada. Assuma a conversa para reabrir e responder.</div>
                ) : null}
              </div>

              <div className="atendimento-messages-wrap">
                {showNewMessagesHint ? (
                  <button type="button" className="atendimento-new-messages" onClick={scrollToLatest}>
                    Novas mensagens disponiveis. Ir para o fim.
                  </button>
                ) : null}
                <div className="atendimento-messages" ref={containerRef} onScroll={handleContainerScroll}>
                  {resumo ? (
                    <div className="atendimento-thread-hint" aria-label="Resumo da reserva">
                      <strong>Resumo da reserva</strong>
                      <pre style={{ margin: "4px 0 0", whiteSpace: "pre-wrap", font: "inherit" }}>{resumo}</pre>
                    </div>
                  ) : null}
                  {trechos.length > 0 ? (
                    <div className="atendimento-thread-hint" aria-label="Trechos da compra">
                      <strong>Trechos ({trechos.length})</strong>
                      <ul style={{ margin: "4px 0 0", paddingLeft: 18 }}>
                        {trechos.map((t, i) => (
                          <li key={`${t.viagem.trip_id ?? i}-${i}`}>
                            {t.viagem.origem} para {t.viagem.destino}, {t.viagem.data.split("-").reverse().join("/")} {t.viagem.horario} -{" "}
                            reserva {t.reserva_id ? "criada" : "pendente"}, PIX {t.pagamento_id ? "gerado" : "pendente"}
                          </li>
                        ))}
                      </ul>
                    </div>
                  ) : null}
                  {detailQuery.isLoading ? <LoadingState label="Carregando mensagens..." /> : null}
                  {detailQuery.error ? (
                    <InlineAlert tone="error">
                      {errorMessage(detailQuery.error, "Nao foi possivel carregar as mensagens.")}
                    </InlineAlert>
                  ) : null}
                  {detail ? (
                    <div className="atendimento-bubble-list">
                      {mensagens.map((m) => {
                        const side = m.autor === "CLIENTE" ? "inbound" : "outbound";
                        return (
                          <div key={m.id} className={`atendimento-bubble-row ${side}`}>
                            <div className={`atendimento-bubble ${side} autor-${m.autor.toLowerCase()}`}>
                              <strong style={{ fontSize: 11, opacity: 0.75 }}>{AUTOR_LABEL[m.autor] ?? m.autor}</strong>
                              <div style={{ whiteSpace: "pre-wrap" }}>{m.texto}</div>
                              <small>{formatTime(m.criado_em)}</small>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  ) : null}
                </div>
              </div>

              {replyError ? <InlineAlert tone="error">{replyError}</InlineAlert> : null}
              <ReplyComposer
                value={replyBody}
                onChange={setReplyBody}
                onSubmit={handleSubmit}
                canReply={canReply}
                pending={responder.isPending}
                disabled={!canReply || responder.isPending}
              />
            </>
          )}
        </div>
      </div>
    </section>
  );
}
