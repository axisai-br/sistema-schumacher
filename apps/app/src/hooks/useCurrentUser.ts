import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../services/api";
import type { CurrentUser } from "../types/affiliate";

export function useCurrentUser() {
  return useQuery({
    queryKey: ["current-user"],
    queryFn: () => apiGet<CurrentUser>("/users/me"),
    staleTime: 30_000,
  });
}
