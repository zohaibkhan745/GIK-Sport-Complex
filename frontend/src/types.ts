/**
 * Domain types matching the PostgreSQL-backed REST API for GIKI Sports Complex.
 */

export type OperationalStatus = "active" | "maintenance" | "closed";

/** One recurring weekly operating window for a facility. */
export interface Schedule {
  id: string;
  dayOfWeek: number; // 1 = Monday ... 7 = Sunday (ISO-8601)
  startTime: string; // 24-hour "HH:MM:SS" local time (Asia/Karachi)
  endTime: string;   // 24-hour "HH:MM:SS" local time (Asia/Karachi), preserves "24:00:00"
  audience: string;  // "student" | "faculty" | "women_only" | "general" | "staff"
}

/** A sports complex facility as returned by GET /api/facilities. */
export interface Facility {
  id: string;
  slug: string;
  name: string;
  description: string | null;
  location: string | null;
  status: OperationalStatus;
  schedules: Schedule[];
}

/** Human-readable weekday mapping for ISO-8601 days (1..7). */
export const WEEKDAY_NAMES: Record<number, string> = {
  1: "Monday",
  2: "Tuesday",
  3: "Wednesday",
  4: "Thursday",
  5: "Friday",
  6: "Saturday",
  7: "Sunday",
};

/** Human-readable audience labels. */
export const AUDIENCE_LABELS: Record<string, string> = {
  student: "Students",
  faculty: "Faculty",
  women_only: "Women Only",
  general: "General Community",
  staff: "Staff",
};

/**
 * Format a 24-hour schedule time range without parsing through Date,
 * preserving "24:00:00" as end of day.
 */
export function formatTimeRange(start: string, end: string): string {
  const cleanStart = start.length >= 5 ? start.slice(0, 5) : start;
  let cleanEnd = end.length >= 5 ? end.slice(0, 5) : end;
  if (end.startsWith("24:00")) {
    cleanEnd = "24:00 (Midnight)";
  }
  return `${cleanStart} – ${cleanEnd}`;
}

// ---------------------------------------------------------------------------
// Runtime Response Validation (Lightweight TypeScript Type Guards)
// ---------------------------------------------------------------------------

export function isSchedule(value: unknown): value is Schedule {
  if (typeof value !== "object" || value === null) return false;
  const s = value as Record<string, unknown>;
  return (
    typeof s.id === "string" &&
    s.id.trim().length > 0 &&
    typeof s.dayOfWeek === "number" &&
    Number.isInteger(s.dayOfWeek) &&
    s.dayOfWeek >= 1 &&
    s.dayOfWeek <= 7 &&
    typeof s.startTime === "string" &&
    /^\d{2}:\d{2}:\d{2}$/.test(s.startTime) &&
    typeof s.endTime === "string" &&
    /^\d{2}:\d{2}:\d{2}$/.test(s.endTime) &&
    typeof s.audience === "string" &&
    s.audience.trim().length > 0
  );
}

export function parseSchedule(value: unknown): Schedule {
  if (!isSchedule(value)) {
    throw new Error("Invalid schedule response: payload does not match the schedule schema");
  }
  return value;
}

/** Input payload for updating a recurring weekly operating schedule window. */
export interface UpdateScheduleInput {
  dayOfWeek: number;
  startTime: string; // "HH:MM:SS" (e.g. "09:00:00")
  endTime: string;   // "HH:MM:SS" (e.g. "12:00:00" or "24:00:00")
  audience: string;  // "student" | "faculty" | "women_only" | "general" | "staff"
}

/** Input payload for creating a recurring weekly operating schedule window. */
export interface CreateScheduleInput {
  dayOfWeek: number;
  startTime: string; // "HH:MM:SS" (e.g. "09:00:00")
  endTime: string;   // "HH:MM:SS" (e.g. "12:00:00" or "24:00:00")
  audience: string;  // "student" | "faculty" | "women_only" | "general" | "staff"
}

export function isFacility(value: unknown): value is Facility {
  if (typeof value !== "object" || value === null) return false;
  const f = value as Record<string, unknown>;
  return (
    typeof f.id === "string" &&
    f.id.trim().length > 0 &&
    typeof f.slug === "string" &&
    f.slug.trim().length > 0 &&
    typeof f.name === "string" &&
    f.name.trim().length > 0 &&
    (f.description === null || typeof f.description === "string") &&
    (f.location === null || typeof f.location === "string") &&
    (f.status === "active" || f.status === "maintenance" || f.status === "closed") &&
    Array.isArray(f.schedules) &&
    f.schedules.every(isSchedule)
  );
}

export function parseFacility(value: unknown): Facility {
  if (!isFacility(value)) {
    throw new Error("Invalid facility response: payload does not match the facility schema");
  }
  return value;
}

export function parseFacilities(value: unknown): Facility[] {
  if (!Array.isArray(value)) {
    throw new Error("Invalid facilities response: expected an array");
  }
  return value.map((item, index) => {
    if (!isFacility(item)) {
      throw new Error(`Invalid facility at index ${index}: payload does not match the facility schema`);
    }
    return item;
  });
}

// ---------------------------------------------------------------------------
// Authentication & User Types
// ---------------------------------------------------------------------------

/** Safe public user profile returned by GET /api/auth/me and POST /api/auth/login. */
export interface AuthUser {
  id: string;
  email: string;
  displayName: string;
  roles: string[];
}

export function isAuthUser(value: unknown): value is AuthUser {
  if (typeof value !== "object" || value === null) return false;
  const u = value as Record<string, unknown>;
  return (
    typeof u.id === "string" &&
    u.id.trim().length > 0 &&
    typeof u.email === "string" &&
    u.email.trim().length > 0 &&
    typeof u.displayName === "string" &&
    u.displayName.trim().length > 0 &&
    Array.isArray(u.roles) &&
    u.roles.every((r) => typeof r === "string" && r.trim().length > 0)
  );
}

export function parseAuthUser(value: unknown): AuthUser {
  if (!isAuthUser(value)) {
    throw new Error("Invalid user profile response: payload does not match the AuthUser schema");
  }
  return value;
}

// ---------------------------------------------------------------------------
// Unrelated Application Types (Unchanged)
// ---------------------------------------------------------------------------

export interface AchievementPost {
  id: string;
  author: string;
  initials: string;
  caption: string;
  timeAgo: string;
  likes: number;
  comments: number;
  imageUrl?: string;
}

export interface Stat {
  value: string;
  label: string;
}

export interface CreateAchievementPostInput {
  caption: string;
  photo?: File;
}

export interface Slot {
  id: string;
  facilitySlug: string;
  startTime: string;
  endTime: string;
  isAvailable: boolean;
}

export interface CreateBookingInput {
  slotId: string;
  name: string;
  email: string;
}

export interface Booking {
  id: string;
  slotId: string;
  name: string;
  email: string;
  createdAt: string;
}

// Legacy types preserved for data/facilities.ts backwards compatibility
export interface ScheduleEntry {
  days: string;
  activity: string;
}

export interface Coach {
  name: string;
  title: string;
  initials: string;
}

export interface FacilityMeta {
  icon: "clock" | "users" | "thermometer" | "shield";
  label: string;
}

export type FacilityIconName =
  | "grid"
  | "feather"
  | "target"
  | "circle"
  | "goal"
  | "disc"
  | "dumbbell"
  | "waves";

export interface LegacyFacility {
  slug: string;
  name: string;
  shortName: string;
  tagline: string;
  description: string;
  icon: FacilityIconName;
  meta: FacilityMeta[];
  schedule: ScheduleEntry[];
  coach?: Coach;
  galleryCount: number;
}
