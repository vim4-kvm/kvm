import { useEffect, useState } from "react";
import { Button } from "@components/Button";
import { SettingsItem } from "@components/SettingsItem";
import { m } from "@localizations/messages.js";

export function Vim4BitrateInput({ factor, disabled, onApply }: {
  factor: number;
  disabled: boolean;
  onApply: (factor: number) => void;
}) {
  const [kbps, setKbps] = useState(Math.round(factor * 12000));
  useEffect(() => setKbps(Math.round(factor * 12000)), [factor]);
  return <SettingsItem title={m.video_stream_quality_title()} description="1–60 Mbps (1000–60000 kbps)">
    <form className="flex items-center gap-2" onSubmit={event => {
      event.preventDefault();
      if (!disabled && Number.isInteger(kbps) && kbps >= 1000 && kbps <= 60000) onApply(kbps / 12000);
    }}>
      <input aria-label="kbps" type="number" min={1000} max={60000} step={1} required
        className="w-28 rounded border border-slate-300 bg-transparent px-2 py-1 dark:border-slate-700"
        value={Number.isNaN(kbps) ? "" : kbps} disabled={disabled}
        onChange={event => setKbps(event.target.valueAsNumber)} />
      <span>kbps</span>
      <Button type="submit" size="SM" theme="light" disabled={disabled}>{m.network_save_settings()}</Button>
    </form>
  </SettingsItem>;
}
