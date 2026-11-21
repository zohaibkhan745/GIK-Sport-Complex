import { useState, useEffect, useRef, type FormEvent } from "react";
import { X, Plus, AlertCircle, Sparkles, Building2 } from "lucide-react";
import { api, ApiError } from "../api";
import type { Facility, OperationalStatus } from "../types";

interface CreateFacilityModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated: (facility: Facility) => void;
}

export function slugifyText(text: string): string {
  return text
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\s-_/]/g, "")
    .replace(/[\s-_/]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export function CreateFacilityModal({
  isOpen,
  onClose,
  onCreated,
}: CreateFacilityModalProps) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [isCustomSlug, setIsCustomSlug] = useState(false);
  const [status, setStatus] = useState<OperationalStatus>("active");
  const [statusUntil, setStatusUntil] = useState("");
  const [statusNote, setStatusNote] = useState("");
  const [reservedFor, setReservedFor] = useState("");
  const [description, setDescription] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const nameInputRef = useRef<HTMLInputElement>(null);

  // Auto-generate slug when name changes unless customized
  useEffect(() => {
    if (!isCustomSlug) {
      setSlug(slugifyText(name));
    }
  }, [name, isCustomSlug]);

  // Focus name input when modal opens
  useEffect(() => {
    if (isOpen) {
      setName("");
      setSlug("");
      setIsCustomSlug(false);
      setStatus("active");
      setStatusUntil("");
      setStatusNote("");
      setReservedFor("");
      setDescription("");
      setErrorMessage(null);
      setIsSubmitting(false);

      const timeout = setTimeout(() => {
        nameInputRef.current?.focus();
      }, 50);
      return () => clearTimeout(timeout);
    }
  }, [isOpen]);

  // Close on Escape
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !isSubmitting) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, isSubmitting, onClose]);

  if (!isOpen) return null;

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setErrorMessage(null);

    const trimmedName = name.trim();
    if (!trimmedName) {
      setErrorMessage("Facility name is required.");
      nameInputRef.current?.focus();
      return;
    }

    const trimmedSlug = (slug.trim() || slugifyText(trimmedName)).toLowerCase();
    if (!trimmedSlug) {
      setErrorMessage("A valid slug is required. Please provide a facility name or slug.");
      return;
    }

    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(trimmedSlug)) {
      setErrorMessage("Slug must contain only lowercase letters, numbers, and hyphens (no trailing or double hyphens).");
      return;
    }

    const trimmedDesc = description.trim();
    if (trimmedDesc.length > 2000) {
      setErrorMessage("Description cannot exceed 2,000 characters.");
      return;
    }

    setIsSubmitting(true);

    try {
      let isoStatusUntil: string | null = null;
      if (statusUntil && status !== "active") {
        const d = new Date(statusUntil);
        if (!isNaN(d.getTime())) {
          isoStatusUntil = d.toISOString();
        }
      }

      const created = await api.createFacility({
        name: trimmedName,
        slug: trimmedSlug,
        status,
        statusUntil: status !== "active" ? (isoStatusUntil || null) : null,
        statusNote: statusNote.trim() || null,
        reservedFor: status === "reserved" ? (reservedFor.trim() || null) : null,
        description: trimmedDesc || null,
      });

      onCreated(created);
      onClose();
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
          setErrorMessage(`A facility with slug "${trimmedSlug}" already exists. Please customize the slug.`);
          setIsCustomSlug(true);
        } else if (err.status === 401) {
          setErrorMessage("Your session has expired. Please sign in again.");
        } else if (err.status === 403) {
          setErrorMessage("You do not have permission to add facilities (Director or Super Admin role required).");
        } else if (err.status === 400) {
          setErrorMessage(err.message || "Invalid facility details.");
        } else {
          setErrorMessage("Failed to create facility. Please try again.");
        }
      } else {
        setErrorMessage("Network error. Please check your connection and try again.");
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-facility-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-6"
    >
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/60 backdrop-blur-xs transition-opacity"
        onClick={!isSubmitting ? onClose : undefined}
      />

      {/* Modal Dialog Card */}
      <div className="relative w-full max-w-lg rounded-2xl border border-[var(--color-gold)]/40 bg-white p-6 shadow-2xl transition-all sm:p-8">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-gray-100 pb-4">
          <div className="flex items-center gap-3">
            <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-[var(--color-maroon)] text-[#F3E6C4] shadow-xs">
              <Building2 size={20} />
            </span>
            <div>
              <h2
                id="create-facility-title"
                className="font-display text-lg font-bold text-[var(--color-navy)]"
              >
                Add New Facility
              </h2>
              <p className="text-xs text-[var(--color-navy-soft)]">
                Create a sports facility and define its initial status.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            aria-label="Close dialog"
            className="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 transition-colors cursor-pointer"
          >
            <X size={18} />
          </button>
        </div>

        {/* Error Notification Banner */}
        {errorMessage && (
          <div
            data-testid="create-facility-error"
            className="mt-4 flex items-start gap-2.5 rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-800"
          >
            <AlertCircle size={16} className="mt-0.5 flex-shrink-0 text-red-600" />
            <div className="flex-1 font-medium leading-relaxed">{errorMessage}</div>
          </div>
        )}

        {/* Form */}
        <form onSubmit={handleSubmit} className="mt-5 space-y-4">
          {/* Facility Name */}
          <div>
            <label
              htmlFor="facility-name-input"
              className="block text-xs font-semibold text-[var(--color-navy)] uppercase tracking-wider"
            >
              Facility Name <span className="text-red-500">*</span>
            </label>
            <div className="mt-1.5 relative">
              <input
                ref={nameInputRef}
                id="facility-name-input"
                data-testid="create-facility-name-input"
                type="text"
                required
                maxLength={255}
                disabled={isSubmitting}
                placeholder="e.g. Squash Courts, Basketball Arena, Gymnasium"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-[var(--color-navy)] placeholder-gray-400 focus:border-[var(--color-maroon)] focus:outline-none focus:ring-1 focus:ring-[var(--color-maroon)]"
              />
            </div>
          </div>

          {/* Slug Configuration */}
          <div>
            <div className="flex items-center justify-between">
              <label
                htmlFor="facility-slug-input"
                className="block text-xs font-semibold text-[var(--color-navy)] uppercase tracking-wider"
              >
                URL Slug <span className="text-red-500">*</span>
              </label>
              {!isCustomSlug ? (
                <button
                  type="button"
                  onClick={() => setIsCustomSlug(true)}
                  className="text-[11px] font-medium text-[var(--color-maroon)] hover:underline cursor-pointer"
                >
                  Edit manually
                </button>
              ) : (
                <button
                  type="button"
                  onClick={() => {
                    setIsCustomSlug(false);
                    setSlug(slugifyText(name));
                  }}
                  className="inline-flex items-center gap-1 text-[11px] font-medium text-[var(--color-navy-soft)] hover:text-[var(--color-navy)] cursor-pointer"
                >
                  <Sparkles size={11} /> Auto-generate
                </button>
              )}
            </div>
            <div className="mt-1.5 flex items-center rounded-lg border border-gray-300 bg-gray-50 px-3 py-1.5 text-xs text-gray-500 focus-within:border-[var(--color-maroon)] focus-within:ring-1 focus-within:ring-[var(--color-maroon)] focus-within:bg-white">
              <span className="select-none font-mono text-gray-400">/facilities/</span>
              <input
                id="facility-slug-input"
                data-testid="create-facility-slug-input"
                type="text"
                required
                maxLength={100}
                disabled={isSubmitting || !isCustomSlug}
                value={slug}
                onChange={(e) => setSlug(e.target.value.toLowerCase())}
                placeholder="facility-slug"
                className="w-full bg-transparent font-mono text-xs text-[var(--color-navy)] focus:outline-none"
              />
            </div>
          </div>

          {/* Operational Status */}
          <div>
            <label className="block text-xs font-semibold text-[var(--color-navy)] uppercase tracking-wider">
              Operational Status <span className="text-red-500">*</span>
            </label>
            <div className="mt-1.5 grid grid-cols-2 sm:grid-cols-4 gap-2">
              <button
                type="button"
                data-testid="status-opt-active"
                onClick={() => setStatus("active")}
                className={`flex items-center justify-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors cursor-pointer ${
                  status === "active"
                    ? "border-emerald-600 bg-emerald-50 text-emerald-900 font-semibold ring-1 ring-emerald-600"
                    : "border-gray-200 bg-white text-gray-600 hover:bg-gray-50"
                }`}
              >
                <span className="h-2 w-2 rounded-full bg-emerald-500" />
                Active
              </button>
              <button
                type="button"
                data-testid="status-opt-reserved"
                onClick={() => setStatus("reserved")}
                className={`flex items-center justify-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors cursor-pointer ${
                  status === "reserved"
                    ? "border-purple-600 bg-purple-50 text-purple-900 font-semibold ring-1 ring-purple-600"
                    : "border-gray-200 bg-white text-gray-600 hover:bg-gray-50"
                }`}
              >
                <span className="h-2 w-2 rounded-full bg-purple-500" />
                Reserved
              </button>
              <button
                type="button"
                data-testid="status-opt-maintenance"
                onClick={() => setStatus("maintenance")}
                className={`flex items-center justify-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors cursor-pointer ${
                  status === "maintenance"
                    ? "border-amber-600 bg-amber-50 text-amber-900 font-semibold ring-1 ring-amber-600"
                    : "border-gray-200 bg-white text-gray-600 hover:bg-gray-50"
                }`}
              >
                <span className="h-2 w-2 rounded-full bg-amber-500" />
                Maintenance
              </button>
              <button
                type="button"
                data-testid="status-opt-closed"
                onClick={() => setStatus("closed")}
                className={`flex items-center justify-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors cursor-pointer ${
                  status === "closed"
                    ? "border-red-600 bg-red-50 text-red-900 font-semibold ring-1 ring-red-600"
                    : "border-gray-200 bg-white text-gray-600 hover:bg-gray-50"
                }`}
              >
                <span className="h-2 w-2 rounded-full bg-red-500" />
                Closed
              </button>
            </div>

            {/* Contextual Status Settings (Until time & note) */}
            <div className="mt-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3 space-y-3">
              {status === "reserved" && (
                <div>
                  <label
                    htmlFor="create-facility-reserved-for-input"
                    className="block text-xs font-semibold text-[var(--color-navy)] uppercase tracking-wider"
                  >
                    Reserved For <span className="text-gray-400 text-[10px] lowercase">(optional event / society name)</span>
                  </label>
                  <input
                    id="create-facility-reserved-for-input"
                    data-testid="create-facility-reserved-for-input"
                    type="text"
                    maxLength={255}
                    disabled={isSubmitting}
                    value={reservedFor}
                    onChange={(e) => setReservedFor(e.target.value)}
                    placeholder="e.g. ACM Gaming Tournament, GIKI Sports Gala, Badminton Trials"
                    className="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-xs text-[var(--color-navy)] placeholder-gray-400 focus:border-purple-600 focus:outline-none focus:ring-1 focus:ring-purple-600"
                  />
                </div>
              )}

              {/* Status Until (for non-active statuses) */}
              {status !== "active" && (
                <div>
                  <div className="flex items-center justify-between">
                    <label
                      htmlFor="create-facility-status-until-input"
                      className="block text-xs font-semibold text-[var(--color-navy)] uppercase tracking-wider"
                    >
                      {status === "reserved"
                        ? "Reserved Until"
                        : status === "maintenance"
                        ? "Maintenance Until"
                        : "Closed Until"}{" "}
                      <span className="text-gray-400 text-[10px] lowercase">(optional auto-revert to active)</span>
                    </label>
                    {statusUntil && (
                      <button
                        type="button"
                        onClick={() => setStatusUntil("")}
                        className="text-[10px] text-red-600 hover:underline cursor-pointer"
                      >
                        Clear time
                      </button>
                    )}
                  </div>
                  <input
                    id="create-facility-status-until-input"
                    data-testid="create-facility-status-until-input"
                    type="datetime-local"
                    disabled={isSubmitting}
                    value={statusUntil}
                    onChange={(e) => setStatusUntil(e.target.value)}
                    className="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none focus:ring-1 focus:ring-[var(--color-maroon)]"
                  />
                  <p className="mt-1 text-[11px] text-gray-500">
                    When this time completes, the facility status automatically reverts to <strong className="text-emerald-700">Active</strong>.
                  </p>
                </div>
              )}

              {/* Status Note (for all statuses) */}
              <div>
                <label
                  htmlFor="create-facility-status-note-input"
                  className="block text-xs font-semibold text-[var(--color-navy)] uppercase tracking-wider"
                >
                  Status Note / Reason <span className="text-gray-400 text-[10px] lowercase">(optional notice for campus users)</span>
                </label>
                <input
                  id="create-facility-status-note-input"
                  data-testid="create-facility-status-note-input"
                  type="text"
                  maxLength={500}
                  disabled={isSubmitting}
                  value={statusNote}
                  onChange={(e) => setStatusNote(e.target.value)}
                  placeholder={
                    status === "reserved"
                      ? "e.g. Courts reopen for regular student session at 18:00."
                      : status === "maintenance"
                      ? "e.g. Floor resurfacing and lighting fixture upgrade."
                      : status === "closed"
                      ? "e.g. Closed for semester break."
                      : "e.g. Operational for all students and faculty."
                  }
                  className="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-xs text-[var(--color-navy)] placeholder-gray-400 focus:border-[var(--color-maroon)] focus:outline-none focus:ring-1 focus:ring-[var(--color-maroon)]"
                />
              </div>
            </div>
          </div>

          {/* Description */}
          <div>
            <div className="flex items-center justify-between">
              <label
                htmlFor="facility-description-input"
                className="block text-xs font-semibold text-[var(--color-navy)] uppercase tracking-wider"
              >
                Description <span className="text-gray-400 text-[10px] lowercase">(optional)</span>
              </label>
              <span className="text-[11px] text-gray-400">
                {description.length} / 2,000
              </span>
            </div>
            <div className="mt-1.5">
              <textarea
                id="facility-description-input"
                data-testid="create-facility-description-input"
                rows={3}
                maxLength={2000}
                disabled={isSubmitting}
                placeholder="Provide a description of the facility, courts, gear rules, and features..."
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                className="w-full rounded-lg border border-gray-300 p-3 text-xs text-[var(--color-navy)] placeholder-gray-400 focus:border-[var(--color-maroon)] focus:outline-none focus:ring-1 focus:ring-[var(--color-maroon)]"
              />
            </div>
          </div>

          {/* Action Buttons */}
          <div className="flex items-center justify-end gap-3 pt-3 border-t border-gray-100">
            <button
              type="button"
              disabled={isSubmitting}
              onClick={onClose}
              className="rounded-lg border border-gray-300 px-4 py-2 text-xs font-semibold text-gray-700 hover:bg-gray-50 transition-colors cursor-pointer"
            >
              Cancel
            </button>
            <button
              type="submit"
              data-testid="create-facility-submit-btn"
              disabled={isSubmitting}
              className="inline-flex items-center gap-2 rounded-lg bg-[var(--color-maroon)] px-5 py-2 text-xs font-semibold text-white shadow-xs hover:opacity-90 disabled:opacity-50 transition-all cursor-pointer"
            >
              {isSubmitting ? (
                <>
                  <span className="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-white border-t-transparent" />
                  Creating Facility...
                </>
              ) : (
                <>
                  <Plus size={15} />
                  Create Facility
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
