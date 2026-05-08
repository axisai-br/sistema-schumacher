import { useMemo, useState } from "react";
import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
} from "../../../components/layout/CRUDListPage";
import StatusBadge from "../../../components/StatusBadge";
import useConfirmedAction from "../../../hooks/shared/useConfirmedAction";
import { apiGet, apiPatch, apiPost } from "../../../services/api";
import type { ServiceOrder, ServiceOrderType } from "../../../hooks/useServiceOrders";
import { formatDateTime, formatShortId } from "../../../utils/format";
import { createStatusPresenter } from "../../../utils/statusPresentation";
import { createCrudPageConfig } from "../../shared/createCrudPageConfig";

type BusItem = { id: string; license_plate: string };
type DriverItem = { id: string; name: string };

type ServiceOrderForm = {
  bus_id: string;
  driver_id: string;
  order_type: ServiceOrderType | "";
  description: string;
  odometer_km: number | "";
  location: string;
  notes: string;
};

const presentServiceOrderStatus = createStatusPresenter(
  {
    OPEN: "Aberta",
    IN_PROGRESS: "Em Andamento",
    CLOSED: "Fechada",
    CANCELLED: "Cancelada",
  },
  {
    OPEN: "warning",
    IN_PROGRESS: "info",
    CLOSED: "success",
    CANCELLED: "danger",
  }
);

const presentServiceOrderType = createStatusPresenter(
  {
    PREVENTIVE: "Preventiva",
    CORRECTIVE: "Corretiva",
  },
  {
    PREVENTIVE: "info",
    CORRECTIVE: "warning",
  }
);

