import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../services/api";
import { createCrudMutations } from "./shared/createCrudMutations";
import { createEntityActions } from "./shared/createEntityActions";
import { useEntityList } from "./shared/useEntityList";

export type PurchaseOrderStatus = "DRAFT" | "SENT" | "PARTIAL" | "RECEIVED" | "CANCELLED";

export type PurchaseOrderItem = {
  id: string;
  purchase_order_id: string;
  product_id: string;
  product_name?: string;
  product_code?: string;
  quantity: number;
  unit_price: number;
  discount: number;
  total: number;
  received_quantity: number;
  created_at: string;
};

export type PurchaseOrder = {
  id: string;
  order_number: number;
  service_order_id?: string;
  supplier_id: string;
  supplier_name?: string;
  status: PurchaseOrderStatus;
  order_date: string;
  expected_delivery?: string;
  own_delivery: boolean;
  subtotal: number;
  discount: number;
  freight: number;
  total: number;
  notes?: string;
  created_at: string;
  updated_at: string;
  items?: PurchaseOrderItem[];
};

export type CreatePurchaseOrderInput = {
  service_order_id?: string;
  supplier_id: string;
  expected_delivery?: string;
  own_delivery?: boolean;
  discount?: number;
  freight?: number;
  notes?: string;
  items: { product_id: string; quantity: number; unit_price: number; discount?: number }[];
};

type UsePurchaseOrderFilters = {
  status?: PurchaseOrderStatus;
  supplier_id?: string;
  service_order_id?: string;
};

const purchaseOrderCrud = createCrudMutations<
  CreatePurchaseOrderInput,
  {
    expected_delivery?: string;
    own_delivery?: boolean;
    discount?: number;
    freight?: number;
    notes?: string;
  },
  PurchaseOrder
>({
  basePath: "/purchase-orders",
  queryKey: ["purchase-orders"],
});

const purchaseOrderActions = createEntityActions({ queryKey: ["purchase-orders"] });

export function usePurchaseOrders(
  limit = 200,
  offset = 0,
  status?: PurchaseOrderStatus,
  supplier_id?: string,
  service_order_id?: string
) {
  return useEntityList<PurchaseOrder, UsePurchaseOrderFilters>({
    queryKey: ["purchase-orders"],
    path: "/purchase-orders",
    limit,
    offset,
    filters: { status, supplier_id, service_order_id },
  });
}

export function usePurchaseOrder(id: string) {
  return useQuery({
    queryKey: ["purchase-order", id],
    queryFn: () => apiGet<PurchaseOrder>(`/purchase-orders/${id}`),
    enabled: !!id,
  });
}

export const useCreatePurchaseOrder = purchaseOrderCrud.useCreateEntity;
export const useUpdatePurchaseOrder = purchaseOrderCrud.useUpdateEntity;
export const useDeletePurchaseOrder = purchaseOrderCrud.useDeleteEntity;

export function useSendPurchaseOrder() {
  return purchaseOrderActions.useEntityAction<void, PurchaseOrder>({
    buildPath: (id) => `/purchase-orders/${id}/send`,
    method: "post",
    getBody: () => ({}),
  });
}

export function useReceivePurchaseOrder() {
  return purchaseOrderActions.useEntityAction<void, PurchaseOrder>({
    buildPath: (id) => `/purchase-orders/${id}/receive`,
    method: "post",
    getBody: () => ({}),
  });
}

export function useCancelPurchaseOrder() {
  return purchaseOrderActions.useEntityAction<void, PurchaseOrder>({
    buildPath: (id) => `/purchase-orders/${id}/cancel`,
    method: "post",
    getBody: () => ({}),
  });
}
