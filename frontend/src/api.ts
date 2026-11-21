import {
  type Facility,
  type CreateFacilityInput,
  type UpdateFacilityInput,
  type Schedule,
  type UpdateScheduleInput,
  type CreateScheduleInput,
  type AchievementPost,
  type CreateAchievementPostInput,
  type Slot,
  type Booking,
  type CreateBookingInput,
  type AuthUser,
  type DraftSummary,
  type DraftFilterKind,
  type Draft,
  type CreateDraftInput,
  type UpdateDraftInput,
  type PublicContentItem,
  type WalletMeResponse,
  type PadelFacility,
  type PadelSlotsResponse,
  type PadelHoldResponse,
  type PadelCheckoutResponse,
  type PadelVerifyTicketResponse,
  type PadelCancelResponse,
  type GymMembership,
  type GymMembershipSummary,
  type GymMembershipPlan,
  type UpdateGymMembershipPlanInput,
  parseFacilities,
  parseFacility,
  parseSchedule,
  parseAuthUser,
  parseDraftSummaries,
  parseDraft,
  parsePublicContentItem,
  parsePublicContentItems,
  parseWalletMeResponse,
  parsePadelSlotsResponse,
} from "./types";

const BASE_URL = "/api";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = {
    ...(init?.body instanceof FormData || !init?.body
      ? {}
      : { "Content-Type": "application/json" }),
    ...init?.headers,
  };

  const res = await fetch(`${BASE_URL}${path}`, {
    credentials: "include",
    ...init,
    headers,
  });

  if (!res.ok) {
    let message = `Request to ${path} failed (${res.status})`;
    try {
      const data = await res.json();
      if (data && typeof data.error === "string") {
        message = data.error;
      }
    } catch {
      const text = await res.text().catch(() => "");
      if (text) message = text;
    }
    throw new ApiError(res.status, message);
  }

  // 204 No Content has no body to parse
  if (res.status === 204) {
    return undefined as T;
  }

  const contentType = res.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

export interface RequestOptions {
  signal?: AbortSignal;
}

export interface LoginInput {
  email: string;
  password: string;
}

