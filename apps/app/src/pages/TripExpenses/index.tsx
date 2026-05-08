import { useMemo, useState } from "react";
import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
  type VisibilityOption,
} from "../../components/layout/CRUDListPage";
import StatusBadge from "../../components/StatusBadge";
import FileUpload from "../../components/form/FileUpload";
import useMediaQuery from "../../hooks/useMediaQuery";
import useConfirmedAction from "../../hooks/shared/useConfirmedAction";
import { useTripRouteLookup } from "../../hooks/useTripRouteLookup";
import { useFinancialFiltersOptional } from "../Financial/FinancialContext";
import { apiGet, apiPatch, apiPost } from "../../services/api";
import type { DriverCard, TripExpense } from "../../types/financial";
import {
  expenseTypeLabel,
  formatCurrency,
  paymentMethodLabel,
} from "../../utils/financialLabels";
import { formatDateTime, formatShortId } from "../../utils/format";
import { createCrudPageConfig } from "../shared/createCrudPageConfig";

type TripExpenseForm = {
  trip_id: string;
  driver_id: string;
  expense_type: string;
  amount: number;
  description: string;
  expense_date: string;
  payment_method: string;
  driver_card_id: string;
  receipt_number: string;
  receipt_photo: File | null;
  notes: string;
};

type TripItem = { id: string; route_id: string; departure_at: string };
type RouteItem = { id: string; origin_city: string; destination_city: string };
type DriverItem = { id: string; name: string };

type TripExpensesProps = {
  embedded?: boolean;
};

