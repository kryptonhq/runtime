import { useState } from "react";
import { Button, Card, Input } from "../components/ui";
import { config } from "../api";

export default function Settings() {
  const [api, setApi] = useState(config.getApiBase());
  const [gw, setGw] = useState(config.getGatewayBase());
  const [saved, setSaved] = useState(false);

  function save() {
    config.setApiBase(api.trim());
    config.setGatewayBase(gw.trim());
    setSaved(true);
    setTimeout(() => setSaved(false), 1500);
  }

  return (
    <div className="max-w-xl space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-slate-500 mt-1">
          Override default endpoints (empty = same-origin). Stored in
          localStorage.
        </p>
      </div>
      <Card className="space-y-4">
        <div>
          <label className="text-xs text-slate-500">
            Control plane API base
          </label>
          <Input
            placeholder="(same-origin)"
            value={api}
            onChange={(e) => setApi(e.target.value)}
          />
          <p className="mt-1 text-xs text-slate-500">
            E.g. <code className="font-mono">http://localhost:8090</code>
          </p>
        </div>
        <div>
          <label className="text-xs text-slate-500">Gateway base URL</label>
          <Input
            placeholder="(same-origin)"
            value={gw}
            onChange={(e) => setGw(e.target.value)}
          />
          <p className="mt-1 text-xs text-slate-500">
            E.g. <code className="font-mono">http://localhost:8080</code>
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button onClick={save}>Save</Button>
          {saved && (
            <span className="text-xs text-emerald-600 dark:text-emerald-300">
              Saved
            </span>
          )}
        </div>
      </Card>
    </div>
  );
}
