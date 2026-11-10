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
  isContentKind,
  isDraftSummary,
  parseDraftSummary,
  parseDraftSummaries,
  type DraftSummary,
  isDraft,
  parseDraft,
  type Draft,
  isPublicContentItem,
  parsePublicContentItem,
  parsePublicContentItems,
  type PublicContentItem,
  isWalletSummary,
  isWalletActivityItem,
  isWalletMeResponse,
  parseWalletMeResponse,
  formatPaisaToPKR,
  formatTransactionTimestamp,
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

describe("Draft Summary Runtime Validation", () => {
  const validDraft: DraftSummary = {
    id: "e45db77a-6eb2-43ce-a128-406c10b7f6fa",
    kind: "announcement",
    title: "Upcoming Badminton Championship",
    updatedAt: "2026-11-08T15:30:00Z",
  };

  const validAchievementDraft: DraftSummary = {
    id: "f15db77a-6eb2-43ce-a128-406c10b7f6fb",
    kind: "achievement",
    title: null,
    updatedAt: "2026-11-08T16:00:00Z",
  };

  it("validates content kinds correctly", () => {
    expect(isContentKind("announcement")).toBe(true);
    expect(isContentKind("achievement")).toBe(true);
    expect(isContentKind("other")).toBe(false);
    expect(isContentKind(null)).toBe(false);
    expect(isContentKind("")).toBe(false);
  });

  it("validates a conforming draft summary with title", () => {
    expect(isDraftSummary(validDraft)).toBe(true);
    expect(parseDraftSummary(validDraft)).toEqual(validDraft);
  });

  it("validates a conforming draft summary with null title (incomplete draft)", () => {
    expect(isDraftSummary(validAchievementDraft)).toBe(true);
    expect(parseDraftSummary(validAchievementDraft)).toEqual(validAchievementDraft);
  });

  it("rejects draft summary with missing or empty id", () => {
    expect(isDraftSummary({ ...validDraft, id: "" })).toBe(false);
    expect(isDraftSummary({ ...validDraft, id: "   " })).toBe(false);
    expect(isDraftSummary({ ...validDraft, id: null })).toBe(false);
  });

  it("rejects draft summary with invalid kind", () => {
    expect(isDraftSummary({ ...validDraft, kind: "article" })).toBe(false);
  });

  it("rejects draft summary with non-string non-null title", () => {
    expect(isDraftSummary({ ...validDraft, title: 123 })).toBe(false);
    expect(isDraftSummary({ ...validDraft, title: {} })).toBe(false);
  });

  it("rejects draft summary with invalid updatedAt date string", () => {
    expect(isDraftSummary({ ...validDraft, updatedAt: "not-a-date" })).toBe(false);
    expect(isDraftSummary({ ...validDraft, updatedAt: "" })).toBe(false);
  });

  it("parses valid array of draft summaries", () => {
    const list = parseDraftSummaries([validDraft, validAchievementDraft]);
    expect(list).toHaveLength(2);
    expect(list[0].id).toBe(validDraft.id);
    expect(list[1].title).toBeNull();
  });

  it("parses empty array as []", () => {
    expect(parseDraftSummaries([])).toEqual([]);
  });

  it("throws when parseDraftSummaries receives non-array", () => {
    expect(() => parseDraftSummaries({ error: "not array" })).toThrow(
      "Invalid drafts response: expected an array"
    );
  });

  it("throws when parseDraftSummaries receives array with invalid element", () => {
    expect(() => parseDraftSummaries([validDraft, { invalid: true }])).toThrow(
      "Invalid draft summary at index 1: payload does not match schema"
    );
  });
});

