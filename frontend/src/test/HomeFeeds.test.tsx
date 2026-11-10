import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Home } from "../pages/Home";
import { api } from "../api";
import type { PublicContentItem } from "../types";

vi.mock("../api", () => ({
  api: {
    getFacilities: vi.fn().mockResolvedValue([]),
    getFacility: vi.fn(),
    getAnnouncements: vi.fn().mockResolvedValue([]),
    getAchievementsFeed: vi.fn().mockResolvedValue([]),
  },
}));

describe("Homepage Public Feeds Component (Step 10M)", () => {
  const mockAnnouncement: PublicContentItem = {
    id: "ann-uuid-1",
    kind: "announcement",
    title: "Badminton Hall Floor Refinishing",
    content: "Court 1 and 2 will remain closed on Saturday morning.",
    publishedAt: "2026-11-08T10:00:00Z",
  };

  const mockAchievement: PublicContentItem = {
    id: "ach-uuid-1",
    kind: "achievement",
    title: "Inter-Varsity Table Tennis Championship Gold",
    content: "GIKI Table Tennis team won gold against LUMS in the grand finals.",
    publishedAt: "2026-11-08T12:00:00Z",
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading states for both announcements and achievements sections while pending", () => {
    vi.mocked(api.getAnnouncements).mockReturnValue(new Promise(() => {}));
    vi.mocked(api.getAchievementsFeed).mockReturnValue(new Promise(() => {}));

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    expect(screen.getByText("Loading announcements...")).toBeInTheDocument();
    expect(screen.getByText("Loading achievements...")).toBeInTheDocument();
  });

  it("shows empty states when public feeds return empty arrays", async () => {
    vi.mocked(api.getAnnouncements).mockResolvedValue([]);
    vi.mocked(api.getAchievementsFeed).mockResolvedValue([]);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(
        screen.getByText("No announcements published yet. Check back soon for campus sports updates.")
      ).toBeInTheDocument();
      expect(
        screen.getByText("No sports achievements published yet. Recent match achievements will appear here.")
      ).toBeInTheDocument();
    });
  });

  it("renders populated announcements and achievements with plain text title and body", async () => {
    vi.mocked(api.getAnnouncements).mockResolvedValue([mockAnnouncement]);
    vi.mocked(api.getAchievementsFeed).mockResolvedValue([mockAchievement]);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    await waitFor(() => {
      // Announcements section
      expect(screen.getByText("Badminton Hall Floor Refinishing")).toBeInTheDocument();
      expect(screen.getByText("Court 1 and 2 will remain closed on Saturday morning.")).toBeInTheDocument();

      // Achievements section
      expect(screen.getByText("Inter-Varsity Table Tennis Championship Gold")).toBeInTheDocument();
      expect(screen.getByText("GIKI Table Tennis team won gold against LUMS in the grand finals.")).toBeInTheDocument();
    });

    // Ensure the prototype composer is not present
    expect(screen.queryByPlaceholderText("Share an update or achievement...")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Post" })).not.toBeInTheDocument();
  });

  it("handles announcement load failure and recovers on retry", async () => {
    vi.mocked(api.getAnnouncements)
      .mockRejectedValueOnce(new Error("Network connection error"))
      .mockResolvedValueOnce([mockAnnouncement]);
    vi.mocked(api.getAchievementsFeed).mockResolvedValue([]);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Network connection error")).toBeInTheDocument();
    });

    // Click retry inside announcements section
    const retryBtn = screen.getByRole("button", { name: "Retry" });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByText("Badminton Hall Floor Refinishing")).toBeInTheDocument();
      expect(screen.queryByText("Network connection error")).not.toBeInTheDocument();
    });
  });

  it("handles achievements load failure and recovers on retry", async () => {
    vi.mocked(api.getAnnouncements).mockResolvedValue([]);
    vi.mocked(api.getAchievementsFeed)
      .mockRejectedValueOnce(new Error("Database connection pool exhausted"))
      .mockResolvedValueOnce([mockAchievement]);

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Database connection pool exhausted")).toBeInTheDocument();
    });

    // Click retry inside achievements section
    const retryBtn = screen.getByRole("button", { name: "Retry" });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByText("Inter-Varsity Table Tennis Championship Gold")).toBeInTheDocument();
      expect(screen.queryByText("Database connection pool exhausted")).not.toBeInTheDocument();
    });
  });
});

