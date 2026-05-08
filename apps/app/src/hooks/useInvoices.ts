import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../services/api";
import { createCrudMutations } from "./shared/createCrudMutations";
import { createEntityActions } from "./shared/createEntityActions";
import { useEntityList } from "./shared/useEntityList";

export type InvoiceStatus = "PENDING" | "PROCESSED" | "CANCELLED";

export type InvoiceItem = {
  id: string;
  invoice_id: string;
  product_id: string;
  product_name?: string;
  product_code?: string;
  quantity: number;
  unit_price: number;
  discount: number;
  total: number;
  created_at: string;
};

export type Invoice = {
  id: string;
  invoice_number: string;
  barcode?: string;
  supplier_id: string;
  supplier_name?: string;
  purchase_order_id?: string;
  service_order_id?: string;
  bus_id?: string;
  bus_plate?: string;
  issue_date: string;
  issue_time?: string;
  entry_date: string;
  entry_time?: string;
  cfop: string;
  payment_type?: string;
  due_date?: string;
  subtotal: number;
  discount: number;
  freight: number;
  total: number;
  status: InvoiceStatus;
  notes?: string;
  driver_id?: string;
  driver_name?: string;
  odometer_km?: number;
  created_at: string;
  items?: InvoiceItem[];
};

export type CreateInvoiceInput = {
  invoice_number: string;
  barcode?: string;
  supplier_id: string;
  purchase_order_id?: string;
  service_order_id?: string;
  bus_id?: string;
  issue_date: string;
  issue_time?: string;
  cfop?: string;
  payment_type?: string;
  due_date?: string;
  discount?: number;
  freight?: number;
  notes?: string;
  driver_id?: string;
  odometer_km?: number;
  items: { product_id: string; quantity: number; unit_price: number; discount?: number }[];
};

type UseInvoicesFilters = {
  status?: InvoiceStatus;
  supplier_id?: string;
  bus_id?: string;
};

const invoiceCrud = createCrudMutations<
  CreateInvoiceInput,
  {
    barcode?: string;
    payment_type?: string;
    due_date?: string;
    notes?: string;
    driver_id?: string;
    odometer_km?: number;
  },
  Invoice
>({
  basePath: "/invoices",
  queryKey: ["invoices"],
  invalidateQueryKeys: [["products"]],
});

const invoiceActions = createEntityActions({ queryKey: ["invoices"] });

export function useInvoices(
  limit = 200,
  offset = 0,
  status?: InvoiceStatus,
  supplier_id?: string,
  bus_id?: string
) {
  return useEntityList<Invoice, UseInvoicesFilters>({
    queryKey: ["invoices"],
    path: "/invoices",
    limit,
    offset,
    filters: { status, supplier_id, bus_id },
  });
}

export function useInvoice(id: string) {
  return useQuery({
    queryKey: ["invoice", id],
    queryFn: () => apiGet<Invoice>(`/invoices/${id}`),
    enabled: !!id,
  });
}

export const useCreateInvoice = invoiceCrud.useCreateEntity;
export const useUpdateInvoice = invoiceCrud.useUpdateEntity;
export const useDeleteInvoice = invoiceCrud.useDeleteEntity;

export function useProcessInvoice() {
  return invoiceActions.useEntityAction<void, Invoice>({
    buildPath: (id) => `/invoices/${id}/process`,
    method: "post",
    getBody: () => ({}),
  });
}

export function useCancelInvoice() {
  return invoiceActions.useEntityAction<void, Invoice>({
    buildPath: (id) => `/invoices/${id}/cancel`,
    method: "post",
    getBody: () => ({}),
  });
}
