import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import CRUDListPage, {
  type ColumnConfig,
  type FormFieldConfig,
  type VisibilityOption,
} from "../../components/layout/CRUDListPage";
import StatusBadge, { type StatusTone } from "../../components/StatusBadge";
import { apiGet, apiPatch, apiPost } from "../../services/api";
import { useBuses } from "../../hooks/useBuses";
import { useDrivers } from "../../hooks/useDrivers";
import { useRoutes } from "../../hooks/useRoutes";
import { formatDateTime, formatShortId } from "../../utils/format";
import { tripOperationalStatusLabel, tripStatusLabel } from "../../utils/labels";
import { TripWizard } from "./TripWizard";
import "./wizard.css";

type TripItem = {
  id: string;
  route_id: string;
  bus_id: string;
  driver_id?: string;
  request_id?: string;
  departure_at: string;
  status: string;
  operational_status: string;
  estimated_km: number;
};

type RouteItem = { id: string; name: string; origin_city: string; destination_city: string };
type BusItem = { id: string; name: string };
type DriverItem = { id: string; name: string };

type TripForm = {
  route_id: string;
  bus_id: string;
  driver_id: string;
  request_id: string;
  departure_at: string;
  estimated_km: number | "";
};

type LegacyTripsVariant = "toggle" | "wizard-first";

type LegacyTripsPageProps = {
  variant: LegacyTripsVariant;
};

const statusTone = (status: string): StatusTone => {
  if (status === "IN_PROGRESS") return "info";
  if (status === "SCHEDULED") return "success";
  if (status === "CANCELLED") return "danger";
  return "neutral";
};