export const api = {
  // -- Authentication --------------------------------------------------------
  login: async (input: LoginInput, options?: RequestOptions): Promise<AuthUser> => {
    const raw = await request<unknown>("/auth/login", {
      method: "POST",
      body: JSON.stringify(input),
      signal: options?.signal,
    });
    return parseAuthUser(raw);
  },

  me: async (options?: RequestOptions): Promise<AuthUser> => {
    const raw = await request<unknown>("/auth/me", {
      signal: options?.signal,
    });
    return parseAuthUser(raw);
  },

  logout: async (options?: RequestOptions): Promise<void> => {
    await request<void>("/auth/logout", {
      method: "POST",
      signal: options?.signal,
    });
  },
  // -- Facilities (PostgreSQL Relational Storage) ----------------------------
  getFacilities: async (options?: RequestOptions): Promise<Facility[]> => {
    const raw = await request<unknown>("/facilities", {
      signal: options?.signal,
    });
    return parseFacilities(raw);
  },

  getFacility: async (slug: string, options?: RequestOptions): Promise<Facility> => {
    const encoded = encodeURIComponent(slug);
    const raw = await request<unknown>(`/facilities/${encoded}`, {
      signal: options?.signal,
    });
    return parseFacility(raw);
  },

  createFacility: async (
    input: CreateFacilityInput,
    options?: RequestOptions
  ): Promise<Facility> => {
    const raw = await request<unknown>("/facilities", {
      method: "POST",
      body: JSON.stringify(input),
      signal: options?.signal,
    });
    return parseFacility(raw);
  },

  updateFacility: async (
    facilitySlug: string,
    input: UpdateFacilityInput,
    options?: RequestOptions
  ): Promise<Facility> => {
    const encodedSlug = encodeURIComponent(facilitySlug);
    const raw = await request<unknown>(`/facilities/${encodedSlug}`, {
      method: "PATCH",
      body: JSON.stringify(input),
      signal: options?.signal,
    });
    return parseFacility(raw);
  },

  updateFacilityDescription: async (
    facilitySlug: string,
    description: string | null,
    options?: RequestOptions
  ): Promise<Facility> => {
    return api.updateFacility(facilitySlug, { description }, options);
  },

  updateSchedule: async (
    facilitySlug: string,
    scheduleId: string,
    input: UpdateScheduleInput,
    options?: RequestOptions
  ): Promise<Schedule> => {
    const encodedSlug = encodeURIComponent(facilitySlug);
    const encodedId = encodeURIComponent(scheduleId);
    const raw = await request<unknown>(
      `/facilities/${encodedSlug}/schedules/${encodedId}`,
      {
        method: "PUT",
        body: JSON.stringify(input),
        signal: options?.signal,
      }
    );
    return parseSchedule(raw);
  },

  createSchedule: async (
    facilitySlug: string,
    input: CreateScheduleInput,
    options?: RequestOptions
  ): Promise<Schedule> => {
    const encodedSlug = encodeURIComponent(facilitySlug);
    const raw = await request<unknown>(
      `/facilities/${encodedSlug}/schedules`,
      {
        method: "POST",
        body: JSON.stringify(input),
        signal: options?.signal,
      }
    );
    return parseSchedule(raw);
  },

  deleteSchedule: async (
    facilitySlug: string,
    scheduleId: string,
    options?: RequestOptions
  ): Promise<void> => {
    const encodedSlug = encodeURIComponent(facilitySlug);
    const encodedId = encodeURIComponent(scheduleId);
    await request<void>(
      `/facilities/${encodedSlug}/schedules/${encodedId}`,
      {
        method: "DELETE",
        signal: options?.signal,
      }
    );
  },

  // -- Slots & bookings (Legacy JSON Storage) --------------------------------
  getSlots: (facilitySlug: string, options?: RequestOptions) =>
    request<Slot[]>(`/facilities/${encodeURIComponent(facilitySlug)}/slots`, {
      signal: options?.signal,
    }),

  createBooking: (input: CreateBookingInput, options?: RequestOptions) =>
    request<Booking>("/bookings", {
      method: "POST",
      body: JSON.stringify(input),
      signal: options?.signal,
    }),

  // -- Achievements / posts (Legacy JSON Storage) ----------------------------
  getAchievements: (options?: RequestOptions) =>
    request<AchievementPost[]>("/achievements", { signal: options?.signal }),

  createAchievement: (input: CreateAchievementPostInput, options?: RequestOptions) => {
    const form = new FormData();
    form.append("caption", input.caption);
    if (input.photo) form.append("photo", input.photo);
    return request<AchievementPost>("/achievements", {
      method: "POST",
      body: form,
      signal: options?.signal,
    });
  },

  // -- Content Drafts (Administrative List - Step 10I) ----------------------
  listDrafts: async (
    kind?: DraftFilterKind,
    options?: RequestOptions
  ): Promise<DraftSummary[]> => {
    const query = kind && kind !== "all" ? `?kind=${encodeURIComponent(kind)}` : "";
    const raw = await request<unknown>(`/admin/content/drafts${query}`, {
      signal: options?.signal,
    });
    return parseDraftSummaries(raw);
  },

  // -- Content Draft Operations (Step 10J) ----------------------------------
  uploadContentImages: async (
    files: File[],
    options?: RequestOptions
  ): Promise<string[]> => {
    const formData = new FormData();
    for (const file of files) {
      formData.append("images", file);
    }
    const res = await fetch("/api/admin/content/upload", {
      method: "POST",
      body: formData,
      signal: options?.signal,
      credentials: "include",
    });
    if (!res.ok) {
      let msg = "Failed to upload image";
      try {
        const data = await res.json();
        if (data && data.error) msg = data.error;
      } catch {
        // fallback
      }
      throw new Error(msg);
    }
    const data = await res.json();
    return Array.isArray(data.urls) ? data.urls : [];
  },

  createDraft: async (
    input: CreateDraftInput,
    options?: RequestOptions
  ): Promise<Draft> => {
    const raw = await request<unknown>("/admin/content/drafts", {
      method: "POST",
      body: JSON.stringify({
        kind: input.kind,
        title: input.title !== undefined ? input.title : null,
        content: input.content !== undefined ? input.content : null,
        ...(input.imageUrls !== undefined ? { imageUrls: input.imageUrls } : {}),
      }),
      signal: options?.signal,
    });
    return parseDraft(raw);
  },

  getDraft: async (
    id: string,
    options?: RequestOptions
  ): Promise<Draft> => {
    const encoded = encodeURIComponent(id);
    const raw = await request<unknown>(`/admin/content/drafts/${encoded}`, {
      signal: options?.signal,
    });
    return parseDraft(raw);
  },

  updateDraft: async (
    id: string,
    input: UpdateDraftInput,
    options?: RequestOptions
  ): Promise<Draft> => {
    const encoded = encodeURIComponent(id);
    const raw = await request<unknown>(`/admin/content/drafts/${encoded}`, {
      method: "PUT",
      body: JSON.stringify({
        title: input.title !== undefined ? input.title : null,
        content: input.content !== undefined ? input.content : null,
        ...(input.imageUrls !== undefined ? { imageUrls: input.imageUrls } : {}),
      }),
      signal: options?.signal,
    });
    return parseDraft(raw);
  },

  publishDraft: async (
    id: string,
    options?: RequestOptions
  ): Promise<PublicContentItem> => {
    const encoded = encodeURIComponent(id);
    const raw = await request<unknown>(
      `/admin/content/drafts/${encoded}/publish`,
      {
        method: "POST",
        signal: options?.signal,
      }
    );
    return parsePublicContentItem(raw);
  },

  // -- Public Content Feeds (Step 10M) ---------------------------------------
  getAnnouncements: async (options?: RequestOptions): Promise<PublicContentItem[]> => {
    const raw = await request<unknown>("/content/announcements", {
      signal: options?.signal,
    });
    return parsePublicContentItems(raw);
  },

  getAchievementsFeed: async (options?: RequestOptions): Promise<PublicContentItem[]> => {
    const raw = await request<unknown>("/content/achievements", {
      signal: options?.signal,
    });
    return parsePublicContentItems(raw);
  },

  // -- Sports Wallet (Step 11F) ---------------------------------------------
  getMyWallet: async (options?: RequestOptions): Promise<WalletMeResponse> => {
    const raw = await request<unknown>("/wallet/me", {
      signal: options?.signal,
    });
    return parseWalletMeResponse(raw);
  },

  // -- Padel Court Booking (Step 12I) ---------------------------------------
  getPadelFacilities: async (options?: RequestOptions): Promise<PadelFacility[]> => {
    return request<PadelFacility[]>("/padel/facilities", {
      signal: options?.signal,
    });
  },

  getPadelSlots: async (
    date: string,
    facilityId?: string,
    options?: RequestOptions
  ): Promise<PadelSlotsResponse> => {
    const params = new URLSearchParams({ date });
    if (facilityId) params.append("facility_id", facilityId);
    const raw = await request<unknown>(`/padel/slots?${params.toString()}`, {
      signal: options?.signal,
    });
    return parsePadelSlotsResponse(raw);
  },

  holdPadelSlot: async (
    slotId: string,
    options?: RequestOptions
  ): Promise<PadelHoldResponse> => {
    return request<PadelHoldResponse>(`/padel/slots/${encodeURIComponent(slotId)}/hold`, {
      method: "POST",
      signal: options?.signal,
    });
  },

  releasePadelHold: async (
    slotId: string,
    options?: RequestOptions
  ): Promise<PadelHoldResponse> => {
    return request<PadelHoldResponse>(`/padel/slots/${encodeURIComponent(slotId)}/release-hold`, {
      method: "POST",
      signal: options?.signal,
    });
  },

  checkoutPadelSlot: async (
    input: { slotId: string; idempotencyKey: string },
    options?: RequestOptions
  ): Promise<PadelCheckoutResponse> => {
    return request<PadelCheckoutResponse>("/padel/checkout", {
      method: "POST",
      body: JSON.stringify({
        slot_id: input.slotId,
        idempotency_key: input.idempotencyKey,
      }),
      signal: options?.signal,
    });
  },

  verifyPadelTicket: async (
    checkInCode: string,
    bookingId?: string,
    options?: RequestOptions
  ): Promise<PadelVerifyTicketResponse> => {
    return request<PadelVerifyTicketResponse>("/padel/bookings/verify-ticket", {
      method: "POST",
      body: JSON.stringify({
        checkInCode,
        ...(bookingId ? { bookingId } : {}),
      }),
      signal: options?.signal,
    });
  },

  cancelPadelBooking: async (
    bookingId: string,
    reason?: string,
    options?: RequestOptions
  ): Promise<PadelCancelResponse> => {
    return request<PadelCancelResponse>(`/admin/bookings/${encodeURIComponent(bookingId)}/cancel`, {
      method: "POST",
      body: JSON.stringify({ reason: reason || "Administrative cancellation" }),
      signal: options?.signal,
    });
  },

  generatePadelSlots: async (
    input: {
      facility_id: string;
      date: string;
      start_hour?: number;
      end_hour?: number;
      slot_duration_minutes?: number;
      price_paisa?: string;
    },
    options?: RequestOptions
  ): Promise<unknown> => {
    return request<unknown>("/admin/padel/slots/generate", {
      method: "POST",
      body: JSON.stringify(input),
      signal: options?.signal,
    });
  },

  // -- Gym Membership (Phase 7 / Step 7C) ------------------------------------
  getMyGymMembership: async (options?: RequestOptions): Promise<GymMembershipSummary> => {
    return request<GymMembershipSummary>("/memberships/me", {
      signal: options?.signal,
    });
  },

  purchaseGymMembership: async (
    planType: string = "STANDARD_30_DAY",
    idempotencyKey?: string,
    options?: RequestOptions
  ): Promise<GymMembership> => {
    const key =
      idempotencyKey ||
      (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
        ? crypto.randomUUID()
        : `gym-${Date.now()}`);
    return request<GymMembership>("/memberships/purchase", {
      method: "POST",
      body: JSON.stringify({
        planType,
        idempotencyKey: key,
      }),
      signal: options?.signal,
    });
  },

  getAdminGymMemberships: async (
    limit: number = 50,
    offset: number = 0,
    options?: RequestOptions
  ): Promise<GymMembership[]> => {
    return request<GymMembership[]>(
      `/admin/memberships?limit=${limit}&offset=${offset}`,
      {
        signal: options?.signal,
      }
    );
  },

  getGymMembershipPlan: async (options?: RequestOptions): Promise<GymMembershipPlan> => {
    return request<GymMembershipPlan>("/membership/plan", {
      signal: options?.signal,
    });
  },

  updateGymMembershipPlan: async (
    input: UpdateGymMembershipPlanInput,
    options?: RequestOptions
  ): Promise<GymMembershipPlan> => {
    return request<GymMembershipPlan>("/admin/membership/plan", {
      method: "PATCH",
      body: JSON.stringify(input),
      signal: options?.signal,
    });
  },
};
