import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useBookings } from "../useBookings";
import { usePayments } from "../usePayments";
import { useTrips } from "../useTrips";
import { createTestQueryClient, withQueryClient } from "../../test/queryTestUtils";
import { apiGet } from "../../services/api";

vi.mock("../../services/api", () => ({
  apiGet: vi.fn(),
}));

type PaginatedCase = {
  name: string;
  hook: () => ReturnType<typeof useBookings>;
  expectedUrl: string;
  response: Record<string, unknown>[];
  assert: (result: unknown[]) => void;
};

const cases: PaginatedCase[] = [
  {
    name: "bookings",
    hook: () => useBookings(25, 50),
    expectedUrl: "/bookings?limit=25&offset=50",
    response: [
      {
        id: "booking-1",
        trip_id: "trip-1",
        status: "PENDING",
        passenger_name: "Joao Silva",
        seat_number: 4,
        total_amount: 120,
        deposit_amount: 40,
        remainder_amount: 80,
      },
    ],
    assert: (result) => {
      expect(result[0]?.passenger_name).toBe("Joao Silva");
    },
  },
  {
    name: "payments",
    hook: () => usePayments(15, 30),
    expectedUrl: "/payments?limit=15&offset=30",
    response: [
      {
        id: "payment-1",
        booking_id: "booking-1",
        amount: 40,
        method: "PIX",
        status: "PENDING",
        created_at: "2026-02-06T10:00:00Z",
      },
    ],
    assert: (result) => {
      expect(result[0]?.method).toBe("PIX");
    },
  },
  {
    name: "trips",
    hook: () => useTrips(10, 20),
    expectedUrl: "/trips?limit=10&offset=20",
    response: [
      {
        id: "trip-1",
        route_id: "route-1",
        bus_id: "bus-1",
        departure_at: "2026-02-06T10:00:00Z",
        status: "SCHEDULED",
      },
    ],
    assert: (result) => {
      expect(result[0]?.id).toBe("trip-1");
    },
  },
];

describe("paginated data hooks", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it.each(cases)("loads $name with pagination", async ({ hook, expectedUrl, response, assert }) => {
    vi.mocked(apiGet).mockResolvedValueOnce(response as never);

    const client = createTestQueryClient();
    const { result } = renderHook(hook, {
      wrapper: withQueryClient(client),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(apiGet).toHaveBeenCalledWith(expectedUrl);
    assert((result.current.data ?? []) as unknown[]);
  });
});
