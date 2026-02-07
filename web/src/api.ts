// API 客户端：与 Go 后端通信

export interface DaemonStatus {
  state: "stopped" | "connecting" | "connected" | "reconnecting" | "error";
  connectedAt?: string;
  uptime?: string;
  reconnects: number;
  lastError?: string;
  computer?: string;
  browser?: string;
}

export interface StatusResponse {
  daemon: DaemonStatus;
  configured: boolean;
  configPath: string;
}

export interface Config {
  agent: { address: string; token: string };
  computer: { id: string; description: string; allowedPaths: string[] };
  browser: { id: string; description: string; port: number };
  web: { port: number };
}

export interface LogEntry {
  time: string;
  level: "DEBUG" | "INFO" | "WARN" | "ERROR";
  module: string;
  message: string;
}

const BASE = "";

export async function getStatus(): Promise<StatusResponse> {
  const res = await fetch(`${BASE}/api/status`);
  return res.json();
}

export async function getConfig(): Promise<Config> {
  const res = await fetch(`${BASE}/api/config`);
  return res.json();
}

export async function putConfig(cfg: Config): Promise<void> {
  await fetch(`${BASE}/api/config`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(cfg),
  });
}

export async function getLogs(): Promise<{ entries: LogEntry[] }> {
  const res = await fetch(`${BASE}/api/logs`);
  return res.json();
}

export function subscribeToLogs(
  onEntry: (entry: LogEntry) => void,
  onError?: () => void
): () => void {
  const es = new EventSource(`${BASE}/api/logs/stream`);
  es.onmessage = (e) => {
    try {
      const entry: LogEntry = JSON.parse(e.data);
      onEntry(entry);
    } catch {
      // 忽略解析错误
    }
  };
  es.onerror = () => {
    onError?.();
  };
  return () => es.close();
}
