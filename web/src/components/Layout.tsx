import { NavLink, Outlet } from "react-router-dom";
import { useEffect, useState } from "react";
import { getStatus, type StatusResponse } from "../api";

const navItems = [
  { to: "/", label: "Dashboard" },
  { to: "/config", label: "Config" },
  { to: "/logs", label: "Logs" },
];

const stateColors: Record<string, string> = {
  connected: "bg-emerald-500",
  connecting: "bg-amber-500 animate-pulse",
  reconnecting: "bg-amber-500 animate-pulse",
  stopped: "bg-zinc-500",
  error: "bg-red-500",
};

export default function Layout() {
  const [status, setStatus] = useState<StatusResponse | null>(null);

  useEffect(() => {
    const fetch = () => getStatus().then(setStatus).catch(() => {});
    fetch();
    const id = setInterval(fetch, 3000);
    return () => clearInterval(id);
  }, []);

  const state = status?.daemon.state ?? "stopped";

  return (
    <div className="min-h-screen flex flex-col">
      {/* Header */}
      <header className="border-b border-zinc-800 px-6 py-3 flex items-center justify-between">
        <div className="flex items-center gap-6">
          <h1 className="text-lg font-semibold tracking-tight">Epiral CLI</h1>
          <nav className="flex gap-1">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === "/"}
                className={({ isActive }) =>
                  `px-3 py-1.5 rounded-md text-sm transition-colors ${
                    isActive
                      ? "bg-zinc-800 text-zinc-100"
                      : "text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50"
                  }`
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
        </div>
        <div className="flex items-center gap-2 text-sm text-zinc-400">
          <span className={`w-2 h-2 rounded-full ${stateColors[state] ?? "bg-zinc-500"}`} />
          <span className="capitalize">{state}</span>
        </div>
      </header>

      {/* Content */}
      <main className="flex-1 p-6 max-w-4xl mx-auto w-full">
        <Outlet />
      </main>
    </div>
  );
}