describe("Full Draft Runtime Validation (Step 10J)", () => {
  const completeDraft: Draft = {
    id: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    kind: "announcement",
    title: "Badminton Trial Matches",
    content: "Trial matches commence Friday at 4 PM in Hall 1.",
    status: "draft",
    creatorId: "usr-director-1",
    createdAt: "2026-11-08T10:00:00Z",
    updatedAt: "2026-11-08T12:00:00Z",
    publishedAt: null,
  };

  const incompleteDraft: Draft = {
    id: "4fa85f64-5717-4562-b3fc-2c963f66afa7",
    kind: "achievement",
    title: null,
    content: null,
    status: "draft",
    creatorId: "usr-admin-1",
    createdAt: "2026-11-08T10:00:00Z",
    updatedAt: "2026-11-08T10:00:00Z",
  };

  it("validates a conforming complete draft", () => {
    expect(isDraft(completeDraft)).toBe(true);
    expect(parseDraft(completeDraft)).toEqual(completeDraft);
  });

  it("validates an incomplete draft with null title and content", () => {
    expect(isDraft(incompleteDraft)).toBe(true);
    expect(parseDraft(incompleteDraft)).toEqual(incompleteDraft);
  });

  it("rejects draft missing required fields or having invalid date strings", () => {
    expect(isDraft({ ...completeDraft, id: "" })).toBe(false);
    expect(isDraft({ ...completeDraft, kind: "other" })).toBe(false);
    expect(isDraft({ ...completeDraft, status: 123 })).toBe(false);
    expect(isDraft({ ...completeDraft, createdAt: "invalid-date" })).toBe(false);
    expect(isDraft({ ...completeDraft, updatedAt: "invalid-date" })).toBe(false);
  });

  it("throws descriptive error when parseDraft receives non-draft", () => {
    expect(() => parseDraft({ id: "only-id" })).toThrow(
      "Invalid draft response: payload does not match schema"
    );
  });
});

describe("Public Content Item Runtime Validation (Step 10K)", () => {
  const validPublishedItem: PublicContentItem = {
    id: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    kind: "announcement",
    title: "Badminton Trial Matches",
    content: "Trial matches commence Friday at 4 PM in Hall 1.",
    publishedAt: "2026-11-08T16:00:00Z",
  };

  it("validates a conforming published content item", () => {
    expect(isPublicContentItem(validPublishedItem)).toBe(true);
    expect(parsePublicContentItem(validPublishedItem)).toEqual(validPublishedItem);
  });

  it("rejects non-object or null values", () => {
    expect(isPublicContentItem(null)).toBe(false);
    expect(isPublicContentItem(undefined)).toBe(false);
    expect(isPublicContentItem("item")).toBe(false);
    expect(isPublicContentItem([])).toBe(false);
  });

  it("rejects published item with missing or empty id, kind, title, or content", () => {
    expect(isPublicContentItem({ ...validPublishedItem, id: "" })).toBe(false);
    expect(isPublicContentItem({ ...validPublishedItem, kind: "blog" })).toBe(false);
    expect(isPublicContentItem({ ...validPublishedItem, title: null })).toBe(false);
    expect(isPublicContentItem({ ...validPublishedItem, content: null })).toBe(false);
  });

  it("rejects published item with invalid publishedAt date string", () => {
    expect(isPublicContentItem({ ...validPublishedItem, publishedAt: "invalid" })).toBe(false);
    expect(isPublicContentItem({ ...validPublishedItem, publishedAt: "" })).toBe(false);
  });

  it("throws descriptive error when parsePublicContentItem receives invalid payload", () => {
    expect(() => parsePublicContentItem({ id: "only-id" })).toThrow(
      "Invalid published content response: payload does not match schema"
    );
  });

  it("parses valid array of published content items", () => {
    const list = parsePublicContentItems([validPublishedItem]);
    expect(list).toHaveLength(1);
    expect(list[0]).toEqual(validPublishedItem);
  });

  it("parses empty array as empty array", () => {
    expect(parsePublicContentItems([])).toEqual([]);
  });

  it("throws descriptive error when parsePublicContentItems receives non-array", () => {
    expect(() => parsePublicContentItems({ error: "not array" })).toThrow(
      "Invalid content feed response: expected an array"
    );
  });

  it("throws descriptive error when parsePublicContentItems receives item not matching schema", () => {
    expect(() => parsePublicContentItems([validPublishedItem, { bad: true }])).toThrow(
      "Invalid content item at index 1: payload does not match schema"
    );
  });
});

