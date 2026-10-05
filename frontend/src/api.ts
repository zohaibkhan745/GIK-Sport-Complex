import type {
  Facility,
  AchievementPost,
  CreateAchievementPostInput,
  Slot,
  Booking,
  CreateBookingInput,
} from "./types";

const BASE_URL = "/api";

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers:
      init?.body instanceof FormData
        ? undefined
        : { "Content-Type": "application/json" },
    ...init,
  });

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new ApiError(res.status, text || `Request to ${path} failed`);
  }

  // Some endpoints (e.g. DELETE) may return no body.
  const contentType = res.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

export const api = {
  // -- Facilities ---------------------------------------------------------
  getFacilities: () => request<Facility[]>("/facilities"),
  getFacility: (slug: string) => request<Facility>(`/facilities/${slug}`),

  // -- Slots & bookings -----------------------------------------------------
  getSlots: (facilitySlug: string) =>
    request<Slot[]>(`/facilities/${facilitySlug}/slots`),
  createBooking: (input: CreateBookingInput) =>
    request<Booking>("/bookings", {
      method: "POST",
      body: JSON.stringify(input),
    }),

  // -- Achievements / posts -------------------------------------------------
  getAchievements: () => request<AchievementPost[]>("/achievements"),
  createAchievement: (input: CreateAchievementPostInput) => {
    const form = new FormData();
    form.append("caption", input.caption);
    if (input.photo) form.append("photo", input.photo);
    return request<AchievementPost>("/achievements", {
      method: "POST",
      body: form,
    });
  },
};

export { ApiError };
