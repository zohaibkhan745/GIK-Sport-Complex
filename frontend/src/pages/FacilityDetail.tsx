import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  ArrowLeft,
  Activity,
  Clock,
  AlertCircle,
  CheckCircle2,
  Edit2,
  Trash2,
  AlertTriangle,
  Plus,
  ArrowRight,
  Dumbbell,
  Pencil,
  Info,
} from "lucide-react";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { EditFacilityModal } from "../components/EditFacilityModal";
import { TimePicker } from "../components/TimePicker";
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
  if (/^\d{1,2}:\d{2}$/.test(trimmed)) {
    const parts = trimmed.split(":");
    return `${parts[0].padStart(2, "0")}:${parts[1]}:00`;
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

export function formatClockTime(timeStr: string): string {
  const norm = normalizeTimeToSeconds(timeStr);
  if (norm === "24:00:00") return "Midnight";
  const parts = norm.split(":").map(Number);
  if (parts.length < 2 || isNaN(parts[0]) || isNaN(parts[1])) return timeStr;
  let h = parts[0];
  const m = parts[1];
  const period = h >= 12 ? "PM" : "AM";
  h = h % 12;
  if (h === 0) h = 12;
  const minuteStr = m === 0 ? "00" : m < 10 ? `0${m}` : `${m}`;
  return `${h}:${minuteStr} ${period}`;
}

export function getPKTCurrentSeconds(now = new Date()): number {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: "Asia/Karachi",
      hour: "numeric",
      minute: "numeric",
      second: "numeric",
      hour12: false,
    }).formatToParts(now);
    const h = Number(parts.find((p) => p.type === "hour")?.value ?? 0);
    const m = Number(parts.find((p) => p.type === "minute")?.value ?? 0);
    const s = Number(parts.find((p) => p.type === "second")?.value ?? 0);
    return (h % 24) * 3600 + m * 60 + s;
  } catch {
    return now.getHours() * 3600 + now.getMinutes() * 60 + now.getSeconds();
  }
}

export function getPKTDayOfWeek(now = new Date()): number {
  try {
    const str = new Intl.DateTimeFormat("en-US", {
      timeZone: "Asia/Karachi",
      weekday: "short",
    }).format(now);
    const map: Record<string, number> = {
      Mon: 1,
      Tue: 2,
      Wed: 3,
      Thu: 4,
      Fri: 5,
      Sat: 6,
      Sun: 7,
    };
    return map[str] ?? 1;
  } catch {
    const day = now.getDay();
    return day === 0 ? 7 : day;
  }
}

export function isWindowActiveNow(startTime: string, endTime: string, now = new Date()): boolean {
  try {
    const currentSec = getPKTCurrentSeconds(now);
    const startSec = parseTimeToSeconds(startTime, false);
    const endSec = parseTimeToSeconds(endTime, true);
    if (currentSec === null || startSec === null || endSec === null) return false;
    return currentSec >= startSec && currentSec < endSec;
  } catch {
    return false;
  }
}

export interface GoogleMapsLiveStatus {
  isOpen: boolean;
  badge: "Open now" | "Closed" | "Temporarily Closed" | "Schedule Pending" | "Reserved";
  detail: string;
  badgeClass: string;
  dotClass: string;
}

function getAudienceLabel(audience: string): string {
  if (AUDIENCE_LABELS[audience]) return AUDIENCE_LABELS[audience];
  if (audience === "all") return "Open to All";
  return audience;
}

function formatTimeSafe(iso?: string | null): string {
  if (!iso) return "";
  try {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return "";
    return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  } catch {
    return "";
  }
}

function formatDateTimeSafe(iso?: string | null): string {
  if (!iso) return "";
  try {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return "";
    return d.toLocaleDateString([], { month: "short", day: "numeric" }) + " at " + d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  } catch {
    return "";
  }
}

export function getGoogleMapsStatus(
  facility: Facility | null | undefined,
  now = new Date()
): GoogleMapsLiveStatus {
  if (!facility) {
    return {
      isOpen: false,
      badge: "Closed",
      detail: "Facility information unavailable",
      badgeClass: "border-slate-400/40 bg-slate-500/20 text-slate-300",
      dotClass: "bg-slate-400",
    };
  }

  const formatUntil = (iso?: string | null) => {
    const t = formatTimeSafe(iso);
    return t ? ` until ${t}` : "";
  };

  if (facility.status === "reserved") {
    let detail = facility.reservedFor ? `Reserved for ${facility.reservedFor}` : "Reserved for campus event";
    detail += formatUntil(facility.statusUntil);
    return {
      isOpen: false,
      badge: "Reserved",
      detail,
      badgeClass: "border-purple-400/40 bg-purple-500/20 text-purple-200",
      dotClass: "bg-purple-400",
    };
  }

  if (facility.status === "maintenance") {
    let detail = "Under scheduled maintenance";
    detail += formatUntil(facility.statusUntil);
    return {
      isOpen: false,
      badge: "Temporarily Closed",
      detail,
      badgeClass: "border-amber-400/40 bg-amber-500/20 text-amber-200",
      dotClass: "bg-amber-400",
    };
  }

  if (facility.status === "closed") {
    let detail = facility.statusUntil ? `Closed${formatUntil(facility.statusUntil)}` : "Closed until further notice";
    return {
      isOpen: false,
      badge: "Closed",
      detail,
      badgeClass: "border-slate-400/40 bg-slate-500/20 text-slate-300",
      dotClass: "bg-slate-400",
    };
  }

  const schedules = facility.schedules ?? [];
  if (schedules.length === 0) {
    return {
      isOpen: false,
      badge: "Schedule Pending",
      detail: "Operating hours not registered",
      badgeClass: "border-slate-400/40 bg-slate-500/20 text-slate-300",
      dotClass: "bg-slate-400",
    };
  }

  const currentDay = getPKTDayOfWeek(now);
  const currentSec = getPKTCurrentSeconds(now);

  // 1. Check if currently inside an active window today
  const todaySchedules = schedules
    .filter((s) => s.dayOfWeek === currentDay)
    .sort((a, b) => a.startTime.localeCompare(b.startTime));

  const activeWindow = todaySchedules.find((s) => {
    const startSec = parseTimeToSeconds(s.startTime, false);
    const endSec = parseTimeToSeconds(s.endTime, true);
    if (startSec === null || endSec === null) return false;
    return currentSec >= startSec && currentSec < endSec;
  });

  if (activeWindow) {
    const audienceLabel = getAudienceLabel(activeWindow.audience);
    const audienceSuffix = audienceLabel ? ` (${audienceLabel})` : "";
    return {
      isOpen: true,
      badge: "Open now",
      detail: `Closes at ${formatClockTime(activeWindow.endTime)}${audienceSuffix}`,
      badgeClass: "border-emerald-400/40 bg-emerald-500/20 text-emerald-200",
      dotClass: "bg-emerald-400 animate-pulse",
    };
  }

  // 2. Check if opening later today
  const nextWindowToday = todaySchedules.find((s) => {
    const startSec = parseTimeToSeconds(s.startTime, false);
    return startSec !== null && startSec > currentSec;
  });

  if (nextWindowToday) {
    const audienceLabel = getAudienceLabel(nextWindowToday.audience);
    const audienceSuffix = audienceLabel ? ` (${audienceLabel})` : "";
    return {
      isOpen: false,
      badge: "Closed",
      detail: `Opens at ${formatClockTime(nextWindowToday.startTime)}${audienceSuffix}`,
      badgeClass: "border-rose-400/40 bg-rose-500/20 text-rose-200",
      dotClass: "bg-rose-400",
    };
  }

  // 3. Look forward up to 7 days for the next scheduled opening
  for (let offset = 1; offset <= 7; offset++) {
    const targetDay = ((currentDay - 1 + offset) % 7) + 1;
    const targetDaySchedules = schedules
      .filter((s) => s.dayOfWeek === targetDay)
      .sort((a, b) => a.startTime.localeCompare(b.startTime));

    if (targetDaySchedules.length > 0) {
      const firstWindow = targetDaySchedules[0];
      const audienceLabel = getAudienceLabel(firstWindow.audience);
      const audienceSuffix = audienceLabel ? ` (${audienceLabel})` : "";
      const dayLabel =
        offset === 1
          ? "tomorrow"
          : `${WEEKDAY_NAMES[targetDay] ?? `Day ${targetDay}`}`;
      return {
        isOpen: false,
        badge: "Closed",
        detail: `Opens ${dayLabel} at ${formatClockTime(firstWindow.startTime)}${audienceSuffix}`,
        badgeClass: "border-rose-400/40 bg-rose-500/20 text-rose-200",
        dotClass: "bg-rose-400",
      };
    }
  }

  return {
    isOpen: false,
    badge: "Closed",
    detail: "No upcoming sessions scheduled",
    badgeClass: "border-rose-400/40 bg-rose-500/20 text-rose-200",
    dotClass: "bg-rose-400",
  };
}

