import { useMemo } from "react";
import { formatDateTime, formatShortId } from "../utils/format";

type RouteLike = {
  id: string;
  origin_city: string;
  destination_city: string;
};

type TripLike = {
  id: string;
  route_id: string;
  departure_at: string;
};

type TripOption = {
  value: string;
  label: string;
};

type UseTripRouteLookupOptions = {
  separator?: string;
};

export function useTripRouteLookup<TRoute extends RouteLike, TTrip extends TripLike>(
  routes: TRoute[],
  trips: TTrip[],
  options: UseTripRouteLookupOptions = {}
) {
  const separator = options.separator ?? " -> ";

  const routeMap = useMemo(
    () => new Map(routes.map((route) => [route.id, route])),
    [routes]
  );

  const tripMap = useMemo(
    () => new Map(trips.map((trip) => [trip.id, trip])),
    [trips]
  );

  const tripLabel = (tripId: string) => {
    const trip = tripMap.get(tripId);
    if (!trip) return formatShortId(tripId);

    const route = routeMap.get(trip.route_id);
    const routeLabel = route
      ? `${route.origin_city}${separator}${route.destination_city}`
      : formatShortId(trip.route_id);

    return `${routeLabel} - ${formatDateTime(trip.departure_at)}`;
  };

  const tripOptions = useMemo<TripOption[]>(() => {
    return trips
      .slice()
      .sort((a, b) => new Date(b.departure_at).getTime() - new Date(a.departure_at).getTime())
      .map((trip) => {
        const route = routeMap.get(trip.route_id);
        const routeLabel = route
          ? `${route.origin_city}${separator}${route.destination_city}`
          : formatShortId(trip.route_id);

        return {
          value: trip.id,
          label: `${routeLabel} - ${formatDateTime(trip.departure_at)}`,
        };
      });
  }, [routeMap, separator, trips]);

  return {
    routeMap,
    tripMap,
    tripLabel,
    tripOptions,
  };
}
