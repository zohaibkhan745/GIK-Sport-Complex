import type { Facility } from "../types";
import { FacilityCard } from "./FacilityCard";

interface FacilitiesGridProps {
  facilities: Facility[];
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}

export function FacilitiesGrid({
  facilities,
  isLoading,
  error,
  onRetry,
}: FacilitiesGridProps) {
  return (
    <section id="facilities">
      <div className="flex items-center justify-between">
        <h2 className="font-display text-lg font-semibold text-[var(--color-navy)]">
          Explore facilities
        </h2>
        {!isLoading && !error && (
          <span className="text-xs text-[var(--color-navy-soft)]">
            {facilities.length} {facilities.length === 1 ? "facility" : "facilities"}
          </span>
        )}
      </div>

      {isLoading && (
        <div
          data-testid="facilities-loading"
          className="mt-3 flex items-center justify-center rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] py-12 text-sm text-[var(--color-navy-soft)]"
        >
          <div className="flex items-center gap-2">
            <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-maroon)] border-t-transparent" />
            Loading live facilities...
          </div>
        </div>
      )}

      {!isLoading && error && (
        <div
          data-testid="facilities-error"
          className="mt-3 rounded-xl border border-red-200 bg-red-50 p-6 text-center text-sm text-red-800"
        >
          <p className="font-semibold">Unable to load sports facilities</p>
          <p className="mt-1 text-xs text-red-600">{error}</p>
          <button
            type="button"
            onClick={onRetry}
            className="mt-4 rounded-md bg-[var(--color-maroon)] px-4 py-2 text-xs font-semibold text-white transition-opacity hover:opacity-90"
          >
            Retry
          </button>
        </div>
      )}

      {!isLoading && !error && facilities.length === 0 && (
        <div
          data-testid="facilities-empty"
          className="mt-3 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center text-sm text-[var(--color-navy-soft)]"
        >
          <p className="font-semibold text-[var(--color-navy)]">No facilities available</p>
          <p className="mt-1 text-xs">
            There are currently no sports facilities listed in the database.
          </p>
        </div>
      )}

      {!isLoading && !error && facilities.length > 0 && (
        <div
          data-testid="facilities-grid"
          className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4"
        >
          {facilities.map((f) => (
            <FacilityCard key={f.slug} facility={f} />
          ))}
        </div>
      )}
    </section>
  );
}
