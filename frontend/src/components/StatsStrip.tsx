import type { Stat } from "../types";

interface StatsStripProps {
  facilityCount?: number;
}

export function StatsStrip({ facilityCount }: StatsStripProps) {
  const stats: Stat[] = [
    { value: "12", label: "National titles" },
    { value: "30+", label: "Tournaments hosted" },
    {
      value: facilityCount !== undefined ? String(facilityCount) : "—",
      label: "Live facilities",
    },
    { value: "1200+", label: "Active students" },
  ];

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {stats.map((s) => (
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
