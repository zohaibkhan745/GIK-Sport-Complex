/**
 * Domain types matching the PostgreSQL-backed REST API for GIKI Sports Complex.
 */

export type OperationalStatus = "active" | "maintenance" | "closed" | "reserved";

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
  status: OperationalStatus;
  statusUntil?: string | null;
  statusNote?: string | null;
  reservedFor?: string | null;
  schedules: Schedule[];
}

/** Input payload for creating a new facility via POST /api/facilities. */
export interface CreateFacilityInput {
  name: string;
  slug?: string;
  description?: string | null;
  status?: OperationalStatus;
  statusUntil?: string | null;
  statusNote?: string | null;
  reservedFor?: string | null;
}

/** Input payload for updating facility attributes via PATCH /api/facilities/:slug. */
export interface UpdateFacilityInput {
  name?: string;
  description?: string | null;
  status?: OperationalStatus;
  statusUntil?: string | null;
  statusNote?: string | null;
  reservedFor?: string | null;
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
  // Campus Cohorts
  student: "Students",
  staff: "Staff",
  faculty: "Faculty",
  women_only: "Women Only",
  general: "General Community",

  // GIKI Sports Teams & Dedicated Cohorts
  badminton_team: "GIK Badminton Team + Girls",
  basketball_boys: "Basketball Boys (GIKI Team)",
  basketball_girls: "Basketball Girls (GIKI Team)",
  volleyball_boys: "Volleyball Boys",
  volleyball_girls: "Volleyball Girls",
  cricket_team: "Cricket GIKI Teams",
  football_team: "GIKI Football Team",
  tennis_team: "Lawn Tennis (GIKI Team)",
  squash_team: "Squash (GIKI Team)",
  squash_faculty: "Squash Faculty",
  table_tennis_team: "Table Tennis (GIKI Team)",
};

/**
 * Format an individual time string to 12-hour AM/PM format (e.g. "9:00 AM", "12:00 PM", "4:30 PM", "12:00 AM (Midnight)").
 * Handles "24:00:00" or "24:00" gracefully as midnight end-of-day.
 */
export function formatClockTime(timeStr: string, isEndTime = false): string {
  const trimmed = timeStr.trim();
  if (trimmed.startsWith("24:00")) {
    return isEndTime ? "12:00 AM (Midnight)" : "Midnight";
  }
  const parts = trimmed.split(":");
  if (parts.length < 2) return timeStr;
  let h = parseInt(parts[0], 10);
  const m = parseInt(parts[1], 10);
  if (isNaN(h) || isNaN(m)) return timeStr;
  if (h === 24) return isEndTime ? "12:00 AM (Midnight)" : "Midnight";
  const period = h >= 12 ? "PM" : "AM";
  h = h % 12;
  if (h === 0) h = 12;
  const minuteStr = m < 10 ? `0${m}` : `${m}`;
  return `${h}:${minuteStr} ${period}`;
}

/**
 * Format a schedule time range into a clean, human-friendly 12-hour AM/PM string,
 * preserving "24:00:00" as "12:00 AM (Midnight)".
 *
 * Example:
 *   formatTimeRange("09:00:00", "12:00:00") => "9:00 AM – 12:00 PM"
 *   formatTimeRange("16:00:00", "18:00:00") => "4:00 PM – 6:00 PM"
 *   formatTimeRange("22:00:00", "24:00:00") => "10:00 PM – 12:00 AM (Midnight)"
 */
export function formatTimeRange(start: string, end: string): string {
  return `${formatClockTime(start, false)} – ${formatClockTime(end, true)}`;
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
    (f.status === "active" || f.status === "maintenance" || f.status === "closed" || f.status === "reserved") &&
    (f.statusUntil === undefined || f.statusUntil === null || typeof f.statusUntil === "string") &&
    (f.statusNote === undefined || f.statusNote === null || typeof f.statusNote === "string") &&
    (f.reservedFor === undefined || f.reservedFor === null || typeof f.reservedFor === "string") &&
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
  imageUrls?: string[];
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
  imageUrls?: string[];
}