export default function ServiceOrdersTab() {
  const runConfirmedAction = useConfirmedAction();
  const [buses, setBuses] = useState<BusItem[]>([]);
  const [drivers, setDrivers] = useState<DriverItem[]>([]);
  const [reloadKey, setReloadKey] = useState(0);

  const busMap = useMemo(() => new Map(buses.map((b) => [b.id, b.license_plate])), [buses]);

  const config = createCrudPageConfig<ServiceOrder, ServiceOrderForm>({
    formFields: [
      {
        key: "bus_id",
        label: "Veiculo",
        type: "select",
        required: true,
        options: [
          { label: "Selecione o veiculo", value: "" },
          ...buses.map((bus) => ({ label: bus.license_plate, value: bus.id })),
        ],
      },
      {
        key: "driver_id",
        label: "Motorista",
        type: "select",
        options: [
          { label: "Selecione (opcional)", value: "" },
          ...drivers.map((driver) => ({ label: driver.name, value: driver.id })),
        ],
      },
      {
        key: "order_type",
        label: "Tipo de OS",
        type: "select",
        required: true,
        options: [
          { label: "Selecione o tipo", value: "" },
          { label: "Preventiva", value: "PREVENTIVE" },
          { label: "Corretiva", value: "CORRECTIVE" },
        ],
      },
      {
        key: "odometer_km",
        label: "Km Atual",
        type: "number",
        hint: "Quilometragem atual do veiculo",
        inputProps: { min: 0 },
      },
      {
        key: "location",
        label: "Local",
        type: "select",
        options: [
          { label: "Schumacher", value: "SCHUMACHER" },
          { label: "Externo", value: "EXTERNO" },
        ],
      },
      {
        key: "description",
        label: "Descricao",
        type: "textarea",
        required: true,
        colSpan: "full",
        hint: "Descreva o servico a ser realizado",
      },
      {
        key: "notes",
        label: "Observacoes",
        type: "textarea",
        colSpan: "full",
      },
    ] as FormFieldConfig<ServiceOrderForm>[],
    columns: [
      { label: "Nº OS", accessor: (item) => `#${item.order_number}` },
      {
        label: "Veiculo",
        accessor: (item) => item.bus_plate ?? busMap.get(item.bus_id) ?? formatShortId(item.bus_id),
      },
      {
        label: "Tipo",
        render: (item) => {
          const status = presentServiceOrderType(item.order_type);
          return <StatusBadge tone={status.tone}>{status.label}</StatusBadge>;
        },
      },
      {
        label: "Descricao",
        accessor: (item) => item.description.substring(0, 50) + (item.description.length > 50 ? "..." : ""),
      },
      {
        label: "Status",
        render: (item) => {
          const status = presentServiceOrderStatus(item.status);
          return <StatusBadge tone={status.tone}>{status.label}</StatusBadge>;
        },
      },
      { label: "Abertura", accessor: (item) => formatDateTime(item.opened_at) },
    ] as ColumnConfig<ServiceOrder>[],
    initialForm: {
      bus_id: "",
      driver_id: "",
      order_type: "",
      description: "",
      odometer_km: "",
      location: "SCHUMACHER",
      notes: "",
    },
    mapItemToForm: (item) => ({
      bus_id: item.bus_id,
      driver_id: item.driver_id ?? "",
      order_type: item.order_type,
      description: item.description,
      odometer_km: item.odometer_km ?? "",
      location: item.location,
      notes: item.notes ?? "",
    }),
    searchFilter: (item, term) => {
      return (
        item.order_number.toString().includes(term) ||
        item.description.toLowerCase().includes(term) ||
        (item.bus_plate?.toLowerCase().includes(term) ?? false)
      );
    },
  });

  return (
    <CRUDListPage<ServiceOrder, ServiceOrderForm>
      key={reloadKey}
      hidePageHeader
      title="Ordens de Servico"
      subtitle="Gestao de manutencao preventiva e corretiva."
      formTitle="Nova OS"
      listTitle="Ordens de servico"
      createLabel="Criar OS"
      updateLabel="Salvar OS"
      emptyState={{
        title: "Nenhuma OS encontrada",
        description: "Crie uma ordem de servico para comecar.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const data = await apiGet<ServiceOrder[]>(
          `/service-orders?limit=${pageSize}&offset=${page * pageSize}`
        );
        const [busesData, driversData] = await Promise.all([
          apiGet<BusItem[]>("/buses?limit=500&offset=0"),
          apiGet<DriverItem[]>("/drivers?limit=500&offset=0"),
        ]);
        setBuses(busesData);
        setDrivers(driversData);
        return data;
      }}
      createItem={(form) =>
        apiPost("/service-orders", {
          bus_id: form.bus_id,
          driver_id: form.driver_id || undefined,
          order_type: form.order_type,
          description: form.description,
          odometer_km: form.odometer_km ? Number(form.odometer_km) : undefined,
          location: form.location || undefined,
          notes: form.notes || undefined,
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/service-orders/${id}`, {
          driver_id: form.driver_id || undefined,
          description: form.description,
          odometer_km: form.odometer_km ? Number(form.odometer_km) : undefined,
          location: form.location || undefined,
          notes: form.notes || undefined,
        })
      }
      searchFilter={config.searchFilter}
      rowActions={(item) =>
        item.status === "OPEN" ? (
          <button
            className="button ghost sm"
            type="button"
            onClick={() =>
              void runConfirmedAction({
                confirmMessage: "Iniciar execucao da OS?",
                request: () => apiPost(`/service-orders/${item.id}/start`, {}),
                successMessage: "OS iniciada com sucesso.",
                errorMessage: "Erro ao iniciar OS",
                onSuccess: () => setReloadKey((v) => v + 1),
              })
            }
          >
            Iniciar
          </button>
        ) : item.status === "IN_PROGRESS" ? (
          <button
            className="button ghost sm"
            type="button"
            onClick={() =>
              void runConfirmedAction({
                confirmMessage: "Fechar esta OS?",
                request: () => apiPost(`/service-orders/${item.id}/close`, {}),
                successMessage: "OS fechada com sucesso.",
                errorMessage: "Erro ao fechar OS",
                onSuccess: () => setReloadKey((v) => v + 1),
              })
            }
          >
            Fechar
          </button>
        ) : null
      }
    />
  );
}
