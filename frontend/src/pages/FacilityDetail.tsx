import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  ArrowLeft,
  Activity,
  MapPin,
  Clock,
  ShieldCheck,
  AlertCircle,
  CheckCircle2,
  Edit2,
  Trash2,
  AlertTriangle,
  Plus,
} from "lucide-react";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { api, ApiError } from "../api";
import { useAuth } from "../context/AuthContext";
import {
  type Facility,
  type Schedule,
  type UpdateScheduleInput,
  type CreateScheduleInput,
  WEEKDAY_NAMES,
  AUDIENCE_LABELS,
  formatTimeRange,
} from "../types";

function normalizeTimeToSeconds(val: string): string {
  const trimmed = val.trim();
  if (/^\d{2}:\d{2}$/.test(trimmed)) {
    return `${trimmed}:00`;
  }
  return trimmed;
}

function parseTimeToSeconds(val: string, allow24: boolean): number | null {
  const normalized = normalizeTimeToSeconds(val);
  if (!/^\d{2}:\d{2}:\d{2}$/.test(normalized)) {
    return null;
  }
  const parts = normalized.split(":").map(Number);
  const h = parts[0];
  const m = parts[1];
  const s = parts[2];
  if (allow24 && normalized === "24:00:00") {
    return 86400;
  }
  if (h < 0 || h > 23 || m < 0 || m > 59 || s < 0 || s > 59) {
    return null;
  }
  return h * 3600 + m * 60 + s;
}

function sortSchedules(schedules: Schedule[]): Schedule[] {
  return [...schedules].sort((a, b) => {
    if (a.dayOfWeek !== b.dayOfWeek) {
      return a.dayOfWeek - b.dayOfWeek;
    }
    if (a.startTime !== b.startTime) {
      return a.startTime.localeCompare(b.startTime);
    }
    if (a.endTime !== b.endTime) {
      return a.endTime.localeCompare(b.endTime);
    }
    if (a.audience !== b.audience) {
      return a.audience.localeCompare(b.audience);
    }
    return a.id.localeCompare(b.id);
  });
}

