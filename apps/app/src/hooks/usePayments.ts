import { useEntityList } from "./shared/useEntityList";

export type PaymentItem = {
  id: string;
  booking_id: string;
  amount: number;
  method: string;
  status: string;
  provider?: string;
  provider_ref?: string;
  paid_at?: string | null;
  created_at: string;
};

type UsePaymentsOptions = {
  booking_id?: string;
  status?: string;
  search?: string;
};

export function usePayments(limit = 200, offset = 0, options: UsePaymentsOptions = {}) {
  return useEntityList<PaymentItem, UsePaymentsOptions>({
    queryKey: ["payments"],
    path: "/payments",
    limit,
    offset,
    filters: options,
  });
}
