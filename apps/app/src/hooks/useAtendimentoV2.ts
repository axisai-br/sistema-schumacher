import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  assumirConversa,
  devolverConversa,
  encerrarConversa,
  listarConversas,
  obterConversa,
  responderConversa,
  type ListarConversasParams,
} from "../services/atendimentoV2";

export const atendimentoV2Keys = {
  all: ["atendimento-v2"] as const,
  lista: (params: ListarConversasParams) => ["atendimento-v2", "conversas", params] as const,
  detalhe: (id: string | null) => ["atendimento-v2", "conversa", id] as const,
};

export function useConversasV2(params: ListarConversasParams) {
  return useQuery({
    queryKey: atendimentoV2Keys.lista(params),
    queryFn: () => listarConversas(params),
    select: (data) => data.conversas ?? [],
    refetchInterval: 5000,
  });
}

export function useConversaV2(id: string | null) {
  return useQuery({
    queryKey: atendimentoV2Keys.detalhe(id),
    queryFn: () => obterConversa(id as string),
    enabled: Boolean(id),
    refetchInterval: id ? 3000 : false,
  });
}

function useInvalidate() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: atendimentoV2Keys.all });
}

export function useAssumirConversaV2() {
  const invalidate = useInvalidate();
  return useMutation({ mutationFn: (id: string) => assumirConversa(id), onSuccess: invalidate });
}

export function useDevolverConversaV2() {
  const invalidate = useInvalidate();
  return useMutation({ mutationFn: (id: string) => devolverConversa(id), onSuccess: invalidate });
}

export function useEncerrarConversaV2() {
  const invalidate = useInvalidate();
  return useMutation({ mutationFn: (id: string) => encerrarConversa(id), onSuccess: invalidate });
}

export function useResponderConversaV2() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ id, texto }: { id: string; texto: string }) => responderConversa(id, texto),
    onSuccess: invalidate,
    // 409: a conversa deixou de estar HUMANO; atualiza para refletir o estado real.
    onError: invalidate,
  });
}
