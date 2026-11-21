import { Link, useNavigate } from "react-router-dom";
import { ArrowRight, Activity, Dumbbell } from "lucide-react";
import type { Facility } from "../types";

export type FacilityAccessModel = "padel" | "gym" | "timetable";

export function getFacilityAccessModel(facility: { slug: string; name: string }): FacilityAccessModel {
  const s = facility.slug.toLowerCase();
  const n = facility.name.toLowerCase();
  if (s.includes("padel") || n.includes("padel")) {
    return "padel";
  }
  if (
    s.includes("gym") ||
    s.includes("fitness") ||
    n.includes("gym") ||
    n.includes("fitness")
  ) {
    return "gym";
  }
  return "timetable";
}

export function FacilityCard({ facility }: { facility: Facility }) {
  const navigate = useNavigate();
  const accessModel = getFacilityAccessModel(facility);

  const handleCardClick = (e: React.MouseEvent<HTMLElement>) => {
    // If the click is inside an explicit interactive link or button (e.g., Book Court, Get Pass, or an anchor tag), don't intercept it
    const target = e.target as HTMLElement;
    if (target.closest("a, button")) {
      return;
    }
    navigate(`/facilities/${facility.slug}`);
  };

  const statusLabels: Record<string, { label: string; badgeClass: string }> = {
    active: {
      label: "Active",
      badgeClass: "bg-emerald-50 text-emerald-700 border-emerald-200",
    },
    reserved: {
      label: "Reserved",
      badgeClass: "bg-purple-50 text-purple-700 border-purple-200",
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
    <article
      data-testid={`facility-card-${facility.slug}`}
      onClick={handleCardClick}
      className="group flex flex-col justify-between rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-all hover:border-[var(--color-gold)] hover:shadow-md cursor-pointer"
    >
      <div>
        {/* Top header row: Category Icon + Status Badge (only if non-active) */}
        <div className="flex items-start justify-between gap-2">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-white border border-[var(--color-border)] text-[var(--color-maroon)] shadow-2xs">
            {accessModel === "padel" ? (
              <Activity size={20} className="text-[var(--color-maroon)]" />
            ) : accessModel === "gym" ? (
              <Dumbbell size={20} className="text-[var(--color-navy)]" />
            ) : (
              <Activity size={20} className="text-[var(--color-maroon)]" />
            )}
          </div>

          {facility.status !== "active" && (
            <span
              className={`rounded-full border px-2 py-0.5 text-[10px] font-medium ${statusInfo.badgeClass}`}
            >
              {statusInfo.label}
            </span>
          )}
        </div>

        {/* Facility Name (with clickable link to detail page) */}
        <Link
          to={`/facilities/${facility.slug}`}
          className="mt-3 block text-sm font-semibold text-[var(--color-navy)] hover:text-[var(--color-maroon)] transition-colors"
        >
          {facility.name}
        </Link>

        {accessModel === "gym" && (
          <p className="mt-1 text-[11px] font-medium text-[var(--color-navy-soft)]">
            New Gym (Pass) &bull; Old Gym (Free)
          </p>
        )}

        {facility.status === "reserved" && facility.reservedFor && (
          <p className="mt-1 text-[11px] font-medium text-purple-700 truncate">
            Reserved for: {facility.reservedFor}
          </p>
        )}

        {facility.status !== "active" && facility.statusNote && (
          <p className="mt-1 text-[11px] text-gray-500 line-clamp-1">
            {facility.statusNote}
          </p>
        )}
      </div>

      {/* Action Area: Context-specific based on access model */}
      <div className="mt-4 pt-3 border-t border-[var(--color-border)]/60">
        {accessModel === "padel" ? (
          <div className="flex items-center justify-between gap-2">
            <Link
              to={`/padel?facility=${encodeURIComponent(facility.slug)}`}
              className="inline-flex items-center gap-1 rounded-md bg-[var(--color-maroon)] px-2.5 py-1.5 text-xs font-semibold text-white transition-opacity hover:opacity-90"
            >
              Book Court
              <ArrowRight size={12} />
            </Link>
            <Link
              to={`/facilities/${facility.slug}`}
              className="text-xs font-medium text-[var(--color-navy-soft)] hover:text-[var(--color-maroon)]"
            >
              Schedule
            </Link>
          </div>
        ) : accessModel === "gym" ? (
          <div className="flex items-center justify-between gap-2">
            <Link
              to="/membership"
              className="inline-flex items-center gap-1 rounded-md bg-[var(--color-navy)] px-2.5 py-1.5 text-xs font-semibold text-white transition-opacity hover:opacity-90"
            >
              Get Pass
              <ArrowRight size={12} />
            </Link>
            <Link
              to={`/facilities/${facility.slug}`}
              className="text-xs font-medium text-[var(--color-navy-soft)] hover:text-[var(--color-maroon)]"
            >
              Schedule
            </Link>
          </div>
        ) : (
          <Link
            to={`/facilities/${facility.slug}`}
            className="flex items-center justify-between text-xs font-medium text-[var(--color-maroon)] hover:underline"
          >
            <span>View Schedule</span>
            <ArrowRight
              size={13}
              className="transition-transform group-hover:translate-x-0.5"
            />
          </Link>
        )}
      </div>
    </article>
  );
}
