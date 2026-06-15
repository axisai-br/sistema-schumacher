import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import InlineAlert from "../../components/InlineAlert";
import PageHeader from "../../components/PageHeader";
import StatusBadge from "../../components/StatusBadge";
import { useBookings, useBookingDetail, type BookingPassenger } from "../../hooks/useBookings";
import Drawer from "../../components/overlay/Drawer";
import { useRoutes } from "../../hooks/useRoutes";
import { useTrips } from "../../hooks/useTrips";
import { apiGet, apiPatch, apiPost } from "../../services/api";
import type { PassengerSuggestion, PreferredSeatOption } from "../../types/checkout";
import useToast from "../../hooks/useToast";
import { formatBirthDate, formatCurrency, formatDateTime, formatShortId } from "../../utils/format";
import BookingList from "./BookingList";
import { BookingFormProvider, type BookingFormState, type BookingStep } from "./BookingFormContext";
import BookingStepForm from "./BookingStepForm";

type TripItem = {
  id: string;
  route_id: string;
  bus_id: string;
  departure_at: string;
  seats_total?: number;
  seats_available?: number;
};
type RouteItem = { id: string; origin_city: string; destination_city: string };
type SeatItem = { id: string; seat_number: number; is_active: boolean; is_taken: boolean };

type TripStop = {
  id: string;
  route_stop_id: string;
  city: string;
  stop_order: number;
  arrive_at?: string | null;
  depart_at?: string | null;
};

type QuoteResult = {
  base_amount: number;
  calc_amount: number;
  final_amount: number;
  currency: string;
  fare_mode: string;
  occupancy_ratio: number;
};

function passengerIdentityFields(passenger: BookingPassenger) {
  return [
    passenger.cpf ? ["CPF", passenger.cpf] : null,
    passenger.rg ? ["RG", passenger.rg] : null,
    passenger.cnh ? ["CNH", passenger.cnh] : null,
    passenger.birth_date ? ["Nascimento", formatBirthDate(passenger.birth_date)] : null,
    passenger.birth_certificate_number ? ["Matrícula", passenger.birth_certificate_number] : null,
    passenger.birth_city ? ["Naturalidade", passenger.birth_city] : null,
  ].filter(Boolean) as [string, string][];
}

type BookingItem = {
  id: string;
  trip_id: string;
  status: string;
  created_at?: string;
  passenger_name: string;
  passenger_document?: string;
  passenger_phone: string;
  passenger_email: string;
  seat_number: number;
  total_amount: number;
  deposit_amount: number;
  remainder_amount: number;
};

type CheckoutResult = {
  booking: { booking: { id: string; status: string } };
  payment: { id: string; status: string; amount: number; provider_ref?: string };
  provider_raw?: any;
  checkout_url?: string | null;
  pix_code?: string | null;
};

type BookingCreateResult = {
  booking: { id: string; status: string };
};

type PaymentCreateResult = {
  payment: { id: string; status: string; amount: number; provider_ref?: string };
  provider_raw?: any;
};

type PaymentSyncResponse = {
  payment: { id: string; status: string; amount: number; provider_ref?: string };
  booking_status: string;
  synced: boolean;
};

const initialForm: BookingFormState = {
  trip_id: "",
  seat_id: "",
  board_stop_id: "",
  alight_stop_id: "",
  fare_mode: "AUTO",
  name: "",
  document: "",
  phone: "",
  email: "",
  total_amount: 0,
  deposit_amount: 0,
  remainder_amount: 0,
  payment_method: "PIX",
  payment_description: "Passagem",
  payment_notes: "",
};

type LastBookingSnapshot = BookingFormState & {
  saved_at: string;
};

const LAST_BOOKING_STORAGE_KEY = "booking.last_success";

function normalizeToken(value: string | null | undefined): string {
  return (value ?? "").trim().toLowerCase();
}

function readSnapshotFromStorage(): LastBookingSnapshot | null {
  if (typeof window === "undefined") return null;
  const raw = window.localStorage.getItem(LAST_BOOKING_STORAGE_KEY);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as LastBookingSnapshot;
    if (!parsed?.trip_id || !parsed?.name) return null;
    return parsed;
  } catch {
    return null;
  }
}

