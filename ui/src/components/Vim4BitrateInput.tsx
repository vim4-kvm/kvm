import { useEffect, useState } from "react";
import { Button } from "@components/Button";
import { SettingsItem } from "@components/SettingsItem";
import { SelectMenuBasic } from "@components/SelectMenuBasic";
import { m } from "@localizations/messages.js";

const presets = [1000, 1200, 2000, 3000, 5000, 6000, 8000, 12000, 20000, 30000, 40000, 60000];

export function Vim4BitrateInput({
  factor,
  disabled,
  onApply,
}: {
  factor: number;
  disabled: boolean;
  onApply: (factor: number) => void;
}) {
  const savedKbps = Math.round(factor * 12000);
  const [kbps, setKbps] = useState(savedKbps);
  // Completion also resets the draft when a failed request leaves factor unchanged.
  useEffect(() => setKbps(savedKbps), [savedKbps, disabled]);
  const valid = Number.isInteger(kbps) && kbps >= 1000 && kbps <= 60000;
  const options = presets.map(value => ({ value: String(value), label: `${value / 1000} Mbps` }));
  if (!presets.includes(kbps)) {
    options.push({ value: String(kbps), label: valid ? `${kbps / 1000} Mbps` : "—" });
  }
  return (
    <SettingsItem
      title={m.video_stream_quality_title()}
      description={m.video_stream_quality_description()}
      loading={disabled}
    >
      <form
        className="flex w-64 max-w-full flex-col gap-3"
        onSubmit={event => {
          event.preventDefault();
          if (!disabled && valid && kbps !== savedKbps) onApply(kbps / 12000);
        }}
      >
        <SelectMenuBasic
          size="SM"
          label=""
          disabled={disabled}
          value={String(kbps)}
          options={options}
          onChange={event => setKbps(Number(event.target.value))}
        />
        <div>
          <input
            aria-label="Bitrate (Mbps)"
            type="range"
            min={1000}
            max={60000}
            step={100}
            className="h-2 w-full cursor-pointer appearance-none rounded-lg bg-gray-200 disabled:opacity-50 dark:bg-gray-700"
            value={valid ? kbps : savedKbps}
            disabled={disabled}
            aria-valuetext={`${(valid ? kbps : savedKbps) / 1000} Mbps`}
            onChange={event => setKbps(event.target.valueAsNumber)}
          />
          <div className="flex justify-between text-xs text-slate-500 dark:text-slate-400">
            <span>1 Mbps</span>
            <span>60 Mbps</span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <input
            aria-label="kbps"
            type="number"
            min={1000}
            max={60000}
            step={1}
            required
            className="w-24 rounded border border-slate-300 bg-transparent px-2 py-1 text-sm dark:border-slate-700"
            value={Number.isNaN(kbps) ? "" : kbps}
            disabled={disabled}
            onChange={event => setKbps(event.target.valueAsNumber)}
          />
          <span className="text-xs text-slate-500 dark:text-slate-400">kbps</span>
          <Button
            type="submit"
            size="SM"
            theme="light"
            text={m.network_save_settings()}
            disabled={disabled || !valid || kbps === savedKbps}
          />
        </div>
      </form>
    </SettingsItem>
  );
}
