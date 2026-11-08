import { describe, it, expect } from "vitest";
import {
  isSchedule,
  parseSchedule,
  isFacility,
  parseFacility,
  parseFacilities,
  formatTimeRange,
  isAuthUser,
  parseAuthUser,
  type Facility,
} from "../types";

describe("Facility & Schedule Runtime Validation", () => {
  const validSchedule = {
    id: "sched-1",
    dayOfWeek: 1,
    startTime: "09:00:00",
    endTime: "12:00:00",
    audience: "student",
  };

  const validFacility: Facility = {
    id: "fac-1",
    slug: "demo-badminton-court",
    name: "[Demo] Campus Badminton Court",
    description: "Synthetic test description",
    location: "Wing B",
    status: "active",
    schedules: [validSchedule],
  };

  it("validates a conforming schedule", () => {
    expect(isSchedule(validSchedule)).toBe(true);
  });

  it("rejects schedules with invalid dayOfWeek", () => {
    expect(isSchedule({ ...validSchedule, dayOfWeek: 0 })).toBe(false);
    expect(isSchedule({ ...validSchedule, dayOfWeek: 8 })).toBe(false);
    expect(isSchedule({ ...validSchedule, dayOfWeek: "Monday" })).toBe(false);
  });

  it("rejects schedules with invalid time formats", () => {
    expect(isSchedule({ ...validSchedule, startTime: "9:00" })).toBe(false);
    expect(isSchedule({ ...validSchedule, endTime: "24:00" })).toBe(false);
    expect(isSchedule({ ...validSchedule, startTime: "invalid" })).toBe(false);
  });

  it("parses valid schedule and throws on invalid schedule", () => {
    expect(parseSchedule(validSchedule)).toEqual(validSchedule);
    expect(() => parseSchedule({ ...validSchedule, dayOfWeek: 9 })).toThrow(
      "Invalid schedule response: payload does not match the schedule schema"
    );
  });

  it("validates a conforming facility", () => {
    expect(isFacility(validFacility)).toBe(true);
    expect(parseFacility(validFacility)).toEqual(validFacility);
  });

  it("accepts null description and null location", () => {
    const nullableFacility = {
      ...validFacility,
      description: null,
      location: null,
    };
    expect(isFacility(nullableFacility)).toBe(true);
    expect(parseFacility(nullableFacility)).toEqual(nullableFacility);
  });

  it("rejects facilities with invalid operational status", () => {
    expect(isFacility({ ...validFacility, status: "open" })).toBe(false);
    expect(isFacility({ ...validFacility, status: "available" })).toBe(false);
  });

  it("rejects facilities with non-array schedules", () => {
    expect(isFacility({ ...validFacility, schedules: null })).toBe(false);
    expect(isFacility({ ...validFacility, schedules: "daily" })).toBe(false);
  });

  it("throws understandable error when parseFacility receives malformed payload", () => {
    expect(() => parseFacility({ slug: "only-slug" })).toThrow(
      "Invalid facility response: payload does not match the facility schema"
    );
  });

  it("throws understandable error when parseFacilities receives non-array", () => {
    expect(() => parseFacilities({ error: "failed" })).toThrow(
      "Invalid facilities response: expected an array"
    );
  });

  it("parses valid facilities list", () => {
    const parsed = parseFacilities([validFacility]);
    expect(parsed).toHaveLength(1);
    expect(parsed[0].slug).toBe("demo-badminton-court");
  });
});

describe("Schedule Time Formatting", () => {
  it("formats standard daytime windows without Date parsing", () => {
    expect(formatTimeRange("09:00:00", "12:00:00")).toBe("09:00 – 12:00");
    expect(formatTimeRange("16:00:00", "18:00:00")).toBe("16:00 – 18:00");
  });

  it("preserves end-time 24:00:00 as midnight end-of-day without wrap-around", () => {
    const result = formatTimeRange("22:00:00", "24:00:00");
    expect(result).toBe("22:00 – 24:00 (Midnight)");
    expect(result).not.toContain("00:00");
  });
});

describe("AuthUser Runtime Validation", () => {
  const validUser = {
    id: "f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
    email: "director@example.test",
    displayName: "Sports Director (Dev)",
    roles: ["sports_director"],
  };

  it("validates a conforming auth user", () => {
    expect(isAuthUser(validUser)).toBe(true);
    expect(parseAuthUser(validUser)).toEqual(validUser);
  });

  it("rejects non-object or null values", () => {
    expect(isAuthUser(null)).toBe(false);
    expect(isAuthUser(undefined)).toBe(false);
    expect(isAuthUser("string")).toBe(false);
    expect(isAuthUser(123)).toBe(false);
  });

  it("rejects missing or empty id, email, or displayName", () => {
    expect(isAuthUser({ ...validUser, id: "" })).toBe(false);
    expect(isAuthUser({ ...validUser, id: "   " })).toBe(false);
    expect(isAuthUser({ ...validUser, email: "" })).toBe(false);
    expect(isAuthUser({ ...validUser, displayName: "" })).toBe(false);
  });

  it("rejects non-array roles or empty role strings", () => {
    expect(isAuthUser({ ...validUser, roles: "sports_director" })).toBe(false);
    expect(isAuthUser({ ...validUser, roles: null })).toBe(false);
    expect(isAuthUser({ ...validUser, roles: [""] })).toBe(false);
    expect(isAuthUser({ ...validUser, roles: ["sports_director", "  "] })).toBe(false);
  });

  it("throws understandable error when parseAuthUser receives invalid payload", () => {
    expect(() => parseAuthUser({ email: "invalid" })).toThrow(
      "Invalid user profile response: payload does not match the AuthUser schema"
    );
  });
});