function saveSnapshotToStorage(snapshot: LastBookingSnapshot) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(LAST_BOOKING_STORAGE_KEY, JSON.stringify(snapshot));
  } catch {
    // no-op for blocked storage
  }
}

function extractCheckoutUrl(providerRaw: any): string | null {
  if (!providerRaw || typeof providerRaw !== "object") return null;
  if (typeof providerRaw.url === "string" && providerRaw.url.trim() !== "") return providerRaw.url;
  const nested = providerRaw.data?.url;
  if (typeof nested === "string" && nested.trim() !== "") return nested;
  return null;
}

function extractPixCode(providerRaw: any): string | null {
  if (!providerRaw || typeof providerRaw !== "object") return null;
  const code = providerRaw.data?.pixQrCode ?? providerRaw.pixQrCode;
  if (typeof code === "string" && code.trim() !== "") return code;
  return null;
}

export default function Bookings() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const checkoutV2Enabled = (import.meta.env.VITE_BOOKING_CHECKOUT_V2 ?? "true").toLowerCase() !== "false";

  const tripsQuery = useTrips(200, 0);
  const routesQuery = useRoutes(200, 0);
  const trips = (tripsQuery.data as TripItem[] | undefined) ?? [];
  const routes = (routesQuery.data as RouteItem[] | undefined) ?? [];
  const [seats, setSeats] = useState<SeatItem[]>([]);
  const [stops, setStops] = useState<TripStop[]>([]);
  const [quote, setQuote] = useState<QuoteResult | null>(null);
  const [quoteLoading, setQuoteLoading] = useState(false);
  const [quoteWarning, setQuoteWarning] = useState<string | null>(null);
  const [quoteRefreshKey, setQuoteRefreshKey] = useState(0);
  const [actionError, setActionError] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [query, setQuery] = useState("");
  const [selectedBookingId, setSelectedBookingId] = useState<string | null>(null);
  const bookingDetailQuery = useBookingDetail(selectedBookingId);
  const detailTripId = bookingDetailQuery.data?.booking.trip_id ?? null;
  const [detailTripStops, setDetailTripStops] = useState<TripStop[]>([]);
  useEffect(() => {
    if (!detailTripId) { setDetailTripStops([]); return; }
    apiGet<TripStop[]>(`/trips/${detailTripId}/stops`).then(setDetailTripStops).catch(() => {});
  }, [detailTripId]);
  const stopName = (stopId: string) => detailTripStops.find((s) => s.id === stopId)?.city ?? stopId;
  const pageSize = 50;
  const bookingsQuery = useBookings(pageSize, page * pageSize);
  const bookingsHistoryQuery = useBookings(500, 0);
  const bookings = (bookingsQuery.data as BookingItem[] | undefined) ?? [];
  const bookingsHistory = (bookingsHistoryQuery.data as BookingItem[] | undefined) ?? [];
  const loading = bookingsQuery.isLoading;

  const [form, setForm] = useState<BookingFormState>(initialForm);
  const [activeStep, setActiveStep] = useState<BookingStep>("trip_passenger");
  const [checkoutLoading, setCheckoutLoading] = useState(false);
  const [syncLoading, setSyncLoading] = useState(false);
  const [checkoutResult, setCheckoutResult] = useState<CheckoutResult | null>(null);
  const [lastSnapshot, setLastSnapshot] = useState<LastBookingSnapshot | null>(() =>
    readSnapshotFromStorage()
  );

  const loadError = useMemo(() => {
    const sourceError =
      (bookingsQuery.error as Error | undefined) ||
      (bookingsHistoryQuery.error as Error | undefined) ||
      (tripsQuery.error as Error | undefined) ||
      (routesQuery.error as Error | undefined);
    return sourceError?.message || null;
  }, [bookingsHistoryQuery.error, bookingsQuery.error, tripsQuery.error, routesQuery.error]);

  useEffect(() => {
    if (!form.trip_id) {
      setSeats([]);
      setStops([]);
      setQuote(null);
      return;
    }
    apiGet<TripStop[]>(`/trips/${form.trip_id}/stops`)
      .then(setStops)
      .catch((err) => setActionError(err.message || "Erro ao carregar paradas"));
  }, [form.trip_id]);

  useEffect(() => {
    if (!form.trip_id) {
      setSeats([]);
      return;
    }
    const params =
      form.board_stop_id && form.alight_stop_id
        ? `?board_stop_id=${form.board_stop_id}&alight_stop_id=${form.alight_stop_id}`
        : "";
    apiGet<SeatItem[]>(`/trips/${form.trip_id}/seats${params}`)
      .then(setSeats)
      .catch((err) => setActionError(err.message || "Erro ao carregar poltronas"));
  }, [form.trip_id, form.board_stop_id, form.alight_stop_id]);

  useEffect(() => {
    if (!form.trip_id || !form.board_stop_id || !form.alight_stop_id) {
      setQuote(null);
      setQuoteWarning(null);
      return;
    }
    if (form.fare_mode === "MANUAL") {
      setQuote(null);
      setQuoteWarning(null);
      return;
    }
    setQuoteWarning(null);
    setActionError(null);
    setQuoteLoading(true);
    let cancelled = false;
    apiPost<QuoteResult>("/pricing/quote", {
      trip_id: form.trip_id,
      board_stop_id: form.board_stop_id,
      alight_stop_id: form.alight_stop_id,
      fare_mode: form.fare_mode,
    })
      .then((data) => {
        if (cancelled) return;
        setQuote(data);
        setForm((prev) => ({ ...prev, total_amount: data.final_amount }));
      })
      .catch((err) => {
        if (cancelled) return;
        const message = err?.message || "Erro ao calcular tarifa";
        const code = typeof err?.code === "string" ? err.code.toUpperCase() : "";
        if (code === "FARE_NOT_FOUND" || message === "segment fare not found") {
          setQuote(null);
          setForm((prev) => ({ ...prev, total_amount: 0 }));
          setQuoteWarning("Tarifa do trecho nao encontrada. Informe o valor manualmente.");
          setActionError(null);
          return;
        }
        if (code === "STOP_NOT_FOUND" || code === "INVALID_STOPS") {
          setQuote(null);
          setForm((prev) => ({ ...prev, total_amount: 0 }));
          setQuoteWarning("Trecho invalido para esta viagem. Revise embarque e desembarque.");
          setActionError(null);
          return;
        }
        setQuote(null);
        setActionError(message);
      })
      .finally(() => {
        if (cancelled) return;
        setQuoteLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [form.trip_id, form.board_stop_id, form.alight_stop_id, form.fare_mode, quoteRefreshKey]);

  useEffect(() => {
    if (activeStep !== "payment") return;
    if (form.fare_mode === "MANUAL") return;
    if (!form.trip_id || !form.board_stop_id || !form.alight_stop_id) return;
    if (Number(form.total_amount) > 0) return;
    setQuoteRefreshKey((prev) => prev + 1);
  }, [
    activeStep,
    form.trip_id,
    form.board_stop_id,
    form.alight_stop_id,
    form.fare_mode,
    form.total_amount,
  ]);

  const minInitialAmount = useMemo(() => {
    if (!form.total_amount || form.total_amount <= 0) return 0;
    return Math.round(Math.max(form.total_amount * 0.3, 0.01) * 100) / 100;
  }, [form.total_amount]);

  useEffect(() => {
    const next = Math.max((Number(form.total_amount) || 0) - (Number(form.deposit_amount) || 0), 0);
    if (next !== form.remainder_amount) {
      setForm((prev) => ({ ...prev, remainder_amount: next }));
    }
  }, [form.total_amount, form.deposit_amount, form.remainder_amount]);

  const availableSeats = seats.filter((s) => s.is_active && !s.is_taken);
  const boardStop = stops.find((stop) => stop.id === form.board_stop_id);
  const alightStops = boardStop
    ? stops.filter((stop) => stop.stop_order > boardStop.stop_order)
    : stops;
  const alightStop = stops.find((stop) => stop.id === form.alight_stop_id);
  const selectedSeat = seats.find((seat) => seat.id === form.seat_id);
  const seatStillAvailable = availableSeats.some((seat) => seat.id === form.seat_id);

  const passengerSuggestions = useMemo<PassengerSuggestion[]>(() => {
    const grouped = new Map<
      string,
      {
        id: string;
        name: string;
        document: string;
        phone: string;
        email: string;
        lastBookingAt: string | null;
        seatFrequency: Map<number, number>;
      }
    >();

    for (const booking of bookingsHistory) {
      const name = booking.passenger_name?.trim() ?? "";
      if (!name) continue;
      const phone = normalizeToken(booking.passenger_phone);
      const email = normalizeToken(booking.passenger_email);
      const document = normalizeToken(booking.passenger_document);
      const fallbackName = normalizeToken(name);
      const identity = document || email || phone || `name:${fallbackName}`;
      if (!identity) continue;

      const existing = grouped.get(identity);
      if (!existing) {
        const seatFrequency = new Map<number, number>();
        if (booking.seat_number > 0) {
          seatFrequency.set(booking.seat_number, 1);
        }
        grouped.set(identity, {
          id: identity,
          name,
          document: booking.passenger_document ?? "",
          phone: booking.passenger_phone ?? "",
          email: booking.passenger_email ?? "",
          lastBookingAt: booking.created_at ?? null,
          seatFrequency,
        });
        continue;
      }

      if (!existing.phone && booking.passenger_phone) existing.phone = booking.passenger_phone;
      if (!existing.email && booking.passenger_email) existing.email = booking.passenger_email;
      if (!existing.document && booking.passenger_document) existing.document = booking.passenger_document;

      if (booking.created_at) {
        const prev = existing.lastBookingAt ? new Date(existing.lastBookingAt).getTime() : 0;
        const next = new Date(booking.created_at).getTime();
        if (next > prev) existing.lastBookingAt = booking.created_at;
      }

      if (booking.seat_number > 0) {
        existing.seatFrequency.set(
          booking.seat_number,
          (existing.seatFrequency.get(booking.seat_number) ?? 0) + 1
        );
      }
    }

    return Array.from(grouped.values())
      .map((item) => ({
        id: item.id,
        name: item.name,
        document: item.document,
        phone: item.phone,
        email: item.email,
        lastBookingAt: item.lastBookingAt,
        preferredSeats: Array.from(item.seatFrequency.entries())
          .sort((a, b) => b[1] - a[1])
          .slice(0, 5)
          .map(([seatNumber]) => seatNumber),
      }))
      .sort((a, b) => {
        const aDate = a.lastBookingAt ? new Date(a.lastBookingAt).getTime() : 0;
        const bDate = b.lastBookingAt ? new Date(b.lastBookingAt).getTime() : 0;
        return bDate - aDate;
      });
  }, [bookingsHistory]);

  const selectedPassengerSuggestion = useMemo(() => {
    const name = normalizeToken(form.name);
    const document = normalizeToken(form.document);
    const phone = normalizeToken(form.phone);
    const email = normalizeToken(form.email);
    if (!name && !document && !phone && !email) return null;
    return (
      passengerSuggestions.find((item) => {
        const suggestionName = normalizeToken(item.name);
        const suggestionDocument = normalizeToken(item.document);
        const suggestionPhone = normalizeToken(item.phone);
        const suggestionEmail = normalizeToken(item.email);
        if (document && suggestionDocument && suggestionDocument === document) return true;
        if (email && suggestionEmail && suggestionEmail === email) return true;
        if (phone && suggestionPhone && suggestionPhone === phone) return true;
        return Boolean(name && suggestionName && suggestionName === name);
      }) ?? null
    );
  }, [form.document, form.email, form.name, form.phone, passengerSuggestions]);

  const preferredSeatOptions = useMemo<PreferredSeatOption[]>(() => {
    if (!selectedPassengerSuggestion) return [];
    const availableByNumber = new Map(availableSeats.map((seat) => [seat.seat_number, seat]));
    return selectedPassengerSuggestion.preferredSeats
      .map((seatNumber, index) => {
        const seat = availableByNumber.get(seatNumber);
        if (!seat) return null;
        return {
          seat_id: seat.id,
          seat_number: seat.seat_number,
          score: selectedPassengerSuggestion.preferredSeats.length - index,
          source: "history" as const,
        };
      })
      .filter((item): item is PreferredSeatOption => Boolean(item));
  }, [availableSeats, selectedPassengerSuggestion]);

  const paymentError = useMemo(() => {
    if (form.deposit_amount > form.total_amount) return "Valor inicial nao pode ser maior que o total.";
    if (form.deposit_amount <= 0) return "Informe um valor inicial maior que zero.";
    if (form.total_amount > 0 && form.deposit_amount < minInitialAmount) {
      return `O valor inicial deve ser no minimo ${formatCurrency(minInitialAmount)}.`;
    }
    return null;
  }, [form.deposit_amount, form.total_amount, minInitialAmount]);

  const stepTripComplete = Boolean(
    form.trip_id && form.board_stop_id && form.alight_stop_id && form.seat_id && seatStillAvailable
  );
  const stepPassengerComplete = Boolean(form.name.trim());
  const stepTripPassengerComplete = stepTripComplete && stepPassengerComplete;
  const stepPaymentReady =
    stepTripPassengerComplete && !paymentError && (!quoteWarning || form.fare_mode === "MANUAL");

  useEffect(() => {
    if (activeStep === "payment" && !stepTripPassengerComplete) {
      setActiveStep("trip_passenger");
    }
  }, [activeStep, stepTripPassengerComplete]);

  const filtered = useMemo(() => {
    const term = query.trim().toLowerCase();
    if (!term) return bookings;
    return bookings.filter((b) =>
      [b.passenger_name, b.passenger_email, String(b.seat_number), b.status].some((value) =>
        value?.toLowerCase().includes(term)
      )
    );
  }, [bookings, query]);

  const routeLabel = (routeId: string) => {
    const route = routes.find((r) => r.id === routeId);
    return route ? `${route.origin_city} -> ${route.destination_city}` : formatShortId(routeId);
  };

  const tripLabel = (tripId: string) => {
    const trip = trips.find((t) => t.id === tripId);
    if (!trip) return formatShortId(tripId);
    const seatsTotal = Number.isFinite(trip.seats_total) ? Math.max(0, Number(trip.seats_total)) : 0;
    const seatsAvailable = Number.isFinite(trip.seats_available)
      ? Math.max(0, Number(trip.seats_available))
      : 0;
    const capacitySuffix = seatsTotal > 0 ? ` - vagas ${seatsAvailable}/${seatsTotal}` : "";
    return `${routeLabel(trip.route_id)} - ${formatDateTime(trip.departure_at)}${capacitySuffix}`;
  };

  const tripPassengerSummary = stepTripPassengerComplete
    ? `${form.name.trim()} em ${tripLabel(form.trip_id)}`
    : "Selecione viagem, trecho, poltrona e passageiro.";

  const paymentSummary = !stepTripPassengerComplete
    ? "Complete viagem e passageiro para continuar."
    : `Entrada ${formatCurrency(form.deposit_amount || 0)} de ${formatCurrency(form.total_amount || 0)}`;

  const summaryItems = [
    {
      label: "Viagem",
      value: form.trip_id ? tripLabel(form.trip_id) : "Selecione a viagem",
    },
    {
      label: "Trecho",
      value:
        boardStop && alightStop
          ? `${boardStop.city} -> ${alightStop.city}`
          : "Defina as paradas",
    },
    {
      label: "Poltrona",
      value: selectedSeat ? `Poltrona ${selectedSeat.seat_number}` : "Selecione a poltrona",
    },
    {
      label: "Passageiro",
      value: form.name ? form.name : "Informe o passageiro",
    },
  ];

  const summaryTotals = [
    { label: "Total", value: formatCurrency(form.total_amount || 0) },
    { label: "Entrada", value: formatCurrency(form.deposit_amount || 0) },
    { label: "Saldo", value: formatCurrency(form.remainder_amount || 0) },
  ];

  const statusTone = (status: string): "neutral" | "info" | "success" | "warning" | "danger" => {
    if (status === "PAID" || status === "CONFIRMED") return "success";
    if (status === "PENDING") return "warning";
    if (status === "FAILED") return "danger";
    return "neutral";
  };

  const handleApplyPassengerSuggestion = (suggestion: PassengerSuggestion) => {
    setForm((prev) => ({
      ...prev,
      name: suggestion.name || prev.name,
      document: suggestion.document || prev.document,
      phone: suggestion.phone || prev.phone,
      email: suggestion.email || prev.email,
    }));
  };

  const handleSelectPreferredSeat = (option: PreferredSeatOption) => {
    setForm((prev) => ({
      ...prev,
      seat_id: option.seat_id,
    }));
  };

  const handleRepeatLastReservation = () => {
    if (!lastSnapshot) return;
    const { saved_at: _savedAt, ...snapshotForm } = lastSnapshot;
    setForm(snapshotForm);
    setActiveStep("trip_passenger");
    setCheckoutResult(null);
    setActionError(null);
    toast.success("Ultima reserva carregada. Revise os dados antes de concluir.");
  };

  const handleCheckout = async () => {
    setActionError(null);
    if (!stepPaymentReady) return;
    try {
      setCheckoutLoading(true);
      const automatic = form.payment_method === "PIX" || form.payment_method === "CARD";
      const payload = {
        trip_id: form.trip_id,
        seat_id: form.seat_id,
        board_stop_id: form.board_stop_id,
        alight_stop_id: form.alight_stop_id,
        fare_mode: form.fare_mode,
        fare_amount_final: form.fare_mode === "MANUAL" ? Number(form.total_amount) : undefined,
        passenger: {
          name: form.name,
          document: form.document,
          phone: form.phone,
          email: form.email,
        },
        total_amount: Number(form.total_amount),
        deposit_amount: Number(form.deposit_amount),
        remainder_amount: Number(form.remainder_amount),
        initial_payment: {
          method: form.payment_method,
          amount: Number(form.deposit_amount),
          description: form.payment_description || undefined,
          notes: form.payment_notes || undefined,
          customer: automatic
            ? {
                name: form.name || undefined,
                email: form.email || undefined,
                phone: form.phone || undefined,
                document: form.document || undefined,
              }
            : undefined,
        },
      };

      let result: CheckoutResult;

      if (checkoutV2Enabled) {
        result = await apiPost<CheckoutResult>("/bookings/checkout", payload);
      } else {
        const createdBooking = await apiPost<BookingCreateResult>("/bookings", {
          trip_id: payload.trip_id,
          seat_id: payload.seat_id,
          board_stop_id: payload.board_stop_id,
          alight_stop_id: payload.alight_stop_id,
          fare_mode: payload.fare_mode,
          fare_amount_final: payload.fare_amount_final,
          passenger: payload.passenger,
          total_amount: payload.total_amount,
          deposit_amount: payload.deposit_amount,
          remainder_amount: payload.remainder_amount,
        });
        const createdBookingID = createdBooking.booking.id;

        let createdPayment: PaymentCreateResult;
        try {
          if (automatic) {
            createdPayment = await apiPost<PaymentCreateResult>("/payments", {
              booking_id: createdBookingID,
              amount: payload.initial_payment.amount,
              method: payload.initial_payment.method,
              description: payload.initial_payment.description,
              customer: payload.initial_payment.customer,
            });
          } else {
            createdPayment = await apiPost<PaymentCreateResult>("/payments/manual", {
              booking_id: createdBookingID,
              amount: payload.initial_payment.amount,
              method: payload.initial_payment.method,
              notes: payload.initial_payment.notes,
            });
          }
        } catch {
          let compensationFailed = false;
          try {
            await apiPatch(`/bookings/${createdBookingID}`, { status: "CANCELLED" });
          } catch (cancelErr) {
            compensationFailed = true;
            console.error("checkout fallback compensation failed", cancelErr);
            toast.error("Falha ao cancelar reserva apos erro de pagamento.");
          } finally {
            await Promise.all([
              queryClient.invalidateQueries({ queryKey: ["bookings"] }),
              queryClient.invalidateQueries({ queryKey: ["payments"] }),
              queryClient.invalidateQueries({ queryKey: ["trips"] }),
            ]);
          }

          if (compensationFailed) {
            setActionError(
              `Pagamento inicial falhou e nao foi possivel cancelar automaticamente a reserva ${createdBookingID}. Cancele manualmente.`
            );
          } else {
            setActionError("Pagamento inicial falhou e a reserva foi cancelada automaticamente.");
          }
          return;
        }

        result = {
          booking: createdBooking,
          payment: createdPayment.payment,
          provider_raw: createdPayment.provider_raw,
          checkout_url: extractCheckoutUrl(createdPayment.provider_raw),
          pix_code: extractPixCode(createdPayment.provider_raw),
        };
      }

      setCheckoutResult(result);
      const snapshot: LastBookingSnapshot = {
        ...form,
        saved_at: new Date().toISOString(),
      };
      setLastSnapshot(snapshot);
      saveSnapshotToStorage(snapshot);
      setPage(0);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["bookings"] }),
        queryClient.invalidateQueries({ queryKey: ["payments"] }),
        queryClient.invalidateQueries({ queryKey: ["reports"] }),
        queryClient.invalidateQueries({ queryKey: ["trips"] }),
      ]);
      toast.success("Reserva concluida com pagamento inicial.");
    } catch (err: any) {
      setActionError(err.message || "Erro ao concluir checkout da reserva");
    } finally {
      setCheckoutLoading(false);
    }
  };

  const handleSync = async () => {
    if (!checkoutResult?.payment.id) return;
    try {
      setSyncLoading(true);
      const synced = await apiPost<PaymentSyncResponse>(`/payments/${checkoutResult.payment.id}/sync`, {});
      setCheckoutResult((prev) =>
        prev
          ? {
              ...prev,
              booking: {
                ...prev.booking,
                booking: {
                  ...prev.booking.booking,
                  status: synced.booking_status,
                },
              },
              payment: {
                ...prev.payment,
                status: synced.payment.status,
              },
            }
          : prev
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["bookings"] }),
        queryClient.invalidateQueries({ queryKey: ["payments"] }),
        queryClient.invalidateQueries({ queryKey: ["reports"] }),
        queryClient.invalidateQueries({ queryKey: ["trips"] }),
      ]);
      toast.success("Status atualizado.");
    } catch (err: any) {
      setActionError(err.message || "Erro ao sincronizar status");
    } finally {
      setSyncLoading(false);
    }
  };

  const handleNewCheckout = () => {
    setForm({ ...initialForm });
    setActiveStep("trip_passenger");
    setCheckoutResult(null);
    setActionError(null);
  };

  return (
    <section className="page">
      <PageHeader
        eyebrow="VENDAS"
        title="🎫 Reservas"
        subtitle="Fluxo integrado com pagamento inicial e validacao automatica."
        meta={<StatusBadge tone="info" label={`${bookings.length} reservas`} />}
      />

      <BookingFormProvider value={{ form, setForm, activeStep, setActiveStep }}>
        <div className="section" id="booking-form">
          <div className="section-header">
            <div className="section-title">Nova reserva</div>
            <div className="toolbar-group">
              {lastSnapshot && !checkoutResult ? (
                <button className="button secondary sm" type="button" onClick={handleRepeatLastReservation}>
                  Repetir ultima reserva
                </button>
              ) : null}
              {checkoutResult ? (
                <button className="button secondary sm" type="button" onClick={handleNewCheckout}>
                  Nova reserva
                </button>
              ) : null}
            </div>
          </div>
          <BookingStepForm
            trips={trips}
            stops={stops}
            alightStops={alightStops}
            availableSeats={availableSeats}
            passengerSuggestions={passengerSuggestions}
            preferredSeatOptions={preferredSeatOptions}
            tripLabel={tripLabel}
            tripPassengerSummary={tripPassengerSummary}
            paymentSummary={paymentSummary}
            stepTripPassengerComplete={stepTripPassengerComplete}
            canSubmitCheckout={stepPaymentReady}
            minInitialAmount={minInitialAmount}
            paymentError={paymentError}
            checkoutLoading={checkoutLoading || quoteLoading}
            checkoutResult={checkoutResult}
            syncLoading={syncLoading}
            onSubmitCheckout={handleCheckout}
            onSync={handleSync}
            onApplyPassengerSuggestion={handleApplyPassengerSuggestion}
            onSelectPreferredSeat={handleSelectPreferredSeat}
            statusTone={statusTone}
            summaryItems={summaryItems}
            summaryTotals={summaryTotals}
          />
          {form.seat_id && !seatStillAvailable ? (
            <InlineAlert tone="warning">
              A poltrona escolhida nao esta mais disponivel para este trecho. Selecione outra para continuar.
            </InlineAlert>
          ) : null}
          {quoteWarning ? <InlineAlert tone="warning">{quoteWarning}</InlineAlert> : null}
          {quote && form.fare_mode !== "MANUAL" ? (
            <InlineAlert tone="info">Tarifa automatica aplicada: {formatCurrency(quote.final_amount)}.</InlineAlert>
          ) : null}
        </div>
      </BookingFormProvider>

      {loadError ? <InlineAlert tone="error">{loadError}</InlineAlert> : null}
      {actionError ? <InlineAlert tone="error">{actionError}</InlineAlert> : null}

      <BookingList
        bookings={filtered}
        loading={loading}
        query={query}
        onQueryChange={setQuery}
        page={page}
        pageSize={pageSize}
        onPageChange={setPage}
        tripLabel={tripLabel}
        onRowClick={(b) => setSelectedBookingId(b.id)}
        selectedBookingId={selectedBookingId ?? undefined}
      />

      <Drawer
        open={!!selectedBookingId}
        title="Detalhe da reserva"
        description={selectedBookingId ? `Reserva ${formatShortId(selectedBookingId)}` : undefined}
        onClose={() => setSelectedBookingId(null)}
      >
        {bookingDetailQuery.isLoading ? (
          <div style={{ padding: "16px" }}>Carregando...</div>
        ) : bookingDetailQuery.error ? (
          <InlineAlert tone="error">Erro ao carregar reserva.</InlineAlert>
        ) : bookingDetailQuery.data ? (() => {
          const { booking, passengers } = bookingDetailQuery.data;
          const passenger = passengers[0];
          return (
            <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
              <div className="card">
                <div className="section-title" style={{ marginBottom: "8px" }}>Situação</div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "8px" }}>
                  <div><span style={{ opacity: 0.6 }}>Status</span><br />{booking.status}</div>
                  <div><span style={{ opacity: 0.6 }}>Total</span><br />{formatCurrency(booking.total_amount)}</div>
                  <div><span style={{ opacity: 0.6 }}>Pago</span><br />{formatCurrency(booking.deposit_amount)}</div>
                  <div><span style={{ opacity: 0.6 }}>Pendente</span><br />{formatCurrency(booking.remainder_amount)}</div>
                </div>
              </div>
              {passengers.length > 0 ? (
                <div className="card">
                  <div className="section-title" style={{ marginBottom: "8px" }}>
                    {passengers.length > 1 ? "Passageiros" : "Passageiro"}
                  </div>
                  <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
                    {passengers.map((item, index) => {
                      const identityFields = passengerIdentityFields(item);
                      return (
                        <div key={item.id || `${item.name}-${index}`} style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                          {passengers.length > 1 ? <strong>{index + 1}. {item.name || "-"}</strong> : null}
                          {passengers.length <= 1 ? <div><span style={{ opacity: 0.6 }}>Nome</span>: {item.name || "-"}</div> : null}
                          <div><span style={{ opacity: 0.6 }}>Documento</span>: {item.document_type ? `${item.document_type} ` : ""}{item.document || "-"}</div>
                          {identityFields.map(([label, value]) => (
                            <div key={label}><span style={{ opacity: 0.6 }}>{label}</span>: {value}</div>
                          ))}
                          <div><span style={{ opacity: 0.6 }}>Telefone</span>: {item.phone || "-"}</div>
                          <div><span style={{ opacity: 0.6 }}>E-mail</span>: {item.email || "-"}</div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : null}
              {passengers.length > 0 ? (
                <div className="card">
                  <div className="section-title" style={{ marginBottom: "8px" }}>Viagem</div>
                  <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                    <div><span style={{ opacity: 0.6 }}>Viagem</span>: {tripLabel(booking.trip_id)}</div>
                    <div><span style={{ opacity: 0.6 }}>Embarque</span>: {passenger?.board_stop_id ? stopName(passenger.board_stop_id) : "-"}</div>
                    <div><span style={{ opacity: 0.6 }}>Desembarque</span>: {passenger?.alight_stop_id ? stopName(passenger.alight_stop_id) : "-"}</div>
                  </div>
                </div>
              ) : null}
            </div>
          );
        })() : null}
      </Drawer>
    </section>
  );
}