export interface UpdateDraftInput {
  title: string | null;
  content: string | null;
  imageUrls?: string[];
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
    (d.imageUrls === undefined ||
      (Array.isArray(d.imageUrls) &&
        d.imageUrls.every((url: unknown) => typeof url === "string"))) &&
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
  imageUrls?: string[];
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
    (p.imageUrls === undefined ||
      (Array.isArray(p.imageUrls) &&
        p.imageUrls.every((url: unknown) => typeof url === "string"))) &&
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
  paisaInput: string | number | bigint,
  options?: { showExplicitPositiveSign?: boolean }
): string {
  if (paisaInput === null || paisaInput === undefined) {
    return "PKR 0.00";
  }
  const paisaStr = String(paisaInput);
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
    case "PADEL_REFUND":
      return "Padel Booking Refund";
    case "TOPUP":
    case "CASHIER_CASH_TOPUP":
      return "Wallet Top-Up";
    default:
      return type.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
  }
}

// ---------------------------------------------------------------------------
// Padel Court Booking (Step 12I)
// ---------------------------------------------------------------------------

export interface PadelFacility {
  id: string;
  slug: string;
  name: string;
  status: string;
}

export type PadelSlotStatus = "AVAILABLE" | "HELD" | "BOOKED" | "BLOCKED";

export interface PadelSlot {
  id: string;
  facilityId: string;
  startTime: string;
  endTime: string;
  status: PadelSlotStatus;
  pricePaisa: string;
  isHoldExpired: boolean;
  holdExpiresAt: string | null;
  heldByCaller: boolean;
}

export interface PadelSlotsResponse {
  date: string;
  facility: PadelFacility | null;
  slots: PadelSlot[];
}

export interface PadelHoldResponse {
  id: string;
  status: string;
  holdExpiresAt: string | null;
  heldByCaller: boolean;
}

export interface PadelCheckoutResponse {
  bookingId: string;
  slotId: string;
  facilityId: string;
  facilityName?: string;
  startTime: string;
  endTime: string;
  status: string;
  feePaisa: string;
  checkInCode: string;
  debitTransactionId?: string;
  createdAt: string;
  alreadyBooked?: boolean;
}

export interface PadelVerifyTicketResponse {
  bookingId: string;
  slotId: string;
  facilityId: string;
  facilityName?: string;
  userId: string;
  userEmail?: string;
  userName?: string;
  startTime: string;
  endTime: string;
  status: string;
  feePaisa: string;
  checkInCode: string;
  checkedInAt: string;
  checkedInBy: string;
}

export interface PadelCancelResponse {
  bookingId: string;
  slotId: string;
  userId: string;
  status: string;
  feePaisa: string;
  refundTransactionId: string;
  cancelledAt: string;
  cancelledBy: string;
  reason?: string;
  message: string;
}

export function isPadelSlot(value: unknown): value is PadelSlot {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const s = value as Record<string, unknown>;
  return (
    typeof s.id === "string" &&
    s.id.trim().length > 0 &&
    typeof s.facilityId === "string" &&
    typeof s.startTime === "string" &&
    typeof s.endTime === "string" &&
    typeof s.status === "string" &&
    typeof s.pricePaisa === "string"
  );
}

export function isPadelSlotsResponse(value: unknown): value is PadelSlotsResponse {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const r = value as Record<string, unknown>;
  if (typeof r.date !== "string") return false;
  if (!Array.isArray(r.slots)) return false;
  return r.slots.every((slot) => isPadelSlot(slot));
}

export function parsePadelSlotsResponse(value: unknown): PadelSlotsResponse {
  if (!isPadelSlotsResponse(value)) {
    throw new Error("Invalid padel slots response: payload does not match schema");
  }
  return value;
}

// -- Gym Membership (Phase 7 / Step 7C) ---------------------------------------
export type GymMembershipStatus = "ACTIVE" | "EXPIRED" | "CANCELLED";
export type GymPlanType = "STANDARD_30_DAY";

export interface GymMembership {
  id: string;
  userId: string;
  planType: GymPlanType;
  status: GymMembershipStatus;
  startTime: string;
  endTime: string;
  pricePaisa: number;
  debitTransactionId: string;
  idempotencyKey: string;
  createdAt: string;
}

export interface GymMembershipSummary {
  isActive: boolean;
  currentMembership?: GymMembership;
  effectiveExpiry?: string;
  daysRemaining: number;
  secondsRemaining: number;
  history: GymMembership[];
}

export interface GymMembershipPlan {
  id: string;
  planCode: string;
  name: string;
  durationDays: number;
  pricePaisa: number;
  isActive: boolean;
  description?: string;
}

export interface UpdateGymMembershipPlanInput {
  name?: string;
  durationDays: number;
  pricePaisa: number;
  description?: string;
}






