import { useState } from "react";
import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
} from "../../components/layout/CRUDListPage";
import StatusBadge from "../../components/StatusBadge";
import Timeline from "../../components/data-display/Timeline";
import useConfirmedAction from "../../hooks/shared/useConfirmedAction";
import { useTripRouteLookup } from "../../hooks/useTripRouteLookup";
import { useFinancialFiltersOptional } from "../Financial/FinancialContext";
import { apiGet, apiPost } from "../../services/api";
import type { TripSettlement } from "../../types/financial";
import { formatCurrency, settlementStatusLabel } from "../../utils/financialLabels";
import { formatDateTime } from "../../utils/format";
import { createStatusPresenter } from "../../utils/statusPresentation";
import { createCrudPageConfig } from "../shared/createCrudPageConfig";

type SettlementForm = {
  trip_id: string;
  notes: string;
};

type TripItem = { id: string; route_id: string; departure_at: string };
type RouteItem = { id: string; origin_city: string; destination_city: string };

type TripSettlementsProps = {
  embedded?: boolean;
};

const presentSettlementStatus = createStatusPresenter(
  {
    DRAFT: settlementStatusLabel.DRAFT,
    UNDER_REVIEW: settlementStatusLabel.UNDER_REVIEW,
    APPROVED: settlementStatusLabel.APPROVED,
    REJECTED: settlementStatusLabel.REJECTED,
    COMPLETED: settlementStatusLabel.COMPLETED,
  },
  {
    DRAFT: "neutral",
    UNDER_REVIEW: "info",
    APPROVED: "success",
    REJECTED: "danger",
    COMPLETED: "success",
  }
);

export default function TripSettlements({ embedded = false }: TripSettlementsProps) {
  const runConfirmedAction = useConfirmedAction();
  const financialFilters = useFinancialFiltersOptional();
  const tripFilter = embedded ? financialFilters?.tripFilter ?? "" : "";

  const [trips, setTrips] = useState<TripItem[]>([]);
  const [routes, setRoutes] = useState<RouteItem[]>([]);
  const [reloadKey, setReloadKey] = useState(0);

  const { tripLabel } = useTripRouteLookup(routes, trips);

  const config = createCrudPageConfig<TripSettlement, SettlementForm>({
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
        key: "notes",
        label: "Observacoes",
        type: "textarea",
        colSpan: "full",
      },
    ] as FormFieldConfig<SettlementForm>[],
    columns: [
      { label: "Viagem", accessor: (item) => tripLabel(item.trip_id) },
      { label: "Adiantamento", accessor: (item) => formatCurrency(item.advance_amount) },
      { label: "Despesas", accessor: (item) => formatCurrency(item.expenses_total) },
      {
        label: "Saldo",
        render: (item) => (
          <span style={{ color: item.balance >= 0 ? "green" : "red" }}>{formatCurrency(item.balance)}</span>
        ),
      },
      { label: "A Devolver", accessor: (item) => formatCurrency(item.amount_to_return) },
      { label: "A Reembolsar", accessor: (item) => formatCurrency(item.amount_to_reimburse) },
      {
        label: "Status",
        render: (item) => {
          const status = presentSettlementStatus(item.status);
          return <StatusBadge tone={status.tone}>{status.label}</StatusBadge>;
        },
      },
      {
        label: "Timeline",
        hideOnMobile: true,
        render: (item) => (
          <Timeline
            compact
            items={buildSettlementTimeline(item).map((event) => ({
              id: event.label,
              title: event.label,
              timestamp: event.date ? formatDateTime(event.date) : "aguardando",
              tone: event.tone,
            }))}
          />
        ),
      },
    ] as ColumnConfig<TripSettlement>[],
    initialForm: { trip_id: "", notes: "" },
    mapItemToForm: (item) => ({ trip_id: item.trip_id, notes: item.notes ?? "" }),
    searchFilter: (item, term) => {
      const trip = tripLabel(item.trip_id).toLowerCase();
      return trip.includes(term) || item.id.toLowerCase().includes(term);
    },
  });

  const runAction = (id: string, action: string, confirmMessage: string, successMessage: string) =>
    runConfirmedAction({
      confirmMessage,
      request: () => apiPost(`/trip-settlements/${id}/${action}`, {}),
      successMessage,
      errorMessage: "Erro ao atualizar acerto",
      onSuccess: () => setReloadKey((value) => value + 1),
    });

  return (
    <CRUDListPage<TripSettlement, SettlementForm>
      key={`${reloadKey}-${tripFilter}`}
      hidePageHeader={embedded}
      title="Acertos de Viagem"
      subtitle="Reconciliacao financeira pos-viagem."
      formTitle="Novo acerto"
      listTitle="Acertos registrados"
      createLabel="Criar acerto"
      updateLabel="Salvar acerto"
      emptyState={{
        title: "Nenhum acerto encontrado",
        description: "Crie um acerto para consolidar a viagem.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const tripFilterQuery = tripFilter ? `&trip_id=${encodeURIComponent(tripFilter)}` : "";
        const data = await apiGet<TripSettlement[]>(
          `/trip-settlements?limit=${pageSize}&offset=${page * pageSize}${tripFilterQuery}`
        );
        const [tripsData, routesData] = await Promise.all([
          apiGet<TripItem[]>("/trips?limit=500&offset=0"),
          apiGet<RouteItem[]>("/routes?limit=500&offset=0"),
        ]);
        setTrips(tripsData);
        setRoutes(routesData);
        return data;
      }}
      createItem={(form) =>
        apiPost("/trip-settlements", {
          trip_id: form.trip_id,
          notes: form.notes || undefined,
        })
      }
      updateItem={undefined}
      searchFilter={config.searchFilter}
      rowActions={(item) => {
        switch (item.status) {
          case "DRAFT":
            return (
              <button
                className="button ghost sm"
                type="button"
                onClick={() =>
                  void runAction(item.id, "review", "Enviar acerto para revisao?", "Acerto enviado para revisao.")
                }
              >
                Enviar revisao
              </button>
            );
          case "UNDER_REVIEW":
            return (
              <>
                <button
                  className="button success sm"
                  type="button"
                  onClick={() =>
                    void runAction(item.id, "approve", "Aprovar este acerto?", "Acerto aprovado.")
                  }
                >
                  Aprovar
                </button>
                <button
                  className="button danger sm"
                  type="button"
                  onClick={() =>
                    void runAction(item.id, "reject", "Rejeitar este acerto?", "Acerto rejeitado.")
                  }
                >
                  Rejeitar
                </button>
              </>
            );
          case "APPROVED":
            return (
              <button
                className="button success sm"
                type="button"
                onClick={() =>
                  void runAction(item.id, "complete", "Concluir este acerto?", "Acerto concluido.")
                }
              >
                Concluir
              </button>
            );
          default:
            return null;
        }
      }}
    />
  );
}

function buildSettlementTimeline(item: TripSettlement) {
  return [
    { label: "Criado", date: item.created_at, tone: "neutral" as const },
    { label: "Revisado", date: item.reviewed_at, tone: "info" as const },
    { label: "Aprovado", date: item.approved_at, tone: "success" as const },
    { label: "Concluido", date: item.completed_at, tone: "success" as const },
  ];
}
