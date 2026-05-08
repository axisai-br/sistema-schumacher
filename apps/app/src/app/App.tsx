import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import LoadingState from "../components/LoadingState";
import Layout from "./Layout";

const Dashboard = lazy(() => import("../pages/Dashboard"));
const Trips = lazy(() => import("../pages/Trips"));
const TripsLegacy = lazy(() => import("../pages/Trips/index_old"));
const TripDetailsPage = lazy(() => import("../pages/Trips/TripDetailsPage"));
const RoutesPage = lazy(() => import("../pages/Routes"));
const Bookings = lazy(() => import("../pages/Bookings"));
const Atendimentos = lazy(() => import("../pages/Atendimentos"));
const TripOperations = lazy(() => import("../pages/TripOperations"));
const Buses = lazy(() => import("../pages/Buses"));
const Drivers = lazy(() => import("../pages/Drivers"));
const Payments = lazy(() => import("../pages/Payments"));
const Reports = lazy(() => import("../pages/Reports"));
const Pricing = lazy(() => import("../pages/Pricing"));
const Financial = lazy(() => import("../pages/Financial"));
const Warehouse = lazy(() => import("../pages/Warehouse"));
const Saldo = lazy(() => import("../pages/Saldo"));
const Users = lazy(() => import("../pages/Users"));

export default function App() {
  const legacyMode = (import.meta.env.VITE_LEGACY_MODE ?? "false").toLowerCase() === "true";
  const tripsLegacyFallback =
    (import.meta.env.VITE_TRIPS_LEGACY_FALLBACK ?? "false").toLowerCase() === "true";

  return (
    <Layout>
      <Suspense
        fallback={
          <section className="page">
            <LoadingState label="Carregando modulo..." />
          </section>
        }
        >
        <Routes>
          <Route path="/" element={<Navigate to="/bookings" replace />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/trips" element={tripsLegacyFallback ? <TripsLegacy /> : <Trips />} />
          <Route path="/trips-legacy" element={<TripsLegacy />} />
          <Route path="/trips/:tripId" element={<TripDetailsPage />} />
          <Route path="/routes" element={<RoutesPage />} />
          <Route path="/bookings" element={<Bookings />} />
          <Route path="/atendimentos" element={<Atendimentos />} />
          <Route path="/saldo" element={<Saldo />} />
          <Route path="/users" element={<Users />} />
          {legacyMode ? (
            <>
              <Route path="/trips/:tripId/operations" element={<TripOperations />} />
              <Route path="/buses" element={<Buses />} />
              <Route path="/drivers" element={<Drivers />} />
              <Route path="/payments" element={<Payments />} />
              <Route path="/reports" element={<Reports />} />
              <Route path="/pricing" element={<Pricing />} />
              <Route path="/financial" element={<Financial />} />
              <Route path="/warehouse" element={<Warehouse />} />
              <Route path="/trip-advances" element={<Navigate to="/financial?tab=advances" replace />} />
              <Route path="/trip-expenses" element={<Navigate to="/financial?tab=expenses" replace />} />
              <Route path="/trip-settlements" element={<Navigate to="/financial?tab=settlements" replace />} />
              <Route path="/driver-cards" element={<Navigate to="/financial?tab=cards" replace />} />
              <Route path="/trip-validations" element={<Navigate to="/financial?tab=validations" replace />} />
              <Route path="/fiscal-documents" element={<Navigate to="/financial?tab=documents" replace />} />
            </>
          ) : null}
          <Route path="*" element={<Navigate to="/bookings" replace />} />
        </Routes>
      </Suspense>
    </Layout>
  );
}
