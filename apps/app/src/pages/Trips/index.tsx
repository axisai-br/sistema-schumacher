import { useEffect, useMemo, useState } from "react";
import DataTable, { type DataTableColumn } from "../../components/table/DataTable";
import EmptyState from "../../components/EmptyState";
import InlineAlert from "../../components/InlineAlert";
import LoadingState from "../../components/LoadingState";
import Modal from "../../components/overlay/Modal";
import PageHeader from "../../components/PageHeader";
import SearchToolbar from "../../components/input/SearchToolbar";
import { useBuses } from "../../hooks/useBuses";
import { useRouteStops } from "../../hooks/useRouteStops";
import { useRoutes, type RouteItem } from "../../hooks/useRoutes";
import {
  useTrips,
  useTripDetailsBatch,
  type TripDetailsPassenger,
  type TripDetailsTotals,
  type TripItem,
} from "../../hooks/useTrips";
import { APIRequestError, apiPost } from "../../services/api";
import { formatCurrency, formatDateTime } from "../../utils/format";

type RouteTripSummary = {
  activeCount: number;
  nextTrip: TripItem | null;
};

const normalizeTripStatus = (status: string | null | undefined) =>
  String(status ?? "").trim().toUpperCase();

const isActiveTripStatus = (status: string | null | undefined) => {
  const s = normalizeTripStatus(status);
  return s === "ATIVO" || s === "ACTIVE" || s === "SCHEDULED" || s === "IN_PROGRESS";
};

const isTemplateTripStatus = (status: string | null | undefined) =>
  normalizeTripStatus(status) === "TEMPLATE";

const isCancelledTripStatus = (status: string | null | undefined) => {
  const s = normalizeTripStatus(status);
  return s === "CANCELLED" || s === "CANCELED";
};

const normalizeFilePart = (value: string) =>
  value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^a-zA-Z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .toLowerCase();

