import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";

import { Layout } from "@/components/Layout";
import { RequireAuth } from "@/components/RequireAuth";
import { CapabilitiesPage } from "@/pages/CapabilitiesPage";
import { DataPage } from "@/pages/DataPage";
import { LayersPage } from "@/pages/LayersPage";
import { LoginPage } from "@/pages/LoginPage";
import { MapPage } from "@/pages/MapPage";
import { StylesPage } from "@/pages/StylesPage";
import { UsersPage } from "@/pages/UsersPage";

// OpenLayers 体积较大，仅在访问投影页时按需加载。
const ProjectionPage = lazy(() =>
  import("@/pages/ProjectionPage").then((m) => ({ default: m.ProjectionPage }))
);

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<Navigate to="/map" replace />} />
        <Route path="/map" element={<MapPage />} />
        <Route
          path="/projection"
          element={
            <Suspense
              fallback={
                <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                  正在加载投影视图…
                </div>
              }
            >
              <ProjectionPage />
            </Suspense>
          }
        />
        <Route path="/layers" element={<LayersPage />} />
        <Route path="/data" element={<DataPage />} />
        <Route path="/styles" element={<StylesPage />} />
        <Route path="/capabilities" element={<CapabilitiesPage />} />
        <Route path="/users" element={<UsersPage />} />
        <Route path="*" element={<Navigate to="/map" replace />} />
      </Route>
    </Routes>
  );
}
