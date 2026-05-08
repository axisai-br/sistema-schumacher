import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiPatch, apiPost } from "../../services/api";

type ActionMethod = "post" | "patch" | "delete";

type EntityActionOptions<TPayload, TResponse> = {
  buildPath: (id: string) => string;
  method?: ActionMethod;
  getBody?: (payload: TPayload) => unknown;
};

type EntityActionsConfig = {
  queryKey: readonly unknown[];
  invalidateQueryKeys?: readonly (readonly unknown[])[];
};

function invalidateAll(
  queryClient: ReturnType<typeof useQueryClient>,
  queryKey: readonly unknown[],
  invalidateQueryKeys?: readonly (readonly unknown[])[]
) {
  queryClient.invalidateQueries({ queryKey });
  for (const key of invalidateQueryKeys ?? []) {
    queryClient.invalidateQueries({ queryKey: key });
  }
}

export function createEntityActions({ queryKey, invalidateQueryKeys }: EntityActionsConfig) {
  function useEntityAction<TPayload = void, TResponse = unknown>({
    buildPath,
    method = "post",
    getBody,
  }: EntityActionOptions<TPayload, TResponse>) {
    const queryClient = useQueryClient();

    return useMutation({
      mutationFn: ({ id, payload }: { id: string; payload: TPayload }) => {
        const path = buildPath(id);

        if (method === "delete") {
          return apiDelete(path) as Promise<TResponse>;
        }

        const body = getBody ? getBody(payload) : (payload as unknown);

        if (method === "patch") {
          return apiPatch<TResponse>(path, body);
        }

        return apiPost<TResponse>(path, body);
      },
      onSuccess: () => invalidateAll(queryClient, queryKey, invalidateQueryKeys),
    });
  }

  return { useEntityAction };
}
