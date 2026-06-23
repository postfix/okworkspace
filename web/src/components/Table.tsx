import type { ReactNode } from "react";
import "./Table.css";

export interface Column<T> {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
}

export interface TableProps<T> {
  columns: Column<T>[];
  rows: T[];
  rowKey: (row: T) => string | number;
  // actions renders the per-row action cell (e.g. Reset password / Deactivate).
  actions?: (row: T) => ReactNode;
  actionsHeader?: string;
}

// Table is a token-driven data table with an optional per-row actions column.
export default function Table<T>({
  columns,
  rows,
  rowKey,
  actions,
  actionsHeader = "Actions",
}: TableProps<T>) {
  return (
    <div className="table-container">
      <table className="table">
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c.key} className="table-th">
                {c.header}
              </th>
            ))}
            {actions && <th className="table-th table-th-actions">{actionsHeader}</th>}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={rowKey(row)} className="table-row">
              {columns.map((c) => (
                <td key={c.key} className="table-td">
                  {c.render(row)}
                </td>
              ))}
              {actions && <td className="table-td table-td-actions">{actions(row)}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
