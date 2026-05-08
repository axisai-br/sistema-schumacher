import type { AtendimentoTab } from "./sessionFilters";

type TabCounts = Record<AtendimentoTab, number>;

type AtendimentosRailProps = {
  activeTab: AtendimentoTab;
  tabCounts: TabCounts;
  onSelectTab: (tab: AtendimentoTab) => void;
};

const RAIL_ITEMS: Array<{ id: AtendimentoTab; label: string; helper: string }> = [
  { id: "NAO_ATENDIDO", label: "Nao atendido", helper: "Fila bot aguardando operador" },
  { id: "ABERTO", label: "Aberto", helper: "Conversas em posse humana" },
  { id: "FINALIZADO", label: "Finalizado", helper: "Atendimentos encerrados" },
];

export default function AtendimentosRail({ activeTab, tabCounts, onSelectTab }: AtendimentosRailProps) {
  return (
    <div className="atendimentos-rail" aria-label="Resumo de filas">
      {RAIL_ITEMS.map((item) => (
        <button
          key={item.id}
          type="button"
          className={`atendimentos-rail-item ${activeTab === item.id ? "active" : ""}`}
          onClick={() => onSelectTab(item.id)}
        >
          <span className="atendimentos-rail-kpi">{tabCounts[item.id]}</span>
          <span className="atendimentos-rail-label">{item.label}</span>
          <span className="atendimentos-rail-helper">{item.helper}</span>
        </button>
      ))}
    </div>
  );
}
