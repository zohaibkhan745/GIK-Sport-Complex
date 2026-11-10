import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AuthProvider } from "../context/AuthContext";
import { Navbar } from "../components/Navbar";
import { AdminContent } from "../pages/AdminContent";
import { api, ApiError } from "../api";
import type { AuthUser, DraftSummary } from "../types";

vi.mock("../api", () => {
  class MockApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
      this.name = "ApiError";
    }
  }

  return {
    ApiError: MockApiError,
    api: {
      getFacilities: vi.fn().mockResolvedValue([]),
      getFacility: vi.fn(),
      login: vi.fn(),
      me: vi.fn(),
      logout: vi.fn(),
      listDrafts: vi.fn(),
    },
  };
});

describe("Admin Content Drafts Page (Step 10I)", () => {
  const mockDirectorUser: AuthUser = {
    id: "usr-director-1",
    email: "director@example.test",
    displayName: "Sports Director (Dev)",
    roles: ["sports_director"],
  };

  const mockAdminUser: AuthUser = {
    id: "usr-admin-1",
    email: "admin@example.test",
    displayName: "Super Admin (Dev)",
    roles: ["super_admin"],
  };

  const mockStudentUser: AuthUser = {
    id: "usr-student-2",
    email: "student@example.test",
    displayName: "Student (Dev)",
    roles: ["student"],
  };

  const mockDrafts: DraftSummary[] = [
    {
      id: "draft-1",
      kind: "announcement",
      title: "Inter-Faculty Table Tennis",
      updatedAt: "2026-11-08T14:30:00Z",
    },
    {
      id: "draft-2",
      kind: "achievement",
      title: null, // Incomplete draft with null title
      updatedAt: "2026-11-08T15:00:00Z",
    },
    {
      id: "draft-3",
      kind: "announcement",
      title: "   ", // Whitespace title treated as untitled
      updatedAt: "2026-11-08T15:15:00Z",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  // -------------------------------------------------------------------------
  // 1. Role Gating & Direct Navigation
  // -------------------------------------------------------------------------
  it("waits for the initial session check before fetching drafts and does not prematurely call listDrafts", async () => {
    // Keep session check pending indefinitely
    vi.mocked(api.me).mockReturnValue(new Promise(() => {}));

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    expect(screen.getByText("Checking authorization...")).toBeInTheDocument();
    expect(api.listDrafts).not.toHaveBeenCalled();
  });

  it("shows sign-in link on direct navigation when signed out, and does not fetch drafts", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(401, "unauthorized"));

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Sign In Required")).toBeInTheDocument();
      const signInLink = screen.getByRole("link", { name: "Sign in" });
      expect(signInLink).toBeInTheDocument();
      expect(signInLink).toHaveAttribute("href", "/login");
      expect(api.listDrafts).not.toHaveBeenCalled();
    });
  });

  it("shows access denied on direct navigation when signed in without allowed role, and does not fetch drafts", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Access Denied")).toBeInTheDocument();
      expect(
        screen.getByText(/reserved for sports directors and super administrators/i)
      ).toBeInTheDocument();
      expect(api.listDrafts).not.toHaveBeenCalled();
    });
  });

  it("keeps session-check outages distinct from being signed out and displays retry", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(
      new Error("Database connection pool exhausted")
    );

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(
        screen.getByText("Session Check Unavailable")
      ).toBeInTheDocument();
      expect(
        screen.getByText(/Database connection pool exhausted/)
      ).toBeInTheDocument();
      expect(screen.queryByText("Sign In Required")).not.toBeInTheDocument();
      expect(api.listDrafts).not.toHaveBeenCalled();
    });

    // Retrying session check
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.listDrafts).mockResolvedValueOnce([]);

    const retryBtn = screen.getByRole("button", {
      name: "Retry Session Check",
    });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(api.listDrafts).toHaveBeenCalledTimes(1);
      expect(screen.getByText("Content Drafts")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 2. Navigation Link for Directors and Super Admins
  // -------------------------------------------------------------------------
  it("renders 'Manage content' link in Navbar only for directors and super admins", async () => {
    // 1. Student: link not visible
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);

    const { unmount } = render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Student (Dev)")).toBeInTheDocument();
      expect(
        screen.queryByRole("link", { name: "Manage content" })
      ).not.toBeInTheDocument();
    });

    unmount();

    // 2. Sports Director: link visible
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);

    const { unmount: unmountDirector } = render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      const link = screen.getByRole("link", { name: "Manage content" });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute("href", "/admin/content");
    });

    unmountDirector();

    // 3. Super Admin: link visible
    vi.mocked(api.me).mockResolvedValueOnce(mockAdminUser);

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      const link = screen.getByRole("link", { name: "Manage content" });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute("href", "/admin/content");
    });
  });

  // -------------------------------------------------------------------------
  // 3. Draft List Rendering: Populated, Null Titles, 50-Item Notice, No Edit/Publish
  // -------------------------------------------------------------------------
  it("renders populated drafts with kind badges, formatted dates, and handles null titles", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.listDrafts).mockResolvedValueOnce(mockDrafts);

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Content Drafts")).toBeInTheDocument();
      expect(
        screen.getByText("Inter-Faculty Table Tennis")
      ).toBeInTheDocument();
    });

    // 50-Item limit disclaimer
    expect(
      screen.getByText(/showing up to 50 most recently updated drafts/i)
    ).toBeInTheDocument();

    // Null and whitespace titles rendered as "Untitled draft"
    const untitledDrafts = screen.getAllByText("Untitled draft");
    expect(untitledDrafts).toHaveLength(2);

    // Kinds
    expect(screen.getAllByText("Announcement").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Achievement").length).toBeGreaterThanOrEqual(1);

    // New draft action link
    const newDraftLink = screen.getByRole("link", { name: /\+ New draft/i });
    expect(newDraftLink).toBeInTheDocument();
    expect(newDraftLink).toHaveAttribute("href", "/admin/content/new");

    // Open links for saved drafts
    const openLinks = screen.getAllByRole("link", { name: /Open/i });
    expect(openLinks.length).toBeGreaterThanOrEqual(3);
    expect(openLinks[0]).toHaveAttribute("href", "/admin/content/draft-1");

    // Strictly no inactive Edit or Publish buttons per requirement
    expect(
      screen.queryByRole("button", { name: /edit/i })
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /publish/i })
    ).not.toBeInTheDocument();
  });

  it("renders empty state when no drafts are found", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.listDrafts).mockResolvedValueOnce([]);

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("No drafts found.")).toBeInTheDocument();
      expect(
        screen.getByText("There are currently no saved drafts.")
      ).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 4. Filtering: All, Announcements, Achievements
  // -------------------------------------------------------------------------
  it("supports switching filters between All, Announcements, and Achievements", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.listDrafts).mockResolvedValue(mockDrafts);

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(api.listDrafts).toHaveBeenCalledWith("all", expect.any(Object));
    });

    // Switch to Announcements
    const announcementBtn = screen.getByRole("button", {
      name: "Announcements",
    });
    fireEvent.click(announcementBtn);

    await waitFor(() => {
      expect(api.listDrafts).toHaveBeenCalledWith(
        "announcement",
        expect.any(Object)
      );
    });

    // Switch to Achievements
    const achievementBtn = screen.getByRole("button", { name: "Achievements" });
    fireEvent.click(achievementBtn);

    await waitFor(() => {
      expect(api.listDrafts).toHaveBeenCalledWith(
        "achievement",
        expect.any(Object)
      );
    });

    // Switch back to All
    const allBtn = screen.getByRole("button", { name: "All" });
    fireEvent.click(allBtn);

    await waitFor(() => {
      expect(api.listDrafts).toHaveBeenCalledWith("all", expect.any(Object));
    });
  });

  // -------------------------------------------------------------------------
  // 5. Error & Retry State
  // -------------------------------------------------------------------------
  it("displays error state with retry button when listDrafts fails, and recovers on retry", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.listDrafts).mockRejectedValueOnce(
      new Error("Failed to fetch drafts: network error")
    );

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(
        screen.getByText("Failed to fetch drafts: network error")
      ).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    });

    // When clicking retry, succeeds
    vi.mocked(api.listDrafts).mockResolvedValueOnce(mockDrafts);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(
        screen.getByText("Inter-Faculty Table Tennis")
      ).toBeInTheDocument();
      expect(
        screen.queryByText("Failed to fetch drafts: network error")
      ).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 6. Stale Response & Request Cancellation Protection
  // -------------------------------------------------------------------------
  it("cancels obsolete requests and prevents slow earlier response from overwriting newer results", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);

    let resolveFirstCall: (value: DraftSummary[]) => void;
    const slowFirstPromise = new Promise<DraftSummary[]>((resolve) => {
      resolveFirstCall = resolve;
    });

    let resolveSecondCall: (value: DraftSummary[]) => void;
    const fastSecondPromise = new Promise<DraftSummary[]>((resolve) => {
      resolveSecondCall = resolve;
    });

    // First call ("all") is slow
    vi.mocked(api.listDrafts).mockReturnValueOnce(slowFirstPromise);
    // Second call ("announcements") is fast
    vi.mocked(api.listDrafts).mockReturnValueOnce(fastSecondPromise);

    render(
      <MemoryRouter initialEntries={["/admin/content"]}>
        <AuthProvider>
          <AdminContent />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(api.listDrafts).toHaveBeenCalledWith("all", expect.any(Object));
    });

    // Quickly switch filter to Announcements before first completes
    const announcementBtn = screen.getByRole("button", {
      name: "Announcements",
    });
    fireEvent.click(announcementBtn);

    await waitFor(() => {
      expect(api.listDrafts).toHaveBeenCalledWith(
        "announcement",
        expect.any(Object)
      );
    });

    // Resolve the second (announcement) request first with one announcement
    resolveSecondCall!([mockDrafts[0]]);

    await waitFor(() => {
      expect(
        screen.getByText("Inter-Faculty Table Tennis")
      ).toBeInTheDocument();
    });

    // Now resolve the older, slow first request ("all" with all 3 drafts)
    resolveFirstCall!(mockDrafts);

    // Ensure the older response did NOT overwrite the announcements view
    // (i.e. untitled drafts from the older "all" request should not appear)
    expect(screen.queryByText("Untitled draft")).not.toBeInTheDocument();
  });
});
