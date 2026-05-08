import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
} from "../../../components/layout/CRUDListPage";
import StatusBadge from "../../../components/StatusBadge";
import { apiGet, apiPatch, apiPost } from "../../../services/api";
import type { Product } from "../../../hooks/useProducts";
import { formatCurrency } from "../../../utils/financialLabels";
import { createCrudPageConfig } from "../../shared/createCrudPageConfig";

type ProductForm = {
  code: string;
  name: string;
  category: string;
  unit: string;
  min_stock: number | "";
};

export default function ProductsTab() {
  const reloadKey = 0;

  const config = createCrudPageConfig<Product, ProductForm>({
    formFields: [
      {
        key: "code",
        label: "Codigo",
        type: "text",
        required: true,
        hint: "Codigo unico do produto",
      },
      {
        key: "name",
        label: "Nome",
        type: "text",
        required: true,
      },
      {
        key: "category",
        label: "Categoria",
        type: "select",
        options: [
          { label: "Selecione", value: "" },
          { label: "Pecas", value: "PECAS" },
          { label: "Oleos e Lubrificantes", value: "OLEOS" },
          { label: "Pneus", value: "PNEUS" },
          { label: "Combustivel", value: "COMBUSTIVEL" },
          { label: "Outros", value: "OUTROS" },
        ],
      },
      {
        key: "unit",
        label: "Unidade",
        type: "select",
        options: [
          { label: "Unidade (UN)", value: "UN" },
          { label: "Litro (L)", value: "L" },
          { label: "Quilograma (KG)", value: "KG" },
          { label: "Metro (M)", value: "M" },
          { label: "Par (PAR)", value: "PAR" },
        ],
      },
      {
        key: "min_stock",
        label: "Estoque Minimo",
        type: "number",
        hint: "Quantidade minima para alerta",
        inputProps: { min: 0, step: 1 },
      },
    ] as FormFieldConfig<ProductForm>[],
    columns: [
      { label: "Codigo", accessor: (item) => item.code },
      { label: "Nome", accessor: (item) => item.name },
      { label: "Categoria", accessor: (item) => item.category ?? "-" },
      { label: "Unidade", accessor: (item) => item.unit },
      {
        label: "Estoque",
        render: (item) => {
          const isLow = item.current_stock < item.min_stock;
          return (
            <StatusBadge tone={isLow ? "danger" : "success"}>
              {item.current_stock} {item.unit}
            </StatusBadge>
          );
        },
      },
      {
        label: "Ultimo Custo",
        accessor: (item) => (item.last_cost ? formatCurrency(item.last_cost) : "-"),
      },
      {
        label: "Status",
        render: (item) => (
          <StatusBadge tone={item.is_active ? "success" : "neutral"}>
            {item.is_active ? "Ativo" : "Inativo"}
          </StatusBadge>
        ),
      },
    ] as ColumnConfig<Product>[],
    initialForm: { code: "", name: "", category: "", unit: "UN", min_stock: "" },
    mapItemToForm: (item) => ({
      code: item.code,
      name: item.name,
      category: item.category ?? "",
      unit: item.unit,
      min_stock: item.min_stock ?? "",
    }),
    searchFilter: (item, term) => {
      return (
        item.code.toLowerCase().includes(term) ||
        item.name.toLowerCase().includes(term) ||
        (item.category?.toLowerCase().includes(term) ?? false)
      );
    },
  });

  return (
    <CRUDListPage<Product, ProductForm>
      key={reloadKey}
      hidePageHeader
      title="Produtos"
      subtitle="Cadastro de produtos e pecas."
      formTitle="Novo produto"
      listTitle="Produtos cadastrados"
      createLabel="Cadastrar produto"
      updateLabel="Salvar produto"
      emptyState={{
        title: "Nenhum produto encontrado",
        description: "Cadastre um produto para comecar.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const data = await apiGet<Product[]>(`/products?limit=${pageSize}&offset=${page * pageSize}`);
        return data;
      }}
      createItem={(form) =>
        apiPost("/products", {
          code: form.code,
          name: form.name,
          category: form.category || undefined,
          unit: form.unit || "UN",
          min_stock: form.min_stock ? Number(form.min_stock) : undefined,
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/products/${id}`, {
          code: form.code,
          name: form.name,
          category: form.category || undefined,
          unit: form.unit || undefined,
          min_stock: form.min_stock ? Number(form.min_stock) : undefined,
        })
      }
      searchFilter={config.searchFilter}
    />
  );
}