describe("Digital Wallet Runtime Validation & Currency Formatting (Step 11F)", () => {
  const validWallet = {
    status: "ACTIVE",
    currency: "PKR",
    balancePaisa: "40000",
  };

  const validActivityItem = {
    entryId: "e-1",
    transactionId: "tx-1",
    type: "PADEL_BOOKING",
    amountPaisa: "-20000",
    createdAt: "2026-11-09T08:00:00Z",
  };

  it("validates wallet summary correctly", () => {
    expect(isWalletSummary(validWallet)).toBe(true);
    expect(isWalletSummary({ ...validWallet, balancePaisa: "0" })).toBe(true);
    expect(isWalletSummary({ ...validWallet, balancePaisa: "9007199254740992" })).toBe(true);
    expect(isWalletSummary({ ...validWallet, balancePaisa: "-5000" })).toBe(true);

    expect(isWalletSummary(null)).toBe(false);
    expect(isWalletSummary({})).toBe(false);
    expect(isWalletSummary({ ...validWallet, status: "" })).toBe(false);
    expect(isWalletSummary({ ...validWallet, currency: "" })).toBe(false);
    expect(isWalletSummary({ ...validWallet, balancePaisa: "not-a-number" })).toBe(false);
    expect(isWalletSummary({ ...validWallet, balancePaisa: "123.45" })).toBe(false);
  });

  it("validates wallet activity item correctly", () => {
    expect(isWalletActivityItem(validActivityItem)).toBe(true);
    expect(isWalletActivityItem({ ...validActivityItem, amountPaisa: "50000", type: "TOPUP" })).toBe(true);

    expect(isWalletActivityItem(null)).toBe(false);
    expect(isWalletActivityItem({})).toBe(false);
    expect(isWalletActivityItem({ ...validActivityItem, entryId: "" })).toBe(false);
    expect(isWalletActivityItem({ ...validActivityItem, createdAt: "invalid-date" })).toBe(false);
    expect(isWalletActivityItem({ ...validActivityItem, amountPaisa: "35.5" })).toBe(false);
  });

  it("validates and parses wallet me response", () => {
    // Funded wallet with activity
    const fullResp = {
      wallet: validWallet,
      activity: [validActivityItem],
    };
    expect(isWalletMeResponse(fullResp)).toBe(true);
    expect(parseWalletMeResponse(fullResp)).toEqual(fullResp);

    // No wallet yet (wallet: null, activity: [])
    const noWalletResp = {
      wallet: null,
      activity: [],
    };
    expect(isWalletMeResponse(noWalletResp)).toBe(true);
    expect(parseWalletMeResponse(noWalletResp)).toEqual(noWalletResp);

    // Zero balance wallet with empty activity
    const zeroBalResp = {
      wallet: { status: "ACTIVE", currency: "PKR", balancePaisa: "0" },
      activity: [],
    };
    expect(isWalletMeResponse(zeroBalResp)).toBe(true);
    expect(parseWalletMeResponse(zeroBalResp)).toEqual(zeroBalResp);

    // Rejection cases
    expect(isWalletMeResponse(null)).toBe(false);
    expect(isWalletMeResponse({ wallet: null, activity: null })).toBe(false);
    expect(isWalletMeResponse({ wallet: { bad: true }, activity: [] })).toBe(false);
    expect(() => parseWalletMeResponse({ wallet: { bad: true }, activity: [] })).toThrow(
      "Invalid wallet response: payload does not match schema"
    );
  });

  it("formats integer paisa to exact PKR without floating-point conversion", () => {
    // Zero balance
    expect(formatPaisaToPKR("0")).toBe("PKR 0.00");
    expect(formatPaisaToPKR("")).toBe("PKR 0.00");
    expect(formatPaisaToPKR("invalid")).toBe("PKR 0.00");

    // Standard credits and debits
    expect(formatPaisaToPKR("5")).toBe("PKR 0.05");
    expect(formatPaisaToPKR("50")).toBe("PKR 0.50");
    expect(formatPaisaToPKR("100")).toBe("PKR 1.00");
    expect(formatPaisaToPKR("40000")).toBe("PKR 400.00");
    expect(formatPaisaToPKR("100000")).toBe("PKR 1,000.00");
    expect(formatPaisaToPKR("-35000")).toBe("-PKR 350.00");

    // Explicit positive sign for credits
    expect(formatPaisaToPKR("50000", { showExplicitPositiveSign: true })).toBe("+PKR 500.00");
    expect(formatPaisaToPKR("-20000", { showExplicitPositiveSign: true })).toBe("-PKR 200.00");
    expect(formatPaisaToPKR("0", { showExplicitPositiveSign: true })).toBe("PKR 0.00");

    // Large amounts exceeding JavaScript Number.MAX_SAFE_INTEGER (9007199254740991 = 2^53 - 1)
    const largePaisa = "9007199254740992"; // 2^53
    const formattedLarge = formatPaisaToPKR(largePaisa);
    expect(formattedLarge).toBe("PKR 90,071,992,547,409.92");

    const signedLarge = formatPaisaToPKR(largePaisa, { showExplicitPositiveSign: true });
    expect(signedLarge).toBe("+PKR 90,071,992,547,409.92");
  });

  it("formats ISO timestamps into readable strings", () => {
    const ts = formatTransactionTimestamp("2026-11-09T08:30:00Z");
    expect(ts).toContain("2026");
    expect(ts).toContain("Nov");
    expect(formatTransactionTimestamp("not-a-date")).toBe("not-a-date");
  });
});


