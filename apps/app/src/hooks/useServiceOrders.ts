import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../services/api";
import { createCrudMutations } from "./shared/createCrudMutations";
import { createEntityActions } from "./shared/createEntityActions";
import { useEntityList } from "./shared/useEntityList";

export type ServiceOrderType = "PREVENTIVE" | "CORRECTIVE";
export type ServiceOrderStatus = "OPEN" | "IN_PROGRESS" | "CLOSED" | "CANCELLED";

export type ServiceOrder = {
  id: string;
  order_number: number;
  bus_id: string;
  bus_plate?: string;
  driver_id?: string;
  driver_name?: string;
  order_type: ServiceOrderType;
  status: ServiceOrderStatus;
  description: string;
  odometer_km?: number;
  scheduled_date?: string;
  location: string;
  opened_at: string;
  closed_at?: string;
  closed_odometer_km?: number;
  next_preventive_km?: number;
  notes?: string;
  created_at: string;
  updated_at: string;
};

export type CreateServiceOrderInput = {
  bus_id: string;
  driver_id?: string;
  order_type: ServiceOrderType;
  description: string;
  odometer_km?: number;
  scheduled_date?: string;
  location?: string;
  notes?: string;
};

type UseServiceOrdersFilters = {
  status?: ServiceOrderStatus;
  order_type?: ServiceOrderType;
  bus_id?: string;
};

const serviceOrderCrud = createCrudMutations<
  CreateServiceOrderInput,
  Partial<CreateServiceOrderInput>,
  ServiceOrder
>({
  basePath: "/service-orders",
  queryKey: ["service-orders"],
});

const serviceOrderActions = createEntityActions({ queryKey: ["service-orders"] });

export function useServiceOrders(
  limit = 200,
  offset = 0,
  status?: ServiceOrderStatus,
  order_type?: ServiceOrderType,
  bus_id?: string
) {
  return useEntityList<ServiceOrder, UseServiceOrdersFilters>({
    queryKey: ["service-orders"],
    path: "/service-orders",
    limit,
    offset,
    filters: { status, order_type, bus_id },
  });
}

export function useServiceOrder(id: string) {
  return useQuery({
    queryKey: ["service-order", id],
    queryFn: () => apiGet<ServiceOrder>(`/service-orders/${id}`),
    enabled: !!id,
  });
}

export const useCreateServiceOrder = serviceOrderCrud.useCreateEntity;
export const useUpdateServiceOrder = serviceOrderCrud.useUpdateEntity;
export const useDeleteServiceOrder = serviceOrderCrud.useDeleteEntity;

export function useStartServiceOrder() {
  return serviceOrderActions.useEntityAction<void, ServiceOrder>({
    buildPath: (id) => `/service-orders/${id}/start`,
    method: "post",
    getBody: () => ({}),
  });
}

export function useCloseServiceOrder() {
  return serviceOrderActions.useEntityAction<
    { closed_odometer_km?: number; next_preventive_km?: number },
    ServiceOrder
  >({
    buildPath: (id) => `/service-orders/${id}/close`,
    method: "post",
  });
}

export function useCancelServiceOrder() {
  return serviceOrderActions.useEntityAction<void, ServiceOrder>({
    buildPath: (id) => `/service-orders/${id}/cancel`,
    method: "post",
    getBody: () => ({}),
  });
}
