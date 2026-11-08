import { Link } from "react-router-dom";
import { ArrowRight, Activity, MapPin } from "lucide-react";
import type { Facility } from "../types";

export function FacilityCard({ facility }: { facility: Facility }) {
  const statusLabels: Record<string, { label: string; badgeClass: string }> = {
    active: {
      label: "Active",
      badgeClass: "bg-emerald-50 text-emerald-700 border-emerald-200",
    },
    maintenance: {
      label: "Maintenance",
      badgeClass: "bg-amber-50 text-amber-700 border-amber-200",
    },
    closed: {
      label: "Closed",
      badgeClass: "bg-gray-100 text-gray-600 border-gray-200",
    },
  };

  const statusInfo = statusLabels[facility.status] ?? {
    label: facility.status,
    badgeClass: "bg-gray-100 text-gray-600 border-gray-200",
  };

  return (
    <Link
      to={`/facilities/${facility.slug}`}
      className="group flex flex-col justify-between rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-gold)]"
    >
      <div>
        <div className="flex items-center justify-between">
          <Activity size={22} className="text-[var(--color-maroon)]" />
          <span
            className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${statusInfo.badgeClass}`}
          >
            {statusInfo.label}
          </span>
        </div>
        <p className="mt-2.5 text-sm font-semibold text-[var(--color-navy)]">
          {facility.name}
        </p>
        {facility.location ? (
          <p className="mt-1 flex items-center gap-1 text-xs text-[var(--color-navy-soft)]">
            <MapPin size={11} className="flex-shrink-0" />
            <span className="truncate">{facility.location}</span>
          </p>
        ) : (
          <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
            {facility.schedules.length === 1
              ? "1 weekly window"
              : `${facility.schedules.length} weekly windows`}
          </p>
        )}
      </div>
      <div className="mt-3 flex items-center gap-1 text-xs font-medium text-[var(--color-maroon)]">
        View Schedule
        <ArrowRight
          size={13}
          className="transition-transform group-hover:translate-x-0.5"
        />
      </div>
    </Link>
  );
}
