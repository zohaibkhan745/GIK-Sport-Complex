import {
  type Facility,
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
  parseFacilities,
  parseFacility,
  parseSchedule,
  parseAuthUser,
  parseDraftSummaries,
  parseDraft,
  parsePublicContentItem,
  parsePublicContentItems,
  parseWalletMeResponse,
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
};
