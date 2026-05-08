import { useMemo, useState } from "react";
import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
} from "../../../components/layout/CRUDListPage";
import StatusBadge from "../../../components/StatusBadge";
import useConfirmedAction from "../../../hooks/shared/useConfirmedAction";
import { apiGet, apiPatch, apiPost } from "../../../services/api";
import type { Invoice } from "../../../hooks/useInvoices";
import { formatCurrency } from "../../../utils/financialLabels";
import { formatDateTime, formatShortId } from "../../../utils/format";
import type { Supplier } from "../../../hooks/useSuppliers";
import { createStatusPresenter } from "../../../utils/statusPresentation";
import { createCrudPageConfig } from "../../shared/createCrudPageConfig";

type InvoiceForm = {
  invoice_number: string;
  barcode: string;
  supplier_id: string;
  issue_date: string;
  cfop: string;
  payment_type: string;
  due_date: string;
  discount: number | "";
  freight: number | "";
  notes: string;
};

const presentInvoiceStatus = createStatusPresenter(
  {
    PENDING: "Pendente",
    PROCESSED: "Processada",
    CANCELLED: "Cancelada",
  },
  {
    PENDING: "warning",
    PROCESSED: "success",
    CANCELLED: "danger",
  }
);

export default function InvoicesTab() {
  const runConfirmedAction = useConfirmedAction();
  const [suppliers, setSuppliers] = useState<Supplier[]>([]);
  const [reloadKey, setReloadKey] = useState(0);

  const supplierMap = useMemo(() => new Map(suppliers.map((s) => [s.id, s.name])), [suppliers]);

  const config = createCrudPageConfig<Invoice, InvoiceForm>({
    formFields: [
      {
        key: "invoice_number",
        label: "Numero da NF",
        type: "text",
        required: true,
      },
      {
        key: "barcode",
        label: "Codigo de Barras",
        type: "text",
      },
      {
        key: "supplier_id",
        label: "Fornecedor",
        type: "select",
        required: true,
        options: [
          { label: "Selecione o fornecedor", value: "" },
          ...suppliers.map((sup) => ({ label: sup.name, value: sup.id })),
        ],
      },
      {
        key: "issue_date",
        label: "Data de Emissao",
        type: "text",
        required: true,
        hint: "Formato: AAAA-MM-DD",
      },
      {
        key: "cfop",
        label: "CFOP",
        type: "text",
        hint: "Ex: 1000, 2403",
      },
      {
        key: "payment_type",
        label: "Tipo de Pagamento",
        type: "select",
        options: [
          { label: "Selecione", value: "" },
          { label: "Boleto", value: "BOLETO" },
          { label: "Pix", value: "PIX" },
          { label: "Transferencia", value: "TRANSFERENCIA" },
          { label: "Cartao", value: "CARTAO" },
          { label: "Dinheiro", value: "DINHEIRO" },
        ],
      },
      {
        key: "due_date",
        label: "Vencimento",
        type: "text",
        hint: "Formato: AAAA-MM-DD",
      },
      {
        key: "discount",
        label: "Desconto (R$)",
        type: "number",
        inputProps: { min: 0, step: 0.01 },
      },
      {
        key: "freight",
        label: "Frete (R$)",
        type: "number",
        inputProps: { min: 0, step: 0.01 },
      },
      {
        key: "notes",
        label: "Observacoes",
        type: "textarea",
        colSpan: "full",
      },
    ] as FormFieldConfig<InvoiceForm>[],
    columns: [
      { label: "Nº NF", accessor: (item) => item.invoice_number },
      {
        label: "Fornecedor",
        accessor: (item) => item.supplier_name ?? supplierMap.get(item.supplier_id) ?? formatShortId(item.supplier_id),
      },
      { label: "Total", accessor: (item) => formatCurrency(item.total) },
      {
        label: "Status",
        render: (item) => {
          const status = presentInvoiceStatus(item.status);
          return <StatusBadge tone={status.tone}>{status.label}</StatusBadge>;
        },
      },
      { label: "Emissao", accessor: (item) => formatDateTime(item.issue_date) },
      { label: "Entrada", accessor: (item) => formatDateTime(item.entry_date) },
    ] as ColumnConfig<Invoice>[],
    initialForm: {
      invoice_number: "",
      barcode: "",
      supplier_id: "",
      issue_date: "",
      cfop: "1000",
      payment_type: "",
      due_date: "",
      discount: "",
      freight: "",
      notes: "",
    },
    mapItemToForm: (item) => ({
      invoice_number: item.invoice_number,
      barcode: item.barcode ?? "",
      supplier_id: item.supplier_id,
      issue_date: item.issue_date?.split("T")[0] ?? "",
      cfop: item.cfop,
      payment_type: item.payment_type ?? "",
      due_date: item.due_date?.split("T")[0] ?? "",
      discount: item.discount ?? "",
      freight: item.freight ?? "",
      notes: item.notes ?? "",
    }),
    searchFilter: (item, term) => {
      return (
        item.invoice_number.toLowerCase().includes(term) ||
        (item.supplier_name?.toLowerCase().includes(term) ?? false)
      );
    },
  });

  return (
    <CRUDListPage<Invoice, InvoiceForm>
      key={reloadKey}
      hidePageHeader
      title="Notas Fiscais"
      subtitle="Entrada de notas fiscais de fornecedores."
      formTitle="Nova NF"
      listTitle="Notas fiscais"
      createLabel="Lancar NF"
      updateLabel="Salvar NF"
      emptyState={{
        title: "Nenhuma NF encontrada",
        description: "Lance uma nota fiscal para comecar.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const data = await apiGet<Invoice[]>(`/invoices?limit=${pageSize}&offset=${page * pageSize}`);
        const suppliersData = await apiGet<Supplier[]>("/suppliers?limit=500&offset=0&active=true");
        setSuppliers(suppliersData);
        return data;
      }}
      createItem={(form) =>
        apiPost("/invoices", {
          invoice_number: form.invoice_number,
          barcode: form.barcode || undefined,
          supplier_id: form.supplier_id,
          issue_date: form.issue_date,
          cfop: form.cfop || undefined,
          payment_type: form.payment_type || undefined,
          due_date: form.due_date || undefined,
          discount: form.discount ? Number(form.discount) : undefined,
          freight: form.freight ? Number(form.freight) : undefined,
          notes: form.notes || undefined,
          items: [],
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/invoices/${id}`, {
          barcode: form.barcode || undefined,
          payment_type: form.payment_type || undefined,
          due_date: form.due_date || undefined,
          notes: form.notes || undefined,
        })
      }
      searchFilter={config.searchFilter}
      rowActions={(item) =>
        item.status === "PENDING" ? (
          <button
            className="button ghost sm"
            type="button"
            onClick={() =>
              void runConfirmedAction({
                confirmMessage: "Processar esta nota fiscal?",
                request: () => apiPost(`/invoices/${item.id}/process`, {}),
                successMessage: "NF processada com sucesso.",
                errorMessage: "Erro ao processar NF",
                onSuccess: () => setReloadKey((v) => v + 1),
              })
            }
          >
            Processar
          </button>
        ) : null
      }
    />
  );
}
