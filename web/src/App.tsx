import { Navigate, Route, Routes } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";

import Login from "./routes/Login";
import AppShell from "./routes/AppShell";
import { me, type Me } from "./api/client";

// useSession loads the current user via /auth/me. A 401 means unauthenticated.
function useSession() {
  return useQuery<Me>({
    queryKey: ["me"],
    queryFn: me,
  });
}

function RequireAuth({ children }: { children: React.ReactNode }) {
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
      <Route path="*" element={<Navigate to="/app" replace />} />
    </Routes>
  );
}
