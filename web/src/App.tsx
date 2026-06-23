import type { ReactNode } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";

import Login from "./routes/Login";
import AppShell from "./routes/AppShell";
import Admin from "./routes/Admin";
import Profile from "./routes/Profile";
import PageView from "./routes/PageView";
import PageEditor from "./routes/PageEditor";
import TrashView from "./components/TrashView";
import ForcePasswordChange from "./routes/ForcePasswordChange";
import { me, type Me } from "./api/client";

// useSession loads the current user via /auth/me. A 401 means unauthenticated.
function useSession() {
  return useQuery<Me>({
    queryKey: ["me"],
    queryFn: me,
  });
}

// RequireAuth gates a protected route. When the session reports
// must_change_password it renders the forced password-change view INSTEAD of
// the requested page — the flag is enforced from the server (/auth/me), so the
// gate cannot be skipped by navigating directly (D-02, T-00.03-04).
function RequireAuth({ children }: { children: ReactNode }) {
  const { data, isLoading, isError } = useSession();
  if (isLoading) {
    return (
      <div style={{ padding: "var(--space-2xl)", color: "var(--color-text-muted)" }}>
        Loading…
      </div>
    );
  }
  if (isError || !data) {
    return <Navigate to="/login" replace />;
  }
  if (data.must_change_password) {
    return <ForcePasswordChange />;
  }
  return <>{children}</>;
}

// RequireAdmin additionally enforces the admin role on the client (the server
// is the authority — a non-admin hitting the API gets 403 regardless). A
// non-admin is redirected to the app rather than shown the screen.
function RequireAdmin({ children }: { children: ReactNode }) {
  const { data, isLoading, isError } = useSession();
  if (isLoading) {
    return (
      <div style={{ padding: "var(--space-2xl)", color: "var(--color-text-muted)" }}>
        Loading…
      </div>
    );
  }
  if (isError || !data) {
    return <Navigate to="/login" replace />;
  }
  if (data.must_change_password) {
    return <ForcePasswordChange />;
  }
  if (data.role !== "admin") {
    return <Navigate to="/app" replace />;
  }
  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/app"
        element={
          <RequireAuth>
            <AppShell />
          </RequireAuth>
        }
      />
      <Route
        path="/app/page/*"
        element={
          <RequireAuth>
            <AppShell>
              <PageView />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/app/edit/*"
        element={
          <RequireAuth>
            <AppShell>
              <PageEditor />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/profile"
        element={
          <RequireAuth>
            <AppShell>
              <Profile />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/trash"
        element={
          <RequireAuth>
            <AppShell>
              <TrashView />
            </AppShell>
          </RequireAuth>
        }
      />
      <Route
        path="/admin"
        element={
          <RequireAdmin>
            <AppShell>
              <Admin />
            </AppShell>
          </RequireAdmin>
        }
      />
      <Route path="*" element={<Navigate to="/app" replace />} />
    </Routes>
  );
}
