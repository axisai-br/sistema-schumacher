import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import InlineAlert from "../../components/InlineAlert";
import LoadingState from "../../components/LoadingState";
import SearchToolbar from "../../components/input/SearchToolbar";
import { useCurrentUser } from "../../hooks/useCurrentUser";
import useDebouncedValue from "../../hooks/useDebouncedValue";
import useToast from "../../hooks/useToast";
import { apiGet, apiPost, apiPostForm } from "../../services/api";
import { buildListQuery } from "../../hooks/buildListQuery";
import { isSessionInTab, sortByLastMessageDesc, type AtendimentoTab } from "./sessionFilters";
import AtendimentosRail from "./AtendimentosRail";
import ConversationListItem from "./ConversationListItem";
import ConversationThreadHeader from "./ConversationThreadHeader";
import MessageBubble from "./MessageBubble";
import ReplyComposer from "./ReplyComposer";
import type { ChatMessage, ChatSession } from "./types";
import useMessageAutoScroll from "./useMessageAutoScroll";
import {
  canUserReplyToSession,
  getTabEmptyStateMessage,
  mapMessageToBubbleVM,
  mapSessionToConversationListItemVM,
} from "./viewModels";

function dedupeMessages(messages: ChatMessage[]) {
  const seen = new Set<string>();
  return messages.filter((message) => {
    const body = (message.body ?? "").trim().replace(/\s+/g, " ");
    const tsRaw = message.sent_at ?? message.received_at ?? message.created_at;
    const ts = Number.isNaN(Date.parse(tsRaw)) ? tsRaw : new Date(tsRaw).toISOString().slice(0, 16);
    const fingerprint = [message.direction, body, ts].join("|");
    if (seen.has(fingerprint)) return false;
    seen.add(fingerprint);
    return true;
  });
}
export default function AtendimentosPage() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const currentUserQuery = useCurrentUser();
  const currentUserId = currentUserQuery.data?.user_id ?? "";

  const [activeTab, setActiveTab] = useState<AtendimentoTab>("NAO_ATENDIDO");
  const [search, setSearch] = useState("");
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);
  const [mobileThreadOpen, setMobileThreadOpen] = useState(false);
  const [replyBody, setReplyBody] = useState("");
  const debouncedSearch = useDebouncedValue(search, 250);

  const sessionsQuery = useQuery({
    queryKey: ["chat-sessions", "WHATSAPP"],
    queryFn: () =>
      apiGet<ChatSession[]>(
        buildListQuery("/chat/sessions", {
          channel: "WHATSAPP",
          limit: 200,
        })
      ),
    refetchInterval: 5000,
  });

  const allSessions = sessionsQuery.data ?? [];
  const tabCounts = useMemo(() => {
    return {
      NAO_ATENDIDO: allSessions.filter((session) => isSessionInTab(session, "NAO_ATENDIDO")).length,
      ABERTO: allSessions.filter((session) => isSessionInTab(session, "ABERTO")).length,
      FINALIZADO: allSessions.filter((session) => isSessionInTab(session, "FINALIZADO")).length,
    };
  }, [allSessions]);

  const visibleSessions = useMemo(() => {
    const q = debouncedSearch.trim().toLowerCase();
    return allSessions
      .filter((session) => isSessionInTab(session, activeTab))
      .filter((session) => {
        if (!q) return true;
        return [
          session.customer_name ?? "",
          session.customer_phone ?? "",
          session.contact_key ?? "",
        ]
          .join(" ")
          .toLowerCase()
          .includes(q);
      })
      .sort(sortByLastMessageDesc);
  }, [activeTab, allSessions, debouncedSearch]);

  useEffect(() => {
    if (!selectedSessionId) {
      if (visibleSessions.length > 0) {
        setSelectedSessionId(visibleSessions[0].id);
      }
      return;
    }
    const stillExists = allSessions.some((item) => item.id === selectedSessionId);
    if (!stillExists) {
      setSelectedSessionId(visibleSessions[0]?.id ?? null);
    }
  }, [allSessions, selectedSessionId, visibleSessions]);

  const activeSession = useMemo(
    () => allSessions.find((item) => item.id === selectedSessionId) ?? null,
    [allSessions, selectedSessionId]
  );
  const threadStatusHint = useMemo(() => {
    if (!activeSession) return null;
    if (activeSession.status === "RESOLVED") {
      return "Atendimento finalizado. Reabre automaticamente quando o cliente enviar nova mensagem.";
    }
    if (!canUserReplyToSession(activeSession, currentUserId)) {
      return "Sem posse ativa. Clique em Assumir para responder.";
    }
    return null;
  }, [activeSession, currentUserId]);

  const messagesQuery = useQuery({
    queryKey: ["chat-session-messages", selectedSessionId],
    queryFn: () =>
      apiGet<ChatMessage[]>(
        buildListQuery(`/chat/sessions/${selectedSessionId}/messages`, {
          limit: 200,
        })
      ),
    enabled: Boolean(selectedSessionId),
    refetchInterval: selectedSessionId ? 5000 : false,
  });
  const messageItems = useMemo(
    () => dedupeMessages(messagesQuery.data ?? []).map(mapMessageToBubbleVM),
    [messagesQuery.data]
  );
  const {
    containerRef: messagesContainerRef,
    showNewMessagesHint,
    scrollToLatest,
    handleContainerScroll,
  } = useMessageAutoScroll(messageItems.length);

  const invalidateChatQueries = async () => {
    await queryClient.invalidateQueries({ queryKey: ["chat-sessions"] });
    if (selectedSessionId) {
      await queryClient.invalidateQueries({ queryKey: ["chat-session-messages", selectedSessionId] });
    }
  };

  const assumeMutation = useMutation({
    mutationFn: async (sessionId: string) =>
      apiPost(`/chat/sessions/${sessionId}/handoff`, {
        assigned_user_id: currentUserId,
        requested_by: "APP_ATTENDIMENTOS",
        reason: "Assumido no painel de atendimentos",
        metadata: { source: "app_attendimentos" },
      }),
    onSuccess: async () => {
      toast.success("Atendimento assumido.");
      await invalidateChatQueries();
    },
    onError: (error: Error) => {
      toast.error(error.message || "Nao foi possivel assumir o atendimento.");
    },
  });

  const replyMutation = useMutation({
    mutationFn: async ({ sessionId, body }: { sessionId: string; body: string }) =>
      apiPost(`/chat/sessions/${sessionId}/reply`, {
        owner_user_id: currentUserId,
        body,
        sender_name: "OPERADOR",
        idempotency_key: `app-reply-${sessionId}-${Date.now()}`,
        metadata: { source: "app_atendimentos" },
      }),
    onSuccess: async () => {
      setReplyBody("");
      toast.success("Resposta enviada.");
      await invalidateChatQueries();
    },
    onError: (error: Error) => {
      toast.error(error.message || "Nao foi possivel enviar a resposta.");
    },
  });

  const resolveMutation = useMutation({
    mutationFn: async (sessionId: string) =>
      apiPost(`/chat/sessions/${sessionId}/resolve`, {
        resolved_by_user_id: currentUserId,
        reason: "Atendimento finalizado no painel",
        metadata: { source: "app_atendimentos" },
      }),
    onSuccess: async () => {
      toast.success("Atendimento finalizado.");
      await invalidateChatQueries();
    },
    onError: (error: Error) => {
      toast.error(error.message || "Nao foi possivel finalizar o atendimento.");
    },
  });

  const replyMediaMutation = useMutation({
    mutationFn: async ({
      sessionId,
      file,
      caption,
      mediaType,
    }: {
      sessionId: string;
      file: File;
      caption: string;
      mediaType: "IMAGE" | "AUDIO" | "DOCUMENT";
    }) => {
      const formData = new FormData();
      formData.append("owner_user_id", currentUserId);
      formData.append("sender_name", "OPERADOR");
      formData.append("idempotency_key", `app-media-${sessionId}-${Date.now()}`);
      formData.append("media_type", mediaType);
      if (caption.trim()) {
        formData.append("caption", caption.trim());
      }
      formData.append("file", file, file.name);
      return apiPostForm(`/chat/sessions/${sessionId}/reply/media`, formData);
    },
    onSuccess: async () => {
      setReplyBody("");
      toast.success("Midia enviada.");
      await invalidateChatQueries();
    },
    onError: (error: Error) => {
      toast.error(error.message || "Nao foi possivel enviar a midia.");
    },
  });

  const canReply = canUserReplyToSession(activeSession, currentUserId);
  const canAssume =
    Boolean(activeSession) && activeSession.status === "ACTIVE" && currentUserId !== "";
  const composerPending = replyMutation.isPending || replyMediaMutation.isPending;

  const handleSendMedia = async (file: File) => {
    if (!activeSession) return;
    if (!canReply) {
      toast.error("Assuma o atendimento para enviar mensagens.");
      return;
    }

    const mimeType = file.type.toLowerCase();
    let mediaType: "IMAGE" | "AUDIO" | "DOCUMENT" = "DOCUMENT";
    if (mimeType.startsWith("image/")) {
      mediaType = "IMAGE";
    } else if (mimeType.startsWith("audio/")) {
      mediaType = "AUDIO";
    }

    await replyMediaMutation.mutateAsync({
      sessionId: activeSession.id,
      file,
      caption: replyBody,
      mediaType,
    });
  };

  const handleSubmitReply = async (event: FormEvent) => {
    event.preventDefault();
    if (!activeSession || !canReply || !replyBody.trim()) return;
    await replyMutation.mutateAsync({
      sessionId: activeSession.id,
      body: replyBody.trim(),
    });
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
              <AtendimentosRail activeTab={activeTab} tabCounts={tabCounts} onSelectTab={setActiveTab} />

              <SearchToolbar
                value={search}
                onChange={setSearch}
                placeholder="Buscar por nome, telefone ou contato"
                inputLabel="Buscar atendimentos"
                resultCount={visibleSessions.length}
              />
            </div>
            <div className="atendimentos-list-body">
              {sessionsQuery.isLoading ? <LoadingState label="Carregando atendimentos..." /> : null}
              {sessionsQuery.error ? (
                <InlineAlert tone="error">
                  {(sessionsQuery.error as Error).message || "Nao foi possivel carregar os atendimentos."}
                </InlineAlert>
              ) : null}

              {!sessionsQuery.isLoading && !sessionsQuery.error ? (
                <div className="atendimentos-session-list">
                  {visibleSessions.length === 0 ? (
                    <div className="empty-state">{getTabEmptyStateMessage(activeTab)}</div>
                  ) : (
                    visibleSessions.map((session) => {
                      const item = mapSessionToConversationListItemVM(session);
                      return (
                        <ConversationListItem
                          key={session.id}
                          item={item}
                          active={selectedSessionId === session.id}
                          onSelect={() => {
                            setSelectedSessionId(session.id);
                            setMobileThreadOpen(true);
                          }}
                        />
                      );
                    })
                  )}
                </div>
              ) : null}
            </div>
          </div>
        </div>

        <div className={`atendimentos-chat-panel ${mobileThreadOpen ? "mobile-open" : ""}`}>
          {!activeSession ? (
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
              <ConversationThreadHeader
                activeSession={activeSession}
                canReply={canReply}
                canAssume={canAssume}
                statusHint={threadStatusHint}
                assumePending={assumeMutation.isPending}
                resolvePending={resolveMutation.isPending}
                onAssume={() => assumeMutation.mutate(activeSession.id)}
                onResolve={() => resolveMutation.mutate(activeSession.id)}
                onBackToList={() => setMobileThreadOpen(false)}
              />

              <div className="atendimento-messages-wrap">
                {showNewMessagesHint ? (
                  <button type="button" className="atendimento-new-messages" onClick={scrollToLatest}>
                    Novas mensagens disponiveis. Ir para o fim.
                  </button>
                ) : null}
                <div className="atendimento-messages" ref={messagesContainerRef} onScroll={handleContainerScroll}>
                  {messagesQuery.isLoading ? <LoadingState label="Carregando mensagens..." /> : null}
                  {messagesQuery.error ? (
                    <InlineAlert tone="error">
                      {(messagesQuery.error as Error).message || "Nao foi possivel carregar as mensagens."}
                    </InlineAlert>
                  ) : null}
                  {!messagesQuery.isLoading && !messagesQuery.error ? (
                    <div className="atendimento-bubble-list">
                      {messageItems.map((message) => (
                        <MessageBubble key={message.id} message={message} />
                      ))}
                    </div>
                  ) : null}
                </div>
              </div>

              <ReplyComposer
                value={replyBody}
                onChange={setReplyBody}
                onSubmit={handleSubmitReply}
                canReply={canReply}
                pending={composerPending}
                disabled={!canReply || composerPending}
                onMicClick={() => {
                  toast.error("Seu navegador nao suporta gravacao de audio nesta sessao.");
                }}
                onAttachFile={handleSendMedia}
              />
            </>
          )}
        </div>
      </div>
    </section>
  );
}



