import { useQuery, type QueryKey } from "@tanstack/react-query";
import { apiGet } from "../../services/api";
import { buildListQuery } from "../buildListQuery";

type EntityListConfig<TFilters extends Record<string, unknown>> = {
  queryKey: QueryKey;
  path: string;
  limit?: number;
  offset?: number;
  filters?: TFilters;
  enabled?: boolean;
};

function normalizeFilters<TFilters extends Record<string, unknown>>(filters: TFilters | undefined) {
  if (!filters) return {} as TFilters;

  return Object.fromEntries(
    Object.entries(filters).map(([key, value]) => {
      if (typeof value === "string") {
        return [key, value.trim()];
      }
      return [key, value];
    })
  ) as TFilters;
}

export function useEntityList<TItem, TFilters extends Record<string, unknown> = Record<string, never>>({
  queryKey,
  path,
  limit = 200,
  offset = 0,
  filters,
  enabled,
}: EntityListConfig<TFilters>) {
  const normalizedFilters = normalizeFilters(filters);

  return useQuery({
    queryKey: [...queryKey, limit, offset, ...Object.values(normalizedFilters)],
    enabled,
    queryFn: () =>
      apiGet<TItem[]>(
        buildListQuery(path, {
          limit,
          offset,
          ...normalizedFilters,
        })
      ),
  });
}
