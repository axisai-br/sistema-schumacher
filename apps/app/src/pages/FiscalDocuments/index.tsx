import { useState } from "react";
import CRUDListPage, { type ColumnConfig, type FormFieldConfig } from "../../components/layout/CRUDListPage";
import { useTripRouteLookup } from "../../hooks/useTripRouteLookup";
import { apiGet, apiPatch, apiPost } from "../../services/api";
import type { FiscalDocument } from "../../types/financial";
import { formatCurrency } from "../../utils/financialLabels";
import { formatDateTime } from "../../utils/format";
import { createCrudPageConfig } from "../shared/createCrudPageConfig";

type FiscalDocumentForm = {
  trip_id: string;
  document_type: string;
  document_number: string;
  issue_date: string;
  amount: number;
  recipient_name: string;
  recipient_document: string;
  status: string;
  external_id: string;
  metadata: string;
};

type TripItem = { id: string; route_id: string; departure_at: string };
type RouteItem = { id: string; origin_city: string; destination_city: string };

type FiscalDocumentsProps = {
  embedded?: boolean;
};

export default function FiscalDocuments({ embedded = false }: FiscalDocumentsProps) {
  const [trips, setTrips] = useState<TripItem[]>([]);
  const [routes, setRoutes] = useState<RouteItem[]>([]);

  const { tripLabel } = useTripRouteLookup(routes, trips, { separator: " -> " });

  const config = createCrudPageConfig<FiscalDocument, FiscalDocumentForm>({
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
        key: "document_type",
        label: "Tipo de documento",
        required: true,
      },
      {
        key: "document_number",
        label: "Numero",
      },
      {
        key: "issue_date",
        label: "Data de emissao",
        type: "datetime",
      },
      {
        key: "amount",
        label: "Valor",
        type: "number",
        required: true,
        inputProps: { min: 0, step: 0.01 },
      },
      {
        key: "recipient_name",
        label: "Destinatario",
      },
      {
        key: "recipient_document",
        label: "Documento do destinatario",
      },
      {
        key: "status",
        label: "Status",
      },
      {
        key: "external_id",
        label: "ID externo",
      },
      {
        key: "metadata",
        label: "Metadata (JSON)",
        type: "textarea",
        colSpan: "full",
        hint: "Cole um JSON valido se precisar de dados extras",
      },
    ] as FormFieldConfig<FiscalDocumentForm>[],
    columns: [
      { label: "Viagem", accessor: (item) => tripLabel(item.trip_id) },
      { label: "Tipo", accessor: (item) => item.document_type },
      { label: "Numero", accessor: (item) => item.document_number ?? "-" },
      { label: "Valor", accessor: (item) => formatCurrency(item.amount) },
      { label: "Status", accessor: (item) => item.status },
      { label: "Emissao", accessor: (item) => formatDateTime(item.issue_date) },
    ] as ColumnConfig<FiscalDocument>[],
    initialForm: {
      trip_id: "",
      document_type: "",
      document_number: "",
      issue_date: "",
      amount: 0,
      recipient_name: "",
      recipient_document: "",
      status: "PENDING",
      external_id: "",
      metadata: "",
    },
    mapItemToForm: (item) => ({
      trip_id: item.trip_id,
      document_type: item.document_type,
      document_number: item.document_number ?? "",
      issue_date: item.issue_date ? item.issue_date.slice(0, 16) : "",
      amount: item.amount,
      recipient_name: item.recipient_name ?? "",
      recipient_document: item.recipient_document ?? "",
      status: item.status ?? "",
      external_id: item.external_id ?? "",
      metadata: item.metadata ? JSON.stringify(item.metadata, null, 2) : "",
    }),
    searchFilter: (item, term) => {
      const trip = tripLabel(item.trip_id).toLowerCase();
      return (
        trip.includes(term) ||
        item.document_type.toLowerCase().includes(term) ||
        item.document_number?.toLowerCase().includes(term) ||
        item.status?.toLowerCase().includes(term)
      );
    },
  });

  const parseMetadata = (raw: string) => {
    const trimmed = raw.trim();
    if (!trimmed) return undefined;
    return JSON.parse(trimmed);
  };

  return (
    <CRUDListPage<FiscalDocument, FiscalDocumentForm>
      hidePageHeader={embedded}
      title="Documentos Fiscais"
      subtitle="Registro basico de NFS-e, CT-e e outros documentos."
      formTitle="Novo documento"
      listTitle="Documentos registrados"
      createLabel="Criar documento"
      updateLabel="Salvar documento"
      emptyState={{
        title: "Nenhum documento encontrado",
        description: "Cadastre um documento fiscal para comecar.",
      }}
      formFields={config.formFields}
      columns={config.columns}
      initialForm={config.initialForm}
      mapItemToForm={config.mapItemToForm}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize }) => {
        const data = await apiGet<FiscalDocument[]>(
          `/fiscal-documents?limit=${pageSize}&offset=${page * pageSize}`
        );
        const [tripsData, routesData] = await Promise.all([
          apiGet<TripItem[]>("/trips?limit=500&offset=0"),
          apiGet<RouteItem[]>("/routes?limit=500&offset=0"),
        ]);
        setTrips(tripsData);
        setRoutes(routesData);
        return data;
      }}
      createItem={async (form) => {
        const metadata = parseMetadata(form.metadata);
        await apiPost("/fiscal-documents", {
          trip_id: form.trip_id,
          document_type: form.document_type,
          document_number: form.document_number || undefined,
          issue_date: form.issue_date ? new Date(form.issue_date).toISOString() : undefined,
          amount: Number(form.amount),
          recipient_name: form.recipient_name || undefined,
          recipient_document: form.recipient_document || undefined,
          status: form.status || undefined,
          external_id: form.external_id || undefined,
          metadata,
        });
      }}
      updateItem={async (id, form) => {
        const metadata = parseMetadata(form.metadata);
        await apiPatch(`/fiscal-documents/${id}`, {
          document_number: form.document_number || undefined,
          issue_date: form.issue_date ? new Date(form.issue_date).toISOString() : undefined,
          amount: Number(form.amount),
          recipient_name: form.recipient_name || undefined,
          recipient_document: form.recipient_document || undefined,
          status: form.status || undefined,
          external_id: form.external_id || undefined,
          metadata,
        });
      }}
      searchFilter={config.searchFilter}
    />
  );
}
