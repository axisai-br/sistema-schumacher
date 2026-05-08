import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../services/api";
import { createCrudMutations } from "./shared/createCrudMutations";
import { useEntityList } from "./shared/useEntityList";

export type Supplier = {
  id: string;
  name: string;
  document?: string;
  phone?: string;
  email?: string;
  payment_terms?: string;
  billing_day?: number;
  is_active: boolean;
  notes?: string;
  created_at: string;
  updated_at: string;
};

export type CreateSupplierInput = {
  name: string;
  document?: string;
  phone?: string;
  email?: string;
  payment_terms?: string;
  billing_day?: number;
  notes?: string;
};

type UseSuppliersFilters = {
  active?: boolean;
};

const supplierCrud = createCrudMutations<CreateSupplierInput, Partial<CreateSupplierInput>, Supplier>({
  basePath: "/suppliers",
  queryKey: ["suppliers"],
});

export function useSuppliers(limit = 200, offset = 0, active?: boolean) {
  return useEntityList<Supplier, UseSuppliersFilters>({
    queryKey: ["suppliers"],
    path: "/suppliers",
    limit,
    offset,
    filters: { active },
  });
}

export function useSupplier(id: string) {
  return useQuery({
    queryKey: ["supplier", id],
    queryFn: () => apiGet<Supplier>(`/suppliers/${id}`),
    enabled: !!id,
  });
}

export const useCreateSupplier = supplierCrud.useCreateEntity;
export const useUpdateSupplier = supplierCrud.useUpdateEntity;
export const useDeleteSupplier = supplierCrud.useDeleteEntity;
