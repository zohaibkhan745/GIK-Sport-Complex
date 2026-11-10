import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { MemoryRouter, Routes, Route, useNavigate } from "react-router-dom";
import { AuthProvider } from "../context/AuthContext";
import { DraftEditor } from "../pages/DraftEditor";
import { AdminContent } from "../pages/AdminContent";
import { api, ApiError } from "../api";
import type { AuthUser, Draft, PublicContentItem } from "../types";

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
      listDrafts: vi.fn().mockResolvedValue([]),
      createDraft: vi.fn(),
      getDraft: vi.fn(),
      updateDraft: vi.fn(),
      publishDraft: vi.fn(),
    },
  };
});

describe("Draft Editor Component (Step 10J)", () => {
  const mockDirectorUser: AuthUser = {
    id: "usr-director-1",
    email: "director@example.test",
    displayName: "Sports Director (Dev)",
    roles: ["sports_director"],
  };

  const mockStudentUser: AuthUser = {
    id: "usr-student-2",
    email: "student@example.test",
    displayName: "Student (Dev)",
    roles: ["student"],
  };

  const mockExistingDraft: Draft = {
    id: "e45db77a-6eb2-43ce-a128-406c10b7f6fa",
    kind: "announcement",
    title: "Badminton Trial Matches",
    content: "Trial matches commence Friday at 4 PM in Hall 1.",
    status: "draft",
    creatorId: "usr-director-1",
    createdAt: "2026-11-08T10:00:00Z",
    updatedAt: "2026-11-08T12:00:00Z",
    publishedAt: null,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  // -------------------------------------------------------------------------
  // 1. Role Gating & Direct Navigation
  // -------------------------------------------------------------------------
  it("waits for session check before fetching draft and does not prematurely call getDraft", async () => {
    vi.mocked(api.me).mockReturnValue(new Promise(() => {}));

    render(
      <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    expect(screen.getByText("Checking authorization...")).toBeInTheDocument();
    expect(api.getDraft).not.toHaveBeenCalled();
  });

  it("denies signed-out visitors on direct editor navigation and does not fetch drafts", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(401, "unauthorized"));

    render(
      <MemoryRouter initialEntries={["/admin/content/new"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/new" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Sign In Required")).toBeInTheDocument();
      expect(screen.getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
      expect(api.getDraft).not.toHaveBeenCalled();
      expect(api.createDraft).not.toHaveBeenCalled();
    });
  });

  it("denies signed-in students on direct editor navigation without fetching drafts", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);

    render(
      <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Access Denied")).toBeInTheDocument();
      expect(api.getDraft).not.toHaveBeenCalled();
    });
  });

  it("distinguishes session check outage from signed-out on editor direct navigation", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(new Error("Connection refused"));

    render(
      <MemoryRouter initialEntries={["/admin/content/new"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/new" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Session Check Unavailable")).toBeInTheDocument();
      expect(screen.getByText(/Connection refused/)).toBeInTheDocument();
      expect(screen.queryByText("Sign In Required")).not.toBeInTheDocument();
      expect(api.createDraft).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // 2. Draft Creation: Incomplete Draft & Server ID Adoption
  // -------------------------------------------------------------------------
  it("creates an incomplete draft with null fields, adopts server-returned ID, and subsequent save updates it", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);

    const createdDraft: Draft = {
      id: "new-generated-draft-uuid",
      kind: "achievement",
      title: null,
      content: null,
      status: "draft",
      creatorId: "usr-director-1",
      createdAt: "2026-11-08T15:00:00Z",
      updatedAt: "2026-11-08T15:00:00Z",
    };

    vi.mocked(api.createDraft).mockResolvedValueOnce(createdDraft);

    render(
      <MemoryRouter initialEntries={["/admin/content/new"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/new" element={<DraftEditor />} />
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("New Content Draft")).toBeInTheDocument();
    });

    // Select Achievement kind
    const achievementRadio = screen.getByLabelText("Achievement");
    fireEvent.click(achievementRadio);

    // Leave title and body blank to test incomplete draft creation
    const saveButton = screen.getByRole("button", { name: "Save draft" });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(api.createDraft).toHaveBeenCalledWith({
        kind: "achievement",
        title: null,
        content: null,
      });
      expect(screen.getByText("Draft created successfully.")).toBeInTheDocument();
    });

    // Kind should now be fixed/read-only since it is in edit mode
    expect(screen.getByText("Achievement")).toBeInTheDocument();
    expect(screen.queryByLabelText("Announcement")).not.toBeInTheDocument();

    // Now edit the saved draft and save again
    const updatedDraft: Draft = {
      ...createdDraft,
      title: "First Place at Gala",
      content: "Team GIKI won the finals.",
      updatedAt: "2026-11-08T15:10:00Z",
    };
    vi.mocked(api.updateDraft).mockResolvedValueOnce(updatedDraft);

    const titleInput = screen.getByLabelText("Title");
    const contentInput = screen.getByLabelText("Body Content");

    fireEvent.change(titleInput, { target: { value: "First Place at Gala" } });
    fireEvent.change(contentInput, { target: { value: "Team GIKI won the finals." } });

    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));

    await waitFor(() => {
      // Must call updateDraft with the server-returned ID, NOT createDraft
      expect(api.updateDraft).toHaveBeenCalledWith(
        "new-generated-draft-uuid",
        {
          title: "First Place at Gala",
          content: "Team GIKI won the finals.",
        }
      );
      expect(screen.getByText("Draft saved successfully.")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 3. Reopening Saved Draft & Read-Only Kind
  // -------------------------------------------------------------------------
  it("reopens saved draft by ID, populates fields, and presents kind as read-only", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.getDraft).mockResolvedValueOnce(mockExistingDraft);

    render(
      <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(api.getDraft).toHaveBeenCalledWith(
        "e45db77a-6eb2-43ce-a128-406c10b7f6fa",
        expect.any(Object)
      );
      expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      expect(
        screen.getByDisplayValue("Trial matches commence Friday at 4 PM in Hall 1.")
      ).toBeInTheDocument();
    });

    // Kind is rendered as badge, not radio buttons
    expect(screen.getByText("Announcement")).toBeInTheDocument();
    expect(screen.getByText("(Kind is fixed once created)")).toBeInTheDocument();
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // 4. Saving Both Fields on PUT (Sending null for cleared fields)
  // -------------------------------------------------------------------------
  it("sends both fields on PUT and correctly passes null when title is cleared", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.getDraft).mockResolvedValueOnce(mockExistingDraft);

    const updatedDraft: Draft = {
      ...mockExistingDraft,
      title: null,
      content: "Updated body only",
    };
    vi.mocked(api.updateDraft).mockResolvedValueOnce(updatedDraft);

    render(
      <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
    });

    // Clear the title field
    const titleInput = screen.getByLabelText("Title");
    fireEvent.change(titleInput, { target: { value: "   " } });

    // Update body content
    const contentInput = screen.getByLabelText("Body Content");
    fireEvent.change(contentInput, { target: { value: "Updated body only" } });

    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));

    await waitFor(() => {
      expect(api.updateDraft).toHaveBeenCalledWith(
        "e45db77a-6eb2-43ce-a128-406c10b7f6fa",
        {
          title: null, // Omitted/blank field explicitly passed as null
          content: "Updated body only",
        }
      );
      expect(screen.getByText("Draft saved successfully.")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 5. Cancel Action Without a Write
  // -------------------------------------------------------------------------
  it("cancels without performing a write and navigates back to draft list", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);

    render(
      <MemoryRouter initialEntries={["/admin/content/new"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/new" element={<DraftEditor />} />
            <Route path="/admin/content" element={<div>Content Drafts List Page</div>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("New Content Draft")).toBeInTheDocument();
    });

    // Type into title
    fireEvent.change(screen.getByLabelText("Title"), {
      target: { value: "Unsaved Title" },
    });

    // Click Cancel
    const cancelBtn = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancelBtn);

    await waitFor(() => {
      expect(screen.getByText("Content Drafts List Page")).toBeInTheDocument();
      expect(api.createDraft).not.toHaveBeenCalled();
      expect(api.updateDraft).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // 6. Text Preservation on Failure & No Auto-Retry
  // -------------------------------------------------------------------------
  it("retains draft text on save failure without wiping inputs and does not auto-retry", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.getDraft).mockResolvedValueOnce(mockExistingDraft);
    vi.mocked(api.updateDraft).mockRejectedValueOnce(
      new Error("Database pool timeout")
    );

    render(
      <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
    });

    // Change title and save
    const titleInput = screen.getByLabelText("Title");
    fireEvent.change(titleInput, { target: { value: "New Title Attempt" } });

    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));

    await waitFor(() => {
      expect(screen.getByText("Database pool timeout")).toBeInTheDocument();
      // Ensure the entered text is retained
      expect(titleInput).toHaveValue("New Title Attempt");
      expect(api.updateDraft).toHaveBeenCalledTimes(1);
    });
  });

  // -------------------------------------------------------------------------
  // 7. Error Explanations: 404 Missing & 409 Already Published
  // -------------------------------------------------------------------------
  it("explains 404 as missing and 409 as no longer editable as a draft", async () => {
    // 1. Test 404 on load
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.getDraft).mockRejectedValueOnce(new ApiError(404, "not found"));

    const { unmount } = render(
      <MemoryRouter initialEntries={["/admin/content/missing-id"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "Draft not found. The draft may have been deleted or the ID is invalid."
        )
      ).toBeInTheDocument();
    });

    unmount();

    // 2. Test 409 on update
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.getDraft).mockResolvedValueOnce(mockExistingDraft);
    vi.mocked(api.updateDraft).mockRejectedValueOnce(
      new ApiError(409, "item already published")
    );

    render(
      <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
        <AuthProvider>
          <Routes>
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "This content item has already been published and can no longer be edited as a draft."
        )
      ).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 8. Navigation During Request / Stale Response Protection
  // -------------------------------------------------------------------------
  it("prevents late fetch response from replacing a different draft after navigation", async () => {
    vi.mocked(api.me).mockResolvedValue(mockDirectorUser);

    let resolveFirstCall: (value: Draft) => void;
    const slowFirstPromise = new Promise<Draft>((resolve) => {
      resolveFirstCall = resolve;
    });

    const secondDraft: Draft = {
      id: "second-draft-uuid",
      kind: "achievement",
      title: "Second Draft Title",
      content: "Second draft content",
      status: "draft",
      creatorId: "usr-director-1",
      createdAt: "2026-11-08T11:00:00Z",
      updatedAt: "2026-11-08T11:00:00Z",
    };

    vi.mocked(api.getDraft)
      .mockReturnValueOnce(slowFirstPromise)
      .mockResolvedValueOnce(secondDraft);

    function TestNavigationWrapper() {
      const navigate = useNavigate();
      return (
        <>
          <button
            type="button"
            onClick={() => navigate("/admin/content/second-draft-uuid")}
          >
            Navigate To Draft 2
          </button>
          <Routes>
            <Route path="/admin/content/:id" element={<DraftEditor />} />
          </Routes>
        </>
      );
    }

    render(
      <MemoryRouter initialEntries={["/admin/content/draft-1"]}>
        <AuthProvider>
          <TestNavigationWrapper />
        </AuthProvider>
      </MemoryRouter>
    );

    // Initial draft 1 is fetching slowly
    await waitFor(() => {
      expect(api.getDraft).toHaveBeenCalledWith("draft-1", expect.any(Object));
    });

    // User navigates to draft-2 via client-side routing
    fireEvent.click(screen.getByRole("button", { name: "Navigate To Draft 2" }));

    await waitFor(() => {
      expect(api.getDraft).toHaveBeenCalledWith("second-draft-uuid", expect.any(Object));
      expect(screen.getByDisplayValue("Second Draft Title")).toBeInTheDocument();
    });

    // Now first slow response resolves
    resolveFirstCall!(mockExistingDraft);

    // Should NOT overwrite draft-2 title
    expect(screen.getByDisplayValue("Second Draft Title")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("Badminton Trial Matches")).not.toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // 9. Step 10K: Publish Saved Draft Flows & Error Handling
  // -------------------------------------------------------------------------
  describe("Step 10K: Publish Saved Draft", () => {
    const mockPublishedResponse: PublicContentItem = {
      id: "e45db77a-6eb2-43ce-a128-406c10b7f6fa",
      kind: "announcement",
      title: "Badminton Trial Matches",
      content: "Trial matches commence Friday at 4 PM in Hall 1.",
      publishedAt: "2026-11-08T16:00:00Z",
    };

    it("blocks publication when editor has unsaved changes without opening confirmation or auto-saving", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      // User alters the title without saving
      fireEvent.change(screen.getByLabelText("Title"), {
        target: { value: "Badminton Trial Matches (Unsaved Edits)" },
      });

      // User attempts to publish
      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));

      // Unsaved changes explanation must be presented
      expect(
        screen.getByText("You have unsaved changes. Please save the draft first before publishing.")
      ).toBeInTheDocument();

      // Modal must NOT open and no publish request sent
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(api.publishDraft).not.toHaveBeenCalled();
      expect(api.updateDraft).not.toHaveBeenCalled();
    });

    it("blocks publication when saved draft is incomplete (blank title or body)", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      const incompleteDraft: Draft = {
        ...mockExistingDraft,
        id: "incomplete-uuid",
        title: null,
        content: "Some body text",
      };
      vi.mocked(api.getDraft).mockResolvedValue(incompleteDraft);

      render(
        <MemoryRouter initialEntries={["/admin/content/incomplete-uuid"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Some body text")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));

      expect(
        screen.getByText(
          "Cannot publish: the draft is incomplete. Both a non-blank title and body content must be saved before publishing."
        )
      ).toBeInTheDocument();

      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(api.publishDraft).not.toHaveBeenCalled();
    });

    it("cancels confirmation modal without sending any network request", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      // Open confirmation modal
      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));

      const dialog = screen.getByRole("dialog");
      expect(dialog).toBeInTheDocument();
      expect(screen.getByText("Confirm Publication")).toBeInTheDocument();
      expect(screen.getByText(/"Badminton Trial Matches"/)).toBeInTheDocument();

      // Click Cancel inside the modal
      fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

      // Modal closed, no network request issued
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(api.publishDraft).not.toHaveBeenCalled();
    });

    it("successfully publishes draft via bodyless POST, returns to list, and displays announcement banner", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);
      vi.mocked(api.publishDraft).mockResolvedValue(mockPublishedResponse);
      // When navigated back to list, drafts list returns empty (item published)
      vi.mocked(api.listDrafts).mockResolvedValue([]);

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
              <Route path="/admin/content" element={<AdminContent />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      // Click publish draft
      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));

      // In modal, click Confirm & Publish
      const dialog = screen.getByRole("dialog");
      fireEvent.click(within(dialog).getByRole("button", { name: "Confirm & Publish" }));

      await waitFor(() => {
        expect(api.publishDraft).toHaveBeenCalledWith(
          "e45db77a-6eb2-43ce-a128-406c10b7f6fa",
          expect.any(Object)
        );
      });

      // Verify redirection to draft list and announcement banner
      await waitFor(() => {
        expect(
          screen.getByText('"Badminton Trial Matches" has been published and is now public.')
        ).toBeInTheDocument();
      });
    });

    it("locks UI controls and prevents duplicate publish calls while in progress", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);

      let resolvePublish: (val: PublicContentItem) => void;
      const pendingPublish = new Promise<PublicContentItem>((res) => {
        resolvePublish = res;
      });
      vi.mocked(api.publishDraft).mockReturnValue(pendingPublish);

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));

      const dialog = screen.getByRole("dialog");
      const confirmBtn = within(dialog).getByRole("button", { name: "Confirm & Publish" });
      fireEvent.click(confirmBtn);

      // Verify button changes text and is disabled
      expect(within(dialog).getByRole("button", { name: /Publishing.../ })).toBeDisabled();
      expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeDisabled();

      // Duplicate click attempts must not fire publish again
      fireEvent.click(confirmBtn);
      expect(api.publishDraft).toHaveBeenCalledTimes(1);

      // Resolve publish cleanly
      resolvePublish!(mockPublishedResponse);
    });

    it("handles 422 error from server by retaining draft text and displaying incomplete explanation", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);
      vi.mocked(api.publishDraft).mockRejectedValueOnce(
        new ApiError(422, "title and body required")
      );

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));
      const dialog = screen.getByRole("dialog");
      fireEvent.click(within(dialog).getByRole("button", { name: "Confirm & Publish" }));

      await waitFor(() => {
        expect(
          screen.getByText(
            "Cannot publish: the saved draft is incomplete. Both a non-blank title and body are required."
          )
        ).toBeInTheDocument();
      });

      // Draft inputs must be preserved
      expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      expect(
        screen.getByDisplayValue("Trial matches commence Friday at 4 PM in Hall 1.")
      ).toBeInTheDocument();
    });

    it("handles 409 conflict error when item has already been published", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);
      vi.mocked(api.publishDraft).mockRejectedValueOnce(
        new ApiError(409, "already published")
      );

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));
      const dialog = screen.getByRole("dialog");
      fireEvent.click(within(dialog).getByRole("button", { name: "Confirm & Publish" }));

      await waitFor(() => {
        expect(
          screen.getByText(
            "Cannot publish: this item has already been published and is no longer a draft."
          )
        ).toBeInTheDocument();
      });
    });

    it("handles 500 server failure without claiming success and asks user to refresh", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);
      vi.mocked(api.publishDraft).mockRejectedValueOnce(
        new ApiError(500, "database unreachable")
      );

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));
      const dialog = screen.getByRole("dialog");
      fireEvent.click(within(dialog).getByRole("button", { name: "Confirm & Publish" }));

      await waitFor(() => {
        expect(
          screen.getByText(
            "Publishing failed due to a server error. Please refresh the page to verify the draft's current status before trying again."
          )
        ).toBeInTheDocument();
      });

      // Retains draft text
      expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 10. Step 10L: Publish Confirmation Keyboard Accessibility & Focus Management
  // -------------------------------------------------------------------------
  describe("Step 10L: Keyboard Accessibility & Focus Management", () => {
    it("focuses Cancel button when confirmation modal opens", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));

      const dialog = screen.getByRole("dialog");
      expect(dialog).toBeInTheDocument();

      const cancelBtn = within(dialog).getByRole("button", { name: "Cancel" });
      expect(cancelBtn).toHaveFocus();
    });

    it("traps Tab and Shift+Tab key navigation inside modal", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole("button", { name: "Publish draft" }));

      const dialog = screen.getByRole("dialog");
      const cancelBtn = within(dialog).getByRole("button", { name: "Cancel" });
      const confirmBtn = within(dialog).getByRole("button", { name: "Confirm & Publish" });

      expect(cancelBtn).toHaveFocus();

      // Shift+Tab from Cancel should wrap to Confirm & Publish
      fireEvent.keyDown(window, { key: "Tab", shiftKey: true });
      expect(confirmBtn).toHaveFocus();

      // Tab from Confirm & Publish should wrap back to Cancel
      fireEvent.keyDown(window, { key: "Tab", shiftKey: false });
      expect(cancelBtn).toHaveFocus();
    });

    it("returns focus to Publish draft button when Cancel button is clicked and sends no request", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      const publishBtn = screen.getByRole("button", { name: "Publish draft" });
      fireEvent.click(publishBtn);

      const dialog = screen.getByRole("dialog");
      const cancelBtn = within(dialog).getByRole("button", { name: "Cancel" });

      fireEvent.click(cancelBtn);

      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(publishBtn).toHaveFocus();
      expect(api.publishDraft).not.toHaveBeenCalled();
    });

    it("returns focus to Publish draft button when Escape is pressed and sends no request", async () => {
      vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
      vi.mocked(api.getDraft).mockResolvedValue(mockExistingDraft);

      render(
        <MemoryRouter initialEntries={["/admin/content/e45db77a-6eb2-43ce-a128-406c10b7f6fa"]}>
          <AuthProvider>
            <Routes>
              <Route path="/admin/content/:id" element={<DraftEditor />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Badminton Trial Matches")).toBeInTheDocument();
      });

      const publishBtn = screen.getByRole("button", { name: "Publish draft" });
      fireEvent.click(publishBtn);

      expect(screen.getByRole("dialog")).toBeInTheDocument();

      // Press Escape on window
      fireEvent.keyDown(window, { key: "Escape" });

      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(publishBtn).toHaveFocus();
      expect(api.publishDraft).not.toHaveBeenCalled();
    });
  });
});
