import { Link } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import type { Facility } from "../types";
import { FACILITY_ICONS } from "./icons";

export function FacilityCard({ facility }: { facility: Facility }) {
  const Icon = FACILITY_ICONS[facility.icon];

  return (
    <Link
      to={`/facilities/${facility.slug}`}
      className="group rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-gold)]"
    >
      <Icon size={22} className="text-[var(--color-maroon)]" />
      <p className="mt-2.5 text-sm font-semibold text-[var(--color-navy)]">
        {facility.shortName}
      </p>
      <p className="mt-0.5 text-xs text-[var(--color-navy-soft)]">
        {facility.tagline}
      </p>
      <div className="mt-3 flex items-center gap-1 text-xs font-medium text-[var(--color-maroon)]">
        View
        <ArrowRight
          size={13}
          className="transition-transform group-hover:translate-x-0.5"
        />
      </div>
    </Link>
  );
}
