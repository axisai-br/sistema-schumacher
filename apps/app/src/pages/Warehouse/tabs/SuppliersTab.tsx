import { useState } from "react";
import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
} from "../../../components/layout/CRUDListPage";
import StatusBadge from "../../../components/StatusBadge";
import { apiGet, apiPatch, apiPost } from "../../../services/api";
import type { Supplier } from "../../../hooks/useSuppliers";
import { createCrudPageConfig } from "../../shared/createCrudPageConfig";

type SupplierForm = {
  name: string;
  document: string;
  phone: string;
  email: string;
  payment_terms: string;
  billing_day: number | "";
  notes: string;
};

export default function SuppliersTab() {
  const [reloadKey] = useState(0);

  const config = createCrudPageConfig<Supplier, SupplierForm>({
    formFields: [
      {
        key: "name",
        label: "Nome",
        type: "text",
        required: true,
        colSpan: "full",
      },
      {
        key: "document",
        label: "CNPJ/CPF",
        type: "text",
        hint: "Documento do fornecedor",
      },
      {
        key: "phone",
        label: "Telefone",
        type: "text",
      },
      {
        key: "email",
        label: "E-mail",
        type: "text",
      },
      {
        key: "payment_terms",
        label: "Condicao de Pagamento",
        type: "select",
        options: [
          { label: "Selecione", value: "" },
          { label: "A Vista", value: "A VISTA" },
          { label: "7 dias", value: "7 DIAS" },
          { label: "14 dias", value: "14 DIAS" },
          { label: "21 dias", value: "21 DIAS" },
          { label: "28 dias", value: "28 DIAS" },
          { label: "30 dias", value: "30 DIAS" },
        ],
      },
      {
        key: "billing_day",
        label: "Dia de Faturamento",
        type: "number",
        hint: "Dia do mes para faturamento",
        inputProps: { min: 1, max: 31 },
      },
      {
        key: "notes",
        label: "Observacoes",
        type: "textarea",
        colSpan: "full",
      },
    ] as FormFieldConfig<SupplierForm>[],
    columns: [
      { label: "Nome", accessor: (item) => item.name },
      { label: "CNPJ/CPF", accessor: (item) => item.document ?? "-" },
      { label: "Telefone", accessor: (item) => item.phone ?? "-" },
      { label: "E-mail", accessor: (item) => item.email ?? "-" },
      { label: "Cond. Pagamento", accessor: (item) => item.payment_terms ?? "-" },
      {
        label: "Status",
        render: (item) => (
          <StatusBadge tone={item.is_active ? "success" : "neutral"}>
            {item.is_active ? "Ativo" : "Inativo"}
          </StatusBadge>
        ),
      },
    ] as ColumnConfig<Supplier>[],
    initialForm: {
      name: "",
      document: "",
      phone: "",
      email: "",
      payment_terms: "",
      billing_day: "",
      notes: "",
    },
    mapItemToForm: (item) => ({
      name: item.name,
      document: item.document ?? "",
      phone: item.phone ?? "",
      email: item.email ?? "",
      payment_terms: item.payment_terms ?? "",
      billing_day: item.billing_day ?? "",
      notes: item.notes ?? "",
    }),
    searchFilter: (item, term) => {
      return (
        item.name.toLowerCase().includes(term) ||
        (item.document?.toLowerCase().includes(term) ?? false) ||
        (item.email?.toLowerCase().includes(term) ?? false)
      );
    },
  });

  return (
    <CRUDListPage<Supplier, SupplierForm>
      key={reloadKey}
      hidePageHeader
      title="Fornecedores"
      subtitle="Cadastro de fornecedores."
      formTitle="Novo fornecedor"
      listTitle="Fornecedores cadastrados"
      createLabel="Cadastrar fornecedor"
      updateLabel="Salvar fornecedor"
      emptyState={{
        title: "Nenhum fornecedor encontrado",
        description: "Cadastre um fornecedor para comecar.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const data = await apiGet<Supplier[]>(`/suppliers?limit=${pageSize}&offset=${page * pageSize}`);
        return data;
      }}
      createItem={(form) =>
        apiPost("/suppliers", {
          name: form.name,
          document: form.document || undefined,
          phone: form.phone || undefined,
          email: form.email || undefined,
          payment_terms: form.payment_terms || undefined,
          billing_day: form.billing_day ? Number(form.billing_day) : undefined,
          notes: form.notes || undefined,
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/suppliers/${id}`, {
          name: form.name,
          document: form.document || undefined,
          phone: form.phone || undefined,
          email: form.email || undefined,
          payment_terms: form.payment_terms || undefined,
          billing_day: form.billing_day ? Number(form.billing_day) : undefined,
          notes: form.notes || undefined,
        })
      }
      searchFilter={config.searchFilter}
    />
  );
}