const buildExportTimestamp = () => {
  const date = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}-${pad(date.getHours())}${pad(
    date.getMinutes()
  )}${pad(date.getSeconds())}`;
};

function toErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof APIRequestError) return error.message;
  if (error instanceof Error) return error.message;
  return fallback;
}

export default function Trips() {
  const [search, setSearch] = useState("");
  const [selectedRouteId, setSelectedRouteId] = useState<string | null>(null);
  const [passengerSearch, setPassengerSearch] = useState("");
  const [exportingPassengers, setExportingPassengers] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);

  const [createOpen, setCreateOpen] = useState(false);
  const [createRouteId, setCreateRouteId] = useState("");
  const [createBusID, setCreateBusID] = useState("");
  const [createTripDate, setCreateTripDate] = useState("");
  const [createStopDateTimes, setCreateStopDateTimes] = useState<Record<string, string>>({});
  const [createSaving, setCreateSaving] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const routesQuery = useRoutes(500, 0, { search, status: "active" });
  const routes = routesQuery.data ?? [];
  const createRoutesQuery = useRoutes(500, 0, { status: "active" });
  const createRoutes = createRoutesQuery.data ?? [];
  const tripsQuery = useTrips(500, 0);
  const trips = tripsQuery.data ?? [];
  const busesQuery = useBuses(500, 0);
  const buses = busesQuery.data ?? [];
  const createRouteStopsQuery = useRouteStops(createRouteId || null);
  const createRouteStops = createRouteStopsQuery.data ?? [];

  useEffect(() => {
    if (!createOpen) return;
    setCreateStopDateTimes((previous) => {
      const next: Record<string, string> = {};
      for (const stop of createRouteStops) {
        next[stop.id] = previous[stop.id] ?? "";
      }
      return next;
    });
  }, [createOpen, createRouteStops]);

  const routeTripSummary = useMemo(() => {
    const tripsByRoute = new Map<string, TripItem[]>();
    for (const trip of trips) {
      if (isTemplateTripStatus(trip.status)) continue;
      const existing = tripsByRoute.get(trip.route_id);
      if (existing) existing.push(trip);
      else tripsByRoute.set(trip.route_id, [trip]);
    }

    const now = Date.now();
    const summary = new Map<string, RouteTripSummary>();
    for (const [routeID, routeTrips] of tripsByRoute.entries()) {
      routeTrips.sort((a, b) => Date.parse(a.departure_at) - Date.parse(b.departure_at));
      const activeTrips = routeTrips.filter((t) => isActiveTripStatus(t.status));
      const referenceTrips =
        activeTrips.length > 0 ? activeTrips : routeTrips.filter((t) => !isCancelledTripStatus(t.status));
      const nextTrip =
        referenceTrips.find((t) => {
          const d = Date.parse(t.departure_at);
          return !Number.isNaN(d) && d >= now;
        }) ??
        referenceTrips[referenceTrips.length - 1] ??
        null;
      summary.set(routeID, { activeCount: activeTrips.length, nextTrip });
    }

    return summary;
  }, [trips]);

  const tripsForSelectedRoute = useMemo(() => {
    if (!selectedRouteId) return [];
    return trips
      .filter((t) => t.route_id === selectedRouteId && !isTemplateTripStatus(t.status))
      .sort((a, b) => Date.parse(b.departure_at) - Date.parse(a.departure_at));
  }, [trips, selectedRouteId]);

  const routeTripIds = useMemo(() => tripsForSelectedRoute.map((trip) => trip.id), [tripsForSelectedRoute]);
  const detailsBatchQuery = useTripDetailsBatch(routeTripIds);

  const selectedRoute = useMemo(
    () => routes.find((r) => r.id === selectedRouteId) ?? null,
    [routes, selectedRouteId]
  );

  const consolidatedPassengers = useMemo(() => {
    const deduped = new Map<string, TripDetailsPassenger>();

    for (const details of detailsBatchQuery.data) {
      for (const passenger of details.passengers ?? []) {
        const key = [
          passenger.passenger_id,
          passenger.booking_id,
          passenger.origin_stop_id,
          passenger.destination_stop_id,
        ].join("::");

        if (!deduped.has(key)) {
          deduped.set(key, passenger);
        }
      }
    }

    return Array.from(deduped.values()).sort((a, b) => {
      const origin = (a.origin_name ?? "").localeCompare(b.origin_name ?? "");
      if (origin !== 0) return origin;
      const destination = (a.destination_name ?? "").localeCompare(b.destination_name ?? "");
      if (destination !== 0) return destination;
      return (a.name ?? "").localeCompare(b.name ?? "");
    });
  }, [detailsBatchQuery.data]);

  const consolidatedTotals = useMemo<TripDetailsTotals | null>(() => {
    if (consolidatedPassengers.length === 0) return null;

    return consolidatedPassengers.reduce<TripDetailsTotals>(
      (acc, passenger) => {
        acc.passengers_count += 1;
        acc.total_amount += Number(passenger.total_amount ?? 0);
        acc.paid_amount += Number(passenger.paid_amount ?? 0);
        acc.due_amount += Number(passenger.due_amount ?? 0);
        return acc;
      },
      { passengers_count: 0, total_amount: 0, paid_amount: 0, due_amount: 0 }
    );
  }, [consolidatedPassengers]);

  const visiblePassengers = useMemo(() => {
    const q = passengerSearch.trim().toLowerCase();
    if (!q) return consolidatedPassengers;

    return consolidatedPassengers.filter((p) =>
      [p.name, p.document, p.phone, p.seat_number, p.origin_name, p.destination_name]
        .join(" ")
        .toLowerCase()
        .includes(q)
    );
  }, [consolidatedPassengers, passengerSearch]);

  const loadError =
    (routesQuery.error as Error | undefined)?.message ||
    (tripsQuery.error as Error | undefined)?.message ||
    null;

  const nextCapacityLabel = (routeID: string) => {
    const trip = routeTripSummary.get(routeID)?.nextTrip;
    if (!trip) return "-";

    const total = Number.isFinite(trip.seats_total) ? Math.max(0, Number(trip.seats_total)) : 0;
    const avail = Number.isFinite(trip.seats_available) ? Math.max(0, Number(trip.seats_available)) : 0;
    return total > 0 ? `${avail}/${total}` : "-";
  };

  const nextTripLabel = (routeID: string) => {
    const trip = routeTripSummary.get(routeID)?.nextTrip;
    return trip ? formatDateTime(trip.departure_at) : "-";
  };

  const routeColumns: DataTableColumn<RouteItem>[] = [
    { label: "Rota", accessor: (r) => r.name },
    { label: "Trecho", accessor: (r) => `${r.origin_city} -> ${r.destination_city}`, width: "280px" },
    {
      label: "Viagens ativas",
      accessor: (r) => String(routeTripSummary.get(r.id)?.activeCount ?? 0),
      width: "120px",
      align: "center",
    },
    { label: "Vagas (proxima)", accessor: (r) => nextCapacityLabel(r.id), width: "130px", align: "center" },
    { label: "Proxima saida", accessor: (r) => nextTripLabel(r.id), width: "180px" },
  ];

  const passengerColumns: DataTableColumn<TripDetailsPassenger>[] = [
    { label: "Passageiro", accessor: (p) => p.name, width: "220px" },
    { label: "Embarque", accessor: (p) => p.origin_name || "-", width: "180px" },
    { label: "Desembarque", accessor: (p) => p.destination_name || "-", width: "180px" },
    { label: "Assento", accessor: (p) => p.seat_number || "-", width: "80px", align: "center" },
    { label: "Documento", accessor: (p) => p.document || "-", width: "140px", hideOnMobile: true },
    { label: "Telefone", accessor: (p) => p.phone || "-", width: "140px", hideOnMobile: true },
    { label: "Pago", accessor: (p) => formatCurrency(p.paid_amount), width: "110px", align: "right" },
    { label: "Pendente", accessor: (p) => formatCurrency(p.due_amount), width: "110px", align: "right" },
  ];

  const fallbackBusIDs = useMemo(() => {
    const unique = new Set<string>();
    for (const trip of trips) {
      const id = String(trip.bus_id ?? "").trim();
      if (id) unique.add(id);
    }
    return Array.from(unique).sort((a, b) => a.localeCompare(b));
  }, [trips]);

  const busOptions = useMemo(() => {
    if (buses.length > 0) {
      return buses.map((bus) => ({ value: bus.id, label: bus.name }));
    }
    return fallbackBusIDs.map((busID) => ({ value: busID, label: busID }));
  }, [buses, fallbackBusIDs]);

  const openCreateModal = () => {
    const firstRouteID = createRoutes[0]?.id ?? "";
    setCreateRouteId(firstRouteID);
    setCreateBusID("");
    setCreateTripDate("");
    setCreateStopDateTimes({});
    setCreateError(null);
    setCreateOpen(true);
  };

  const saveTrip = async () => {
    const routeID = createRouteId.trim();
    const busID = createBusID.trim();
    const tripDate = createTripDate.trim();
    if (!routeID || !busID || !tripDate) {
      setCreateError("Rota, onibus e data da viagem sao obrigatorios.");
      return;
    }
    if (createRouteStops.length < 2) {
      setCreateError("A rota precisa ter pelo menos 2 paradas para criar a viagem.");
      return;
    }

    const sortedStops = [...createRouteStops].sort((a, b) => a.stop_order - b.stop_order);
    const stopsPayload = [];
    for (const stop of sortedStops) {
      const stopDateTimeLocal = (createStopDateTimes[stop.id] ?? "").trim();
      if (!stopDateTimeLocal) {
        setCreateError("Informe dia e horario para todas as paradas.");
        return;
      }
      const parsed = new Date(stopDateTimeLocal);
      if (Number.isNaN(parsed.getTime())) {
        setCreateError("Existe parada com dia/horario invalido.");
        return;
      }
      stopsPayload.push({
        route_stop_id: stop.id,
        depart_at: parsed.toISOString(),
      });
    }

    const firstStopDateTimeLocal = createStopDateTimes[sortedStops[0].id];
    const departureAt = firstStopDateTimeLocal
      ? new Date(firstStopDateTimeLocal).toISOString()
      : `${tripDate}T12:00:00Z`;

    setCreateSaving(true);
    setCreateError(null);
    try {
      await apiPost("/trips", {
        route_id: routeID,
        bus_id: busID,
        departure_at: departureAt,
        status: "ATIVO",
        stops: stopsPayload,
      });
      await Promise.all([tripsQuery.refetch(), routesQuery.refetch()]);
      setCreateOpen(false);
    } catch (error) {
      setCreateError(toErrorMessage(error, "Nao foi possivel criar a viagem."));
    } finally {
      setCreateSaving(false);
    }
  };

  const exportPassengersXlsx = async () => {
    if (visiblePassengers.length === 0) return;
    setExportingPassengers(true);
    setExportError(null);
    try {
      const XLSX = await import("xlsx");
      const rows = visiblePassengers.map((passenger) => ({
        Passageiro: passenger.name,
        Embarque: passenger.origin_name || "-",
        Desembarque: passenger.destination_name || "-",
        Assento: passenger.seat_number || "-",
        Documento: passenger.document || "-",
        Telefone: passenger.phone || "-",
        Pago: Number(passenger.paid_amount ?? 0),
        Pendente: Number(passenger.due_amount ?? 0),
      }));

      const worksheet = XLSX.utils.json_to_sheet(rows);
      const workbook = XLSX.utils.book_new();
      XLSX.utils.book_append_sheet(workbook, worksheet, "Passageiros");

      const routePart = normalizeFilePart(selectedRoute?.name ?? "rota");
      const fileName = `passageiros-${routePart || "rota"}-${buildExportTimestamp()}.xlsx`;
      XLSX.writeFile(workbook, fileName);
    } catch (error) {
      setExportError(toErrorMessage(error, "Nao foi possivel exportar os passageiros para .xlsx."));
    } finally {
      setExportingPassengers(false);
    }
  };

  return (
    <section className="page">
      <PageHeader
        title="Viagens"
        subtitle="Selecione uma rota para ver os passageiros embarcados."
        meta={<span className="badge">Gestao</span>}
      />

      {loadError ? <InlineAlert tone="error">{loadError}</InlineAlert> : null}

      <div className="section">
        <div className="route-admin-header">
          <SearchToolbar value={search} onChange={setSearch} resultCount={routes.length} />
          <button className="button sm" type="button" onClick={openCreateModal}>
            Nova viagem
          </button>
        </div>
        <DataTable
          columns={routeColumns}
          rows={routes}
          rowKey={(r) => r.id}
          actions={(r) => (
            <button
              className={`button sm ${selectedRouteId === r.id ? "primary" : "secondary"}`}
              type="button"
              onClick={() => {
                setSelectedRouteId(selectedRouteId === r.id ? null : r.id);
                setPassengerSearch("");
              }}
            >
              {selectedRouteId === r.id ? "Ocultar" : "Ver passageiros"}
            </button>
          )}
          emptyState={
            routesQuery.isLoading ? (
              <EmptyState title="Carregando rotas ativas" description="Aguarde alguns segundos." />
            ) : (
              <EmptyState title="Nenhuma rota ativa encontrada" description="Tente ajustar os filtros." />
            )
          }
        />
      </div>

      {selectedRouteId ? (
        <div className="section">
          <div className="section-header">
            <div>
              <div className="section-title">Passageiros</div>
              {selectedRoute ? (
                <div className="page-subtitle" style={{ marginTop: "2px" }}>
                  {selectedRoute.name} - {selectedRoute.origin_city} {"->"} {selectedRoute.destination_city}
                </div>
              ) : null}
            </div>
          </div>

          {consolidatedTotals && !detailsBatchQuery.isLoading ? (
            <div className="trip-kpi-grid" style={{ marginBottom: "12px" }}>
              <div className="card">
                <div className="section-title">Passageiros</div>
                <div className="page-title">{consolidatedTotals.passengers_count}</div>
              </div>
              <div className="card">
                <div className="section-title">Total pago</div>
                <div className="page-title">{formatCurrency(consolidatedTotals.paid_amount)}</div>
              </div>
              <div className="card">
                <div className="section-title">Total pendente</div>
                <div className="page-title">{formatCurrency(consolidatedTotals.due_amount)}</div>
              </div>
            </div>
          ) : null}

          {detailsBatchQuery.isLoading ? (
            <LoadingState label="Carregando passageiros..." />
          ) : detailsBatchQuery.error ? (
            <InlineAlert tone="error">
              {(detailsBatchQuery.error as Error).message || "Erro ao carregar passageiros."}
            </InlineAlert>
          ) : routeTripIds.length > 0 ? (
            <>
              {exportError ? <InlineAlert tone="error">{exportError}</InlineAlert> : null}
              <div style={{ marginBottom: "12px" }}>
                <div className="route-admin-header">
                  <SearchToolbar
                    value={passengerSearch}
                    onChange={setPassengerSearch}
                    placeholder="Buscar por nome, documento, embarque ou desembarque"
                    inputLabel="Buscar passageiros"
                    resultCount={visiblePassengers.length}
                  />
                  <button
                    className="button secondary sm"
                    type="button"
                    onClick={() => void exportPassengersXlsx()}
                    disabled={visiblePassengers.length === 0 || exportingPassengers}
                  >
                    {exportingPassengers ? "Exportando..." : "Exportar .xlsx"}
                  </button>
                </div>
              </div>
              <DataTable
                columns={passengerColumns}
                rows={visiblePassengers}
                rowKey={(p) => [p.passenger_id, p.booking_id, p.origin_stop_id, p.destination_stop_id].join("::")}
                emptyState={
                  <EmptyState
                    title="Nenhum passageiro encontrado"
                    description={
                      passengerSearch
                        ? "Ajuste o filtro para localizar passageiros."
                        : "Esta rota ainda nao tem passageiros cadastrados."
                    }
                  />
                }
              />
            </>
          ) : (
            <EmptyState title="Sem viagens para esta rota" description="Nenhuma viagem encontrada para esta rota." />
          )}
        </div>
      ) : null}

      <Modal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Nova viagem"
        size="lg"
        footer={
          <>
            <button className="button secondary" type="button" onClick={() => setCreateOpen(false)} disabled={createSaving}>
              Fechar
            </button>
            <button className="button" type="button" onClick={() => void saveTrip()} disabled={createSaving}>
              {createSaving ? "Salvando..." : "Criar viagem"}
            </button>
          </>
        }
      >
        {createError ? <InlineAlert tone="error">{createError}</InlineAlert> : null}
        <InlineAlert tone="info">
          Selecione a rota e defina dia+horario de passagem por parada.
        </InlineAlert>

        <label className="field">
          <span className="field-label">Rota</span>
          <select
            className="input"
            value={createRouteId}
            onChange={(event) => {
              setCreateRouteId(event.target.value);
              setCreateStopDateTimes({});
            }}
          >
            <option value="">Selecione a rota</option>
            {createRoutes.map((route) => (
              <option key={route.id} value={route.id}>
                {route.name} ({route.origin_city} {"->"} {route.destination_city})
              </option>
            ))}
          </select>
        </label>

        <label className="field">
          <span className="field-label">Onibus</span>
          {busOptions.length > 0 ? (
            <select className="input" value={createBusID} onChange={(event) => setCreateBusID(event.target.value)}>
              <option value="">Selecione o onibus</option>
              {busOptions.map((bus) => (
                <option key={bus.value} value={bus.value}>
                  {bus.label}
                </option>
              ))}
            </select>
          ) : (
            <input
              className="input"
              type="text"
              placeholder="Informe o ID do onibus (ex.: schumacher_01)"
              value={createBusID}
              onChange={(event) => setCreateBusID(event.target.value)}
            />
          )}
        </label>
        {busesQuery.error ? (
          <InlineAlert tone="warning">
            Cadastro de onibus indisponivel neste banco. Usando fallback por IDs de viagens existentes ou entrada manual.
          </InlineAlert>
        ) : null}

        <label className="field">
          <span className="field-label">Data base (opcional para preenchimento rapido)</span>
          <input
            className="input"
            type="date"
            value={createTripDate}
            onChange={(event) => {
              const nextDate = event.target.value;
              setCreateTripDate(nextDate);
              if (!nextDate) return;
              setCreateStopDateTimes((previous) => {
                const next: Record<string, string> = {};
                for (const stop of createRouteStops) {
                  const current = previous[stop.id] ?? "";
                  const timePart = current.includes("T") ? current.split("T")[1] : "";
                  next[stop.id] = timePart ? `${nextDate}T${timePart}` : `${nextDate}T08:00`;
                }
                return next;
              });
            }}
          />
        </label>

        {createRouteId ? (
          createRouteStopsQuery.isLoading ? (
            <LoadingState label="Carregando paradas da rota..." />
          ) : createRouteStopsQuery.error ? (
            <InlineAlert tone="error">
              {(createRouteStopsQuery.error as Error).message || "Erro ao carregar paradas da rota."}
            </InlineAlert>
          ) : createRouteStops.length > 0 ? (
            <div className="trip-stops-table-wrap">
              <table className="segment-prices-table">
                <thead>
                  <tr>
                    <th>Ordem</th>
                    <th>Parada</th>
                    <th>Dia e horario</th>
                  </tr>
                </thead>
                <tbody>
                  {createRouteStops.map((stop) => (
                    <tr key={stop.id}>
                      <td>{stop.stop_order}</td>
                      <td>{stop.city}</td>
                      <td>
                        <input
                          className="input trip-create-stop-datetime"
                          type="datetime-local"
                          value={createStopDateTimes[stop.id] ?? ""}
                          onChange={(event) =>
                            setCreateStopDateTimes((previous) => ({
                              ...previous,
                              [stop.id]: event.target.value,
                            }))
                          }
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <EmptyState title="Sem paradas na rota" description="Adicione paradas na rota antes de criar a viagem." />
          )
        ) : (
          <EmptyState title="Selecione uma rota" description="Escolha a rota para definir os horarios por parada." />
        )}
      </Modal>
    </section>
  );
}
