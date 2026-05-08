import { useMemo } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { apiGet } from "../services/api";
import { useEntityList } from "./shared/useEntityList";

export type TripItem = {
  id: string;
  route_id: string;
  bus_id: string;
  driver_id?: string;
  request_id?: string;
  departure_at: string;
  status: string;
  operational_status: string;
  seats_total: number;
  seats_available: number;
  estimated_km: number;
  dispatch_validated_at?: string;
};

export type TripDetailsTotals = {
  passengers_count: number;
  total_amount: number;
  paid_amount: number;
  due_amount: number;
};

export type TripDetailsSegment = {
  origin_stop_id: string;
  origin_name: string;
  destination_stop_id: string;
  destination_name: string;
  passengers_count: number;
  total_amount: number;
  paid_amount: number;
  due_amount: number;
};

export type TripDetailsPassenger = {
  passenger_id: string;
  booking_id: string;
  name: string;
  document: string;
  phone: string;
  seat_number: string;
  origin_stop_id: string;
  origin_name: string;
  destination_stop_id: string;
  destination_name: string;
  booking_status: string;
  payment_status: string;
  is_lap_child: boolean;
  total_amount: number;
  paid_amount: number;
  due_amount: number;
};

export type TripDetails = {
  trip: TripItem;
  totals: TripDetailsTotals;
  segments: TripDetailsSegment[];
  passengers: TripDetailsPassenger[];
};

type UseTripsOptions = {
  search?: string;
  status?: string;
};

export function useTrips(limit = 200, offset = 0, options: UseTripsOptions = {}) {
  return useEntityList<TripItem, UseTripsOptions>({
    queryKey: ["trips"],
    path: "/trips",
    limit,
    offset,
    filters: options,
  });
}

export function useTripDetails(tripId: string | null) {
  return useQuery({
    queryKey: ["trip-details", tripId],
    queryFn: () => apiGet<TripDetails>(`/trips/${tripId}/details`),
    enabled: Boolean(tripId),
  });
}

export function useTripDetailsBatch(tripIds: string[]) {
  const uniqueTripIds = useMemo(
    () => Array.from(new Set(tripIds.map((id) => String(id ?? "").trim()).filter(Boolean))),
    [tripIds]
  );

  const queries = useQueries({
    queries: uniqueTripIds.map((tripId) => ({
      queryKey: ["trip-details", tripId],
      queryFn: () => apiGet<TripDetails>(`/trips/${tripId}/details`),
    })),
  });

  const data = useMemo(
    () =>
      queries
        .map((query) => query.data)
        .filter((item): item is TripDetails => Boolean(item)),
    [queries]
  );

  const error = useMemo(() => {
    const failedQuery = queries.find((query) => query.isError);
    return failedQuery ? ((failedQuery.error as Error) ?? null) : null;
  }, [queries]);

  return {
    data,
    error,
    isLoading: queries.some((query) => query.isLoading),
    isFetching: queries.some((query) => query.isFetching),
  };
}