export default function TripExpenses({ embedded = false }: TripExpensesProps) {
  const isMobile = useMediaQuery("(max-width: 900px)");
  const runConfirmedAction = useConfirmedAction();
  const financialFilters = useFinancialFiltersOptional();
  const tripFilter = embedded ? financialFilters?.tripFilter ?? "" : "";

  const [trips, setTrips] = useState<TripItem[]>([]);
  const [routes, setRoutes] = useState<RouteItem[]>([]);
  const [drivers, setDrivers] = useState<DriverItem[]>([]);
  const [cards, setCards] = useState<DriverCard[]>([]);
  const [reloadKey, setReloadKey] = useState(0);

  const { tripLabel } = useTripRouteLookup(routes, trips);
  const driverMap = useMemo(
    () => new Map(drivers.map((driver) => [driver.id, driver.name])),
    [drivers]
  );
  const cardMap = useMemo(
    () => new Map(cards.map((card) => [card.id, card.card_number])),
    [cards]
  );

  const config = createCrudPageConfig<TripExpense, TripExpenseForm>({
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
        key: "expense_type",
        label: "Tipo de despesa",
        type: "select",
        required: true,
        options: [
          { label: "Selecione o tipo", value: "" },
          { label: "Combustivel", value: "FUEL" },
          { label: "Alimentacao", value: "FOOD" },
          { label: "Hospedagem", value: "LODGING" },
          { label: "Pedagio", value: "TOLL" },
          { label: "Manutencao", value: "MAINTENANCE" },
          { label: "Outros", value: "OTHER" },
        ],
      },
      {
        key: "amount",
        label: "Valor",
        type: "number",
        required: true,
        inputProps: { min: 0, step: 0.01 },
      },
      {
        key: "payment_method",
        label: "Forma de pagamento",
        type: "select",
        required: true,
        options: [
          { label: "Selecione a forma", value: "" },
          { label: "Adiantamento", value: "ADVANCE" },
          { label: "Cartao", value: "CARD" },
          { label: "Pessoal", value: "PERSONAL" },
          { label: "Empresa", value: "COMPANY" },
        ],
      },
      {
        key: "driver_card_id",
        label: "Cartao do motorista",
        type: "select",
        options: [
          { label: "Selecionar cartao (se aplicavel)", value: "" },
          ...cards.map((card) => ({
            label: `${card.card_number} - ${card.card_type}`,
            value: card.id,
          })),
        ],
      },
      {
        key: "expense_date",
        label: "Data da despesa",
        type: "datetime",
        required: true,
      },
      {
        key: "description",
        label: "Descricao",
        type: "textarea",
        required: true,
        colSpan: "full",
      },
      {
        key: "receipt_number",
        label: "Numero do comprovante",
        type: "text",
      },
      {
        key: "receipt_photo",
        label: "Foto do comprovante",
        colSpan: "full",
        hint: "No celular, voce pode abrir a camera para registrar a nota.",
        customRender: ({ value, onChange }) => (
          <FileUpload
            value={(value as File | null) ?? null}
            onChange={onChange as (file: File | null) => void}
            accept="image/*"
            capture="environment"
            hint="Arraste a imagem ou abra a camera do dispositivo."
          />
        ),
        showWhen: "create",
      },
      {
        key: "notes",
        label: "Observacoes",
        type: "textarea",
        colSpan: "full",
      },
    ] as FormFieldConfig<TripExpenseForm>[],
    columns: [
      { label: "Viagem", accessor: (item) => tripLabel(item.trip_id), hideOnMobile: true },
      {
        label: "Motorista",
        accessor: (item) => driverMap.get(item.driver_id) ?? formatShortId(item.driver_id),
      },
      {
        label: "Tipo",
        accessor: (item) => expenseTypeLabel[item.expense_type] ?? item.expense_type,
      },
      { label: "Valor", accessor: (item) => formatCurrency(item.amount) },
      {
        label: "Pagamento",
        accessor: (item) => paymentMethodLabel[item.payment_method] ?? item.payment_method,
        hideOnMobile: true,
      },
      {
        label: "Aprovacao",
        render: (item) => (
          <StatusBadge tone={item.is_approved ? "success" : "warning"}>
            {item.is_approved ? "Aprovada" : "Pendente"}
          </StatusBadge>
        ),
      },
      { label: "Data", accessor: (item) => formatDateTime(item.expense_date), hideOnMobile: true },
    ] as ColumnConfig<TripExpense>[],
    initialForm: {
      trip_id: "",
      driver_id: "",
      expense_type: "",
      amount: 0,
      description: "",
      expense_date: "",
      payment_method: "ADVANCE",
      driver_card_id: "",
      receipt_number: "",
      receipt_photo: null,
      notes: "",
    },
    mapItemToForm: (item) => ({
      trip_id: item.trip_id,
      driver_id: item.driver_id,
      expense_type: item.expense_type,
      amount: item.amount,
      description: item.description,
      expense_date: item.expense_date ? item.expense_date.slice(0, 16) : "",
      payment_method: item.payment_method,
      driver_card_id: item.driver_card_id ?? "",
      receipt_number: item.receipt_number ?? "",
      receipt_photo: null,
      notes: item.notes ?? "",
    }),
    searchFilter: (item, term) => {
      const trip = tripLabel(item.trip_id).toLowerCase();
      const driver = (driverMap.get(item.driver_id) ?? "").toLowerCase();
      const card = item.driver_card_id ? (cardMap.get(item.driver_card_id) ?? "").toLowerCase() : "";
      return (
        trip.includes(term) ||
        driver.includes(term) ||
        item.description.toLowerCase().includes(term) ||
        card.includes(term)
      );
    },
  });

  const visibilityOptions: VisibilityOption<TripExpense>[] = [
    { label: "Pendentes", value: "pending", predicate: (item) => !item.is_approved },
    { label: "Aprovadas", value: "approved", predicate: (item) => item.is_approved },
    { label: "Todas", value: "all", predicate: () => true },
  ];

  return (
    <CRUDListPage<TripExpense, TripExpenseForm>
      key={`${reloadKey}-${tripFilter}`}
      hidePageHeader={embedded}
      title="Despesas de Viagem"
      subtitle="Controle de despesas registradas durante a viagem."
      formTitle="Nova despesa"
      listTitle="Despesas registradas"
      createLabel="Criar despesa"
      updateLabel="Salvar despesa"
      emptyState={{
        title: "Nenhuma despesa encontrada",
        description: "Cadastre uma despesa para comecar.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const tripFilterQuery = tripFilter ? `&trip_id=${encodeURIComponent(tripFilter)}` : "";
        const data = await apiGet<TripExpense[]>(
          `/trip-expenses?limit=${pageSize}&offset=${page * pageSize}${tripFilterQuery}`
        );
        const [tripsData, routesData, driversData, cardsData] = await Promise.all([
          apiGet<TripItem[]>("/trips?limit=500&offset=0"),
          apiGet<RouteItem[]>("/routes?limit=500&offset=0"),
          apiGet<DriverItem[]>("/drivers?limit=500&offset=0"),
          apiGet<DriverCard[]>("/driver-cards?limit=500&offset=0"),
        ]);
        setTrips(tripsData);
        setRoutes(routesData);
        setDrivers(driversData);
        setCards(cardsData);
        return data;
      }}
      createItem={(form) =>
        apiPost("/trip-expenses", {
          trip_id: form.trip_id,
          driver_id: form.driver_id,
          expense_type: form.expense_type,
          amount: Number(form.amount),
          description: form.description,
          expense_date: new Date(form.expense_date).toISOString(),
          payment_method: form.payment_method,
          driver_card_id: form.driver_card_id || undefined,
          receipt_number: form.receipt_number || undefined,
          notes: form.notes || undefined,
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/trip-expenses/${id}`, {
          amount: Number(form.amount),
          description: form.description,
          expense_date: new Date(form.expense_date).toISOString(),
          receipt_number: form.receipt_number || undefined,
          notes: form.notes || undefined,
        })
      }
      searchFilter={config.searchFilter}
      visibilityOptions={visibilityOptions}
      visibilityDefault="pending"
      layout={isMobile ? "stacked" : "split"}
      rowActions={(item) =>
        !item.is_approved ? (
          <button
            className="button success sm"
            type="button"
            onClick={() =>
              void runConfirmedAction({
                confirmMessage: "Confirmar aprovacao da despesa?",
                request: () => apiPost(`/trip-expenses/${item.id}/approve`, {}),
                successMessage: "Despesa aprovada.",
                errorMessage: "Erro ao aprovar despesa",
                onSuccess: () => setReloadKey((value) => value + 1),
              })
            }
          >
            Aprovar
          </button>
        ) : null
      }
    />
  );
}
