import { useEntityList } from "./shared/useEntityList";

export type DriverItem = {
  id: string;
  name: string;
};

type UseDriversOptions = {
  search?: string;
};

export function useDrivers(limit = 200, offset = 0, options: UseDriversOptions = {}) {
  return useEntityList<DriverItem, UseDriversOptions>({
    queryKey: ["drivers"],
    path: "/drivers",
    limit,
    offset,
    filters: options,
  });
}