export const WEEKDAYS = [
  { day: 1, label: "Mon", full: "Monday" },
  { day: 2, label: "Tue", full: "Tuesday" },
  { day: 3, label: "Wed", full: "Wednesday" },
  { day: 4, label: "Thu", full: "Thursday" },
  { day: 5, label: "Fri", full: "Friday" },
  { day: 6, label: "Sat", full: "Saturday" },
  { day: 7, label: "Sun", full: "Sunday" },
] as const;

export const CAMPUS_AUDIENCE_OPTIONS = [
  { value: "student", label: "Students", detail: "General GIKI enrolled students" },
  { value: "staff", label: "Staff & Families", detail: "Employees, staff & employee kids" },
  { value: "faculty", label: "Faculty", detail: "Teaching & research faculty, graduate MS/PhD" },
  { value: "women_only", label: "Women Only", detail: "Designated women-only sessions" },
  { value: "general", label: "General Community", detail: "Open to all campus community members" },
] as const;

export const TEAM_AUDIENCE_OPTIONS = [
  { value: "badminton_team", label: "GIK Badminton Team + Girls", detail: "Varsity badminton squad & girls players" },
  { value: "basketball_boys", label: "Basketball Boys (GIKI Team)", detail: "Varsity men's basketball squad" },
  { value: "basketball_girls", label: "Basketball Girls (GIKI Team)", detail: "Varsity women's basketball squad" },
  { value: "volleyball_boys", label: "Volleyball Boys", detail: "GIKI men's volleyball squad" },
  { value: "volleyball_girls", label: "Volleyball Girls", detail: "GIKI women's volleyball squad" },
  { value: "cricket_team", label: "Cricket GIKI Teams", detail: "Hard ball & tennis ball cricket squads" },
  { value: "football_team", label: "GIKI Football Team", detail: "Varsity football / soccer squad" },
  { value: "tennis_team", label: "Lawn Tennis (GIKI Team)", detail: "Varsity tennis squad" },
  { value: "squash_team", label: "Squash (GIKI Team)", detail: "Varsity squash squad" },
  { value: "squash_faculty", label: "Squash Faculty", detail: "Faculty & senior staff squash players" },
  { value: "table_tennis_team", label: "Table Tennis (GIKI Team)", detail: "Varsity table tennis squad" },
] as const;

export const AUDIENCE_OPTIONS = [
  ...CAMPUS_AUDIENCE_OPTIONS,
  ...TEAM_AUDIENCE_OPTIONS,
] as const;

export interface GroupedScheduleWindow {
  key: string;
  dayOfWeek: number;
  startTime: string;
  endTime: string;
  schedules: Schedule[];
  audiences: string[];
  audienceText: string;
  primaryId: string;
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
  const navigate = useNavigate();
  const { user } = useAuth();
  const [facility, setFacility] = useState<Facility | null>(null);

  const [searchParams, setSearchParams] = useSearchParams();
  const todayDay = useMemo(() => getPKTDayOfWeek(), []);
  const liveStatus = useMemo(() => getGoogleMapsStatus(facility), [facility]);

  const selectedDay = useMemo((): number | "all" => {
    const dayParam = searchParams.get("day");
    if (dayParam === "all") return "all";
    if (dayParam) {
      const parsed = parseInt(dayParam, 10);
      if (parsed >= 1 && parsed <= 7) return parsed;
    }
    return todayDay;
  }, [searchParams, todayDay]);

  const handleSelectDay = useCallback(
    (day: number | "all") => {
      const newParams = new URLSearchParams(searchParams);
      if (day === todayDay) {
        newParams.delete("day");
      } else {
        newParams.set("day", day.toString());
      }
      setSearchParams(newParams, { replace: true });
    },
    [searchParams, setSearchParams, todayDay]
  );

  const scheduleCountsByDay = useMemo(() => {
    const counts: Record<number, number> = {
      1: 0,
      2: 0,
      3: 0,
      4: 0,
      5: 0,
      6: 0,
      7: 0,
    };
    if (!facility) return counts;
    for (const s of facility.schedules) {
      if (counts[s.dayOfWeek] !== undefined) {
        counts[s.dayOfWeek]++;
      }
    }
    return counts;
  }, [facility]);

  const visibleSchedules = useMemo(() => {
    if (!facility) return [];
    if (selectedDay === "all") return facility.schedules;
    return facility.schedules.filter((s) => s.dayOfWeek === selectedDay);
  }, [facility, selectedDay]);

