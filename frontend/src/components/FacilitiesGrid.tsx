import type { Facility } from "../types";
import { FacilityCard } from "./FacilityCard";

export function FacilitiesGrid({ facilities }: { facilities: Facility[] }) {
  return (
    <section id="facilities">
      <h2 className="font-display text-lg font-semibold text-[var(--color-navy)]">
        Explore facilities
      </h2>
      <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        {facilities.map((f) => (
          <FacilityCard key={f.slug} facility={f} />
        ))}
      </div>
    </section>
  );
}
