import { useMemo, useState } from "react";
import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
} from "../../components/layout/CRUDListPage";
import StatusBadge from "../../components/StatusBadge";
import { useTripRouteLookup } from "../../hooks/useTripRouteLookup";
import useConfirmedAction from "../../hooks/shared/useConfirmedAction";
import { useFinancialFiltersOptional } from "../Financial/FinancialContext";
import { apiGet, apiPatch, apiPost } from "../../services/api";
import type { TripAdvance } from "../../types/financial";
import { advanceStatusLabel, formatCurrency } from "../../utils/financialLabels";
import { formatDateTime, formatShortId } from "../../utils/format";
import { createStatusPresenter } from "../../utils/statusPresentation";
import { createCrudPageConfig } from "../shared/createCrudPageConfig";

type TripAdvanceForm = {
  trip_id: string;
  driver_id: string;
  amount: number;
  purpose?: string;
  notes?: string;
};

type TripItem = { id: string; route_id: string; departure_at: string };
type RouteItem = { id: string; origin_city: string; destination_city: string };
type DriverItem = { id: string; name: string };

type TripAdvancesProps = {
  embedded?: boolean;
};

const presentAdvanceStatus = createStatusPresenter(
  {
    PENDING: advanceStatusLabel.PENDING,
    DELIVERED: advanceStatusLabel.DELIVERED,
    SETTLED: advanceStatusLabel.SETTLED,
    CANCELLED: advanceStatusLabel.CANCELLED,
  },
  {
    PENDING: "warning",
    DELIVERED: "info",
    SETTLED: "success",
    CANCELLED: "danger",
  }
);

export default function TripAdvances({ embedded = false }: TripAdvancesProps) {
  const financialFilters = useFinancialFiltersOptional();
  const runConfirmedAction = useConfirmedAction();
  const tripFilter = embedded ? financialFilters?.tripFilter ?? "" : "";

  const [trips, setTrips] = useState<TripItem[]>([]);
  const [routes, setRoutes] = useState<RouteItem[]>([]);
  const [drivers, setDrivers] = useState<DriverItem[]>([]);
  const [reloadKey, setReloadKey] = useState(0);

  const { tripLabel } = useTripRouteLookup(routes, trips);
  const driverMap = useMemo(
    () => new Map(drivers.map((driver) => [driver.id, driver.name])),
    [drivers]
  );

  const config = createCrudPageConfig<TripAdvance, TripAdvanceForm>({
    formFields: [
      {
        key: "trip_id",
        label: "Viagem",
        type: "select",
        required: true,
        options: [
          { label: "Selecione a viagem", value: "" },
          ...trips.map((trip) => ({
            label: tripLabel(trip.id),
            value: trip.id,
          })),
        ],
      },
      {
        key: "driver_id",
        label: "Motorista",
        type: "select",
        required: true,
        options: [
          { label: "Selecione o motorista", value: "" },
          ...drivers.map((driver) => ({
            label: driver.name,
            value: driver.id,
          })),
        ],
      },
      {
        key: "amount",
        label: "Valor",
        type: "number",
        required: true,
        hint: "Valor do adiantamento em reais",
        inputProps: { min: 0, step: 0.01 },
      },
      {
        key: "purpose",
        label: "Finalidade",
        type: "textarea",
        hint: "Descreva o proposito do adiantamento",
        colSpan: "full",
      },
      {
        key: "notes",
        label: "Observacoes",
        type: "textarea",
        colSpan: "full",
      },
    ] as FormFieldConfig<TripAdvanceForm>[],
    columns: [
      { label: "Viagem", accessor: (item) => tripLabel(item.trip_id) },
      {
        label: "Motorista",
        accessor: (item) => driverMap.get(item.driver_id) ?? formatShortId(item.driver_id),
      },
      { label: "Valor", accessor: (item) => formatCurrency(item.amount) },
      {
        label: "Status",
        render: (item) => {
          const status = presentAdvanceStatus(item.status);
          return <StatusBadge tone={status.tone}>{status.label}</StatusBadge>;
        },
      },
      { label: "Criado em", accessor: (item) => formatDateTime(item.created_at) },
    ] as ColumnConfig<TripAdvance>[],
    initialForm: { trip_id: "", driver_id: "", amount: 0, purpose: "", notes: "" },
    mapItemToForm: (item) => ({
      trip_id: item.trip_id,
      driver_id: item.driver_id,
      amount: item.amount,
      purpose: item.purpose ?? "",
      notes: item.notes ?? "",
    }),
    searchFilter: (item, term) => {
      const trip = tripLabel(item.trip_id).toLowerCase();
      const driver = (driverMap.get(item.driver_id) ?? "").toLowerCase();
      return trip.includes(term) || driver.includes(term) || item.id.toLowerCase().includes(term);
    },
  });

  return (
    <CRUDListPage<TripAdvance, TripAdvanceForm>
      key={`${reloadKey}-${tripFilter}`}
      hidePageHeader={embedded}
      title="Adiantamentos de Viagem"
      subtitle="Gestao de adiantamentos para motoristas."
      formTitle="Novo adiantamento"
      listTitle="Adiantamentos registrados"
      createLabel="Criar adiantamento"
      updateLabel="Salvar adiantamento"
      emptyState={{
        title: "Nenhum adiantamento encontrado",
        description: "Cadastre um adiantamento para comecar.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const tripFilterQuery = tripFilter ? `&trip_id=${encodeURIComponent(tripFilter)}` : "";
        const data = await apiGet<TripAdvance[]>(
          `/trip-advances?limit=${pageSize}&offset=${page * pageSize}${tripFilterQuery}`
        );
        const [tripsData, routesData, driversData] = await Promise.all([
          apiGet<TripItem[]>("/trips?limit=500&offset=0"),
          apiGet<RouteItem[]>("/routes?limit=500&offset=0"),
          apiGet<DriverItem[]>("/drivers?limit=500&offset=0"),
        ]);
        setTrips(tripsData);
        setRoutes(routesData);
        setDrivers(driversData);
        return data;
      }}
      createItem={(form) =>
        apiPost("/trip-advances", {
          trip_id: form.trip_id,
          driver_id: form.driver_id,
          amount: Number(form.amount),
          purpose: form.purpose || undefined,
          notes: form.notes || undefined,
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/trip-advances/${id}`, {
          amount: Number(form.amount),
          purpose: form.purpose || undefined,
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
                confirmMessage: "Confirmar entrega do adiantamento?",
                request: () => apiPost(`/trip-advances/${item.id}/deliver`, {}),
                successMessage: "Adiantamento marcado como entregue.",
                errorMessage: "Erro ao marcar adiantamento",
                onSuccess: () => setReloadKey((value) => value + 1),
              })
            }
          >
            Marcar entregue
          </button>
        ) : null
      }
    />
  );
}
