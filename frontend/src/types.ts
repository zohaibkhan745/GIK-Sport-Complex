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

// ---------------------------------------------------------------------------
// Content Drafts (Administrative List - Step 10I)
// ---------------------------------------------------------------------------

export type ContentKind = "announcement" | "achievement";

export type DraftFilterKind = "all" | ContentKind;

export interface DraftSummary {
  id: string;
  kind: ContentKind;
  title: string | null;
  updatedAt: string;
}

export function isContentKind(value: unknown): value is ContentKind {
  return value === "announcement" || value === "achievement";
}

export function isDraftSummary(value: unknown): value is DraftSummary {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const d = value as Record<string, unknown>;
  return (
    typeof d.id === "string" &&
    d.id.trim().length > 0 &&
    isContentKind(d.kind) &&
    (d.title === null || typeof d.title === "string") &&
    typeof d.updatedAt === "string" &&
    d.updatedAt.trim().length > 0 &&
    !isNaN(Date.parse(d.updatedAt))
  );
}

export function parseDraftSummary(value: unknown): DraftSummary {
  if (!isDraftSummary(value)) {
    throw new Error("Invalid draft summary response: payload does not match schema");
  }
  return value;
}

export function parseDraftSummaries(value: unknown): DraftSummary[] {
  if (!Array.isArray(value)) {
    throw new Error("Invalid drafts response: expected an array");
  }
  return value.map((item, index) => {
    if (!isDraftSummary(item)) {
      throw new Error(`Invalid draft summary at index ${index}: payload does not match schema`);
    }
    return item;
  });
}

// ---------------------------------------------------------------------------
// Content Draft Editor (Creation & Editing - Step 10J)
// ---------------------------------------------------------------------------

export interface Draft {
  id: string;
  kind: ContentKind;
  title: string | null;
  content: string | null;
  status: string;
  creatorId: string;
  createdAt: string;
  updatedAt: string;
  publishedAt?: string | null;
}

export interface CreateDraftInput {
  kind: ContentKind;
  title?: string | null;
  content?: string | null;
}

export interface UpdateDraftInput {
  title: string | null;
  content: string | null;
}

export function isDraft(value: unknown): value is Draft {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const d = value as Record<string, unknown>;
  return (
    typeof d.id === "string" &&
    d.id.trim().length > 0 &&
    isContentKind(d.kind) &&
    (d.title === null || typeof d.title === "string") &&
    (d.content === null || typeof d.content === "string") &&
    typeof d.status === "string" &&
    typeof d.creatorId === "string" &&
    typeof d.createdAt === "string" &&
    d.createdAt.trim().length > 0 &&
    !isNaN(Date.parse(d.createdAt)) &&
    typeof d.updatedAt === "string" &&
    d.updatedAt.trim().length > 0 &&
    !isNaN(Date.parse(d.updatedAt)) &&
    (d.publishedAt === undefined ||
      d.publishedAt === null ||
      (typeof d.publishedAt === "string" && !isNaN(Date.parse(d.publishedAt))))
  );
}

export function parseDraft(value: unknown): Draft {
  if (!isDraft(value)) {
    throw new Error("Invalid draft response: payload does not match schema");
  }
  return value;
}

// ---------------------------------------------------------------------------
// Published Content (Step 10K)
// ---------------------------------------------------------------------------

export interface PublicContentItem {
  id: string;
  kind: ContentKind;
  title: string;
  content: string;
  publishedAt: string;
}

export function isPublicContentItem(value: unknown): value is PublicContentItem {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const p = value as Record<string, unknown>;
  return (
    typeof p.id === "string" &&
    p.id.trim().length > 0 &&
    isContentKind(p.kind) &&
    typeof p.title === "string" &&
    typeof p.content === "string" &&
    typeof p.publishedAt === "string" &&
    p.publishedAt.trim().length > 0 &&
    !isNaN(Date.parse(p.publishedAt))
  );
}

export function parsePublicContentItem(value: unknown): PublicContentItem {
  if (!isPublicContentItem(value)) {
    throw new Error("Invalid published content response: payload does not match schema");
  }
  return value;
}

export function parsePublicContentItems(value: unknown): PublicContentItem[] {
  if (!Array.isArray(value)) {
    throw new Error("Invalid content feed response: expected an array");
  }
  return value.map((item, index) => {
    if (!isPublicContentItem(item)) {
      throw new Error(`Invalid content item at index ${index}: payload does not match schema`);
    }
    return item;
  });
}

