import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../services/api";
import { createCrudMutations } from "./shared/createCrudMutations";
import { useEntityList } from "./shared/useEntityList";

export type Product = {
  id: string;
  code: string;
  name: string;
  category?: string;
  unit: string;
  min_stock: number;
  current_stock: number;
  last_cost?: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
};

export type CreateProductInput = {
  code: string;
  name: string;
  category?: string;
  unit?: string;
  min_stock?: number;
};

type UseProductsFilters = {
  active?: boolean;
  category?: string;
  search?: string;
};

const productCrud = createCrudMutations<CreateProductInput, Partial<CreateProductInput>, Product>({
  basePath: "/products",
  queryKey: ["products"],
});

export function useProducts(
  limit = 200,
  offset = 0,
  active?: boolean,
  category?: string,
  search?: string
) {
  return useEntityList<Product, UseProductsFilters>({
    queryKey: ["products"],
    path: "/products",
    limit,
    offset,
    filters: { active, category, search },
  });
}

export function useProduct(id: string) {
  return useQuery({
    queryKey: ["product", id],
    queryFn: () => apiGet<Product>(`/products/${id}`),
    enabled: !!id,
  });
}

export const useCreateProduct = productCrud.useCreateEntity;
export const useUpdateProduct = productCrud.useUpdateEntity;
export const useDeleteProduct = productCrud.useDeleteEntity;
