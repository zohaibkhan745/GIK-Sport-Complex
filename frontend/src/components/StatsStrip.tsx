import type { Stat } from "../types";

const STATS: Stat[] = [
  { value: "12", label: "National titles" },
  { value: "30+", label: "Tournaments hosted" },
  { value: "8", label: "Facilities" },
  { value: "1200+", label: "Active students" },
];

export function StatsStrip() {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {STATS.map((s) => (
        <div
          key={s.label}
          className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center"
        >
          <p className="font-display text-2xl font-semibold text-[var(--color-navy)]">
            {s.value}
          </p>
          <p className="mt-1 text-xs text-[var(--color-navy-soft)]">{s.label}</p>
        </div>
      ))}
    </div>
  );
}