// ---------------------------------------------------------------------------
// Digital Wallet (Step 11F)
// ---------------------------------------------------------------------------

export interface WalletSummary {
  status: string;
  currency: string;
  balancePaisa: string;
}

export interface WalletActivityItem {
  entryId: string;
  transactionId: string;
  type: string;
  amountPaisa: string;
  createdAt: string;
}

export interface WalletMeResponse {
  wallet: WalletSummary | null;
  activity: WalletActivityItem[];
}

export function isWalletSummary(value: unknown): value is WalletSummary {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const w = value as Record<string, unknown>;
  return (
    typeof w.status === "string" &&
    w.status.trim().length > 0 &&
    typeof w.currency === "string" &&
    w.currency.trim().length > 0 &&
    typeof w.balancePaisa === "string" &&
    /^-?\d+$/.test(w.balancePaisa.trim())
  );
}

export function isWalletActivityItem(value: unknown): value is WalletActivityItem {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const a = value as Record<string, unknown>;
  return (
    typeof a.entryId === "string" &&
    a.entryId.trim().length > 0 &&
    typeof a.transactionId === "string" &&
    a.transactionId.trim().length > 0 &&
    typeof a.type === "string" &&
    a.type.trim().length > 0 &&
    typeof a.amountPaisa === "string" &&
    /^-?\d+$/.test(a.amountPaisa.trim()) &&
    typeof a.createdAt === "string" &&
    a.createdAt.trim().length > 0 &&
    !isNaN(Date.parse(a.createdAt))
  );
}

export function isWalletMeResponse(value: unknown): value is WalletMeResponse {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const r = value as Record<string, unknown>;
  if (r.wallet !== null && !isWalletSummary(r.wallet)) {
    return false;
  }
  if (!Array.isArray(r.activity)) {
    return false;
  }
  return r.activity.every((item) => isWalletActivityItem(item));
}

export function parseWalletMeResponse(value: unknown): WalletMeResponse {
  if (!isWalletMeResponse(value)) {
    throw new Error("Invalid wallet response: payload does not match schema");
  }
  return value;
}

/**
 * Formats integer paisa represented as a decimal string into exact PKR with two decimal places.
 * Uses BigInt arithmetic to avoid any floating-point or Number conversion.
 *
 * Example:
 *  "0" -> "PKR 0.00"
 *  "40000" -> "PKR 400.00"
 *  "-35000" -> "-PKR 350.00"
 *  "9007199254740992" -> "PKR 90,071,992,547,409.92"
 */
export function formatPaisaToPKR(
  paisaStr: string,
  options?: { showExplicitPositiveSign?: boolean }
): string {
  if (!paisaStr || !/^-?\d+$/.test(paisaStr.trim())) {
    return "PKR 0.00";
  }

  const clean = paisaStr.trim();
  const isNegative = clean.startsWith("-");
  const digits = isNegative ? clean.slice(1) : clean;
  const bigVal = BigInt(digits);

  const rupees = bigVal / 100n;
  const paisa = bigVal % 100n;

  // Insert thousands comma separators into the rupee portion without float conversion
  const rupeeStr = rupees.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const paisaStrPadded = paisa.toString().padStart(2, "0");

  if (isNegative) {
    return `-PKR ${rupeeStr}.${paisaStrPadded}`;
  }
  if (options?.showExplicitPositiveSign && bigVal > 0n) {
    return `+PKR ${rupeeStr}.${paisaStrPadded}`;
  }
  return `PKR ${rupeeStr}.${paisaStrPadded}`;
}

/**
 * Formats a transaction ISO-8601 timestamp string into a readable date and time.
 */
export function formatTransactionTimestamp(isoString: string): string {
  const d = new Date(isoString);
  if (isNaN(d.getTime())) return isoString;
  return d.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
    hour12: true,
  });
}

/**
 * Formats transaction type string into a human-readable title.
 */
export function formatTransactionType(type: string): string {
  switch (type) {
    case "PADEL_BOOKING":
      return "Padel Court Booking";
    case "TOPUP":
    case "CASHIER_CASH_TOPUP":
      return "Wallet Top-Up";
    default:
      return type.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
  }
}




