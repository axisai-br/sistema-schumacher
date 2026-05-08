import { useState } from "react";
import CRUDListPage, { type ColumnConfig, type FormFieldConfig } from "../../components/layout/CRUDListPage";
import { useTripRouteLookup } from "../../hooks/useTripRouteLookup";
import { apiGet, apiPatch, apiPost } from "../../services/api";
import type { TripValidation } from "../../types/financial";
import { createCrudPageConfig } from "../shared/createCrudPageConfig";

type TripValidationForm = {
  trip_id: string;
  odometer_initial: number | "";
  odometer_final: number | "";
  passengers_expected: number;
  passengers_boarded: number;
  passengers_no_show: number;
  validation_notes: string;
};

type TripItem = { id: string; route_id: string; departure_at: string };
type RouteItem = { id: string; origin_city: string; destination_city: string };

type TripValidationsProps = {
  embedded?: boolean;
};

export default function TripValidations({ embedded = false }: TripValidationsProps) {
  const [trips, setTrips] = useState<TripItem[]>([]);
  const [routes, setRoutes] = useState<RouteItem[]>([]);

  const { tripLabel } = useTripRouteLookup(routes, trips, { separator: " -> " });

  const config = createCrudPageConfig<TripValidation, TripValidationForm>({
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
        key: "odometer_initial",
        label: "Odometro inicial",
        type: "number",
        inputProps: { min: 0 },
      },
      {
        key: "odometer_final",
        label: "Odometro final",
        type: "number",
        inputProps: { min: 0 },
      },
      {
        key: "passengers_expected",
        label: "Passageiros esperados",
        type: "number",
        inputProps: { min: 0 },
      },
      {
        key: "passengers_boarded",
        label: "Passageiros embarcados",
        type: "number",
        inputProps: { min: 0 },
      },
      {
        key: "passengers_no_show",
        label: "No-show",
        type: "number",
        inputProps: { min: 0 },
      },
      {
        key: "validation_notes",
        label: "Observacoes",
        type: "textarea",
        colSpan: "full",
      },
    ] as FormFieldConfig<TripValidationForm>[],
    columns: [
      { label: "Viagem", accessor: (item) => tripLabel(item.trip_id) },
      {
        label: "Odometro",
        accessor: (item) =>
          item.odometer_initial !== undefined && item.odometer_final !== undefined
            ? `${item.odometer_initial} -> ${item.odometer_final}`
            : "-",
      },
      {
        label: "Distancia",
        accessor: (item) => (item.distance_km ? `${item.distance_km} km` : "-"),
      },
      {
        label: "Passageiros",
        accessor: (item) => `${item.passengers_boarded}/${item.passengers_expected}`,
      },
    ] as ColumnConfig<TripValidation>[],
    initialForm: {
      trip_id: "",
      odometer_initial: "",
      odometer_final: "",
      passengers_expected: 0,
      passengers_boarded: 0,
      passengers_no_show: 0,
      validation_notes: "",
    },
    mapItemToForm: (item) => ({
      trip_id: item.trip_id,
      odometer_initial: item.odometer_initial ?? "",
      odometer_final: item.odometer_final ?? "",
      passengers_expected: item.passengers_expected ?? 0,
      passengers_boarded: item.passengers_boarded ?? 0,
      passengers_no_show: item.passengers_no_show ?? 0,
      validation_notes: item.validation_notes ?? "",
    }),
    searchFilter: (item, term) =>
      tripLabel(item.trip_id).toLowerCase().includes(term) || item.id.toLowerCase().includes(term),
  });

  return (
    <CRUDListPage<TripValidation, TripValidationForm>
      hidePageHeader={embedded}
      title="Validacoes de Viagem"
      subtitle="Conferencia de km e passageiros."
      formTitle="Nova validacao"
      listTitle="Validacoes registradas"
      createLabel="Criar validacao"
      updateLabel="Salvar validacao"
      emptyState={{
        title: "Nenhuma validacao encontrada",
        description: "Registre uma validacao para acompanhar a viagem.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const data = await apiGet<TripValidation[]>(
          `/trip-validations?limit=${pageSize}&offset=${page * pageSize}`
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
        apiPost("/trip-validations", {
          trip_id: form.trip_id,
          odometer_initial: form.odometer_initial === "" ? undefined : Number(form.odometer_initial),
          odometer_final: form.odometer_final === "" ? undefined : Number(form.odometer_final),
          passengers_expected: Number(form.passengers_expected),
          passengers_boarded: Number(form.passengers_boarded),
          passengers_no_show: Number(form.passengers_no_show),
          validation_notes: form.validation_notes || undefined,
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/trip-validations/${id}`, {
          odometer_initial: form.odometer_initial === "" ? undefined : Number(form.odometer_initial),
          odometer_final: form.odometer_final === "" ? undefined : Number(form.odometer_final),
          passengers_expected: Number(form.passengers_expected),
          passengers_boarded: Number(form.passengers_boarded),
          passengers_no_show: Number(form.passengers_no_show),
          validation_notes: form.validation_notes || undefined,
        })
      }
      searchFilter={config.searchFilter}
    />
  );
}
