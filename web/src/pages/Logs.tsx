import { useEffect, useRef, useState } from "react";
import { getLogs, subscribeToLogs, type LogEntry } from "../api";

const levels = ["ALL", "DEBUG", "INFO", "WARN", "ERROR"] as const;

const levelColors: Record<string, string> = {
  DEBUG: "text-zinc-500",
  INFO: "text-blue-400",
  WARN: "text-amber-400",
  ERROR: "text-red-400",
};

export default function Logs() {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [filter, setFilter] = useState<string>("ALL");
  const bottomRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  // 加载历史日志 + 订阅实时流
  useEffect(() => {
    let cancelled = false;

    getLogs().then((data) => {
      if (!cancelled) {
        setEntries(data.entries ?? []);
      }
    });

    const unsub = subscribeToLogs((entry) => {
      if (!cancelled) {
        setEntries((prev) => {
          const next = [...prev, entry];
          // 保留最近 2000 条
          return next.length > 2000 ? next.slice(-2000) : next;
        });
      }
    });

    return () => {
      cancelled = true;
      unsub();
    };
  }, []);

  // 自动滚动到底部
  useEffect(() => {
    if (autoScroll) {
      bottomRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [entries, autoScroll]);

  const filtered =
    filter === "ALL"
      ? entries
      : entries.filter((e) => e.level === filter);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Logs</h2>
        <div className="flex items-center gap-3">
          {/* 级别筛选 */}
          <div className="flex gap-1">
            {levels.map((l) => (
              <button
                key={l}
                onClick={() => setFilter(l)}
                className={`px-2.5 py-1 rounded text-xs font-medium transition-colors ${
                  filter === l
                    ? "bg-zinc-700 text-zinc-100"
                    : "text-zinc-500 hover:text-zinc-300"
                }`}
              >
                {l}
              </button>
            ))}
          </div>
          {/* 自动滚动 */}
          <button
            onClick={() => setAutoScroll(!autoScroll)}
            className={`px-2.5 py-1 rounded text-xs font-medium transition-colors ${
              autoScroll
                ? "bg-zinc-700 text-zinc-100"
                : "text-zinc-500 hover:text-zinc-300"
            }`}
          >
            Auto-scroll
          </button>
        </div>
      </div>

      {/* 日志列表 */}
      <div className="rounded-lg border border-zinc-800 bg-zinc-900 overflow-hidden">
        <div className="max-h-[calc(100vh-220px)] overflow-y-auto p-1 font-mono text-xs leading-relaxed">
          {filtered.length === 0 ? (
            <p className="p-4 text-zinc-500 text-center">no logs yet</p>
          ) : (
            filtered.map((entry, i) => (
              <div key={i} className="flex gap-2 px-3 py-0.5 hover:bg-zinc-800/50">
                <span className="text-zinc-600 shrink-0">
                  {new Date(entry.time).toLocaleTimeString("zh-CN", { hour12: false })}
                </span>
                <span
                  className={`shrink-0 w-11 text-right ${levelColors[entry.level] ?? "text-zinc-400"}`}
                >
                  {entry.level}
                </span>
                <span className="text-zinc-500 shrink-0">[{entry.module}]</span>
                <span className="text-zinc-300 break-all">{entry.message}</span>
              </div>
            ))
          )}
          <div ref={bottomRef} />
        </div>
      </div>
    </div>
  );
}
