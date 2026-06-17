import "./RoleBadge.css";

// RoleBadge renders a neutral-surface badge for a user's role (admin / editor /
// reader). Per the UI-SPEC it uses the neutral surface, never the accent color.
export default function RoleBadge({ role }: { role: string }) {
  const label = role.charAt(0).toUpperCase() + role.slice(1);
  return <span className="role-badge">{label}</span>;
}
