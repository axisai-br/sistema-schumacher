import { useMemo, useState } from "react";
import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
} from "../../../components/layout/CRUDListPage";
import StatusBadge from "../../../components/StatusBadge";
import useConfirmedAction from "../../../hooks/shared/useConfirmedAction";
import { apiGet, apiPatch, apiPost } from "../../../services/api";
import type { PurchaseOrder } from "../../../hooks/usePurchaseOrders";
import { formatCurrency } from "../../../utils/financialLabels";
import { formatDateTime, formatShortId } from "../../../utils/format";
import type { Supplier } from "../../../hooks/useSuppliers";
import { createStatusPresenter } from "../../../utils/statusPresentation";
import { createCrudPageConfig } from "../../shared/createCrudPageConfig";

type PurchaseOrderForm = {
  supplier_id: string;
  expected_delivery: string;
  own_delivery: boolean;
  discount: number | "";
  freight: number | "";
  notes: string;
};

const presentPurchaseOrderStatus = createStatusPresenter(
  {
    DRAFT: "Rascunho",
    SENT: "Enviado",
    PARTIAL: "Parcial",
    RECEIVED: "Recebido",
    CANCELLED: "Cancelado",
  },
  {
    DRAFT: "neutral",
    SENT: "info",
    PARTIAL: "warning",
    RECEIVED: "success",
    CANCELLED: "danger",
  }
);

export default function PurchaseOrdersTab() {
  const runConfirmedAction = useConfirmedAction();
  const [suppliers, setSuppliers] = useState<Supplier[]>([]);
  const [reloadKey, setReloadKey] = useState(0);

  const supplierMap = useMemo(() => new Map(suppliers.map((s) => [s.id, s.name])), [suppliers]);

  const config = createCrudPageConfig<PurchaseOrder, PurchaseOrderForm>({
    formFields: [
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
        key: "expected_delivery",
        label: "Previsao de Entrega",
        type: "text",
        hint: "Formato: AAAA-MM-DD",
      },
      {
        key: "own_delivery",
        label: "Retirada Propria",
        type: "checkbox",
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
    ] as FormFieldConfig<PurchaseOrderForm>[],
    columns: [
      { label: "Nº Pedido", accessor: (item) => `#${item.order_number}` },
      {
        label: "Fornecedor",
        accessor: (item) => item.supplier_name ?? supplierMap.get(item.supplier_id) ?? formatShortId(item.supplier_id),
      },
      { label: "Total", accessor: (item) => formatCurrency(item.total) },
      {
        label: "Status",
        render: (item) => {
          const status = presentPurchaseOrderStatus(item.status);
          return <StatusBadge tone={status.tone}>{status.label}</StatusBadge>;
        },
      },
      { label: "Data", accessor: (item) => formatDateTime(item.order_date) },
    ] as ColumnConfig<PurchaseOrder>[],
    initialForm: {
      supplier_id: "",
      expected_delivery: "",
      own_delivery: true,
      discount: "",
      freight: "",
      notes: "",
    },
    mapItemToForm: (item) => ({
      supplier_id: item.supplier_id,
      expected_delivery: item.expected_delivery?.split("T")[0] ?? "",
      own_delivery: item.own_delivery,
      discount: item.discount ?? "",
      freight: item.freight ?? "",
      notes: item.notes ?? "",
    }),
    searchFilter: (item, term) => {
      return (
        item.order_number.toString().includes(term) ||
        (item.supplier_name?.toLowerCase().includes(term) ?? false)
      );
    },
  });

  return (
    <CRUDListPage<PurchaseOrder, PurchaseOrderForm>
      key={reloadKey}
      hidePageHeader
      title="Pedidos de Compra"
      subtitle="Gestao de pedidos de compra."
      formTitle="Novo pedido"
      listTitle="Pedidos de compra"
      createLabel="Criar pedido"
      updateLabel="Salvar pedido"
      emptyState={{
        title: "Nenhum pedido encontrado",
        description: "Crie um pedido de compra para comecar.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const data = await apiGet<PurchaseOrder[]>(
          `/purchase-orders?limit=${pageSize}&offset=${page * pageSize}`
        );
        const suppliersData = await apiGet<Supplier[]>("/suppliers?limit=500&offset=0&active=true");
        setSuppliers(suppliersData);
        return data;
      }}
      createItem={(form) =>
        apiPost("/purchase-orders", {
          supplier_id: form.supplier_id,
          expected_delivery: form.expected_delivery || undefined,
          own_delivery: form.own_delivery,
          discount: form.discount ? Number(form.discount) : undefined,
          freight: form.freight ? Number(form.freight) : undefined,
          notes: form.notes || undefined,
          items: [],
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/purchase-orders/${id}`, {
          expected_delivery: form.expected_delivery || undefined,
          own_delivery: form.own_delivery,
          discount: form.discount ? Number(form.discount) : undefined,
          freight: form.freight ? Number(form.freight) : undefined,
          notes: form.notes || undefined,
        })
      }
      searchFilter={config.searchFilter}
      rowActions={(item) =>
        item.status === "DRAFT" ? (
          <button
            className="button ghost sm"
            type="button"
            onClick={() =>
              void runConfirmedAction({
                confirmMessage: "Enviar pedido ao fornecedor?",
                request: () => apiPost(`/purchase-orders/${item.id}/send`, {}),
                successMessage: "Pedido enviado com sucesso.",
                errorMessage: "Erro ao enviar pedido",
                onSuccess: () => setReloadKey((v) => v + 1),
              })
            }
          >
            Enviar
          </button>
        ) : item.status === "SENT" ? (
          <button
            className="button ghost sm"
            type="button"
            onClick={() =>
              void runConfirmedAction({
                confirmMessage: "Marcar pedido como recebido?",
                request: () => apiPost(`/purchase-orders/${item.id}/receive`, {}),
                successMessage: "Pedido marcado como recebido.",
                errorMessage: "Erro ao marcar recebimento",
                onSuccess: () => setReloadKey((v) => v + 1),
              })
            }
          >
            Receber
          </button>
        ) : null
      }
    />
  );
}