export default function LegacyTripsPage({ variant }: LegacyTripsPageProps) {
  const [statusFilter, setStatusFilter] = useState("ALL");
  const [useWizard, setUseWizard] = useState(variant === "wizard-first");
  const routes = (useRoutes(200, 0).data as RouteItem[] | undefined) ?? [];
  const buses = (useBuses(200, 0).data as BusItem[] | undefined) ?? [];
  const drivers = (useDrivers(200, 0).data as DriverItem[] | undefined) ?? [];

  const routeLabel = (id: string) => {
    const route = routes.find((item) => item.id === id);
    return route ? `${route.origin_city} -> ${route.destination_city}` : formatShortId(id);
  };

  const busLabel = (id: string) => {
    const bus = buses.find((item) => item.id === id);
    return bus ? bus.name : formatShortId(id);
  };

  const driverOptions = useMemo(
    () => drivers.map((driver) => ({ label: driver.name, value: driver.id })),
    [drivers]
  );

  const formFields: FormFieldConfig<TripForm>[] = [
    {
      key: "route_id",
      label: "Rota",
      type: "select",
      required: true,
      options: [
        { label: "Selecione a rota", value: "" },
        ...routes.map((route) => ({
          label: `${route.origin_city} -> ${route.destination_city}`,
          value: route.id,
        })),
      ],
    },
    {
      key: "bus_id",
      label: "Onibus",
      type: "select",
      required: true,
      options: [
        { label: "Selecione o onibus", value: "" },
        ...buses.map((bus) => ({
          label: bus.name,
          value: bus.id,
        })),
      ],
    },
    {
      key: "driver_id",
      label: "Motorista",
      type: "select",
      options: [{ label: "Selecionar depois", value: "" }, ...driverOptions],
    },
    {
      key: "request_id",
      label: "Solicitacao (ID)",
      placeholder: "Opcional",
    },
    {
      key: "departure_at",
      label: "Data e hora de saida",
      type: "datetime",
      required: true,
    },
    {
      key: "estimated_km",
      label: "KM planejada",
      type: "number",
      inputProps: { min: 0, step: 0.1 },
    },
  ];

  const columns: ColumnConfig<TripItem>[] = [
    { label: "Rota", accessor: (item) => routeLabel(item.route_id) },
    { label: "Onibus", accessor: (item) => busLabel(item.bus_id) },
    { label: "Saida", accessor: (item) => formatDateTime(item.departure_at) },
    {
      label: "Status",
      render: (item) => (
        <StatusBadge tone={statusTone(item.status)}>
          {tripStatusLabel[item.status] ?? item.status}
        </StatusBadge>
      ),
    },
    {
      label: "Operacional",
      render: (item) => (
        <StatusBadge tone={item.operational_status === "CLOSED" ? "success" : "info"}>
          {tripOperationalStatusLabel[item.operational_status] ?? item.operational_status}
        </StatusBadge>
      ),
    },
  ];

  const visibilityOptions: VisibilityOption<TripItem>[] = [
    { label: "Ativas", value: "active", predicate: (item) => item.status !== "CANCELLED" },
    { label: "Canceladas", value: "cancelled", predicate: (item) => item.status === "CANCELLED" },
    { label: "Todas", value: "all", predicate: () => true },
  ];

  const handleWizardSubmit = async (formData: TripForm) => {
    await apiPost("/trips", {
      route_id: formData.route_id,
      bus_id: formData.bus_id,
      driver_id: formData.driver_id || undefined,
      request_id: formData.request_id || undefined,
      departure_at: new Date(formData.departure_at).toISOString(),
      estimated_km: formData.estimated_km === "" ? undefined : Number(formData.estimated_km),
      status: "SCHEDULED",
    });
    if (variant === "wizard-first") {
      setUseWizard(false);
    }
  };

  const wizardToggle = (
    <button className="button secondary" onClick={() => setUseWizard((value) => !value)} style={{ whiteSpace: "nowrap" }}>
      {useWizard ? "Usar Formulario Classico" : "Usar Wizard"}
    </button>
  );

  if (variant === "wizard-first") {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
        <div className="page-header">
          <div className="page-header-eyebrow">OPERACAO</div>
          <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between" }}>
            <div>
              <h1 className="page-header-title">Viagens</h1>
              <p className="page-header-subtitle">Cadastro de viagens, datas e vinculo com rotas e onibus.</p>
            </div>
            <span className="badge">Core</span>
          </div>
        </div>

        <div style={{ padding: "var(--space-5)", textAlign: "center" }}>
          {useWizard ? (
            <TripWizard onSubmit={handleWizardSubmit} onCancel={() => setUseWizard(false)} />
          ) : (
            <button
              className="button"
              onClick={() => setUseWizard(true)}
              style={{ fontSize: "var(--body-lg)", padding: "var(--space-3) var(--space-5)" }}
            >
              + Criar nova viagem
            </button>
          )}
        </div>

        <div className="section">
          <div className="section-header">
            <h3 className="section-title">Viagens cadastradas</h3>
          </div>
          <CRUDListPage<TripItem, TripForm>
            title=""
            subtitle=""
            hidePageHeader
            formTitle="Nova viagem"
            listTitle=""
            createLabel="Criar viagem"
            updateLabel="Salvar viagem"
            emptyState={{
              title: "Nenhuma viagem encontrada",
              description: "Tente ajustar a busca ou cadastre uma nova viagem.",
            }}
            formFields={formFields}
            columns={columns}
            initialForm={{ route_id: "", bus_id: "", driver_id: "", request_id: "", departure_at: "", estimated_km: "" }}
            mapItemToForm={(item) => ({
              route_id: item.route_id,
              bus_id: item.bus_id,
              driver_id: item.driver_id ?? "",
              request_id: item.request_id ?? "",
              departure_at: item.departure_at ? item.departure_at.slice(0, 16) : "",
              estimated_km: item.estimated_km ?? "",
            })}
            getId={(item) => item.id}
            fetchItems={async ({ page, pageSize, search }) => {
              const params = new URLSearchParams({ limit: String(pageSize), offset: String(page * pageSize) });
              if (statusFilter !== "ALL") params.set("status", statusFilter);
              if (search) params.set("search", search);
              return apiGet<TripItem[]>(`/trips?${params.toString()}`);
            }}
            createItem={(form) =>
              apiPost("/trips", {
                route_id: form.route_id,
                bus_id: form.bus_id,
                driver_id: form.driver_id || undefined,
                request_id: form.request_id || undefined,
                departure_at: new Date(form.departure_at).toISOString(),
                estimated_km: form.estimated_km === "" ? undefined : Number(form.estimated_km),
              })
            }
            updateItem={(id, form) =>
              apiPatch(`/trips/${id}`, {
                route_id: form.route_id,
                bus_id: form.bus_id,
                driver_id: form.driver_id || undefined,
                request_id: form.request_id || undefined,
                departure_at: new Date(form.departure_at).toISOString(),
                estimated_km: form.estimated_km === "" ? undefined : Number(form.estimated_km),
              })
            }
            softDeleteItem={(item) => apiPatch(`/trips/${item.id}`, { status: "CANCELLED" })}
            restoreItem={(item) => apiPatch(`/trips/${item.id}`, { status: "SCHEDULED" })}
            getIsActive={(item) => item.status !== "CANCELLED"}
            searchFilter={(item, term) => {
              const matchesStatus = statusFilter === "ALL" ? true : item.status === statusFilter;
              if (!matchesStatus) return false;
              if (!term) return true;
              const route = routeLabel(item.route_id).toLowerCase();
              const bus = busLabel(item.bus_id).toLowerCase();
              return route.includes(term) || bus.includes(term) || item.id.toLowerCase().includes(term);
            }}
            visibilityOptions={visibilityOptions}
            visibilityDefault="active"
            layout="stacked"
            queryKey={["trips", statusFilter]}
            serverSideSearch
            rowActions={(item) => (
              <Link className="button secondary sm" to={`/trips/${item.id}/operations`}>
                Operacoes
              </Link>
            )}
            extraFilters={
              <select className="input" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} aria-label="Filtrar status">
                <option value="ALL">Todos os status</option>
                <option value="SCHEDULED">Programada</option>
                <option value="IN_PROGRESS">Em andamento</option>
                <option value="COMPLETED">Concluida</option>
                <option value="CANCELLED">Cancelada</option>
              </select>
            }
          />
        </div>
      </div>
    );
  }

  return (
    <CRUDListPage<TripItem, TripForm>
      title="Viagens"
      subtitle="Cadastro de viagens, datas e vinculo com rotas e onibus."
      meta={<span className="badge">Core</span>}
      formTitle="Nova viagem"
      listTitle="Viagens cadastradas"
      createLabel="Criar viagem"
      updateLabel="Salvar viagem"
      emptyState={{
        title: "Nenhuma viagem encontrada",
        description: "Tente ajustar a busca ou cadastre uma nova viagem.",
      }}
      formFields={formFields}
      columns={columns}
      initialForm={{ route_id: "", bus_id: "", driver_id: "", request_id: "", departure_at: "", estimated_km: "" }}
      mapItemToForm={(item) => ({
        route_id: item.route_id,
        bus_id: item.bus_id,
        driver_id: item.driver_id ?? "",
        request_id: item.request_id ?? "",
        departure_at: item.departure_at ? item.departure_at.slice(0, 16) : "",
        estimated_km: item.estimated_km ?? "",
      })}
      getId={(item) => item.id}
      fetchItems={async ({ page, pageSize, search }) => {
        const params = new URLSearchParams({ limit: String(pageSize), offset: String(page * pageSize) });
        if (statusFilter !== "ALL") params.set("status", statusFilter);
        if (search) params.set("search", search);
        return apiGet<TripItem[]>(`/trips?${params.toString()}`);
      }}
      createItem={(form) =>
        apiPost("/trips", {
          route_id: form.route_id,
          bus_id: form.bus_id,
          driver_id: form.driver_id || undefined,
          request_id: form.request_id || undefined,
          departure_at: new Date(form.departure_at).toISOString(),
          estimated_km: form.estimated_km === "" ? undefined : Number(form.estimated_km),
        })
      }
      updateItem={(id, form) =>
        apiPatch(`/trips/${id}`, {
          route_id: form.route_id,
          bus_id: form.bus_id,
          driver_id: form.driver_id || undefined,
          request_id: form.request_id || undefined,
          departure_at: new Date(form.departure_at).toISOString(),
          estimated_km: form.estimated_km === "" ? undefined : Number(form.estimated_km),
        })
      }
      softDeleteItem={(item) => apiPatch(`/trips/${item.id}`, { status: "CANCELLED" })}
      restoreItem={(item) => apiPatch(`/trips/${item.id}`, { status: "SCHEDULED" })}
      getIsActive={(item) => item.status !== "CANCELLED"}
      searchFilter={(item, term) => {
        const matchesStatus = statusFilter === "ALL" ? true : item.status === statusFilter;
        if (!matchesStatus) return false;
        if (!term) return true;
        const route = routeLabel(item.route_id).toLowerCase();
        const bus = busLabel(item.bus_id).toLowerCase();
        return route.includes(term) || bus.includes(term) || item.id.toLowerCase().includes(term);
      }}
      visibilityOptions={visibilityOptions}
      visibilityDefault="active"
      layout="split"
      queryKey={["trips", statusFilter]}
      serverSideSearch
      rowActions={(item) => (
        <Link className="button secondary sm" to={`/trips/${item.id}/operations`}>
          Operacoes
        </Link>
      )}
      extraFilters={
        <>
          <select className="input" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} aria-label="Filtrar status">
            <option value="ALL">Todos os status</option>
            <option value="SCHEDULED">Programada</option>
            <option value="IN_PROGRESS">Em andamento</option>
            <option value="COMPLETED">Concluida</option>
            <option value="CANCELLED">Cancelada</option>
          </select>
          {wizardToggle}
        </>
      }
    />
  );
}
