import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../services/api";
import { useEntityList } from "./shared/useEntityList";

export type BookingItem = {
  id: string;
  trip_id: string;
  status: string;
  passenger_name: string;
  passenger_phone?: string;
  passenger_email?: string;
  created_at?: string;
  seat_number: number;
  total_amount: number;
  deposit_amount: number;
  remainder_amount: number;
};

export type BookingPassenger = {
  id: string;
  booking_id: string;
  trip_id: string;
  name: string;
  document: string;
  document_type: string;
  phone: string;
  email: string;
  notes: string;
  is_lap_child: boolean;
  seat_id: string;
  board_stop_id: string;
  alight_stop_id: string;
  board_stop_order: number;
  alight_stop_order: number;
  fare_mode: string;
  fare_amount_calc: number;
  fare_amount_final: number;
  status: string;
  created_at: string;
};

export type BookingDetail = {
  booking: BookingItem;
  passenger: BookingPassenger;
  passengers: BookingPassenger[];
};

export function useBookingDetail(bookingId: string | null) {
  return useQuery({
    queryKey: ["booking", bookingId],
    queryFn: () => apiGet<BookingDetail>(`/bookings/${bookingId}`),
    enabled: !!bookingId,
  });
}

type UseBookingsOptions = {
  search?: string;
  status?: string;
  trip_id?: string;
};

export function useBookings(limit = 200, offset = 0, options: UseBookingsOptions = {}) {
  return useEntityList<BookingItem, UseBookingsOptions>({
    queryKey: ["bookings"],
    path: "/bookings",
    limit,
    offset,
    filters: options,
  });
}