export function FacilityDetail() {
  const { slug } = useParams<{ slug: string }>();
  const { user } = useAuth();
  const [facility, setFacility] = useState<Facility | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<{ status: number; message: string } | null>(
    null
  );

  // Authorized role check: only sports_director or super_admin may edit schedule windows
  const canEditSchedule = Boolean(
    user &&
      (user.roles.includes("sports_director") ||
        user.roles.includes("super_admin"))
  );

  // Inline editing state: tracks one schedule row at a time
  const [editingScheduleId, setEditingScheduleId] = useState<string | null>(null);
  const [draftDayOfWeek, setDraftDayOfWeek] = useState<number>(1);
  const [draftStartTime, setDraftStartTime] = useState<string>("09:00:00");
  const [draftEndTime, setDraftEndTime] = useState<string>("12:00:00");
  const [draftAudience, setDraftAudience] = useState<string>("student");
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [editError, setEditError] = useState<string | null>(null);

  // Deletion state: tracks one schedule row confirmation at a time
  const [deletingScheduleId, setDeletingScheduleId] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState<boolean>(false);
  const [deleteError, setDeleteError] = useState<{
    message: string;
    showRefresh?: boolean;
  } | null>(null);

  // Inline add state: tracks creation of a new schedule window
  const [isAdding, setIsAdding] = useState<boolean>(false);
  const [addDayOfWeek, setAddDayOfWeek] = useState<number>(1);
  const [addStartTime, setAddStartTime] = useState<string>("09:00:00");
  const [addEndTime, setAddEndTime] = useState<string>("12:00:00");
  const [addAudience, setAddAudience] = useState<string>("student");
  const [isSavingAdd, setIsSavingAdd] = useState<boolean>(false);
  const [addError, setAddError] = useState<string | null>(null);

  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  // References to guard against late async responses after unmount or navigation
  const isMountedRef = useRef<boolean>(true);
  const currentSlugRef = useRef<string | undefined>(slug);
  const scheduleHeadingRef = useRef<HTMLHeadingElement | null>(null);
  const cancelButtonRef = useRef<HTMLButtonElement | null>(null);
  const addWindowButtonRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (deletingScheduleId) {
      // Focus Cancel button as soon as inline confirmation opens
      cancelButtonRef.current?.focus();
    }
  }, [deletingScheduleId]);

  useEffect(() => {
    isMountedRef.current = true;
    currentSlugRef.current = slug;
    return () => {
      isMountedRef.current = false;
    };
  }, [slug]);

  const fetchDetail = useCallback(() => {
    if (!slug) return;
    setFacility(null);
    setIsLoading(true);
    setError(null);
    setEditingScheduleId(null);
    setDeletingScheduleId(null);
    setIsAdding(false);
    setAddError(null);
    setEditError(null);
    setDeleteError(null);
    setSuccessMessage(null);

    const controller = new AbortController();

    api
      .getFacility(slug, { signal: controller.signal })
      .then((data) => {
        if (!isMountedRef.current || currentSlugRef.current !== slug) return;
        setFacility(data);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!isMountedRef.current || currentSlugRef.current !== slug) return;
        if (err.name === "AbortError") return;
        if (err instanceof ApiError && err.status === 404) {
          setError({ status: 404, message: "Facility not found" });
        } else {
          setError({
            status: err instanceof ApiError ? err.status : 500,
            message:
              err instanceof Error
                ? err.message
                : "Unable to load facility from server",
          });
        }
        setIsLoading(false);
      });

    return () => {
      controller.abort();
    };
  }, [slug]);

  useEffect(() => {
    const cancel = fetchDetail();
    return cancel;
  }, [fetchDetail]);

  const startEditing = (s: Schedule) => {
    if (isSaving || isDeleting || isSavingAdd) return;
    setIsAdding(false);
    setAddError(null);
    setDeletingScheduleId(null);
    setDeleteError(null);
    setEditingScheduleId(s.id);
    setDraftDayOfWeek(s.dayOfWeek);
    setDraftStartTime(s.startTime);
    setDraftEndTime(s.endTime);
    setDraftAudience(s.audience);
    setEditError(null);
    setSuccessMessage(null);
  };

  const cancelEditing = () => {
    if (isSaving) return;
    setEditingScheduleId(null);
    setEditError(null);
  };

  const startDeleting = (s: Schedule) => {
    if (isSaving || isDeleting || isSavingAdd) return;
    setIsAdding(false);
    setAddError(null);
    setEditingScheduleId(null);
    setEditError(null);
    setDeleteError(null);
    setSuccessMessage(null);
    setDeletingScheduleId(s.id);
  };

  const cancelDeleting = () => {
    if (isDeleting) return;
    const targetId = deletingScheduleId;
    setDeletingScheduleId(null);
    setDeleteError(null);
    if (targetId) {
      setTimeout(() => {
        const btn = document.getElementById(`delete-btn-${targetId}`);
        btn?.focus();
      }, 0);
    }
  };

  const handleConfirmDelete = async (scheduleId: string) => {
    if (isDeleting || !slug) return;

    const targetSlug = slug;
    setIsDeleting(true);
    setDeleteError(null);
    setSuccessMessage(null);

    try {
      await api.deleteSchedule(targetSlug, scheduleId);

      // Guard against late response after navigating away
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      // Update timetable only after confirmed 204
      setFacility((prev) => {
        if (!prev || prev.slug !== targetSlug) return prev;
        const idx = prev.schedules.findIndex((item) => item.id === scheduleId);
        const remaining = prev.schedules.filter((item) => item.id !== scheduleId);

        // Move focus to a sensible nearby control after the deleted row disappears
        if (remaining.length === 0) {
          setTimeout(() => {
            scheduleHeadingRef.current?.focus();
          }, 0);
        } else {
          const targetIdx = idx < remaining.length ? idx : remaining.length - 1;
          const target = remaining[targetIdx];
          setTimeout(() => {
            const btn =
              document.getElementById(`delete-btn-${target.id}`) ||
              document.getElementById(`edit-btn-${target.id}`);
            btn?.focus();
          }, 0);
        }

        return {
          ...prev,
          schedules: remaining,
        };
      });

      setDeletingScheduleId(null);
      setDeleteError(null);
      setSuccessMessage("Schedule window deleted successfully.");
    } catch (err) {
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      // Preserve the row on failure
      if (err instanceof ApiError) {
        if (err.status === 400) {
          setDeleteError({
            message: err.message || "Invalid schedule deletion request.",
          });
        } else if (err.status === 401) {
          setDeleteError({
            message:
              "Your session has expired. Please sign in again to delete schedules.",
          });
        } else if (err.status === 403) {
          setDeleteError({
            message:
              "You do not have permission to delete this schedule. Only sports directors and super admins can modify timetables.",
          });
        } else if (err.status === 404) {
          setDeleteError({
            message:
              "Schedule window or facility not found. It may have already been deleted. Please refresh the timetable.",
            showRefresh: true,
          });
        } else {
          setDeleteError({
            message:
              "Server error encountered while deleting schedule window. The change may not have committed. Please refresh before trying again.",
            showRefresh: true,
          });
        }
      } else {
        // Network failure
        setDeleteError({
          message:
            "Network failure occurred. The deletion may have committed on the server. Please refresh this facility to verify before trying again.",
          showRefresh: true,
        });
      }
    } finally {
      if (isMountedRef.current && currentSlugRef.current === targetSlug) {
        setIsDeleting(false);
      }
    }
  };

  const validateScheduleWindow = (
    dayOfWeek: number,
    startTime: string,
    endTime: string,
    audience: string
  ): string | null => {
    if (dayOfWeek < 1 || dayOfWeek > 7) {
      return "Day of week must be between Monday (1) and Sunday (7).";
    }
    const validAudiences = [
      "general",
      "student",
      "faculty",
      "women_only",
      "staff",
    ];
    if (!validAudiences.includes(audience)) {
      return "Please select a supported audience cohort.";
    }

    const startSec = parseTimeToSeconds(startTime, false);
    if (startSec === null) {
      return "Start time must be formatted as HH:MM:SS (e.g. 09:00:00) between 00:00:00 and 23:59:59.";
    }

    const endSec = parseTimeToSeconds(endTime, true);
    if (endSec === null) {
      return "End time must be formatted as HH:MM:SS (e.g. 12:00:00, or 24:00:00 for midnight).";
    }

    if (startSec >= endSec) {
      return `Start time (${normalizeTimeToSeconds(
        startTime
      )}) must be strictly before end time (${normalizeTimeToSeconds(
        endTime
      )}).`;
    }

    return null;
  };

  const validateDraft = (): string | null => {
    return validateScheduleWindow(
      draftDayOfWeek,
      draftStartTime,
      draftEndTime,
      draftAudience
    );
  };

  const startAdding = () => {
    if (isSaving || isDeleting || isSavingAdd) return;
    setEditingScheduleId(null);
    setEditError(null);
    setDeletingScheduleId(null);
    setDeleteError(null);
    setSuccessMessage(null);
    setAddDayOfWeek(1);
    setAddStartTime("09:00:00");
    setAddEndTime("12:00:00");
    setAddAudience("student");
    setAddError(null);
    setIsAdding(true);
  };

  const cancelAdding = () => {
    if (isSavingAdd) return;
    setIsAdding(false);
    setAddError(null);
    setTimeout(() => {
      addWindowButtonRef.current?.focus();
    }, 0);
  };

  const handleAddSave = async () => {
    if (isSavingAdd || !slug) return;

    const clientErr = validateScheduleWindow(
      addDayOfWeek,
      addStartTime,
      addEndTime,
      addAudience
    );
    if (clientErr) {
      setAddError(clientErr);
      return;
    }

    const payload: CreateScheduleInput = {
      dayOfWeek: addDayOfWeek,
      startTime: normalizeTimeToSeconds(addStartTime),
      endTime: normalizeTimeToSeconds(addEndTime),
      audience: addAudience,
    };

    const targetSlug = slug;
    setIsSavingAdd(true);
    setAddError(null);

    try {
      const createdSchedule = await api.createSchedule(targetSlug, payload);

      // Guard against late response after navigating away
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      setFacility((prev) => {
        if (!prev || prev.slug !== targetSlug) return prev;
        return {
          ...prev,
          schedules: sortSchedules([...prev.schedules, createdSchedule]),
        };
      });

      setIsAdding(false);
      setAddError(null);
      setSuccessMessage("Schedule window added successfully.");
    } catch (err) {
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      // Keep draft inputs intact on failure
      if (err instanceof ApiError) {
        if (err.status === 400) {
          setAddError(err.message || "Invalid schedule window input.");
        } else if (err.status === 401) {
          setAddError(
            "Your session has expired. Please sign in again to add schedules."
          );
        } else if (err.status === 403) {
          setAddError(
            "You do not have permission to add schedules. Only sports directors and super admins can modify timetables."
          );
        } else if (err.status === 404) {
          setAddError(
            "Facility not found. It may have been modified or deleted."
          );
        } else if (err.status === 409) {
          setAddError(
            "This schedule window duplicates an existing operating window for this facility."
          );
        } else {
          setAddError(
            "Server failure encountered. Changes may not have been saved. Please reload this facility before retrying."
          );
        }
      } else {
        setAddError(
          "Network failure occurred. Changes may not have been saved. Please reload this facility before retrying."
        );
      }
    } finally {
      if (isMountedRef.current && currentSlugRef.current === targetSlug) {
        setIsSavingAdd(false);
      }
    }
  };

  const handleSave = async (scheduleId: string) => {
    if (isSaving || !slug) return;

    const clientErr = validateDraft();
    if (clientErr) {
      setEditError(clientErr);
      return;
    }

    const payload: UpdateScheduleInput = {
      dayOfWeek: draftDayOfWeek,
      startTime: normalizeTimeToSeconds(draftStartTime),
      endTime: normalizeTimeToSeconds(draftEndTime),
      audience: draftAudience,
    };

    const targetSlug = slug;
    setIsSaving(true);
    setEditError(null);

    try {
      const updatedSchedule = await api.updateSchedule(
        targetSlug,
        scheduleId,
        payload
      );

      // Guard against late response after navigating away
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      // Update timetable in local state directly from confirmed server response
      setFacility((prev) => {
        if (!prev || prev.slug !== targetSlug) return prev;
        return {
          ...prev,
          schedules: prev.schedules.map((item) =>
            item.id === updatedSchedule.id ? updatedSchedule : item
          ),
        };
      });

      setEditingScheduleId(null);
      setEditError(null);
      setSuccessMessage("Schedule window updated successfully.");
    } catch (err) {
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      // Keep draft inputs intact on failure so the user doesn't lose their edits
      if (err instanceof ApiError) {
        if (err.status === 400) {
          setEditError(err.message || "Invalid schedule window input.");
        } else if (err.status === 401) {
          setEditError(
            "Your session has expired. Please sign in again to modify schedules."
          );
        } else if (err.status === 403) {
          setEditError(
            "You do not have permission to edit this schedule. Only sports directors and super admins can modify timetables."
          );
        } else if (err.status === 404) {
          setEditError(
            "Schedule window or facility not found. It may have been modified or deleted."
          );
        } else if (err.status === 409) {
          setEditError(
            "This schedule window overlaps or conflicts with an existing operating window."
          );
        } else {
          setEditError(
            "Server failure encountered. Changes may not have been saved. Please reload this facility before retrying."
          );
        }
      } else {
        // Network or Abort failure: outcome is uncertain; instruct user to reload
        setEditError(
          "Network failure occurred. Changes may not have been saved. Please reload this facility before retrying."
        );
      }
    } finally {
      if (isMountedRef.current && currentSlugRef.current === targetSlug) {
        setIsSaving(false);
      }
    }
  };

  const statusDisplay: Record<
    string,
    { label: string; badgeClass: string; note: string }
  > = {
    active: {
      label: "Active & Operational",
      badgeClass: "bg-emerald-50 text-emerald-800 border-emerald-300",
      note: "Operational status reflects physical readiness, not real-time booking availability.",
    },
    maintenance: {
      label: "Under Maintenance",
      badgeClass: "bg-amber-50 text-amber-800 border-amber-300",
      note: "This facility is currently undergoing scheduled maintenance.",
    },
    closed: {
      label: "Closed",
      badgeClass: "bg-gray-100 text-gray-700 border-gray-300",
      note: "This facility is closed until further notice.",
    },
  };

  return (
    <div className="min-h-screen bg-white">
      <Navbar />
      <main className="mx-auto max-w-3xl px-6 py-8">
        <Link
          to="/#facilities"
          className="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-maroon)] transition-opacity hover:opacity-80"
        >
          <ArrowLeft size={16} />
          Back to facilities
        </Link>

        {isLoading && (
          <div
            data-testid="facility-detail-loading"
            className="flex items-center justify-center py-20 text-sm text-[var(--color-navy-soft)]"
          >
            <div className="flex items-center gap-2">
              <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-maroon)] border-t-transparent" />
              Loading facility details...
            </div>
          </div>
        )}

        {!isLoading && error && error.status === 404 && (
          <div
            data-testid="facility-detail-404"
            className="mt-6 rounded-2xl border border-amber-200 bg-amber-50 p-8 text-center"
          >
            <AlertCircle size={36} className="mx-auto text-amber-600" />
            <h1 className="mt-3 font-display text-xl font-semibold text-amber-900">
              Facility Not Found
            </h1>
            <p className="mt-2 text-sm text-amber-800">
              No facility matching slug &ldquo;{slug}&rdquo; could be found in
              the database.
            </p>
            <Link
              to="/#facilities"
              className="mt-5 inline-block rounded-md bg-[var(--color-maroon)] px-5 py-2 text-xs font-semibold text-white transition-opacity hover:opacity-90"
            >
              Back to facilities
            </Link>
          </div>
        )}

        {!isLoading && error && error.status !== 404 && (
          <div
            data-testid="facility-detail-error"
            className="mt-6 rounded-2xl border border-red-200 bg-red-50 p-8 text-center"
          >
            <AlertCircle size={36} className="mx-auto text-red-600" />
            <h1 className="mt-3 font-display text-xl font-semibold text-red-900">
              Unable to Load Facility
            </h1>
            <p className="mt-2 text-sm text-red-700">{error.message}</p>
            <button
              type="button"
              onClick={fetchDetail}
              className="mt-5 rounded-md bg-[var(--color-maroon)] px-5 py-2 text-xs font-semibold text-white transition-opacity hover:opacity-90"
            >
              Retry
            </button>
          </div>
        )}

        {!isLoading && !error && facility && (
          <div data-testid="facility-detail-content">
            {/* Facility Hero Banner */}
            <div className="mt-4 flex items-start gap-4 rounded-2xl border border-[var(--color-gold)] bg-[var(--color-maroon)] p-6 text-[#F3E6C4]">
              <span className="flex h-13 w-13 flex-shrink-0 items-center justify-center rounded-xl bg-[var(--color-navy)] p-3">
                <Activity size={24} className="text-[var(--color-gold-light)]" />
              </span>
              <div>
                <h1 className="font-display text-2xl font-semibold text-[#F3E6C4]">
                  {facility.name}
                </h1>
                {facility.description ? (
                  <p className="mt-1 text-sm text-[var(--color-gold-light)]">
                    {facility.description}
                  </p>
                ) : (
                  <p className="mt-1 text-sm italic text-[var(--color-gold-light)]/80">
                    No description provided for this facility.
                  </p>
                )}
              </div>
            </div>

            {/* Core Attributes */}
            <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
                <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-[var(--color-navy-soft)]">
                  <MapPin size={15} className="text-[var(--color-maroon)]" />
                  Physical Location
                </div>
                <p className="mt-2 text-sm font-medium text-[var(--color-navy)]">
                  {facility.location ?? "Location not specified"}
                </p>
              </div>

              <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
                <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-[var(--color-navy-soft)]">
                  <ShieldCheck size={15} className="text-[var(--color-maroon)]" />
                  Operational State
                </div>
                <div className="mt-2 flex items-center gap-2">
                  <span
                    className={`rounded-full border px-2.5 py-0.5 text-xs font-medium ${
                      statusDisplay[facility.status]?.badgeClass ??
                      "border-gray-200 bg-gray-100 text-gray-700"
                    }`}
                  >
                    {statusDisplay[facility.status]?.label ?? facility.status}
                  </span>
                </div>
                <p className="mt-1 text-[11px] text-[var(--color-navy-soft)]">
                  {statusDisplay[facility.status]?.note}
                </p>
              </div>
            </div>

            {/* Weekly Timetable */}
            <section className="mt-8">
              <div className="flex items-center justify-between">
                <div>
                  <h2
                    ref={scheduleHeadingRef}
                    tabIndex={-1}
                    className="font-display text-lg font-semibold text-[var(--color-navy)] focus:outline-none"
                  >
                    Weekly Operating Schedule
                  </h2>
                  <p className="mt-0.5 flex items-center gap-1 text-xs text-[var(--color-navy-soft)]">
                    <Clock size={12} />
                    Times displayed in local Asia/Karachi (PKT, UTC+5) wall-clock
                    time. End time 24:00:00 indicates midnight end-of-day.
                  </p>
                </div>
                {canEditSchedule && !isAdding && (
                  <button
                    ref={addWindowButtonRef}
                    type="button"
                    id="add-window-btn"
                    data-testid="add-schedule-window-btn"
                    disabled={isSaving || isDeleting || isSavingAdd}
                    onClick={startAdding}
                    aria-label="Add schedule window"
                    className="inline-flex items-center gap-1.5 rounded-md bg-[var(--color-maroon)] px-3 py-1.5 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50"
                  >
                    <Plus size={14} />
                    Add window
                  </button>
                )}
              </div>

              {/* Success Notification Banner */}
              {successMessage && (
                <div
                  data-testid="schedule-success-banner"
                  role="status"
                  className="mt-3 flex items-center justify-between rounded-lg border border-emerald-300 bg-emerald-50 px-4 py-2.5 text-xs font-medium text-emerald-800"
                >
                  <div className="flex items-center gap-2">
                    <CheckCircle2 size={16} className="text-emerald-600" />
                    <span>{successMessage}</span>
                  </div>
                  <button
                    type="button"
                    onClick={() => setSuccessMessage(null)}
                    className="text-emerald-700 hover:text-emerald-900"
                  >
                    Dismiss
                  </button>
                </div>
              )}

              {/* Inline Add Window Form */}
              {isAdding && (
                <div
                  data-testid="schedule-add-form-card"
                  className="mt-3 rounded-xl border border-[var(--color-gold)] bg-white p-4 shadow-xs"
                >
                  <form
                    data-testid="schedule-add-form"
                    onSubmit={(e) => {
                      e.preventDefault();
                      handleAddSave();
                    }}
                    className="space-y-3"
                  >
                    <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-2">
                      <span className="text-xs font-semibold uppercase tracking-wider text-[var(--color-maroon)]">
                        Add New Operating Window
                      </span>
                      <span className="text-[11px] text-[var(--color-navy-soft)]">
                        Asia/Karachi (PKT) wall-clock time
                      </span>
                    </div>

                    {addError && (
                      <div
                        role="alert"
                        data-testid="schedule-add-error"
                        className="rounded-md border border-red-200 bg-red-50 p-2.5 text-xs text-red-700"
                      >
                        {addError}
                      </div>
                    )}

                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
                      {/* Day of Week */}
                      <div>
                        <label
                          htmlFor="add-day"
                          className="block text-xs font-medium text-[var(--color-navy)] mb-1"
                        >
                          Day of Week
                        </label>
                        <select
                          id="add-day"
                          aria-label="Day of week"
                          disabled={isSavingAdd}
                          value={addDayOfWeek}
                          onChange={(e) =>
                            setAddDayOfWeek(Number(e.target.value))
                          }
                          className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none disabled:opacity-50"
                        >
                          <option value={1}>Monday</option>
                          <option value={2}>Tuesday</option>
                          <option value={3}>Wednesday</option>
                          <option value={4}>Thursday</option>
                          <option value={5}>Friday</option>
                          <option value={6}>Saturday</option>
                          <option value={7}>Sunday</option>
                        </select>
                      </div>

                      {/* Start Time */}
                      <div>
                        <label
                          htmlFor="add-start"
                          className="block text-xs font-medium text-[var(--color-navy)] mb-1"
                        >
                          Start Time (HH:MM:SS)
                        </label>
                        <input
                          id="add-start"
                          aria-label="Start time"
                          type="text"
                          disabled={isSavingAdd}
                          value={addStartTime}
                          onChange={(e) => setAddStartTime(e.target.value)}
                          placeholder="09:00:00"
                          className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs font-mono text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none disabled:opacity-50"
                        />
                      </div>

                      {/* End Time */}
                      <div>
                        <div className="flex items-center justify-between mb-1">
                          <label
                            htmlFor="add-end"
                            className="block text-xs font-medium text-[var(--color-navy)]"
                          >
                            End Time (HH:MM:SS)
                          </label>
                          <button
                            type="button"
                            disabled={isSavingAdd}
                            onClick={() => setAddEndTime("24:00:00")}
                            data-testid="add-set-midnight-btn"
                            className="text-[10px] font-semibold text-[var(--color-maroon)] hover:underline focus:outline-none disabled:opacity-50"
                          >
                            Set 24:00:00
                          </button>
                        </div>
                        <input
                          id="add-end"
                          aria-label="End time"
                          type="text"
                          disabled={isSavingAdd}
                          value={addEndTime}
                          onChange={(e) => setAddEndTime(e.target.value)}
                          placeholder="12:00:00"
                          className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs font-mono text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none disabled:opacity-50"
                        />
                      </div>

                      {/* Audience */}
                      <div>
                        <label
                          htmlFor="add-audience"
                          className="block text-xs font-medium text-[var(--color-navy)] mb-1"
                        >
                          Audience
                        </label>
                        <select
                          id="add-audience"
                          aria-label="Audience"
                          disabled={isSavingAdd}
                          value={addAudience}
                          onChange={(e) => setAddAudience(e.target.value)}
                          className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none disabled:opacity-50"
                        >
                          <option value="general">
                            General Community
                          </option>
                          <option value="student">Students</option>
                          <option value="faculty">Faculty</option>
                          <option value="women_only">
                            Women Only
                          </option>
                          <option value="staff">Staff</option>
                        </select>
                      </div>
                    </div>

                    <div className="mt-4 flex items-center justify-end gap-2 pt-2 border-t border-[var(--color-border)]">
                      <button
                        type="button"
                        id="cancel-add-btn"
                        disabled={isSavingAdd}
                        onClick={cancelAdding}
                        aria-label="Cancel adding schedule window"
                        className="rounded-md border border-[var(--color-border)] bg-white px-3 py-1.5 text-xs font-medium text-[var(--color-navy)] hover:bg-[var(--color-surface)] disabled:opacity-50"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        id="submit-add-btn"
                        data-testid="submit-add-schedule-btn"
                        disabled={isSavingAdd}
                        aria-label="Save new schedule window"
                        className="inline-flex items-center gap-1.5 rounded-md bg-[var(--color-maroon)] px-3 py-1.5 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50"
                      >
                        {isSavingAdd ? (
                          <>
                            <span className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent" />
                            Adding...
                          </>
                        ) : (
                          "Add Window"
                        )}
                      </button>
                    </div>
                  </form>
                </div>
              )}

              {facility.schedules.length === 0 ? (
                <div className="mt-3 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center text-sm text-[var(--color-navy-soft)]">
                  <p>
                    No recurring operating schedules are currently registered for
                    this facility.
                  </p>
                  {canEditSchedule && !isAdding && (
                    <div className="mt-3">
                      <button
                        type="button"
                        id="add-first-window-btn"
                        data-testid="add-first-window-btn"
                        disabled={isSaving || isDeleting || isSavingAdd}
                        onClick={startAdding}
                        className="inline-flex items-center gap-1.5 rounded-md bg-[var(--color-maroon)] px-3 py-1.5 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50"
                      >
                        <Plus size={14} />
                        Add first window
                      </button>
                    </div>
                  )}
                </div>
              ) : (
                <div className="mt-3 divide-y divide-[var(--color-border)] rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
                  {facility.schedules.map((s) => {
                    const isEditingThisRow = editingScheduleId === s.id;

                    if (isEditingThisRow) {
                      return (
                        <div
                          key={s.id}
                          className="bg-white p-4 first:rounded-t-xl last:rounded-b-xl"
                        >
                          <form
                            data-testid={`schedule-edit-form-${s.id}`}
                            onSubmit={(e) => {
                              e.preventDefault();
                              handleSave(s.id);
                            }}
                            className="space-y-3"
                          >
                            <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-2">
                              <span className="text-xs font-semibold uppercase tracking-wider text-[var(--color-maroon)]">
                                Edit Operating Window
                              </span>
                              <span className="text-[11px] text-[var(--color-navy-soft)]">
                                Asia/Karachi (PKT) wall-clock time
                              </span>
                            </div>

                            {editError && (
                              <div
                                role="alert"
                                data-testid="schedule-edit-error"
                                className="rounded-md border border-red-200 bg-red-50 p-2.5 text-xs text-red-700"
                              >
                                {editError}
                              </div>
                            )}

                            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
                              {/* Day of Week */}
                              <div>
                                <label
                                  htmlFor={`edit-day-${s.id}`}
                                  className="block text-xs font-medium text-[var(--color-navy)] mb-1"
                                >
                                  Day of Week
                                </label>
                                <select
                                  id={`edit-day-${s.id}`}
                                  aria-label="Day of week"
                                  disabled={isSaving}
                                  value={draftDayOfWeek}
                                  onChange={(e) =>
                                    setDraftDayOfWeek(Number(e.target.value))
                                  }
                                  className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                                >
                                  <option value={1}>Monday</option>
                                  <option value={2}>Tuesday</option>
                                  <option value={3}>Wednesday</option>
                                  <option value={4}>Thursday</option>
                                  <option value={5}>Friday</option>
                                  <option value={6}>Saturday</option>
                                  <option value={7}>Sunday</option>
                                </select>
                              </div>

                              {/* Start Time */}
                              <div>
                                <label
                                  htmlFor={`edit-start-${s.id}`}
                                  className="block text-xs font-medium text-[var(--color-navy)] mb-1"
                                >
                                  Start Time (HH:MM:SS)
                                </label>
                                <input
                                  id={`edit-start-${s.id}`}
                                  aria-label="Start time"
                                  type="text"
                                  disabled={isSaving}
                                  value={draftStartTime}
                                  onChange={(e) =>
                                    setDraftStartTime(e.target.value)
                                  }
                                  placeholder="09:00:00"
                                  className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs font-mono text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                                />
                              </div>

                              {/* End Time */}
                              <div>
                                <div className="flex items-center justify-between mb-1">
                                  <label
                                    htmlFor={`edit-end-${s.id}`}
                                    className="block text-xs font-medium text-[var(--color-navy)]"
                                  >
                                    End Time (HH:MM:SS)
                                  </label>
                                  <button
                                    type="button"
                                    disabled={isSaving}
                                    onClick={() => setDraftEndTime("24:00:00")}
                                    className="text-[10px] font-semibold text-[var(--color-maroon)] hover:underline focus:outline-none disabled:opacity-50"
                                  >
                                    Set 24:00:00
                                  </button>
                                </div>
                                <input
                                  id={`edit-end-${s.id}`}
                                  aria-label="End time"
                                  type="text"
                                  disabled={isSaving}
                                  value={draftEndTime}
                                  onChange={(e) =>
                                    setDraftEndTime(e.target.value)
                                  }
                                  placeholder="12:00:00"
                                  className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs font-mono text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                                />
                              </div>

                              {/* Audience */}
                              <div>
                                <label
                                  htmlFor={`edit-audience-${s.id}`}
                                  className="block text-xs font-medium text-[var(--color-navy)] mb-1"
                                >
                                  Audience
                                </label>
                                <select
                                  id={`edit-audience-${s.id}`}
                                  aria-label="Audience"
                                  disabled={isSaving}
                                  value={draftAudience}
                                  onChange={(e) =>
                                    setDraftAudience(e.target.value)
                                  }
                                  className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                                >
                                  <option value="general">
                                    General Community
                                  </option>
                                  <option value="student">Students</option>
                                  <option value="faculty">Faculty</option>
                                  <option value="women_only">
                                    Women Only
                                  </option>
                                  <option value="staff">Staff</option>
                                </select>
                              </div>
                            </div>

                            <div className="mt-4 flex items-center justify-end gap-2 pt-2 border-t border-[var(--color-border)]">
                              <button
                                type="button"
                                disabled={isSaving}
                                onClick={cancelEditing}
                                aria-label="Cancel editing"
                                className="rounded-md border border-[var(--color-border)] bg-white px-3 py-1.5 text-xs font-medium text-[var(--color-navy)] hover:bg-[var(--color-surface)] disabled:opacity-50"
                              >
                                Cancel
                              </button>
                              <button
                                type="submit"
                                disabled={isSaving}
                                aria-label="Save schedule window"
                                className="inline-flex items-center gap-1.5 rounded-md bg-[var(--color-maroon)] px-3 py-1.5 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50"
                              >
                                {isSaving ? (
                                  <>
                                    <span className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent" />
                                    Saving...
                                  </>
                                ) : (
                                  "Save Changes"
                                )}
                              </button>
                            </div>
                          </form>
                        </div>
                      );
                    }

                    const isDeletingThisRow = deletingScheduleId === s.id;

                    if (isDeletingThisRow) {
                      return (
                        <div
                          key={s.id}
                          role="region"
                          aria-label={`Confirm deletion of ${
                            WEEKDAY_NAMES[s.dayOfWeek] ?? `Day ${s.dayOfWeek}`
                          } schedule window`}
                          aria-describedby={`delete-desc-${s.id}`}
                          data-testid={`schedule-delete-confirm-${s.id}`}
                          onKeyDown={(e) => {
                            if (e.key === "Escape") {
                              e.stopPropagation();
                              cancelDeleting();
                            }
                          }}
                          className="border-red-200 bg-red-50/70 p-4 first:rounded-t-xl last:rounded-b-xl"
                        >
                          <div className="flex items-start gap-3">
                            <div className="mt-0.5 flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-red-100 text-red-600">
                              <AlertTriangle size={18} />
                            </div>
                            <div className="flex-1">
                              <h3
                                id={`delete-title-${s.id}`}
                                className="text-xs font-semibold uppercase tracking-wider text-red-800"
                              >
                                Confirm Schedule Window Deletion
                              </h3>
                              <p
                                id={`delete-desc-${s.id}`}
                                className="mt-1 text-xs text-gray-700"
                              >
                                Are you sure you want to permanently delete this weekly schedule window for{" "}
                                <strong className="font-semibold text-gray-900">{facility.name}</strong>?
                              </p>

                              <div className="mt-2.5 grid grid-cols-1 gap-2 rounded-md border border-red-200/80 bg-white p-2.5 text-xs sm:grid-cols-3">
                                <div>
                                  <span className="block text-[10px] font-medium uppercase text-gray-500">
                                    Weekday
                                  </span>
                                  <span className="font-semibold text-[var(--color-navy)]">
                                    {WEEKDAY_NAMES[s.dayOfWeek] ?? `Day ${s.dayOfWeek}`}
                                  </span>
                                </div>
                                <div>
                                  <span className="block text-[10px] font-medium uppercase text-gray-500">
                                    Operating Time (PKT)
                                  </span>
                                  <span className="font-mono font-semibold text-[var(--color-navy)]">
                                    {formatTimeRange(s.startTime, s.endTime)}
                                  </span>
                                </div>
                                <div>
                                  <span className="block text-[10px] font-medium uppercase text-gray-500">
                                    Audience
                                  </span>
                                  <span className="font-semibold text-[var(--color-navy)]">
                                    {AUDIENCE_LABELS[s.audience] ?? s.audience}
                                  </span>
                                </div>
                              </div>

                              <p className="mt-2 text-[11px] font-medium text-red-600">
                                This action cannot be undone. An audit record will be logged.
                              </p>

                              {deleteError && (
                                <div
                                  role="alert"
                                  data-testid="schedule-delete-error"
                                  className="mt-2.5 rounded-md border border-red-300 bg-red-100 p-2.5 text-xs text-red-800"
                                >
                                  <div className="flex items-start gap-1.5">
                                    <AlertCircle size={14} className="mt-0.5 flex-shrink-0 text-red-600" />
                                    <div>
                                      <p>{deleteError.message}</p>
                                      {deleteError.showRefresh && (
                                        <button
                                          type="button"
                                          onClick={() => fetchDetail()}
                                          className="mt-1.5 inline-flex items-center gap-1 font-semibold underline text-red-900 hover:text-red-950"
                                        >
                                          Refresh timetable
                                        </button>
                                      )}
                                    </div>
                                  </div>
                                </div>
                              )}

                              <div className="mt-3.5 flex items-center justify-end gap-2">
                                <button
                                  ref={cancelButtonRef}
                                  type="button"
                                  id={`cancel-delete-btn-${s.id}`}
                                  autoFocus
                                  disabled={isDeleting}
                                  onClick={cancelDeleting}
                                  aria-label="Cancel deletion"
                                  className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
                                >
                                  Cancel
                                </button>
                                <button
                                  type="button"
                                  id={`confirm-delete-btn-${s.id}`}
                                  disabled={isDeleting}
                                  onClick={() => handleConfirmDelete(s.id)}
                                  aria-label="Confirm delete schedule window"
                                  className="inline-flex items-center gap-1.5 rounded-md bg-red-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-red-700 disabled:opacity-50"
                                >
                                  {isDeleting ? (
                                    <>
                                      <span className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent" />
                                      Deleting...
                                    </>
                                  ) : (
                                    "Confirm Delete"
                                  )}
                                </button>
                              </div>
                            </div>
                          </div>
                        </div>
                      );
                    }

                    return (
                      <div
                        key={s.id}
                        data-testid={`schedule-row-${s.id}`}
                        className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between"
                      >
                        <div className="flex items-center gap-3">
                          <span className="w-24 flex-shrink-0 rounded-md bg-white px-2.5 py-1 text-center text-xs font-semibold text-[var(--color-navy)] shadow-xs">
                            {WEEKDAY_NAMES[s.dayOfWeek] ?? `Day ${s.dayOfWeek}`}
                          </span>
                          <span className="text-sm font-semibold text-[var(--color-navy)]">
                            {formatTimeRange(s.startTime, s.endTime)}
                          </span>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className="rounded-full bg-[var(--color-border)]/60 px-3 py-0.5 text-xs font-medium text-[var(--color-navy)]">
                            {AUDIENCE_LABELS[s.audience] ?? s.audience}
                          </span>
                          {canEditSchedule && (
                            <div className="flex items-center gap-1.5">
                              <button
                                type="button"
                                id={`edit-btn-${s.id}`}
                                disabled={isSaving || isDeleting || isSavingAdd}
                                onClick={() => startEditing(s)}
                                aria-label={`Edit ${
                                  WEEKDAY_NAMES[s.dayOfWeek] ?? `Day ${s.dayOfWeek}`
                                } schedule window`}
                                className="inline-flex items-center gap-1 rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1 text-xs font-semibold text-[var(--color-navy)] transition-colors hover:bg-[var(--color-surface)] hover:text-[var(--color-maroon)] disabled:opacity-50"
                              >
                                <Edit2 size={12} />
                                Edit window
                              </button>
                              <button
                                type="button"
                                id={`delete-btn-${s.id}`}
                                disabled={isSaving || isDeleting || isSavingAdd}
                                onClick={() => startDeleting(s)}
                                aria-label={`Delete ${
                                  WEEKDAY_NAMES[s.dayOfWeek] ?? `Day ${s.dayOfWeek}`
                                } schedule window`}
                                className="inline-flex items-center gap-1 rounded-md border border-red-200 bg-white px-2.5 py-1 text-xs font-semibold text-red-600 transition-colors hover:bg-red-50 hover:text-red-800 disabled:opacity-50"
                              >
                                <Trash2 size={12} />
                                Delete window
                              </button>
                            </div>
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </section>
          </div>
        )}
      </main>
      <div className="mx-auto max-w-3xl px-6 pb-8">
        <Footer />
      </div>
    </div>
  );
}
