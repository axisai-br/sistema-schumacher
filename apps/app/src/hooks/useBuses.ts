import { useEntityList } from "./shared/useEntityList";

export type BusItem = {
  id: string;
  name: string;
};

type UseBusesOptions = {
  search?: string;
};

export function useBuses(limit = 200, offset = 0, options: UseBusesOptions = {}) {
  return useEntityList<BusItem, UseBusesOptions>({
    queryKey: ["buses"],
    path: "/buses",
    limit,
    offset,
    filters: options,
  });
}
