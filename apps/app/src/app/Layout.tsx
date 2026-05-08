import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import {
  LayoutDashboard,
  Route,
  MapPin,
  Ticket,
  Bus,
  IdCard,
  CreditCard,
  Tag,
  BarChart3,
  Wallet,
  HandCoins,
  Users,
  Box,
  MessageSquare,
  ChevronDown,
  Moon,
  Sun,
} from "lucide-react";
import { ToastProvider } from "../components/state/ToastProvider";
import Drawer from "../components/overlay/Drawer";
import ConfirmDialog from "../components/overlay/ConfirmDialog";
import { getSupabaseClient } from "../services/supabase";
import { useCurrentUser } from "../hooks/useCurrentUser";

const baseNavItems = [
  { path: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { path: "/trips", label: "Viagens", icon: Route },
  { path: "/routes", label: "Rotas", icon: MapPin },
  { path: "/bookings", label: "Reservas", icon: Ticket },
  { path: "/atendimentos", label: "Atendimentos", icon: MessageSquare },
  { path: "/users", label: "Usuarios", icon: Users },
];

const legacyNavItems = [
  { path: "/buses", label: "Onibus", icon: Bus },
  { path: "/drivers", label: "Motoristas", icon: IdCard },
  { path: "/payments", label: "Pagamentos", icon: CreditCard },
  { path: "/pricing", label: "Tarifas", icon: Tag },
  { path: "/reports", label: "Relatorios", icon: BarChart3 },
  { path: "/financial", label: "Financeiro", icon: Wallet },
  { path: "/warehouse", label: "Almoxarifado", icon: Box },
];

const THEME_STORAGE_KEY = "app-theme";
type AppTheme = "light" | "dark";

const SIDEBAR_STORAGE_KEY = "sidebar-collapsed";

export default function Layout({ children }: { children: ReactNode }) {
  const legacyMode = (import.meta.env.VITE_LEGACY_MODE ?? "false").toLowerCase() === "true";
  const currentUserQuery = useCurrentUser();
  const canAccessSaldo = currentUserQuery.data?.can_access_saldo ?? false;
  const navItems = useMemo(() => {
    const items = legacyMode ? [...baseNavItems, ...legacyNavItems] : [...baseNavItems];
    if (canAccessSaldo) {
      items.push({ path: "/saldo", label: "Saldo", icon: HandCoins });
    }
    return items;
  }, [legacyMode, canAccessSaldo]);

  const [menuOpen, setMenuOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    if (typeof window === "undefined") return false;
    const saved = window.localStorage.getItem(SIDEBAR_STORAGE_KEY);
    return saved === "true";
  });
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [signOutOpen, setSignOutOpen] = useState(false);
  const [theme, setTheme] = useState<AppTheme>(() => {
    if (typeof window === "undefined") return "light";
    const saved = window.localStorage.getItem(THEME_STORAGE_KEY);
    return saved === "dark" ? "dark" : "light";
  });
  const location = useLocation();
  const isAtendimentosRoute = location.pathname.startsWith("/atendimentos");

  const currentItem = useMemo(() => {
    return navItems.find((item) => location.pathname.startsWith(item.path));
  }, [location.pathname]);

  const quickAction = useMemo(() => {
    const path = currentItem?.path ?? "";
    const map: Record<string, { label: string; to: string }> = {
      "/bookings": { label: "Nova reserva", to: "/bookings#booking-form" },
      "/trips": { label: "Nova viagem", to: "/trips#crud-form" },
      "/routes": { label: "Nova rota", to: "/routes#crud-form" },
    };
    return map[path] ?? null;
  }, [currentItem]);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  }, [theme]);

  useEffect(() => {
    window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(sidebarCollapsed));
  }, [sidebarCollapsed]);

  const handleSignOut = () => {
    setSignOutOpen(true);
  };

  const confirmSignOut = () => {
    const client = getSupabaseClient();
    client?.auth.signOut();
    setSignOutOpen(false);
  };

  const toggleSidebar = () => {
    setSidebarCollapsed((prev) => !prev);
  };

  return (
    <ToastProvider>
      <a href="#main-content" className="skip-link">
        Pular para o conteudo principal
      </a>
      <div className={`app-shell ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}>
        <div
          className={`sidebar-overlay ${menuOpen ? "open" : ""}`}
          onClick={() => setMenuOpen(false)}
          aria-hidden={!menuOpen}
        />
        <aside id="app-sidebar" className={`sidebar ${menuOpen ? "open" : ""}`} aria-label="Menu principal">
          <div className="sidebar-header">
            <div className="sidebar-brand">
              <div className="brand">Schumacher Turismo</div>
              <div className="page-subtitle">Sistema Interno</div>
            </div>
            <div className="sidebar-actions">
              <button
                className="icon-button"
                type="button"
                aria-label={theme === "dark" ? "Ativar tema claro" : "Ativar tema escuro"}
                onClick={() => setTheme((prev) => (prev === "light" ? "dark" : "light"))}
                title={theme === "dark" ? "Ativar tema claro" : "Ativar tema escuro"}
              >
                {theme === "dark" ? <Sun size={18} /> : <Moon size={18} />}
              </button>
              <button
                className="icon-button"
                type="button"
                onClick={toggleSidebar}
                aria-label={sidebarCollapsed ? "Expandir menu" : "Recolher menu"}
                title={sidebarCollapsed ? "Expandir menu" : "Recolher menu"}
              >
                <ChevronDown
                  size={18}
                  style={{
                    transform: sidebarCollapsed ? "rotate(-90deg)" : "rotate(90deg)",
                    transition: "transform 0.2s ease",
                  }}
                />
              </button>
            </div>
          </div>
          <nav className="nav">
            {navItems.map((item) => (
              <NavLink
                key={item.path}
                to={item.path}
                className={({ isActive }) => (isActive ? "nav-link active" : "nav-link")}
                onClick={() => setMenuOpen(false)}
              >
                <item.icon className="nav-icon" size={18} aria-hidden="true" />
                <span>{item.label}</span>
              </NavLink>
            ))}
          </nav>
          <button className="button button-outline" onClick={handleSignOut}>
            Sair
          </button>
        </aside>
        <main
          id="main-content"
          className={`main ${isAtendimentosRoute ? "main-atendimentos" : ""}`}
          tabIndex={-1}
        >
          {children}
        </main>
        <button
          className="menu-button-mobile"
          type="button"
          onClick={() => setMenuOpen(true)}
          aria-label="Abrir menu"
        >
          <LayoutDashboard size={24} />
        </button>
      </div>
      <Drawer
        open={shortcutsOpen}
        title="Atalhos rapidos"
        description="Acesse as areas mais usadas do sistema."
        onClose={() => setShortcutsOpen(false)}
      >
        <div className="shortcut-list">
          {navItems.map((item) => (
            <Link
              key={item.path}
              className="shortcut-item"
              to={item.path}
              onClick={() => setShortcutsOpen(false)}
            >
              <item.icon size={16} aria-hidden="true" />
              <span>{item.label}</span>
            </Link>
          ))}
        </div>
      </Drawer>
      <ConfirmDialog
        open={signOutOpen}
        title="Encerrar sessao"
        description="Deseja realmente sair do sistema?"
        confirmLabel="Sair"
        tone="danger"
        onCancel={() => setSignOutOpen(false)}
        onConfirm={confirmSignOut}
      />
    </ToastProvider>
  );
}