  const groupedVisibleSchedules = useMemo((): GroupedScheduleWindow[] => {
    const map = new Map<string, GroupedScheduleWindow>();

    for (const s of visibleSchedules) {
      const key = `${s.dayOfWeek}-${s.startTime}-${s.endTime}`;
      const existing = map.get(key);
      if (existing) {
        existing.schedules.push(s);
        if (!existing.audiences.includes(s.audience)) {
          existing.audiences.push(s.audience);
        }
      } else {
        map.set(key, {
          key,
          dayOfWeek: s.dayOfWeek,
          startTime: s.startTime,
          endTime: s.endTime,
          schedules: [s],
          audiences: [s.audience],
          audienceText: "",
          primaryId: s.id,
        });
      }
    }

    const groups = Array.from(map.values());
    for (const g of groups) {
      const order = AUDIENCE_OPTIONS.map((o) => o.value as string);
      g.audiences.sort((a, b) => {
        const ia = order.indexOf(a);
        const ib = order.indexOf(b);
        return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib);
      });
      g.audienceText = g.audiences
        .map((aud) => AUDIENCE_LABELS[aud] ?? aud)
        .join(" + ");
    }

    return groups;
  }, [visibleSchedules]);

  const handleGoBack = useCallback(() => {
    if (window.history.state && typeof window.history.state.idx === "number" && window.history.state.idx > 0) {
      navigate(-1);
    } else if (window.history.length > 1) {
      navigate(-1);
    } else {
      navigate("/#facilities");
    }
  }, [navigate]);
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

  // Inline editing state: tracks one schedule window at a time
  const [editingScheduleId, setEditingScheduleId] = useState<string | null>(null);
  const [_editingGroup, setEditingGroup] = useState<GroupedScheduleWindow | null>(null);
  const [draftDayOfWeek, setDraftDayOfWeek] = useState<number>(1);
  const [draftStartTime, setDraftStartTime] = useState<string>("09:00:00");
  const [draftEndTime, setDraftEndTime] = useState<string>("12:00:00");
  const [draftAudience, setDraftAudience] = useState<string>("student");
  const [_draftAudiences, setDraftAudiences] = useState<string[]>(["student"]);
  const [editAdditionalAudiences, setEditAdditionalAudiences] = useState<string[]>([]);
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [editError, setEditError] = useState<string | null>(null);

  // Deletion state: tracks one schedule window confirmation at a time
  const [deletingScheduleId, setDeletingScheduleId] = useState<string | null>(null);
  const [deletingGroup, setDeletingGroup] = useState<GroupedScheduleWindow | null>(null);
  const [isDeleting, setIsDeleting] = useState<boolean>(false);
  const [deleteError, setDeleteError] = useState<{
    message: string;
    showRefresh?: boolean;
  } | null>(null);

  // Inline add state: tracks creation of a new schedule window
  const [isAdding, setIsAdding] = useState<boolean>(false);
  const [addDayOfWeek, setAddDayOfWeek] = useState<number>(1);
  const [isMultiDay, setIsMultiDay] = useState<boolean>(false);
  const [multiDays, setMultiDays] = useState<number[]>([1]);
  const [multiAudiences, setMultiAudiences] = useState<string[]>(["student"]);
  const [addStartTime, setAddStartTime] = useState<string>("09:00:00");
  const [addEndTime, setAddEndTime] = useState<string>("12:00:00");
  const [addAudience, setAddAudience] = useState<string>("student");
  const [isSavingAdd, setIsSavingAdd] = useState<boolean>(false);
  const [addError, setAddError] = useState<string | null>(null);

  // Facility edit modal state
  const [isEditFacilityModalOpen, setIsEditFacilityModalOpen] = useState<boolean>(false);

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

  const startEditing = (target: Schedule | GroupedScheduleWindow) => {
    if (isSaving || isDeleting || isSavingAdd) return;
    setIsAdding(false);
    setAddError(null);
    setDeletingScheduleId(null);
    setDeletingGroup(null);
    setDeleteError(null);

    if ("schedules" in target) {
      setEditingScheduleId(target.primaryId);
      setEditingGroup(target);
      setDraftDayOfWeek(target.dayOfWeek);
      setDraftStartTime(target.startTime);
      setDraftEndTime(target.endTime);
      const primaryAud = target.audiences[0] || "student";
      setDraftAudience(primaryAud);
      setEditAdditionalAudiences(target.audiences.slice(1));
      setDraftAudiences([...target.audiences]);
    } else {
      setEditingScheduleId(target.id);
      setEditingGroup(null);
      setDraftDayOfWeek(target.dayOfWeek);
      setDraftStartTime(target.startTime);
      setDraftEndTime(target.endTime);
      setDraftAudience(target.audience);
      setEditAdditionalAudiences([]);
      setDraftAudiences([target.audience]);
    }
    setEditError(null);
    setSuccessMessage(null);
  };

  const cancelEditing = () => {
    if (isSaving) return;
    setEditingScheduleId(null);
    setEditingGroup(null);
    setEditAdditionalAudiences([]);
    setEditError(null);
  };

  const startDeleting = (target: Schedule | GroupedScheduleWindow) => {
    if (isSaving || isDeleting || isSavingAdd) return;
    setIsAdding(false);
    setAddError(null);
    setEditingScheduleId(null);
    setEditingGroup(null);
    setEditError(null);
    setDeleteError(null);
    setSuccessMessage(null);

    if ("schedules" in target) {
      setDeletingScheduleId(target.primaryId);
      setDeletingGroup(target);
    } else {
      setDeletingScheduleId(target.id);
      setDeletingGroup(null);
    }
  };

  const cancelDeleting = () => {
    if (isDeleting) return;
    const targetId = deletingScheduleId;
    setDeletingScheduleId(null);
    setDeletingGroup(null);
    setDeleteError(null);
    if (targetId) {
      setTimeout(() => {
        const btn = document.getElementById(`delete-btn-${targetId}`);
        btn?.focus();
      }, 0);
    }
  };

  const handleConfirmDelete = async (
    scheduleId: string,
    group?: GroupedScheduleWindow | null
  ) => {
    if (isDeleting || !slug) return;

    const targetSlug = slug;
    setIsDeleting(true);
    setDeleteError(null);
    setSuccessMessage(null);

    const targetGroup = group || deletingGroup;
    const schedulesToDelete: Schedule[] =
      targetGroup && targetGroup.primaryId === scheduleId
        ? targetGroup.schedules
        : facility?.schedules.filter((s) => s.id === scheduleId) || [];

    const idsToDelete =
      schedulesToDelete.length > 0
        ? schedulesToDelete.map((s) => s.id)
        : [scheduleId];

    try {
      for (const id of idsToDelete) {
        await api.deleteSchedule(targetSlug, id);
      }

      // Guard against late response after navigating away
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      // Update timetable only after confirmed 204
      setFacility((prev) => {
        if (!prev || prev.slug !== targetSlug) return prev;
        const remaining = prev.schedules.filter(
          (item) => !idsToDelete.includes(item.id)
        );

        // Move focus to a sensible nearby control after the deleted row disappears
        if (remaining.length === 0) {
          setTimeout(() => {
            scheduleHeadingRef.current?.focus();
          }, 0);
        } else {
          setTimeout(() => {
            const btn =
              document.getElementById(`delete-btn-${remaining[0]?.id}`) ||
              document.getElementById(`edit-btn-${remaining[0]?.id}`);
            btn?.focus();
          }, 0);
        }

        return {
          ...prev,
          schedules: remaining,
        };
      });

      setDeletingScheduleId(null);
      setDeletingGroup(null);
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
    const validAudiences = AUDIENCE_OPTIONS.map((a) => a.value as string);
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
    setEditAdditionalAudiences([]);
    setEditError(null);
    setDeletingScheduleId(null);
    setDeleteError(null);
    setSuccessMessage(null);
    const initialDay = selectedDay === "all" ? 1 : selectedDay;
    setAddDayOfWeek(initialDay);
    setIsMultiDay(false);
    setMultiDays([initialDay]);
    setMultiAudiences(["student"]);
    setAddStartTime("09:00:00");
    setAddEndTime("12:00:00");
    setAddAudience("student");
    setAddError(null);
    setIsAdding(true);
  };

  const cancelAdding = () => {
    if (isSavingAdd) return;
    setIsAdding(false);
    setIsMultiDay(false);
    setMultiAudiences(["student"]);
    setAddAudience("student");
    setAddError(null);
    setTimeout(() => {
      addWindowButtonRef.current?.focus();
    }, 0);
  };

  const handleAddSave = async () => {
    if (isSavingAdd || !slug) return;

    const targetDays = isMultiDay ? multiDays : [addDayOfWeek];
    const targetAudiences = multiAudiences.length > 0 ? multiAudiences : [addAudience];

    if (targetDays.length === 0) {
      setAddError("Please select at least one day to apply this schedule.");
      return;
    }

    if (targetAudiences.length === 0) {
      setAddError("Please select at least one audience cohort (e.g. Students, Staff & Families).");
      return;
    }

    for (const aud of targetAudiences) {
      const clientErr = validateScheduleWindow(
        targetDays[0],
        addStartTime,
        addEndTime,
        aud
      );
      if (clientErr) {
        setAddError(clientErr);
        return;
      }
    }

    const targetSlug = slug;
    setIsSavingAdd(true);
    setAddError(null);

    const createdSchedules: Schedule[] = [];
    const duplicateSlots: { day: number; audience: string }[] = [];

    try {
      for (const day of targetDays) {
        for (const aud of targetAudiences) {
          const payload: CreateScheduleInput = {
            dayOfWeek: day,
            startTime: normalizeTimeToSeconds(addStartTime),
            endTime: normalizeTimeToSeconds(addEndTime),
            audience: aud,
          };

          try {
            const created = await api.createSchedule(targetSlug, payload);
            createdSchedules.push(created);
          } catch (err) {
            if (err instanceof ApiError && err.status === 409) {
              duplicateSlots.push({ day, audience: aud });
            } else {
              throw err;
            }
          }
        }
      }

      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      if (createdSchedules.length > 0) {
        setFacility((prev) => {
          if (!prev || prev.slug !== targetSlug) return prev;
          return {
            ...prev,
            schedules: sortSchedules([...prev.schedules, ...createdSchedules]),
          };
        });

        setIsAdding(false);
        setAddError(null);
        setIsMultiDay(false);
        setMultiAudiences(["student"]);
        setAddAudience("student");

        if (duplicateSlots.length > 0) {
          const dupInfo = duplicateSlots
            .map((d) => `${WEEKDAY_NAMES[d.day]} (${AUDIENCE_LABELS[d.audience] ?? d.audience})`)
            .join(", ");
          setSuccessMessage(
            `Added ${createdSchedules.length} schedule ${
              createdSchedules.length === 1 ? "window" : "windows"
            }. Skipped duplicates on: ${dupInfo}.`
          );
        } else if (createdSchedules.length === 1) {
          if (selectedDay !== "all" && selectedDay !== createdSchedules[0].dayOfWeek) {
            handleSelectDay(createdSchedules[0].dayOfWeek);
          }
          setSuccessMessage("Schedule window added successfully.");
        } else {
          const uniqueDays = new Set(createdSchedules.map((s) => s.dayOfWeek)).size;
          const uniqueAudiences = new Set(createdSchedules.map((s) => s.audience)).size;
          setSuccessMessage(
            `Schedule windows added successfully (${createdSchedules.length} windows for ${uniqueAudiences} ${
              uniqueAudiences === 1 ? "audience" : "audiences"
            } across ${uniqueDays} ${uniqueDays === 1 ? "day" : "days"}).`
          );
        }
      } else if (duplicateSlots.length > 0) {
        setAddError(
          targetDays.length === 1 && targetAudiences.length === 1
            ? "This schedule window duplicates an existing operating window for this facility."
            : "This schedule window duplicates an existing operating window for all selected days and audiences."
        );
      }
    } catch (err) {
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      if (createdSchedules.length > 0) {
        setFacility((prev) => {
          if (!prev || prev.slug !== targetSlug) return prev;
          return {
            ...prev,
            schedules: sortSchedules([...prev.schedules, ...createdSchedules]),
          };
        });
      }

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

  const handleSave = async (scheduleId: string, _group?: GroupedScheduleWindow) => {
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

      // Also create any additional allocated audiences
      const createdAdditional: Schedule[] = [];
      const duplicateAdditional: string[] = [];
      for (const extraAud of editAdditionalAudiences) {
        try {
          const created = await api.createSchedule(targetSlug, {
            dayOfWeek: draftDayOfWeek,
            startTime: normalizeTimeToSeconds(draftStartTime),
            endTime: normalizeTimeToSeconds(draftEndTime),
            audience: extraAud,
          });
          createdAdditional.push(created);
        } catch (err) {
          if (err instanceof ApiError && err.status === 409) {
            duplicateAdditional.push(extraAud);
          } else {
            throw err;
          }
        }
      }

      // Guard against late response after navigating away
      if (!isMountedRef.current || currentSlugRef.current !== targetSlug) {
        return;
      }

      // Update timetable in local state directly from confirmed server response
      setFacility((prev) => {
        if (!prev || prev.slug !== targetSlug) return prev;
        const updatedList = prev.schedules.map((item) =>
          item.id === updatedSchedule.id ? updatedSchedule : item
        );
        return {
          ...prev,
          schedules: sortSchedules([...updatedList, ...createdAdditional]),
        };
      });

      setEditingScheduleId(null);
      setEditAdditionalAudiences([]);
      setEditError(null);

      if (createdAdditional.length > 0) {
        const extraNames = createdAdditional
          .map((s) => AUDIENCE_LABELS[s.audience] ?? s.audience)
          .join(", ");
        setSuccessMessage(
          `Schedule window updated and also allocated to: ${extraNames}.`
        );
      } else {
        setSuccessMessage("Schedule window updated successfully.");
      }
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

  return (
    <div className="min-h-screen bg-white">
      <Navbar />
      <main className="mx-auto max-w-3xl px-6 py-8">
        <button
          type="button"
          onClick={handleGoBack}
          aria-label="Go back"
          className="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-maroon)] transition-opacity hover:opacity-80 cursor-pointer"
        >
          <ArrowLeft size={16} />
          Go back
        </button>

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
            <button
              type="button"
              onClick={handleGoBack}
              className="mt-5 inline-block rounded-md bg-[var(--color-maroon)] px-5 py-2 text-xs font-semibold text-white transition-opacity hover:opacity-90 cursor-pointer"
            >
              Go back
            </button>
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
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-3">
                  <h1 className="font-display text-2xl font-semibold text-[#F3E6C4]">
                    {facility.name}
                  </h1>
                  <span
                    data-testid="live-operating-status"
                    className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-0.5 text-xs font-medium ${liveStatus.badgeClass}`}
                  >
                    <span className={`h-2 w-2 rounded-full ${liveStatus.dotClass}`} />
                    <span>{liveStatus.badge}</span>
                  </span>
                </div>

                <div
                  data-testid="live-operating-detail"
                  className="mt-2 flex items-center gap-2 text-xs font-medium text-[var(--color-gold-light)]"
                >
                  <Clock size={14} className="flex-shrink-0 text-[var(--color-gold)]" />
                  <span>
                    <strong className="text-white">{liveStatus.badge}</strong>
                    <span className="mx-1.5 opacity-60">•</span>
                    <span>{liveStatus.detail}</span>
                  </span>
                </div>

                  <div className="mt-2">
                    {/* Status Notice / Reservation Banner */}
                    {(facility.status === "reserved" || facility.statusNote || (facility.status !== "active" && facility.statusUntil)) && (
                      <div
                        data-testid="facility-status-notice"
                        className={`mb-3 rounded-lg border p-3 text-xs flex items-start gap-2.5 ${
                          facility.status === "reserved"
                            ? "border-purple-400/50 bg-purple-950/40 text-purple-200"
                            : facility.status === "maintenance"
                            ? "border-amber-400/50 bg-amber-950/40 text-amber-200"
                            : facility.status === "closed"
                            ? "border-slate-400/50 bg-slate-900/50 text-slate-200"
                            : "border-[var(--color-gold)]/40 bg-[var(--color-navy-soft)]/50 text-[var(--color-gold-light)]"
                        }`}
                      >
                        <Info size={16} className="shrink-0 mt-0.5 text-current opacity-90" />
                        <div className="space-y-0.5">
                          {facility.status === "reserved" && (
                            <p className="font-semibold text-white">
                              Reserved {facility.reservedFor ? `for ${facility.reservedFor}` : "for Campus Event"}
                              {facility.statusUntil && (
                                <span className="font-normal text-purple-300 ml-1">
                                  (until {formatTimeSafe(facility.statusUntil)})
                                </span>
                              )}
                            </p>
                          )}
                          {facility.status === "maintenance" && facility.statusUntil && (
                            <p className="font-semibold text-white">
                              Scheduled maintenance until {formatDateTimeSafe(facility.statusUntil)}
                            </p>
                          )}
                          {facility.status === "closed" && facility.statusUntil && (
                            <p className="font-semibold text-white">
                              Temporarily closed until {formatDateTimeSafe(facility.statusUntil)}
                            </p>
                          )}
                          {facility.statusNote && (
                            <p className="leading-relaxed opacity-95">
                              {facility.statusNote}
                            </p>
                          )}
                        </div>
                      </div>
                    )}

                    {facility.description?.trim() ? (
                      <p
                        data-testid="facility-description-text"
                        className="text-sm text-[var(--color-gold-light)] leading-relaxed"
                      >
                        {facility.description}
                      </p>
                    ) : null}

                    {canEditSchedule && (
                      <div className="mt-3 flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          onClick={() => setIsEditFacilityModalOpen(true)}
                          data-testid="edit-facility-btn"
                          aria-label="Edit facility details"
                          className="inline-flex items-center gap-1.5 rounded-md border border-[var(--color-gold)] bg-[var(--color-gold)]/20 px-3 py-1 text-xs font-semibold text-white shadow-2xs transition-colors hover:bg-[var(--color-gold)]/30 hover:border-white focus-visible:outline-2 focus-visible:outline-[var(--color-gold)] cursor-pointer"
                        >
                          <Pencil size={13} className="text-[var(--color-gold)]" />
                          <span>Edit Facility</span>
                        </button>
                      </div>
                    )}
                  </div>
              </div>
            </div>

            {/* Access Model Action Banners: Padel Online Reservation & Gym Membership */}
            {Boolean(
              facility.slug.toLowerCase().includes("padel") ||
                facility.name.toLowerCase().includes("padel")
            ) && (
              <div
                data-testid="padel-booking-banner"
                className="mt-6 rounded-xl border border-emerald-300 bg-emerald-50/80 p-4 text-emerald-950 shadow-2xs"
              >
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <h2 className="font-display text-sm font-bold text-emerald-950">
                      Online Court Booking Available
                    </h2>
                    <p className="mt-0.5 text-xs text-emerald-800">
                      Reserve 60-minute match slots with student wallet checkout and QR check-in.
                    </p>
                  </div>
                  <Link
                    to={`/padel?facility=${encodeURIComponent(facility.slug)}`}
                    className="inline-flex flex-shrink-0 items-center justify-center gap-1.5 rounded-lg bg-[var(--color-maroon)] px-4 py-2 text-xs font-semibold text-white shadow-2xs transition-all hover:bg-[#6b1420] active:scale-98"
                  >
                    <span>Book Court Slot</span>
                    <ArrowRight size={13} />
                  </Link>
                </div>
              </div>
            )}

            {Boolean(
              facility.slug.toLowerCase().includes("gym") ||
                facility.slug.toLowerCase().includes("fitness") ||
                facility.name.toLowerCase().includes("gym") ||
                facility.name.toLowerCase().includes("fitness")
            ) && (
              <div
                data-testid="gym-membership-banner"
                className="mt-6 space-y-3"
              >
                <div className="flex items-center justify-between">
                  <h3 className="font-display text-xs font-bold text-[var(--color-navy)] uppercase tracking-wider">
                    Gymnasium Sections & Access Model
                  </h3>
                  <span className="text-[11px] text-[var(--color-navy-soft)]">
                    Two facilities under one complex
                  </span>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {/* Sub-venue 1: New Gym (Paid Membership Pass) */}
                  <div className="rounded-xl border-2 border-indigo-200 bg-indigo-50/80 p-4.5 flex flex-col justify-between shadow-2xs">
                    <div>
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex items-center gap-2.5">
                          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-[var(--color-navy)] text-white flex-shrink-0">
                            <Dumbbell size={16} />
                          </span>
                          <div>
                            <h4 className="font-display text-sm font-bold text-[var(--color-navy)]">
                              New Gym
                            </h4>
                            <p className="text-[11px] font-medium text-indigo-900">
                              Fitness & Strength Suite
                            </p>
                          </div>
                        </div>
                        <span className="inline-flex items-center rounded-full bg-indigo-100 px-2 py-0.5 text-[10px] font-bold text-indigo-900 border border-indigo-300 flex-shrink-0">
                          PKR 3,000 / 30 Days
                        </span>
                      </div>

                      <p className="mt-2.5 text-xs text-indigo-950 leading-relaxed">
                        Olympic racks, specialized free weights, cardio machines, and functional rigs. Requires an active 30-day membership pass.
                      </p>
                    </div>

                    <div className="mt-3.5 pt-3 border-t border-indigo-200/80 flex items-center justify-between">
                      <span className="text-[11px] font-medium text-indigo-800">
                        Pass verified at entry
                      </span>
                      <Link
                        to="/membership"
                        className="inline-flex items-center justify-center gap-1 rounded-lg bg-[var(--color-maroon)] px-3 py-1.5 text-xs font-semibold text-white shadow-2xs transition-all hover:bg-[var(--color-maroon-dark)] active:scale-98"
                      >
                        <span>Get Pass</span>
                        <ArrowRight size={12} />
                      </Link>
                    </div>
                  </div>

                  {/* Sub-venue 2: Old Gym (100% Free Access) */}
                  <div className="rounded-xl border-2 border-emerald-200 bg-emerald-50/80 p-4.5 flex flex-col justify-between shadow-2xs">
                    <div>
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex items-center gap-2.5">
                          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-800 text-white flex-shrink-0">
                            <Dumbbell size={16} />
                          </span>
                          <div>
                            <h4 className="font-display text-sm font-bold text-emerald-950">
                              Old Gym
                            </h4>
                            <p className="text-[11px] font-medium text-emerald-800">
                              Campus Conditioning Room
                            </p>
                          </div>
                        </div>
                        <span className="inline-flex items-center rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-bold text-emerald-900 border border-emerald-300 flex-shrink-0">
                          100% Free Access
                        </span>
                      </div>

                      <p className="mt-2.5 text-xs text-emerald-950 leading-relaxed">
                        Traditional barbell benches, standard free weights, pull-up rigs, and open workout floor. Open to all registered students without fee.
                      </p>
                    </div>

                    <div className="mt-3.5 pt-3 border-t border-emerald-200/80 flex items-center justify-between">
                      <span className="text-[11px] font-medium text-emerald-800">
                        No pass or payment needed
                      </span>
                      <a
                        href="#schedule"
                        className="inline-flex items-center justify-center gap-1 rounded-lg border border-emerald-300 bg-white px-3 py-1.5 text-xs font-semibold text-emerald-900 transition-all hover:bg-emerald-100/50"
                      >
                        <span>View Hours</span>
                      </a>
                    </div>
                  </div>
                </div>
              </div>
            )}

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

              {/* Day Selector Tabs */}
              {facility.schedules.length > 0 && (
                <div
                  role="tablist"
                  aria-label="Operating Schedule Days"
                  data-testid="schedule-day-tabs"
                  className="mt-4 flex flex-wrap items-center gap-1.5 pb-1"
                >
                  {WEEKDAYS.map(({ day, label, full }) => {
                    const isSelected = selectedDay === day;
                    const isToday = day === todayDay;
                    const count = scheduleCountsByDay[day];

                    return (
                      <button
                        key={day}
                        type="button"
                        role="tab"
                        id={`day-tab-${day}`}
                        data-testid={`day-tab-${day}`}
                        aria-selected={isSelected}
                        aria-label={`${full}${isToday ? " (Today)" : ""}, ${count} scheduled ${
                          count === 1 ? "window" : "windows"
                        }`}
                        onClick={() => handleSelectDay(day)}
                        className={`relative flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-semibold transition-all ${
                          isSelected
                            ? isToday
                              ? "bg-[var(--color-maroon)] text-white shadow-xs"
                              : "bg-[var(--color-navy)] text-white shadow-xs"
                            : "bg-white text-[var(--color-navy)] border border-[var(--color-border)] hover:bg-[var(--color-surface)]"
                        }`}
                      >
                        <span>{label}</span>
                        {isToday && (
                          <span
                            className={`rounded-full px-1.5 py-0.2 text-[9px] font-bold uppercase tracking-wider ${
                              isSelected
                                ? "bg-white/20 text-white"
                                : "bg-[var(--color-maroon)]/10 text-[var(--color-maroon)]"
                            }`}
                          >
                            Today
                          </span>
                        )}
                        {count > 0 && !isToday && (
                          <span
                            className={`h-1.5 w-1.5 rounded-full ${
                              isSelected
                                ? "bg-[var(--color-gold)]"
                                : "bg-[var(--color-navy-soft)]"
                            }`}
                          />
                        )}
                      </button>
                    );
                  })}

                  <div className="hidden sm:block h-4 w-px bg-[var(--color-border)] mx-1" />

                  {/* All Week Tab */}
                  <button
                    type="button"
                    role="tab"
                    id="day-tab-all"
                    data-testid="day-tab-all"
                    aria-selected={selectedDay === "all"}
                    aria-label={`All Week, ${facility.schedules.length} total scheduled windows`}
                    onClick={() => handleSelectDay("all")}
                    className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition-all ${
                      selectedDay === "all"
                        ? "bg-[var(--color-navy)] text-white shadow-xs"
                        : "bg-white text-[var(--color-navy-soft)] border border-[var(--color-border)] hover:text-[var(--color-navy)] hover:bg-[var(--color-surface)]"
                    }`}
                  >
                    All Week
                  </button>
                </div>
              )}

              {/* Active Day Subtitle / Filter Info */}
              {facility.schedules.length > 0 && (
                <div className="mt-3 flex items-center justify-between text-xs text-[var(--color-navy-soft)]">
                  <p>
                    {selectedDay === "all" ? (
                      <span>
                        Showing all{" "}
                        <strong className="text-[var(--color-navy)] font-semibold">
                          {facility.schedules.length}
                        </strong>{" "}
                        weekly operating windows across all days
                      </span>
                    ) : (
                      <span>
                        Timings for{" "}
                        <strong className="text-[var(--color-navy)] font-semibold">
                          {typeof selectedDay === "number"
                            ? WEEKDAY_NAMES[selectedDay]
                            : "All Days"}
                        </strong>
                        {selectedDay === todayDay ? " (Today)" : ""}:
                      </span>
                    )}
                  </p>
                  {selectedDay !== "all" && selectedDay === todayDay && (
                    <span className="text-[11px] font-medium text-[var(--color-maroon)]">
                      PKT local time
                    </span>
                  )}
                </div>
              )}

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

                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
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
                          className="block text-xs font-semibold uppercase tracking-wider text-[var(--color-navy)] mb-1"
                        >
                          Start Time
                        </label>
                        <TimePicker
                          id="add-start"
                          ariaLabel="Start time"
                          value={addStartTime}
                          onChange={setAddStartTime}
                          disabled={isSavingAdd}
                          placeholder="09:00:00"
                        />
                      </div>

                      {/* End Time */}
                      <div>
                        <label
                          htmlFor="add-end"
                          className="block text-xs font-semibold uppercase tracking-wider text-[var(--color-navy)] mb-1"
                        >
                          End Time
                        </label>
                        <TimePicker
                          id="add-end"
                          ariaLabel="End time"
                          value={addEndTime}
                          onChange={setAddEndTime}
                          isEndTime={true}
                          disabled={isSavingAdd}
                          placeholder="12:00:00"
                          midnightButtonTestId="add-set-midnight-btn"
                        />
                      </div>
                    </div>

                    {/* Accessible hidden select for tests & assistive tech */}
                    <select
                      id="add-audience"
                      aria-label="Audience"
                      disabled={isSavingAdd}
                      value={addAudience}
                      onChange={(e) => {
                        const val = e.target.value;
                        setAddAudience(val);
                        setMultiAudiences([val]);
                      }}
                      className="sr-only"
                    >
                      <optgroup label="Section A: Campus Cohorts">
                        {CAMPUS_AUDIENCE_OPTIONS.map((o) => (
                          <option key={o.value} value={o.value}>
                            {o.label}
                          </option>
                        ))}
                      </optgroup>
                      <optgroup label="Section B: GIKI Sports Teams & Dedicated Cohorts">
                        {TEAM_AUDIENCE_OPTIONS.map((o) => (
                          <option key={o.value} value={o.value}>
                            {o.label}
                          </option>
                        ))}
                      </optgroup>
                    </select>

                    {/* Target Audiences: Approach 2 - Both Section A & Section B toggleable in any combination */}
                    <div className="pt-2 border-t border-[var(--color-border)]/60">
                      <div
                        data-testid="multi-audience-pills-container"
                        className="space-y-3"
                      >
                        <div className="flex flex-wrap items-center justify-between gap-1">
                          <span className="text-xs font-semibold text-[var(--color-navy)]">
                            Target Audiences (select any combination across sections):
                          </span>
                          <span className="text-[11px] font-medium text-[var(--color-maroon)]">
                            {multiAudiences.length} {multiAudiences.length === 1 ? "audience" : "audiences"} selected
                          </span>
                        </div>

                        {/* Section A: Campus Cohorts */}
                        <div className="rounded-lg border border-[var(--color-border)]/70 bg-stone-50/50 p-2.5">
                          <div className="mb-2 flex items-center justify-between">
                            <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--color-maroon)]">
                              Section A • Campus Cohorts
                            </span>
                            <span className="text-[10px] text-[var(--color-navy-soft)]">
                              Campus groups & community
                            </span>
                          </div>
                          <div className="flex flex-wrap items-center gap-1.5">
                            {CAMPUS_AUDIENCE_OPTIONS.map(({ value, label, detail }) => {
                              const isChecked = multiAudiences.includes(value);
                              return (
                                <button
                                  key={value}
                                  type="button"
                                  disabled={isSavingAdd}
                                  title={detail}
                                  onClick={() => {
                                    setMultiAudiences((prev) => {
                                      const next = prev.includes(value)
                                        ? prev.filter((a) => a !== value)
                                        : [...prev, value];
                                      if (next.length > 0) {
                                        setAddAudience(next[0]);
                                      }
                                      return next;
                                    });
                                  }}
                                  data-testid={`multi-audience-btn-${value}`}
                                  aria-label={`Toggle ${label}`}
                                  aria-pressed={isChecked}
                                  className={`inline-flex items-center gap-1 rounded-md px-2.5 py-1 text-xs font-semibold transition-colors cursor-pointer ${
                                    isChecked
                                      ? "bg-[var(--color-maroon)] text-white shadow-2xs"
                                      : "border border-[var(--color-border)] bg-white text-[var(--color-navy)] hover:bg-[var(--color-surface)]"
                                  }`}
                                >
                                  {isChecked && <span className="text-[10px]">✓</span>}
                                  <span>{label}</span>
                                </button>
                              );
                            })}
                          </div>
                        </div>

                        {/* Section B: GIKI Sports Teams & Dedicated Cohorts */}
                        <div className="rounded-lg border border-[var(--color-border)]/70 bg-stone-50/50 p-2.5">
                          <div className="mb-2 flex items-center justify-between">
                            <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--color-navy)]">
                              Section B • GIKI Sports Teams & Dedicated Cohorts
                            </span>
                            <span className="text-[10px] text-[var(--color-navy-soft)]">
                              Varsity teams & dedicated sports squads
                            </span>
                          </div>
                          <div className="flex flex-wrap items-center gap-1.5">
                            {TEAM_AUDIENCE_OPTIONS.map(({ value, label, detail }) => {
                              const isChecked = multiAudiences.includes(value);
                              return (
                                <button
                                  key={value}
                                  type="button"
                                  disabled={isSavingAdd}
                                  title={detail}
                                  onClick={() => {
                                    setMultiAudiences((prev) => {
                                      const next = prev.includes(value)
                                        ? prev.filter((a) => a !== value)
                                        : [...prev, value];
                                      if (next.length > 0) {
                                        setAddAudience(next[0]);
                                      }
                                      return next;
                                    });
                                  }}
                                  data-testid={`multi-audience-btn-${value}`}
                                  aria-label={`Toggle ${label}`}
                                  aria-pressed={isChecked}
                                  className={`inline-flex items-center gap-1 rounded-md px-2.5 py-1 text-xs font-semibold transition-colors cursor-pointer ${
                                    isChecked
                                      ? "bg-[var(--color-navy)] text-white shadow-2xs"
                                      : "border border-[var(--color-border)] bg-white text-[var(--color-navy)] hover:bg-[var(--color-surface)]"
                                  }`}
                                >
                                  {isChecked && <span className="text-[10px]">✓</span>}
                                  <span>{label}</span>
                                </button>
                              );
                            })}
                          </div>
                        </div>
                      </div>
                    </div>

                    {/* Multi-Day / Recurring Weekly Application */}
                    <div className="pt-2 border-t border-[var(--color-border)]/60">
                      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                        <label className="flex items-center gap-2 cursor-pointer text-xs font-semibold text-[var(--color-navy)]">
                          <input
                            type="checkbox"
                            data-testid="toggle-multi-day-checkbox"
                            aria-label="Apply to multiple days"
                            checked={isMultiDay}
                            disabled={isSavingAdd}
                            onChange={(e) => {
                              const checked = e.target.checked;
                              setIsMultiDay(checked);
                              if (checked && multiDays.length === 0) {
                                setMultiDays([addDayOfWeek]);
                              }
                            }}
                            className="h-3.5 w-3.5 rounded border-[var(--color-border)] text-[var(--color-maroon)] focus:ring-[var(--color-gold)] cursor-pointer"
                          />
                          <span>Apply to multiple days (recurring weekly)</span>
                        </label>

                        {isMultiDay && (
                          <div className="flex flex-wrap items-center gap-1 text-[11px]">
                            <button
                              type="button"
                              disabled={isSavingAdd}
                              onClick={() => setMultiDays([1, 2, 3, 4, 5])}
                              className="rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-navy-soft)] hover:text-[var(--color-navy)] hover:bg-[var(--color-surface)] cursor-pointer"
                            >
                              Weekdays (Mon–Fri)
                            </button>
                            <button
                              type="button"
                              disabled={isSavingAdd}
                              onClick={() => setMultiDays([1, 2, 3, 4, 5, 6, 7])}
                              className="rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-navy-soft)] hover:text-[var(--color-navy)] hover:bg-[var(--color-surface)] cursor-pointer"
                            >
                              Every Day
                            </button>
                            <button
                              type="button"
                              disabled={isSavingAdd}
                              onClick={() => setMultiDays([6, 7])}
                              className="rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-navy-soft)] hover:text-[var(--color-navy)] hover:bg-[var(--color-surface)] cursor-pointer"
                            >
                              Weekends
                            </button>
                          </div>
                        )}
                      </div>

                      {isMultiDay && (
                        <div
                          data-testid="multi-day-pills-container"
                          className="mt-2.5 flex flex-wrap items-center gap-1.5 pt-1.5 border-t border-[var(--color-border)]/40"
                        >
                          <span className="text-[11px] font-medium text-[var(--color-navy-soft)] mr-1">
                            Days:
                          </span>
                          {WEEKDAYS.map(({ day, label, full }) => {
                            const isChecked = multiDays.includes(day);
                            return (
                              <button
                                key={day}
                                type="button"
                                disabled={isSavingAdd}
                                onClick={() => {
                                  setMultiDays((prev) =>
                                    prev.includes(day)
                                      ? prev.filter((d) => d !== day)
                                      : [...prev, day].sort((a, b) => a - b)
                                  );
                                }}
                                data-testid={`multi-day-btn-${day}`}
                                aria-label={`Toggle ${full}`}
                                aria-pressed={isChecked}
                                className={`rounded-md px-2.5 py-1 text-xs font-semibold transition-colors cursor-pointer ${
                                  isChecked
                                    ? "bg-[var(--color-maroon)] text-white shadow-2xs"
                                    : "border border-[var(--color-border)] bg-white text-[var(--color-navy)] hover:bg-[var(--color-surface)]"
                                }`}
                              >
                                {label}
                              </button>
                            );
                          })}
                        </div>
                      )}
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
              ) : visibleSchedules.length === 0 ? (
                <div
                  data-testid="schedule-day-empty"
                  className="mt-3 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center text-sm text-[var(--color-navy-soft)]"
                >
                  <Clock
                    size={28}
                    className="mx-auto text-[var(--color-navy-soft)] mb-2 opacity-60"
                  />
                  <p className="font-semibold text-[var(--color-navy)]">
                    No operating hours scheduled for{" "}
                    {typeof selectedDay === "number"
                      ? WEEKDAY_NAMES[selectedDay]
                      : "this day"}{" "}
                    {selectedDay === todayDay ? "(Today)" : ""}
                  </p>
                  <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
                    This facility may be closed or reserved on this day.
                  </p>
                  <div className="mt-4 flex flex-wrap items-center justify-center gap-3">
                    <button
                      type="button"
                      onClick={() => handleSelectDay("all")}
                      className="inline-flex items-center gap-1 text-xs font-semibold text-[var(--color-maroon)] hover:underline"
                    >
                      <span>View all week timetable</span>
                      <ArrowRight size={12} />
                    </button>
                    {canEditSchedule && !isAdding && (
                      <button
                        type="button"
                        onClick={startAdding}
                        className="inline-flex items-center gap-1 rounded-md bg-[var(--color-maroon)] px-3 py-1.5 text-xs font-semibold text-white hover:opacity-90"
                      >
                        <Plus size={13} />
                        <span>
                          Add window for{" "}
                          {typeof selectedDay === "number"
                            ? WEEKDAY_NAMES[selectedDay]
                            : "this day"}
                        </span>
                      </button>
                    )}
                  </div>
                </div>
              ) : (
                <div className="mt-3 divide-y divide-[var(--color-border)] rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
                  {groupedVisibleSchedules.map((group) => {
                    const isEditingThisRow = editingScheduleId === group.primaryId;

                    if (isEditingThisRow) {
                      return (
                        <div
                          key={group.key}
                          className="bg-white p-4 first:rounded-t-xl last:rounded-b-xl"
                        >
                          <form
                            data-testid={`schedule-edit-form-${group.primaryId}`}
                            onSubmit={(e) => {
                              e.preventDefault();
                              handleSave(group.primaryId, group);
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
                                  htmlFor={`edit-day-${group.primaryId}`}
                                  className="block text-xs font-medium text-[var(--color-navy)] mb-1"
                                >
                                  Day of Week
                                </label>
                                <select
                                  id={`edit-day-${group.primaryId}`}
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
                                  htmlFor={`edit-start-${group.primaryId}`}
                                  className="block text-xs font-semibold uppercase tracking-wider text-[var(--color-navy)] mb-1"
                                >
                                  Start Time
                                </label>
                                <TimePicker
                                  id={`edit-start-${group.primaryId}`}
                                  ariaLabel="Start time"
                                  value={draftStartTime}
                                  onChange={setDraftStartTime}
                                  disabled={isSaving}
                                  placeholder="09:00:00"
                                />
                              </div>

                              {/* End Time */}
                              <div>
                                <label
                                  htmlFor={`edit-end-${group.primaryId}`}
                                  className="block text-xs font-semibold uppercase tracking-wider text-[var(--color-navy)] mb-1"
                                >
                                  End Time
                                </label>
                                <TimePicker
                                  id={`edit-end-${group.primaryId}`}
                                  ariaLabel="End time"
                                  value={draftEndTime}
                                  onChange={setDraftEndTime}
                                  isEndTime={true}
                                  disabled={isSaving}
                                  placeholder="12:00:00"
                                />
                              </div>

                              {/* Audience */}
                              <div>
                                <label
                                  htmlFor={`edit-audience-${group.primaryId}`}
                                  className="block text-xs font-medium text-[var(--color-navy)] mb-1"
                                >
                                  Audience
                                </label>
                                <select
                                  id={`edit-audience-${group.primaryId}`}
                                  aria-label="Audience"
                                  disabled={isSaving}
                                  value={draftAudience}
                                  onChange={(e) => {
                                    const val = e.target.value;
                                    setDraftAudience(val);
                                    setEditAdditionalAudiences((prev) =>
                                      prev.filter((a) => a !== val)
                                    );
                                  }}
                                  className="w-full rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1.5 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                                >
                                  <optgroup label="Section A: Campus Cohorts">
                                    {CAMPUS_AUDIENCE_OPTIONS.map((o) => (
                                      <option key={o.value} value={o.value}>
                                        {o.label}
                                      </option>
                                    ))}
                                  </optgroup>
                                  <optgroup label="Section B: GIKI Sports Teams & Dedicated Cohorts">
                                    {TEAM_AUDIENCE_OPTIONS.map((o) => (
                                      <option key={o.value} value={o.value}>
                                        {o.label}
                                      </option>
                                    ))}
                                  </optgroup>
                                </select>
                              </div>
                            </div>

                            {/* Additional Audience Allocation in Edit Form */}
                            <div className="pt-2 border-t border-[var(--color-border)]/60">
                              <div className="flex flex-col gap-2">
                                <span className="text-xs font-semibold text-[var(--color-navy)]">
                                  Also allocate this time window to additional audiences:
                                </span>

                                {/* Section A: Campus Cohorts */}
                                <div>
                                  <span className="text-[10px] font-bold uppercase tracking-wider text-[var(--color-maroon)] block mb-1">
                                    Section A • Campus Cohorts
                                  </span>
                                  <div className="flex flex-wrap items-center gap-1.5">
                                    {CAMPUS_AUDIENCE_OPTIONS.filter((a) => a.value !== draftAudience).map(
                                      ({ value, label, detail }) => {
                                        const isChecked = editAdditionalAudiences.includes(value);
                                        return (
                                          <label
                                            key={value}
                                            title={detail}
                                            className={`inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium cursor-pointer border transition-colors ${
                                              isChecked
                                                ? "bg-[var(--color-maroon)]/10 border-[var(--color-maroon)] text-[var(--color-maroon)] font-semibold"
                                                : "bg-white border-[var(--color-border)] text-[var(--color-navy)] hover:bg-[var(--color-surface)]"
                                            }`}
                                          >
                                            <input
                                              type="checkbox"
                                              disabled={isSaving}
                                              data-testid={`edit-add-audience-${value}`}
                                              checked={isChecked}
                                              onChange={(e) => {
                                                const checked = e.target.checked;
                                                setEditAdditionalAudiences((prev) =>
                                                  checked
                                                    ? [...prev, value]
                                                    : prev.filter((a) => a !== value)
                                                );
                                              }}
                                              className="h-3 w-3 rounded text-[var(--color-maroon)] cursor-pointer"
                                            />
                                            <span>{label}</span>
                                          </label>
                                        );
                                      }
                                    )}
                                  </div>
                                </div>

                                {/* Section B: GIKI Sports Teams & Dedicated Cohorts */}
                                <div>
                                  <span className="text-[10px] font-bold uppercase tracking-wider text-[var(--color-navy)] block mb-1">
                                    Section B • GIKI Sports Teams & Dedicated Cohorts
                                  </span>
                                  <div className="flex flex-wrap items-center gap-1.5">
                                    {TEAM_AUDIENCE_OPTIONS.filter((a) => a.value !== draftAudience).map(
                                      ({ value, label, detail }) => {
                                        const isChecked = editAdditionalAudiences.includes(value);
                                        return (
                                          <label
                                            key={value}
                                            title={detail}
                                            className={`inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium cursor-pointer border transition-colors ${
                                              isChecked
                                                ? "bg-[var(--color-navy)]/10 border-[var(--color-navy)] text-[var(--color-navy)] font-semibold"
                                                : "bg-white border-[var(--color-border)] text-[var(--color-navy)] hover:bg-[var(--color-surface)]"
                                            }`}
                                          >
                                            <input
                                              type="checkbox"
                                              disabled={isSaving}
                                              data-testid={`edit-add-audience-${value}`}
                                              checked={isChecked}
                                              onChange={(e) => {
                                                const checked = e.target.checked;
                                                setEditAdditionalAudiences((prev) =>
                                                  checked
                                                    ? [...prev, value]
                                                    : prev.filter((a) => a !== value)
                                                );
                                              }}
                                              className="h-3 w-3 rounded text-[var(--color-navy)] cursor-pointer"
                                            />
                                            <span>{label}</span>
                                          </label>
                                        );
                                      }
                                    )}
                                  </div>
                                </div>

                                <p className="text-[11px] text-[var(--color-navy-soft)]">
                                  Checking additional groups creates concurrent operating windows for this exact time slot (e.g. both students and employee kids).
                                </p>
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

                    const isDeletingThisRow = deletingScheduleId === group.primaryId;

                    if (isDeletingThisRow) {
                      return (
                        <div
                          key={group.key}
                          role="region"
                          aria-label={`Confirm deletion of ${
                            WEEKDAY_NAMES[group.dayOfWeek] ?? `Day ${group.dayOfWeek}`
                          } schedule window`}
                          aria-describedby={`delete-desc-${group.primaryId}`}
                          data-testid={`schedule-delete-confirm-${group.primaryId}`}
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
                                id={`delete-title-${group.primaryId}`}
                                className="text-xs font-semibold uppercase tracking-wider text-red-800"
                              >
                                Confirm Schedule Window Deletion
                              </h3>
                              <p
                                id={`delete-desc-${group.primaryId}`}
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
                                    {WEEKDAY_NAMES[group.dayOfWeek] ?? `Day ${group.dayOfWeek}`}
                                  </span>
                                </div>
                                <div>
                                  <span className="block text-[10px] font-medium uppercase text-gray-500">
                                    Operating Time (PKT)
                                  </span>
                                  <span className="font-mono font-semibold text-[var(--color-navy)]">
                                    {formatTimeRange(group.startTime, group.endTime)}
                                  </span>
                                </div>
                                <div>
                                  <span className="block text-[10px] font-medium uppercase text-gray-500">
                                    Audience
                                  </span>
                                  <span className="font-semibold text-[var(--color-navy)]">
                                    {group.audienceText}
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
                                  id={`cancel-delete-btn-${group.primaryId}`}
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
                                  id={`confirm-delete-btn-${group.primaryId}`}
                                  disabled={isDeleting}
                                  onClick={() => handleConfirmDelete(group.primaryId, group)}
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
                        key={group.key}
                        data-testid={`schedule-row-${group.primaryId}`}
                        className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between"
                      >
                        {/* Hidden anchors for secondary schedules in the group */}
                        {group.schedules.slice(1).map((sec) => (
                          <span
                            key={sec.id}
                            data-testid={`schedule-row-${sec.id}`}
                            className="sr-only"
                            aria-hidden="true"
                          />
                        ))}

                        <div className="flex items-center gap-3">
                          <span className="w-24 flex-shrink-0 rounded-md bg-white px-2.5 py-1 text-center text-xs font-semibold text-[var(--color-navy)] shadow-xs">
                            {WEEKDAY_NAMES[group.dayOfWeek] ?? `Day ${group.dayOfWeek}`}
                          </span>
                          <span className="text-sm font-semibold text-[var(--color-navy)]">
                            {formatTimeRange(group.startTime, group.endTime)}
                          </span>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className="rounded-full bg-[var(--color-border)]/60 px-3 py-0.5 text-xs font-medium text-[var(--color-navy)]">
                            {group.audienceText}
                          </span>
                          {canEditSchedule && (
                            <div className="flex items-center gap-1.5">
                              <button
                                type="button"
                                id={`edit-btn-${group.primaryId}`}
                                disabled={isSaving || isDeleting || isSavingAdd}
                                onClick={() => startEditing(group)}
                                aria-label={`Edit ${
                                  WEEKDAY_NAMES[group.dayOfWeek] ?? `Day ${group.dayOfWeek}`
                                } schedule window`}
                                className="inline-flex items-center gap-1 rounded-md border border-[var(--color-border)] bg-white px-2.5 py-1 text-xs font-semibold text-[var(--color-navy)] transition-colors hover:bg-[var(--color-surface)] hover:text-[var(--color-maroon)] disabled:opacity-50"
                              >
                                <Edit2 size={12} />
                                Edit window
                              </button>
                              <button
                                type="button"
                                id={`delete-btn-${group.primaryId}`}
                                disabled={isSaving || isDeleting || isSavingAdd}
                                onClick={() => startDeleting(group)}
                                aria-label={`Delete ${
                                  WEEKDAY_NAMES[group.dayOfWeek] ?? `Day ${group.dayOfWeek}`
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

            {facility && canEditSchedule && (
              <EditFacilityModal
                isOpen={isEditFacilityModalOpen}
                facility={facility}
                onClose={() => setIsEditFacilityModalOpen(false)}
                onUpdated={(updated) => {
                  setFacility(updated);
                  setSuccessMessage("Facility details updated successfully.");
                }}
              />
            )}
          </div>
        )}
      </main>
      <div className="mx-auto max-w-3xl px-6 pb-8">
        <Footer />
      </div>
    </div>
  );
}
