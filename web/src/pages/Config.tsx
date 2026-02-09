import { useEffect, useState } from "react";
import { getConfig, putConfig, type Config as ConfigType } from "../api";

export default function Config() {
  const [config, setConfig] = useState<ConfigType | null>(null);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: "ok" | "error"; text: string } | null>(null);

  useEffect(() => {
    getConfig().then(setConfig).catch(() => {});
  }, []);

  if (!config) {
    return <div className="text-zinc-500">loading...</div>;
  }

  const handleSave = async () => {
    setSaving(true);
    setMessage(null);
    try {
      // 保存时清理空行和空格
      const cleaned = structuredClone(config);
      cleaned.computer.allowedPaths = (cleaned.computer.allowedPaths ?? [])
        .map((s) => s.trim())
        .filter(Boolean);
      await putConfig(cleaned);
      setConfig(cleaned);
      setMessage({ type: "ok", text: "saved! daemon restarting..." });
      setTimeout(() => setMessage(null), 3000);
    } catch {
      setMessage({ type: "error", text: "save failed" });
    } finally {
      setSaving(false);
    }
  };

  const update = (path: string, value: string | number | string[]) => {
    setConfig((prev) => {
      if (!prev) return prev;
      const next = structuredClone(prev);
      const keys = path.split(".");
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      let obj: any = next;
      for (let i = 0; i < keys.length - 1; i++) {
        obj = obj[keys[i]];
      }
      obj[keys[keys.length - 1]] = value;
      return next;
    });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Config</h2>
        <div className="flex items-center gap-3">
          {message && (
            <span
              className={`text-sm ${message.type === "ok" ? "text-emerald-400" : "text-red-400"}`}
            >
              {message.text}
            </span>
          )}
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-1.5 rounded-md bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-sm font-medium transition-colors"
          >
            {saving ? "saving..." : "Save & Restart"}
          </button>
        </div>
      </div>

      {/* Agent */}
      <Section title="Agent">
        <Field
          label="Address"
          placeholder="http://192.168.1.100:8002"
          value={config.agent.address}
          onChange={(v) => update("agent.address", v)}
        />
        <Field
          label="Token"
          placeholder="optional"
          value={config.agent.token}
          onChange={(v) => update("agent.token", v)}
          type="password"
        />
      </Section>

      {/* Computer */}
      <Section title="Computer">
        <Field
          label="ID"
          placeholder="my-pc"
          value={config.computer.id}
          onChange={(v) => update("computer.id", v)}
        />
        <Field
          label="Description"
          placeholder="optional"
          value={config.computer.description}
          onChange={(v) => update("computer.description", v)}
        />
        <div>
          <label className="block text-sm text-zinc-400 mb-1">
            Allowed Paths
          </label>
          <textarea
            className="w-full bg-zinc-800 border border-zinc-700 rounded-md px-3 py-2 text-sm text-zinc-200 font-mono focus:outline-none focus:border-zinc-500 resize-none"
            rows={3}
            placeholder="/home/user&#10;/tmp"
            value={(config.computer.allowedPaths ?? []).join("\n")}
            onChange={(e) =>
              update(
                "computer.allowedPaths",
                e.target.value.split("\n")
              )
            }
          />
          <p className="text-xs text-zinc-500 mt-1">one path per line</p>
        </div>
      </Section>

      {/* Web */}
      <Section title="Web Panel">
        <Field
          label="Port"
          placeholder="19800"
          value={String(config.web.port || "")}
          onChange={(v) => update("web.port", parseInt(v) || 0)}
          type="number"
        />
        <p className="text-xs text-zinc-500">
          port change takes effect on next restart
        </p>
      </Section>
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-5 space-y-4">
      <h3 className="text-sm font-medium text-zinc-300">{title}</h3>
      {children}
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  placeholder,
  type = "text",
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  type?: string;
}) {
  return (
    <div>
      <label className="block text-sm text-zinc-400 mb-1">{label}</label>
      <input
        type={type}
        className="w-full bg-zinc-800 border border-zinc-700 rounded-md px-3 py-2 text-sm text-zinc-200 font-mono focus:outline-none focus:border-zinc-500"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  );
}
