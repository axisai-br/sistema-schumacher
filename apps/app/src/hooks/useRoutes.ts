import { useEntityList } from "./shared/useEntityList";

export type RouteItem = {
  id: string;
  name: string;
  origin_city: string;
  destination_city: string;
  is_active: boolean;
  stop_count: number;
  configuration_status: "INCOMPLETE" | "READY" | "ACTIVE" | "SUSPENDED";
  missing_requirements: string[];
  has_linked_trips: boolean;
  duplicated_from_route_id?: string | null;
};

type UseRoutesOptions = {
  search?: string;
  status?: "active" | "inactive" | "all";
};

export function useRoutes(limit = 200, offset = 0, options: UseRoutesOptions = {}) {
  return useEntityList<RouteItem, UseRoutesOptions>({
    queryKey: ["routes"],
    path: "/routes",
    limit,
    offset,
    filters: {
      ...options,
      status: options.status ?? "all",
    },
  });
}
