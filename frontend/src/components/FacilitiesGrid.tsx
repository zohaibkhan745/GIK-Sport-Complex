import { useState } from "react";
import { Plus } from "lucide-react";
import type { Facility } from "../types";
import { FacilityCard } from "./FacilityCard";
import { CreateFacilityModal } from "./CreateFacilityModal";
import { useAuth } from "../context/AuthContext";

interface FacilitiesGridProps {
  facilities: Facility[];
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
  onFacilityCreated?: (facility: Facility) => void;
}

export function FacilitiesGrid({
  facilities,
  isLoading,
  error,
  onRetry,
  onFacilityCreated,
}: FacilitiesGridProps) {
  const { user } = useAuth();
  const canAddFacility = Boolean(
    user &&
      (user.roles.includes("sports_director") ||
        user.roles.includes("super_admin"))
  );
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);

  const handleCreated = (newFacility: Facility) => {
    if (onFacilityCreated) {
      onFacilityCreated(newFacility);
    }
  };

  return (
    <section id="facilities" aria-label="Campus Sports Facilities Directory">
      {/* Directory Title */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="font-display text-lg font-bold text-[var(--color-navy)]">
            Facilities
          </h2>
          {!isLoading && !error && (
            <span className="text-xs text-[var(--color-navy-soft)]">
              {facilities.length} {facilities.length === 1 ? "facility" : "facilities"}
            </span>
          )}
        </div>

        {canAddFacility && !isLoading && !error && (
          <button
            type="button"
            onClick={() => setIsCreateModalOpen(true)}
            data-testid="add-facility-btn"
            aria-label="Add new sports facility"
            className="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-maroon)] px-3.5 py-1.5 text-xs font-semibold text-white shadow-xs hover:opacity-90 transition-opacity cursor-pointer"
          >
            <Plus size={14} />
            <span>Add Facility</span>
          </button>
        )}
      </div>

      {/* Loading State */}
      {isLoading && (
        <div
          data-testid="facilities-loading"
          className="mt-4 flex items-center justify-center rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] py-12 text-sm text-[var(--color-navy-soft)]"
        >
          <div className="flex items-center gap-2">
            <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-maroon)] border-t-transparent" />
            Loading live facilities...
          </div>
        </div>
      )}

      {/* Error State */}
      {!isLoading && error && (
        <div
          data-testid="facilities-error"
          className="mt-4 rounded-xl border border-red-200 bg-red-50 p-6 text-center text-sm text-red-800"
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

      {/* Zero Total Facilities in Database */}
      {!isLoading && !error && facilities.length === 0 && (
        <div
          data-testid="facilities-empty"
          className="mt-4 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center text-sm text-[var(--color-navy-soft)]"
        >
          <p className="font-semibold text-[var(--color-navy)]">No facilities available</p>
          <p className="mt-1 text-xs">
            There are currently no sports facilities listed in the database.
          </p>
          {canAddFacility && (
            <button
              type="button"
              onClick={() => setIsCreateModalOpen(true)}
              className="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-maroon)] px-4 py-2 text-xs font-semibold text-white shadow-xs hover:opacity-90 transition-opacity cursor-pointer"
            >
              <Plus size={14} />
              <span>Add First Facility</span>
            </button>
          )}
        </div>
      )}

      {/* Populated Facilities Grid */}
      {!isLoading && !error && facilities.length > 0 && (
        <div
          data-testid="facilities-grid"
          className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
        >
          {facilities.map((f) => (
            <FacilityCard key={f.slug} facility={f} />
          ))}
        </div>
      )}

      {/* Create Facility Modal */}
      {canAddFacility && (
        <CreateFacilityModal
          isOpen={isCreateModalOpen}
          onClose={() => setIsCreateModalOpen(false)}
          onCreated={handleCreated}
        />
      )}
    </section>
  );
}
