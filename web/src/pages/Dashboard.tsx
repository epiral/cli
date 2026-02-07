import { useEffect, useState } from "react";
import { getStatus, type StatusResponse } from "../api";
import { Link } from "react-router-dom";

const stateLabels: Record<string, string> = {
  connected: "已连接",
  connecting: "连接中...",
  reconnecting: "重连中...",
  stopped: "已停止",
  error: "错误",
};

const stateStyles: Record<string, string> = {
  connected: "text-emerald-400 bg-emerald-500/10 border-emerald-500/20",
  connecting: "text-amber-400 bg-amber-500/10 border-amber-500/20",
  reconnecting: "text-amber-400 bg-amber-500/10 border-amber-500/20",
  stopped: "text-zinc-400 bg-zinc-500/10 border-zinc-500/20",
  error: "text-red-400 bg-red-500/10 border-red-500/20",
};

export default function Dashboard() {
  const [status, setStatus] = useState<StatusResponse | null>(null);

  useEffect(() => {
    const load = () => getStatus().then(setStatus).catch(() => {});
    load();
    const id = setInterval(load, 2000);
    return () => clearInterval(id);
  }, []);

  if (!status) {
    return <div className="text-zinc-500">loading...</div>;
  }

  const { daemon, configured } = status;
  const state = daemon.state;

  // 未配置时显示引导
  if (!configured) {
    return (
      <div className="space-y-6">
        <h2 className="text-xl font-semibold">Welcome to Epiral CLI</h2>
        <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-6 space-y-4">
          <p className="text-zinc-300">
            首次使用，请先配置 Agent 连接信息。
          </p>
          <Link
            to="/config"
            className="inline-block px-4 py-2 rounded-md bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium transition-colors"
          >
            Go to Config
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">Dashboard</h2>

      {/* 状态卡片 */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {/* 连接状态 */}
        <Card title="Connection">
          <div
            className={`inline-block px-3 py-1 rounded-full text-sm font-medium border ${stateStyles[state] ?? stateStyles.stopped}`}
          >
            {stateLabels[state] ?? state}
          </div>
          {daemon.uptime && (
            <p className="mt-2 text-sm text-zinc-400">
              uptime: {daemon.uptime}
            </p>
          )}
          {daemon.lastError && (
            <p className="mt-1 text-sm text-red-400 truncate" title={daemon.lastError}>
              {daemon.lastError}
            </p>
          )}
        </Card>

        {/* Computer */}
        <Card title="Computer">
          {daemon.computer ? (
            <p className="text-zinc-200 font-mono">{daemon.computer}</p>
          ) : (
            <p className="text-zinc-500">-</p>
          )}
        </Card>

        {/* Browser */}
        <Card title="Browser">
          {daemon.browser ? (
            <p className="text-zinc-200 font-mono">{daemon.browser}</p>
          ) : (
            <p className="text-zinc-500">-</p>
          )}
        </Card>

        {/* 重连次数 */}
        <Card title="Reconnects">
          <p className="text-2xl font-mono text-zinc-200">
            {daemon.reconnects}
          </p>
        </Card>

        {/* 配置文件 */}
        <Card title="Config Path">
          <p className="text-sm text-zinc-400 font-mono truncate" title={status.configPath}>
            {status.configPath}
          </p>
        </Card>
      </div>
    </div>
  );
}

function Card({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-4">
      <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">
        {title}
      </h3>
      {children}
    </div>
  );
}
