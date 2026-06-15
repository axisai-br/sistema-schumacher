import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import DataTable, { type DataTableColumn } from "../../components/table/DataTable";
import EmptyState from "../../components/EmptyState";
import InlineAlert from "../../components/InlineAlert";
import LoadingState from "../../components/LoadingState";
import PageHeader from "../../components/PageHeader";
import SearchToolbar from "../../components/input/SearchToolbar";
import { useTripDetails, type TripDetailsPassenger } from "../../hooks/useTrips";
import { formatBirthDate, formatCurrency, formatDateTime, formatShortId } from "../../utils/format";

function passengerDocumentDetails(item: TripDetailsPassenger) {
  const rows = [
    item.document ? `${item.document_type ? `${item.document_type} ` : ""}${item.document}` : "",
    item.cpf ? `CPF ${item.cpf}` : "",
    item.rg ? `RG ${item.rg}` : "",
    item.cnh ? `CNH ${item.cnh}` : "",
    item.birth_date ? `Nascimento ${formatBirthDate(item.birth_date)}` : "",
    item.birth_certificate_number ? `Matrícula ${item.birth_certificate_number}` : "",
    item.birth_city ? `Naturalidade ${item.birth_city}` : "",
  ].filter(Boolean);

  if (rows.length === 0) return "-";

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
      {rows.map((row) => (
        <span key={row}>{row}</span>
      ))}
    </div>
  );
}

export default function TripDetailsPage() {
  const { tripId } = useParams();
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const detailsQuery = useTripDetails(tripId ?? null);

  const passengers = detailsQuery.data?.passengers ?? [];
  const visiblePassengers = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return passengers;
    return passengers.filter((item) =>
      [
        item.name,
        item.document,
        item.document_type,
        item.cpf,
        item.rg,
        item.cnh,
        item.birth_date,
        item.birth_certificate_number,
        item.birth_city,
        item.phone,
        item.seat_number,
        item.origin_name,
        item.destination_name,
        item.booking_id,
      ]
        .join(" ")
        .toLowerCase()
        .includes(q)
    );
  }, [passengers, search]);

  const passengerColumns: DataTableColumn<TripDetailsPassenger>[] = [
    { label: "Passageiro", accessor: (item) => item.name, width: "230px" },
    { label: "Embarque", accessor: (item) => item.origin_name || "-", width: "210px" },
    { label: "Desembarque", accessor: (item) => item.destination_name || "-", width: "210px" },
    { label: "Assento", accessor: (item) => item.seat_number || "-", width: "90px", align: "center" },
    { label: "Documento", render: passengerDocumentDetails, width: "220px", hideOnMobile: true },
    { label: "Telefone", accessor: (item) => item.phone || "-", width: "150px", hideOnMobile: true },
    { label: "Pago", accessor: (item) => formatCurrency(item.paid_amount), width: "120px", align: "right" },
    { label: "Falta", accessor: (item) => formatCurrency(item.due_amount), width: "120px", align: "right" },
  ];

  if (!tripId) {
    return (
      <section className="page">
        <InlineAlert tone="error">ID da viagem invalido.</InlineAlert>
      </section>
    );
  }

  return (
    <section className="page">
      <PageHeader
        title={`Detalhe da viagem ${formatShortId(tripId, 12)}`}
        subtitle={
          detailsQuery.data?.trip
            ? `Saida: ${formatDateTime(detailsQuery.data.trip.departure_at)}`
            : "Visao geral com todos os passageiros e seus pontos de embarque/desembarque."
        }
        secondaryActions={
          <button className="button secondary" type="button" onClick={() => navigate("/trips")}>
            Voltar para viagens
          </button>
        }
      />

      {detailsQuery.isLoading ? <LoadingState label="Carregando detalhe geral da viagem..." /> : null}

      {detailsQuery.error ? (
        <InlineAlert tone="error">
          {(detailsQuery.error as Error).message || "Nao foi possivel carregar os detalhes da viagem."}
        </InlineAlert>
      ) : null}

      {detailsQuery.data ? (
        <>
          <div className="trip-kpi-grid">
            <div className="card">
              <div className="section-title">Passageiros</div>
              <div className="page-title">{detailsQuery.data.totals.passengers_count}</div>
            </div>
            <div className="card">
              <div className="section-title">Total pago</div>
              <div className="page-title">{formatCurrency(detailsQuery.data.totals.paid_amount)}</div>
            </div>
            <div className="card">
              <div className="section-title">Total pendente</div>
              <div className="page-title">{formatCurrency(detailsQuery.data.totals.due_amount)}</div>
            </div>
          </div>

          <div className="section" style={{ marginTop: "12px" }}>
            <SearchToolbar
              value={search}
              onChange={setSearch}
              placeholder="Buscar passageiro, documento, assento, embarque ou desembarque"
              inputLabel="Buscar passageiros"
              resultCount={visiblePassengers.length}
            />
          </div>

          <div className="section">
            <div className="section-header">
              <div className="section-title">Todos os passageiros da viagem</div>
            </div>
            <DataTable
              columns={passengerColumns}
              rows={visiblePassengers}
              rowKey={(item) => item.passenger_id}
              emptyState={
                <EmptyState
                  title="Nenhum passageiro encontrado"
                  description="Ajuste o filtro para localizar passageiros desta viagem."
                />
              }
            />
          </div>
        </>
      ) : null}
    </section>
  );
}
