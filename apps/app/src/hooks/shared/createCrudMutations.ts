import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiPatch, apiPost } from "../../services/api";

type CrudMutationsConfig = {
  basePath: string;
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

export function createCrudMutations<TCreateInput, TUpdateInput, TResponse = unknown>({
  basePath,
  queryKey,
  invalidateQueryKeys,
}: CrudMutationsConfig) {
  function useCreateEntity() {
    const queryClient = useQueryClient();
    return useMutation({
      mutationFn: (data: TCreateInput) => apiPost<TResponse>(basePath, data),
      onSuccess: () => invalidateAll(queryClient, queryKey, invalidateQueryKeys),
    });
  }

  function useUpdateEntity() {
    const queryClient = useQueryClient();
    return useMutation({
      mutationFn: ({ id, data }: { id: string; data: TUpdateInput }) =>
        apiPatch<TResponse>(`${basePath}/${id}`, data),
      onSuccess: () => invalidateAll(queryClient, queryKey, invalidateQueryKeys),
    });
  }

  function useDeleteEntity() {
    const queryClient = useQueryClient();
    return useMutation({
      mutationFn: (id: string) => apiDelete(`${basePath}/${id}`),
      onSuccess: () => invalidateAll(queryClient, queryKey, invalidateQueryKeys),
    });
  }

  return {
    useCreateEntity,
    useUpdateEntity,
    useDeleteEntity,
  };
}
