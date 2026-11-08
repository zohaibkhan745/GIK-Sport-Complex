import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router-dom";
import { FacilityDetail } from "../pages/FacilityDetail";
import { api, ApiError } from "../api";
import { AuthContext, type AuthContextType } from "../context/AuthContext";
import { formatTimeRange, type AuthUser, type Facility } from "../types";

vi.mock("../api", () => {
  class MockApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.name = "ApiError";
      this.status = status;
    }
  }
  return {
    api: {
      getFacilities: vi.fn(),
      getFacility: vi.fn(),
      updateSchedule: vi.fn(),
      deleteSchedule: vi.fn(),
      createSchedule: vi.fn(),
    },
    ApiError: MockApiError,
  };
});

describe("Facility Detail Page Component", () => {
  const completeFacility: Facility = {
    id: "fac-1",
    slug: "demo-badminton-court",
    name: "[Demo] Campus Badminton Court",
    description: "Synthetic test description for demonstration court",
    location: "Wing B, Level 1",
    status: "active",
    schedules: [
      {
        id: "sched-1",
        dayOfWeek: 1, // Monday
        startTime: "09:00:00",
        endTime: "12:00:00",
        audience: "student",
      },
      {
        id: "sched-2",
        dayOfWeek: 5, // Friday
        startTime: "22:00:00",
        endTime: "24:00:00",
        audience: "faculty",
      },
    ],
  };

  const nullableFacility: Facility = {
    id: "fac-2",
    slug: "demo-table-tennis",
    name: "[Demo] Table Tennis Arena",
    description: null,
    location: null,
    status: "maintenance",
    schedules: [],
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  const renderWithRoute = (initialPath: string) => {
    return render(
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route path="/facilities/:slug" element={<FacilityDetail />} />
        </Routes>
      </MemoryRouter>
    );
  };

  it("shows loading indicator while fetching detail", () => {
    vi.mocked(api.getFacility).mockReturnValue(new Promise(() => {})); // Never resolves

    renderWithRoute("/facilities/demo-badminton-court");

    expect(screen.getByTestId("facility-detail-loading")).toBeInTheDocument();
  });

  it("renders complete facility details, structured weekly schedules, and Asia/Karachi time notes", async () => {
    vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

    renderWithRoute("/facilities/demo-badminton-court");

    await waitFor(() => {
      expect(
        screen.getByText("[Demo] Campus Badminton Court")
      ).toBeInTheDocument();
    });

    expect(
      screen.getByText("Synthetic test description for demonstration court")
    ).toBeInTheDocument();
    expect(screen.getByText("Wing B, Level 1")).toBeInTheDocument();
    expect(screen.getByText("Active & Operational")).toBeInTheDocument();

    // Verify Schedule items & weekday translation
    expect(screen.getByText("Monday")).toBeInTheDocument();
    expect(screen.getByText("09:00 – 12:00")).toBeInTheDocument();
    expect(screen.getByText("Students")).toBeInTheDocument();

    // Verify Friday schedule with 24:00:00 midnight end-of-day
    expect(screen.getByText("Friday")).toBeInTheDocument();
    expect(screen.getByText("22:00 – 24:00 (Midnight)")).toBeInTheDocument();
    expect(screen.getByText("Faculty")).toBeInTheDocument();

    // Verify timezone label
    expect(
      screen.getByText(/Times displayed in local Asia\/Karachi/i)
    ).toBeInTheDocument();

    // Verify nonfunctional booking button was removed
    expect(screen.queryByText(/Book a session/i)).not.toBeInTheDocument();
  });

  it("handles nullable fields (null description and location) and empty schedules gracefully", async () => {
    vi.mocked(api.getFacility).mockResolvedValue(nullableFacility);

    renderWithRoute("/facilities/demo-table-tennis");

    await waitFor(() => {
      expect(
        screen.getByText("[Demo] Table Tennis Arena")
      ).toBeInTheDocument();
    });

    expect(
      screen.getByText("No description provided for this facility.")
    ).toBeInTheDocument();
    expect(screen.getByText("Location not specified")).toBeInTheDocument();
    expect(screen.getByText("Under Maintenance")).toBeInTheDocument();
    expect(
      screen.getByText(
        "No recurring operating schedules are currently registered for this facility."
      )
    ).toBeInTheDocument();
  });

  it("displays 404 message when facility slug is not found", async () => {
    vi.mocked(api.getFacility).mockRejectedValue(
      new ApiError(404, "Facility not found")
    );

    renderWithRoute("/facilities/unknown-slug");

    await waitFor(() => {
      expect(screen.getByTestId("facility-detail-404")).toBeInTheDocument();
    });

    expect(screen.getByText("Facility Not Found")).toBeInTheDocument();
    expect(
      screen.getByText(/No facility matching slug “unknown-slug” could be found/i)
    ).toBeInTheDocument();
  });

  it("displays server error message with retry option when API fails with 500", async () => {
    vi.mocked(api.getFacility)
      .mockRejectedValueOnce(new ApiError(500, "Internal database error"))
      .mockResolvedValueOnce(completeFacility);

    renderWithRoute("/facilities/demo-badminton-court");

    await waitFor(() => {
      expect(screen.getByTestId("facility-detail-error")).toBeInTheDocument();
    });

    expect(screen.getByText("Unable to Load Facility")).toBeInTheDocument();
    expect(screen.getByText("Internal database error")).toBeInTheDocument();

    const retryBtn = screen.getByRole("button", { name: /retry/i });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(
        screen.getByText("[Demo] Campus Badminton Court")
      ).toBeInTheDocument();
    });
  });

  it("cancels obsolete requests and prevents stale data from replacing current facility when route slug changes", async () => {
    let resolveFirst: (f: Facility) => void = () => {};
    const firstPromise = new Promise<Facility>((resolve) => {
      resolveFirst = resolve;
    });

    let capturedFirstSignal: AbortSignal | undefined;

    vi.mocked(api.getFacility).mockImplementation(
      (slug: string, options?: { signal?: AbortSignal }) => {
        if (slug === "first-slug") {
          capturedFirstSignal = options?.signal;
          return firstPromise;
        }
        if (slug === "second-slug") {
          return Promise.resolve({
            ...nullableFacility,
            slug: "second-slug",
            name: "Second Facility",
          });
        }
        return Promise.reject(new Error("Unknown"));
      }
    );

    function RouteSwitcher() {
      const navigate = useNavigate();
      return (
        <div>
          <button
            type="button"
            data-testid="switch-btn"
            onClick={() => navigate("/facilities/second-slug")}
          >
            Switch
          </button>
          <FacilityDetail />
        </div>
      );
    }

    render(
      <MemoryRouter initialEntries={["/facilities/first-slug"]}>
        <Routes>
          <Route path="/facilities/:slug" element={<RouteSwitcher />} />
        </Routes>
      </MemoryRouter>
    );

    // Initial loading for first-slug
    expect(screen.getByTestId("facility-detail-loading")).toBeInTheDocument();
    expect(capturedFirstSignal).toBeDefined();
    expect(capturedFirstSignal?.aborted).toBe(false);

    // Trigger route change via navigation
    act(() => {
      fireEvent.click(screen.getByTestId("switch-btn"));
    });

    // First request should have been aborted on cleanup
    expect(capturedFirstSignal?.aborted).toBe(true);

    // Wait for second facility to render
    await waitFor(() => {
      expect(screen.getByText("Second Facility")).toBeInTheDocument();
    });

    // If first promise somehow resolves late, it should NOT overwrite Second Facility
    act(() => {
      resolveFirst(completeFacility);
    });

    // Ensure Second Facility remains displayed
    expect(screen.getByText("Second Facility")).toBeInTheDocument();
    expect(
      screen.queryByText("[Demo] Campus Badminton Court")
    ).not.toBeInTheDocument();
  });

  describe("Step 7E: Weekly Schedule Inline Editor", () => {
    const directorUser: AuthUser = {
      id: "usr-dir",
      email: "director@example.test",
      displayName: "Sports Director",
      roles: ["sports_director"],
    };

    const superAdminUser: AuthUser = {
      id: "usr-admin",
      email: "admin@example.test",
      displayName: "Super Admin",
      roles: ["super_admin"],
    };

    const studentUser: AuthUser = {
      id: "usr-student",
      email: "student@example.test",
      displayName: "Student User",
      roles: ["student"],
    };

    const createMockAuthContext = (user: AuthUser | null): AuthContextType => ({
      user,
      isLoading: false,
      checkError: null,
      logoutError: null,
      login: vi.fn(),
      logout: vi.fn(),
      retryCheckSession: vi.fn(),
      clearLogoutError: vi.fn(),
    });

    const renderWithAuth = (
      initialPath: string,
      user: AuthUser | null = null
    ) => {
      return render(
        <AuthContext.Provider value={createMockAuthContext(user)}>
          <MemoryRouter initialEntries={[initialPath]}>
            <Routes>
              <Route path="/facilities/:slug" element={<FacilityDetail />} />
            </Routes>
          </MemoryRouter>
        </AuthContext.Provider>
      );
    };

    it("hides 'Edit window' buttons for anonymous visitors and student users", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      // 1. Anonymous visitor
      const { unmount } = renderWithAuth(
        "/facilities/demo-badminton-court",
        null
      );
      await waitFor(() => {
        expect(
          screen.getByText("[Demo] Campus Badminton Court")
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByRole("button", { name: /edit .* schedule window/i })
      ).not.toBeInTheDocument();
      unmount();

      // 2. Student user
      renderWithAuth("/facilities/demo-badminton-court", studentUser);
      await waitFor(() => {
        expect(
          screen.getByText("[Demo] Campus Badminton Court")
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByRole("button", { name: /edit .* schedule window/i })
      ).not.toBeInTheDocument();
    });

    it("displays 'Edit window' buttons for sports_director and super_admin", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      // 1. Sports Director
      const { unmount } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
        expect(
          screen.getByLabelText("Edit Friday schedule window")
        ).toBeInTheDocument();
      });
      unmount();

      // 2. Super Admin
      renderWithAuth("/facilities/demo-badminton-court", superAdminUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
        expect(
          screen.getByLabelText("Edit Friday schedule window")
        ).toBeInTheDocument();
      });
    });

    it("opens inline editor with prefilled values on clicking 'Edit window' and cancels cleanly without saving", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      // Click Edit window on Monday row
      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));

      // Verify form is displayed with prefilled values
      expect(
        screen.getByTestId("schedule-edit-form-sched-1")
      ).toBeInTheDocument();
      const daySelect = screen.getByLabelText("Day of week") as HTMLSelectElement;
      const startInput = screen.getByLabelText("Start time") as HTMLInputElement;
      const endInput = screen.getByLabelText("End time") as HTMLInputElement;
      const audienceSelect = screen.getByLabelText(
        "Audience"
      ) as HTMLSelectElement;

      expect(daySelect.value).toBe("1");
      expect(startInput.value).toBe("09:00:00");
      expect(endInput.value).toBe("12:00:00");
      expect(audienceSelect.value).toBe("student");

      // Click Cancel
      fireEvent.click(screen.getByRole("button", { name: /cancel editing/i }));

      // Form is closed and timetable row is restored
      expect(
        screen.queryByTestId("schedule-edit-form-sched-1")
      ).not.toBeInTheDocument();
      expect(screen.getByText("Monday")).toBeInTheDocument();
      expect(screen.getByText("09:00 – 12:00")).toBeInTheDocument();
      expect(vi.mocked(api.updateSchedule)).not.toHaveBeenCalled();
    });

    it("validates input on client before sending and blocks invalid submissions", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));

      const startInput = screen.getByLabelText("Start time");
      const endInput = screen.getByLabelText("End time");

      // Invert times so start is after end
      fireEvent.change(startInput, { target: { value: "14:00:00" } });
      fireEvent.change(endInput, { target: { value: "10:00:00" } });

      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );

      // Client validation error appears
      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent(
        "Start time (14:00:00) must be strictly before end time (10:00:00)."
      );
      expect(vi.mocked(api.updateSchedule)).not.toHaveBeenCalled();

      // Form is still open with inputs preserved
      expect(
        screen.getByTestId("schedule-edit-form-sched-1")
      ).toBeInTheDocument();
      expect((startInput as HTMLInputElement).value).toBe("14:00:00");
    });

    it("sends exact PUT payload and updates timetable on success without actor ID or roles", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      vi.mocked(api.updateSchedule).mockResolvedValueOnce({
        id: "sched-1",
        dayOfWeek: 3,
        startTime: "10:00:00",
        endTime: "13:00:00",
        audience: "women_only",
      });

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));

      // Modify values
      fireEvent.change(screen.getByLabelText("Day of week"), {
        target: { value: "3" },
      });
      fireEvent.change(screen.getByLabelText("Start time"), {
        target: { value: "10:00:00" },
      });
      fireEvent.change(screen.getByLabelText("End time"), {
        target: { value: "13:00:00" },
      });
      fireEvent.change(screen.getByLabelText("Audience"), {
        target: { value: "women_only" },
      });

      // Submit
      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );

      await waitFor(() => {
        expect(vi.mocked(api.updateSchedule)).toHaveBeenCalledTimes(1);
      });

      // Assert exact call arguments
      const [calledSlug, calledId, calledPayload] =
        vi.mocked(api.updateSchedule).mock.calls[0];
      expect(calledSlug).toBe("demo-badminton-court");
      expect(calledId).toBe("sched-1");
      expect(calledPayload).toEqual({
        dayOfWeek: 3,
        startTime: "10:00:00",
        endTime: "13:00:00",
        audience: "women_only",
      });

      // Verify NO extraneous actor ID, user ID, or role was sent
      expect(Object.keys(calledPayload)).toEqual([
        "dayOfWeek",
        "startTime",
        "endTime",
        "audience",
      ]);

      // Form is closed and success banner is shown
      await waitFor(() => {
        expect(
          screen.queryByTestId("schedule-edit-form-sched-1")
        ).not.toBeInTheDocument();
      });
      expect(screen.getByTestId("schedule-success-banner")).toHaveTextContent(
        "Schedule window updated successfully."
      );

      // Timetable is updated with server canonical values
      expect(screen.getByText("Wednesday")).toBeInTheDocument();
      expect(screen.getByText("10:00 – 13:00")).toBeInTheDocument();
      expect(screen.getByText("Women Only")).toBeInTheDocument();
    });

    it("supports 24:00:00 as end-of-day midnight via the 'Set 24:00:00' shortcut", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      vi.mocked(api.updateSchedule).mockResolvedValueOnce({
        id: "sched-1",
        dayOfWeek: 1,
        startTime: "09:00:00",
        endTime: "24:00:00",
        audience: "student",
      });

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));

      // Click "Set 24:00:00" button
      const set24Btn = screen.getByRole("button", { name: /set 24:00:00/i });
      fireEvent.click(set24Btn);

      const endInput = screen.getByLabelText("End time") as HTMLInputElement;
      expect(endInput.value).toBe("24:00:00");

      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );

      await waitFor(() => {
        expect(vi.mocked(api.updateSchedule)).toHaveBeenCalledWith(
          "demo-badminton-court",
          "sched-1",
          {
            dayOfWeek: 1,
            startTime: "09:00:00",
            endTime: "24:00:00",
            audience: "student",
          }
        );
      });

      // Timetable shows 24:00 (Midnight)
      await waitFor(() => {
        expect(
          screen.getByText("09:00 – 24:00 (Midnight)")
        ).toBeInTheDocument();
      });
    });

    it("prevents duplicate submissions by disabling form controls while save is pending", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      let resolvePromise: (s: any) => void = () => {};
      const pendingPromise = new Promise((resolve) => {
        resolvePromise = resolve;
      });
      vi.mocked(api.updateSchedule).mockReturnValue(pendingPromise as any);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));

      const saveBtn = screen.getByRole("button", {
        name: /save schedule window/i,
      });
      const cancelBtn = screen.getByRole("button", {
        name: /cancel editing/i,
      });
      const daySelect = screen.getByLabelText("Day of week");

      fireEvent.click(saveBtn);

      // Now pending: save button says "Saving..." and controls are disabled
      expect(saveBtn).toBeDisabled();
      expect(saveBtn).toHaveTextContent("Saving...");
      expect(cancelBtn).toBeDisabled();
      expect(daySelect).toBeDisabled();

      // Further click does not trigger another call
      fireEvent.click(saveBtn);
      expect(vi.mocked(api.updateSchedule)).toHaveBeenCalledTimes(1);

      // Resolve the pending promise
      act(() => {
        resolvePromise({
          id: "sched-1",
          dayOfWeek: 1,
          startTime: "09:00:00",
          endTime: "12:00:00",
          audience: "student",
        });
      });

      await waitFor(() => {
        expect(
          screen.queryByTestId("schedule-edit-form-sched-1")
        ).not.toBeInTheDocument();
      });
    });

    it("maps 400, 401, 403, 404, 409 errors to specific feedback and keeps the draft intact", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      // Test 409 Conflict
      vi.mocked(api.updateSchedule).mockRejectedValueOnce(
        new ApiError(409, "Schedule window conflicts")
      );

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));

      // Modify audience to faculty
      fireEvent.change(screen.getByLabelText("Audience"), {
        target: { value: "faculty" },
      });

      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );

      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent(
        "This schedule window overlaps or conflicts with an existing operating window."
      );

      // Form remains open and draft value is preserved
      expect(
        screen.getByTestId("schedule-edit-form-sched-1")
      ).toBeInTheDocument();
      expect(
        (screen.getByLabelText("Audience") as HTMLSelectElement).value
      ).toBe("faculty");

      // Test 401 Unauthorized
      vi.mocked(api.updateSchedule).mockRejectedValueOnce(
        new ApiError(401, "Unauthorized")
      );
      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );
      await waitFor(() => {
        expect(screen.getByRole("alert")).toHaveTextContent(
          "Your session has expired. Please sign in again to modify schedules."
        );
      });

      // Test 403 Forbidden
      vi.mocked(api.updateSchedule).mockRejectedValueOnce(
        new ApiError(403, "Forbidden")
      );
      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );
      await waitFor(() => {
        expect(screen.getByRole("alert")).toHaveTextContent(
          "You do not have permission to edit this schedule. Only sports directors and super admins can modify timetables."
        );
      });

      // Test 404 Not Found
      vi.mocked(api.updateSchedule).mockRejectedValueOnce(
        new ApiError(404, "Not Found")
      );
      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );
      await waitFor(() => {
        expect(screen.getByRole("alert")).toHaveTextContent(
          "Schedule window or facility not found. It may have been modified or deleted."
        );
      });

      // Test 400 Bad Request
      vi.mocked(api.updateSchedule).mockRejectedValueOnce(
        new ApiError(400, "Validation failed on fields")
      );
      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );
      await waitFor(() => {
        expect(screen.getByRole("alert")).toHaveTextContent(
          "Validation failed on fields"
        );
      });
    });

    it("maps 500 / network failures to reload warning and does not auto-retry", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      vi.mocked(api.updateSchedule).mockRejectedValueOnce(
        new ApiError(500, "Internal Server Error")
      );

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );

      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent(
        "Server failure encountered. Changes may not have been saved. Please reload this facility before retrying."
      );

      // Verify no automatic retries occurred
      expect(vi.mocked(api.updateSchedule)).toHaveBeenCalledTimes(1);

      // Draft form remains visible for user
      expect(
        screen.getByTestId("schedule-edit-form-sched-1")
      ).toBeInTheDocument();
    });

    it("safely handles unmount while save is in flight without throwing state errors", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      let resolveSave: (s: any) => void = () => {};
      const savePromise = new Promise((resolve) => {
        resolveSave = resolve;
      });
      vi.mocked(api.updateSchedule).mockReturnValue(savePromise as any);

      const { unmount } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /save schedule window/i })
      );

      // Unmount while save promise is in flight
      unmount();

      // Now resolve the promise - should not cause an uncaught error
      expect(() => {
        act(() => {
          resolveSave({
            id: "sched-1",
            dayOfWeek: 1,
            startTime: "09:00:00",
            endTime: "12:00:00",
            audience: "student",
          });
        });
      }).not.toThrow();
    });
  });

  describe("Step 8C: Weekly Schedule Deletion Confirmation", () => {
    const directorUser: AuthUser = {
      id: "usr-dir",
      email: "director@example.test",
      displayName: "Sports Director",
      roles: ["sports_director"],
    };

    const superAdminUser: AuthUser = {
      id: "usr-admin",
      email: "admin@example.test",
      displayName: "Super Admin",
      roles: ["super_admin"],
    };

    const studentUser: AuthUser = {
      id: "usr-student",
      email: "student@example.test",
      displayName: "Student User",
      roles: ["student"],
    };

    const facultyUser: AuthUser = {
      id: "usr-faculty",
      email: "faculty@example.test",
      displayName: "Faculty User",
      roles: ["faculty"],
    };

    const createMockAuthContext = (user: AuthUser | null): AuthContextType => ({
      user,
      isLoading: false,
      checkError: null,
      logoutError: null,
      login: vi.fn(),
      logout: vi.fn(),
      retryCheckSession: vi.fn(),
      clearLogoutError: vi.fn(),
    });

    const renderWithAuth = (
      initialPath: string,
      user: AuthUser | null = null
    ) => {
      return render(
        <AuthContext.Provider value={createMockAuthContext(user)}>
          <MemoryRouter initialEntries={[initialPath]}>
            <Routes>
              <Route path="/facilities/:slug" element={<FacilityDetail />} />
            </Routes>
          </MemoryRouter>
        </AuthContext.Provider>
      );
    };

    it("hides 'Delete window' buttons for anonymous visitors, students, and faculty", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      // 1. Anonymous visitor
      const { unmount: unmountAnon } = renderWithAuth(
        "/facilities/demo-badminton-court",
        null
      );
      await waitFor(() => {
        expect(
          screen.getByText("[Demo] Campus Badminton Court")
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByRole("button", { name: /delete .* schedule window/i })
      ).not.toBeInTheDocument();
      unmountAnon();

      // 2. Student user
      const { unmount: unmountStudent } = renderWithAuth(
        "/facilities/demo-badminton-court",
        studentUser
      );
      await waitFor(() => {
        expect(
          screen.getByText("[Demo] Campus Badminton Court")
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByRole("button", { name: /delete .* schedule window/i })
      ).not.toBeInTheDocument();
      unmountStudent();

      // 3. Faculty user
      renderWithAuth("/facilities/demo-badminton-court", facultyUser);
      await waitFor(() => {
        expect(
          screen.getByText("[Demo] Campus Badminton Court")
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByRole("button", { name: /delete .* schedule window/i })
      ).not.toBeInTheDocument();
    });

    it("displays 'Delete window' buttons alongside Edit for sports_director and super_admin", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      // 1. Sports Director
      const { unmount } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
        expect(
          screen.getByLabelText("Delete Friday schedule window")
        ).toBeInTheDocument();
      });
      unmount();

      // 2. Super Admin
      renderWithAuth("/facilities/demo-badminton-court", superAdminUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
        expect(
          screen.getByLabelText("Delete Friday schedule window")
        ).toBeInTheDocument();
      });
    });

    it("shows confirmation details with facility name, weekday, time, and audience", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      // Click delete window on sched-1 (Monday 09:00 - 12:00, student)
      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));

      // Confirmation dialog should be rendered as an inline region without modal semantics
      const confirmDialog = screen.getByTestId(
        "schedule-delete-confirm-sched-1"
      );
      expect(confirmDialog).toBeInTheDocument();
      expect(confirmDialog).toHaveAttribute("role", "region");
      expect(confirmDialog).toHaveAttribute(
        "aria-label",
        "Confirm deletion of Monday schedule window"
      );
      expect(confirmDialog).not.toHaveAttribute("aria-modal");

      // Verify Cancel button receives initial focus upon opening
      const cancelBtn = screen.getByRole("button", { name: /cancel deletion/i });
      expect(document.activeElement).toBe(cancelBtn);

      // Verify required details
      expect(confirmDialog).toHaveTextContent("Confirm Schedule Window Deletion");
      expect(confirmDialog).toHaveTextContent("[Demo] Campus Badminton Court");
      expect(confirmDialog).toHaveTextContent("Monday");
      expect(confirmDialog).toHaveTextContent(
        formatTimeRange("09:00:00", "12:00:00")
      );
      expect(confirmDialog).toHaveTextContent("Students");
      expect(confirmDialog).toHaveTextContent("This action cannot be undone");

      // Verify Cancel and Confirm buttons are rendered
      expect(
        screen.getByRole("button", { name: /cancel deletion/i })
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      ).toBeInTheDocument();
    });

    it("cancels deletion via Cancel button without calling delete API", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      expect(
        screen.getByTestId("schedule-delete-confirm-sched-1")
      ).toBeInTheDocument();

      // Focus should be on Cancel button
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: /cancel deletion/i })
      );

      // Click Cancel
      fireEvent.click(screen.getByRole("button", { name: /cancel deletion/i }));

      // Confirmation closed and row remains intact
      expect(
        screen.queryByTestId("schedule-delete-confirm-sched-1")
      ).not.toBeInTheDocument();
      expect(api.deleteSchedule).not.toHaveBeenCalled();
      expect(
        screen.getByText(formatTimeRange("09:00:00", "12:00:00"))
      ).toBeInTheDocument();

      // Focus must return to original Delete button
      await waitFor(() => {
        expect(document.activeElement).toBe(
          screen.getByLabelText("Delete Monday schedule window")
        );
      });
    });

    it("cancels deletion via Escape key without calling delete API and returns focus to Delete button", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      const confirmDialog = screen.getByTestId(
        "schedule-delete-confirm-sched-1"
      );

      // Focus should be on Cancel button
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: /cancel deletion/i })
      );

      // Press Escape
      fireEvent.keyDown(confirmDialog, { key: "Escape" });

      expect(
        screen.queryByTestId("schedule-delete-confirm-sched-1")
      ).not.toBeInTheDocument();
      expect(api.deleteSchedule).not.toHaveBeenCalled();
      expect(
        screen.getByText(formatTimeRange("09:00:00", "12:00:00"))
      ).toBeInTheDocument();

      // Focus must return to original Delete button
      await waitFor(() => {
        expect(document.activeElement).toBe(
          screen.getByLabelText("Delete Monday schedule window")
        );
      });
    });

    it("keeps only one edit or delete interaction active at a time", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Edit Monday schedule window")
        ).toBeInTheDocument();
      });

      // 1. Open edit form on row 1
      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));
      expect(
        screen.getByTestId("schedule-edit-form-sched-1")
      ).toBeInTheDocument();

      // 2. Click Delete on row 2 -> closes edit form on row 1 and opens delete confirmation on row 2
      fireEvent.click(screen.getByLabelText("Delete Friday schedule window"));
      expect(
        screen.queryByTestId("schedule-edit-form-sched-1")
      ).not.toBeInTheDocument();
      expect(
        screen.getByTestId("schedule-delete-confirm-sched-2")
      ).toBeInTheDocument();

      // 3. Click Delete on row 1 -> closes delete on row 2 and opens delete on row 1
      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      expect(
        screen.queryByTestId("schedule-delete-confirm-sched-2")
      ).not.toBeInTheDocument();
      expect(
        screen.getByTestId("schedule-delete-confirm-sched-1")
      ).toBeInTheDocument();

      // 4. Click Edit on row 2 -> closes delete on row 1 and opens edit on row 2
      fireEvent.click(screen.getByLabelText("Edit Friday schedule window"));
      expect(
        screen.queryByTestId("schedule-delete-confirm-sched-1")
      ).not.toBeInTheDocument();
      expect(
        screen.getByTestId("schedule-edit-form-sched-2")
      ).toBeInTheDocument();
    });

    it("sends exact DELETE request and removes only the selected row on empty 204 success", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      vi.mocked(api.deleteSchedule).mockResolvedValue(undefined);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      );

      // Verify exact call: facility slug and schedule UUID
      expect(api.deleteSchedule).toHaveBeenCalledTimes(1);
      expect(api.deleteSchedule).toHaveBeenCalledWith(
        "demo-badminton-court",
        "sched-1"
      );

      // Wait for success update
      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-success-banner")
        ).toHaveTextContent("Schedule window deleted successfully.");
      });

      // Sched-1 is gone, Sched-2 remains
      expect(
        screen.queryByText(formatTimeRange("09:00:00", "12:00:00"))
      ).not.toBeInTheDocument();
      expect(
        screen.getByText(formatTimeRange("22:00:00", "24:00:00"))
      ).toBeInTheDocument();
    });

    it("moves focus to the timetable heading when the last schedule window is deleted", async () => {
      const singleScheduleFacility: Facility = {
        ...completeFacility,
        schedules: [completeFacility.schedules[0]],
      };
      vi.mocked(api.getFacility).mockResolvedValue(singleScheduleFacility);
      vi.mocked(api.deleteSchedule).mockResolvedValue(undefined);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      );

      await waitFor(() => {
        expect(
          screen.getByText(/no recurring operating schedules/i)
        ).toBeInTheDocument();
      });

      // Heading should receive focus
      await waitFor(() => {
        expect(document.activeElement).toBe(
          screen.getByRole("heading", { name: /weekly operating schedule/i })
        );
      });
    });

    it("prevents duplicate submissions by disabling buttons and displaying spinner while deletion is pending", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      let resolveDelete: () => void = () => {};
      const pendingPromise = new Promise<void>((resolve) => {
        resolveDelete = resolve;
      });
      vi.mocked(api.deleteSchedule).mockReturnValue(pendingPromise);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      const confirmBtn = screen.getByRole("button", {
        name: /confirm delete schedule window/i,
      });
      const cancelBtn = screen.getByRole("button", {
        name: /cancel deletion/i,
      });

      fireEvent.click(confirmBtn);

      // Verify disabled state and spinner text
      expect(confirmBtn).toBeDisabled();
      expect(confirmBtn).toHaveTextContent("Deleting...");
      expect(cancelBtn).toBeDisabled();

      // Click again while pending - should not trigger a second API call
      fireEvent.click(confirmBtn);
      expect(api.deleteSchedule).toHaveBeenCalledTimes(1);

      // Complete the promise
      await act(async () => {
        resolveDelete();
      });

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-success-banner")
        ).toBeInTheDocument();
      });
    });

    it("handles 401, 403, 404, and 500 errors gracefully, leaving the row intact", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      // 1. 401 Unauthorized
      vi.mocked(api.deleteSchedule).mockRejectedValueOnce(
        new ApiError(401, "Session expired")
      );

      const { unmount } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      );

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-delete-error")
        ).toHaveTextContent("Your session has expired");
      });
      expect(
        screen.getByText(formatTimeRange("09:00:00", "12:00:00"))
      ).toBeInTheDocument();
      unmount();

      // 2. 403 Forbidden
      vi.mocked(api.deleteSchedule).mockRejectedValueOnce(
        new ApiError(403, "Forbidden")
      );

      const { unmount: unmount403 } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      );

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-delete-error")
        ).toHaveTextContent("You do not have permission to delete this schedule");
      });
      expect(
        screen.getByText(formatTimeRange("09:00:00", "12:00:00"))
      ).toBeInTheDocument();
      unmount403();

      // 3. 404 Not Found - offers refresh
      vi.mocked(api.deleteSchedule).mockRejectedValueOnce(
        new ApiError(404, "Schedule not found")
      );

      const { unmount: unmount404 } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      );

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-delete-error")
        ).toHaveTextContent("Schedule window or facility not found");
      });
      expect(
        screen.getByRole("button", { name: /refresh timetable/i })
      ).toBeInTheDocument();
      expect(
        screen.getByText(formatTimeRange("09:00:00", "12:00:00"))
      ).toBeInTheDocument();

      // Click refresh timetable
      fireEvent.click(
        screen.getByRole("button", { name: /refresh timetable/i })
      );
      expect(api.getFacility).toHaveBeenCalledWith(
        "demo-badminton-court",
        expect.anything()
      );
      unmount404();

      // 4. 500 Server Error
      vi.mocked(api.deleteSchedule).mockRejectedValueOnce(
        new ApiError(500, "Internal Server Error")
      );

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      );

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-delete-error")
        ).toHaveTextContent("Server error encountered while deleting schedule window");
      });
      expect(
        screen.getByRole("button", { name: /refresh timetable/i })
      ).toBeInTheDocument();
      expect(
        screen.getByText(formatTimeRange("09:00:00", "12:00:00"))
      ).toBeInTheDocument();
    });

    it("handles network failures without automatic retry and asks user to refresh before trying again", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      vi.mocked(api.deleteSchedule).mockRejectedValueOnce(
        new TypeError("Failed to fetch")
      );

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      );

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-delete-error")
        ).toHaveTextContent(
          "Network failure occurred. The deletion may have committed on the server. Please refresh this facility to verify before trying again."
        );
      });

      // No automatic retries
      expect(api.deleteSchedule).toHaveBeenCalledTimes(1);

      // Refresh button is available
      expect(
        screen.getByRole("button", { name: /refresh timetable/i })
      ).toBeInTheDocument();

      // Row is intact
      expect(
        screen.getByText(formatTimeRange("09:00:00", "12:00:00"))
      ).toBeInTheDocument();
    });

    it("safely handles unmount while delete is in flight without throwing state errors", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      let resolveDelete: () => void = () => {};
      const deletePromise = new Promise<void>((resolve) => {
        resolveDelete = resolve;
      });
      vi.mocked(api.deleteSchedule).mockReturnValue(deletePromise);

      const { unmount } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByLabelText("Delete Monday schedule window")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      fireEvent.click(
        screen.getByRole("button", { name: /confirm delete schedule window/i })
      );

      // Unmount while delete promise is in flight
      unmount();

      // Now resolve the promise - should not cause an uncaught error
      expect(() => {
        act(() => {
          resolveDelete();
        });
      }).not.toThrow();
    });
  });

  describe("Step 9C: Weekly Schedule Creation (Add Window)", () => {
    const directorUser: AuthUser = {
      id: "usr-dir",
      email: "director@example.test",
      displayName: "Sports Director",
      roles: ["sports_director"],
    };

    const superAdminUser: AuthUser = {
      id: "usr-admin",
      email: "admin@example.test",
      displayName: "Super Admin",
      roles: ["super_admin"],
    };

    const studentUser: AuthUser = {
      id: "usr-student",
      email: "student@example.test",
      displayName: "Student User",
      roles: ["student"],
    };

    const facultyUser: AuthUser = {
      id: "usr-faculty",
      email: "faculty@example.test",
      displayName: "Faculty User",
      roles: ["faculty"],
    };

    const emptyFacility: Facility = {
      id: "fac-empty",
      slug: "empty-court",
      name: "Empty Sports Court",
      description: "A court with no schedules yet",
      location: "Hall C",
      status: "active",
      schedules: [],
    };

    const createMockAuthContext = (user: AuthUser | null): AuthContextType => ({
      user,
      isLoading: false,
      checkError: null,
      logoutError: null,
      login: vi.fn(),
      logout: vi.fn(),
      retryCheckSession: vi.fn(),
      clearLogoutError: vi.fn(),
    });

    const renderWithAuth = (
      initialPath: string,
      user: AuthUser | null = null
    ) => {
      return render(
        <AuthContext.Provider value={createMockAuthContext(user)}>
          <MemoryRouter initialEntries={[initialPath]}>
            <Routes>
              <Route path="/facilities/:slug" element={<FacilityDetail />} />
            </Routes>
          </MemoryRouter>
        </AuthContext.Provider>
      );
    };

    beforeEach(() => {
      vi.clearAllMocks();
    });

    it("hides 'Add window' button for anonymous visitors, students, and faculty", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      // 1. Anonymous visitor
      const { unmount: unmountAnon } = renderWithAuth(
        "/facilities/demo-badminton-court",
        null
      );
      await waitFor(() => {
        expect(
          screen.getByText("[Demo] Campus Badminton Court")
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByTestId("add-schedule-window-btn")
      ).not.toBeInTheDocument();
      unmountAnon();

      // 2. Student user
      const { unmount: unmountStudent } = renderWithAuth(
        "/facilities/demo-badminton-court",
        studentUser
      );
      await waitFor(() => {
        expect(
          screen.getByText("[Demo] Campus Badminton Court")
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByTestId("add-schedule-window-btn")
      ).not.toBeInTheDocument();
      unmountStudent();

      // 3. Faculty user
      renderWithAuth("/facilities/demo-badminton-court", facultyUser);
      await waitFor(() => {
        expect(
          screen.getByText("[Demo] Campus Badminton Court")
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByTestId("add-schedule-window-btn")
      ).not.toBeInTheDocument();
    });

    it("displays 'Add window' button for sports_director and super_admin", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      // 1. Sports Director
      const { unmount } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });
      unmount();

      // 2. Super Admin
      renderWithAuth("/facilities/demo-badminton-court", superAdminUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });
    });

    it("displays 'Add first window' when timetable is empty for authorized users and hides for unauthorized", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(emptyFacility);

      // 1. Student should see empty message but no button
      const { unmount: unmountStudent } = renderWithAuth(
        "/facilities/empty-court",
        studentUser
      );
      await waitFor(() => {
        expect(
          screen.getByText(/No recurring operating schedules are currently registered/i)
        ).toBeInTheDocument();
      });
      expect(
        screen.queryByTestId("add-first-window-btn")
      ).not.toBeInTheDocument();
      expect(
        screen.queryByTestId("add-schedule-window-btn")
      ).not.toBeInTheDocument();
      unmountStudent();

      // 2. Director should see 'Add first window' button
      renderWithAuth("/facilities/empty-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-first-window-btn")
        ).toBeInTheDocument();
      });

      // Clicking 'Add first window' opens the form
      fireEvent.click(screen.getByTestId("add-first-window-btn"));
      expect(screen.getByTestId("schedule-add-form-card")).toBeInTheDocument();
    });

    it("clicking 'Cancel' dismisses the add form without calling API and restores focus", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      // Open add form
      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));
      expect(screen.getByTestId("schedule-add-form-card")).toBeInTheDocument();

      // Click cancel
      const cancelBtn = screen.getByRole("button", {
        name: /cancel adding schedule window/i,
      });
      fireEvent.click(cancelBtn);

      // Form is dismissed
      expect(
        screen.queryByTestId("schedule-add-form-card")
      ).not.toBeInTheDocument();
      expect(api.createSchedule).not.toHaveBeenCalled();
    });

    it("dispatches exact payload to api.createSchedule on form submission", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      vi.mocked(api.createSchedule).mockResolvedValue({
        id: "sched-new",
        dayOfWeek: 3,
        startTime: "14:00:00",
        endTime: "16:00:00",
        audience: "faculty",
      });

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      // Open add form
      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));

      // Fill in fields
      fireEvent.change(screen.getByLabelText("Day of week"), {
        target: { value: "3" }, // Wednesday
      });
      fireEvent.change(screen.getByLabelText("Start time"), {
        target: { value: "14:00:00" },
      });
      fireEvent.change(screen.getByLabelText("End time"), {
        target: { value: "16:00:00" },
      });
      fireEvent.change(screen.getByLabelText("Audience"), {
        target: { value: "faculty" },
      });

      // Submit form
      fireEvent.click(screen.getByTestId("submit-add-schedule-btn"));

      await waitFor(() => {
        expect(api.createSchedule).toHaveBeenCalledWith(
          "demo-badminton-court",
          {
            dayOfWeek: 3,
            startTime: "14:00:00",
            endTime: "16:00:00",
            audience: "faculty",
          }
        );
      });
    });

    it("inserts new schedule in correctly sorted order and displays announcement banner on success", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      // completeFacility has:
      // sched-1: Monday (1) 09:00:00 - 12:00:00
      // sched-2: Friday (5) 22:00:00 - 24:00:00
      // New: Wednesday (3) 14:00:00 - 16:00:00
      const createdSchedule = {
        id: "sched-3",
        dayOfWeek: 3,
        startTime: "14:00:00",
        endTime: "16:00:00",
        audience: "faculty",
      };
      vi.mocked(api.createSchedule).mockResolvedValue(createdSchedule);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));
      fireEvent.click(screen.getByTestId("submit-add-schedule-btn"));

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-success-banner")
        ).toHaveTextContent("Schedule window added successfully.");
      });

      // Verify form is closed
      expect(
        screen.queryByTestId("schedule-add-form-card")
      ).not.toBeInTheDocument();

      // Verify all 3 schedules are rendered in order: Monday, Wednesday, Friday
      const rows = screen.getAllByTestId(/^schedule-row-/);
      expect(rows).toHaveLength(3);
      expect(rows[0]).toHaveAttribute("data-testid", "schedule-row-sched-1");
      expect(rows[1]).toHaveAttribute("data-testid", "schedule-row-sched-3");
      expect(rows[2]).toHaveAttribute("data-testid", "schedule-row-sched-2");
    });

    it("supports 24:00:00 as end time via the 'Set 24:00:00' shortcut button", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      vi.mocked(api.createSchedule).mockResolvedValue({
        id: "sched-midnight",
        dayOfWeek: 6,
        startTime: "20:00:00",
        endTime: "24:00:00",
        audience: "general",
      });

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));

      fireEvent.change(screen.getByLabelText("Day of week"), {
        target: { value: "6" },
      });
      fireEvent.change(screen.getByLabelText("Start time"), {
        target: { value: "20:00:00" },
      });
      // Click shortcut button
      fireEvent.click(screen.getByTestId("add-set-midnight-btn"));

      // Verify input value changed to 24:00:00
      const endInput = screen.getByLabelText("End time") as HTMLInputElement;
      expect(endInput.value).toBe("24:00:00");

      fireEvent.click(screen.getByTestId("submit-add-schedule-btn"));

      await waitFor(() => {
        expect(api.createSchedule).toHaveBeenCalledWith(
          "demo-badminton-court",
          expect.objectContaining({
            endTime: "24:00:00",
          })
        );
      });
    });

    it("rejects client validation if start time is not strictly before end time", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));

      fireEvent.change(screen.getByLabelText("Start time"), {
        target: { value: "14:00:00" },
      });
      fireEvent.change(screen.getByLabelText("End time"), {
        target: { value: "14:00:00" },
      });

      fireEvent.click(screen.getByTestId("submit-add-schedule-btn"));

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-add-error")
        ).toHaveTextContent("must be strictly before end time");
      });
      expect(api.createSchedule).not.toHaveBeenCalled();
    });

    it("retains form draft and shows clear error message when server returns 409 duplicate", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);
      vi.mocked(api.createSchedule).mockRejectedValue(
        new ApiError(409, "duplicate window")
      );

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));

      fireEvent.change(screen.getByLabelText("Day of week"), {
        target: { value: "1" },
      });
      fireEvent.change(screen.getByLabelText("Start time"), {
        target: { value: "09:00:00" },
      });
      fireEvent.change(screen.getByLabelText("End time"), {
        target: { value: "12:00:00" },
      });
      fireEvent.change(screen.getByLabelText("Audience"), {
        target: { value: "student" },
      });

      fireEvent.click(screen.getByTestId("submit-add-schedule-btn"));

      await waitFor(() => {
        expect(
          screen.getByTestId("schedule-add-error")
        ).toHaveTextContent(
          "This schedule window duplicates an existing operating window for this facility."
        );
      });

      // Draft inputs should be preserved
      expect((screen.getByLabelText("Day of week") as HTMLSelectElement).value).toBe("1");
      expect((screen.getByLabelText("Start time") as HTMLInputElement).value).toBe("09:00:00");
      expect((screen.getByLabelText("End time") as HTMLInputElement).value).toBe("12:00:00");
      expect((screen.getByLabelText("Audience") as HTMLSelectElement).value).toBe("student");
    });

    it("enforces single-interaction exclusivity between add, edit, and delete", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      // 1. Open Add form
      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));
      expect(screen.getByTestId("schedule-add-form-card")).toBeInTheDocument();

      // 2. Click Edit on sched-1 -> Add form closes, Edit form opens
      fireEvent.click(screen.getByLabelText("Edit Monday schedule window"));
      expect(
        screen.queryByTestId("schedule-add-form-card")
      ).not.toBeInTheDocument();
      expect(
        screen.getByTestId("schedule-edit-form-sched-1")
      ).toBeInTheDocument();

      // 3. Click Add window again -> Edit form closes, Add form opens
      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));
      expect(screen.getByTestId("schedule-add-form-card")).toBeInTheDocument();
      expect(
        screen.queryByTestId("schedule-edit-form-sched-1")
      ).not.toBeInTheDocument();

      // 4. Click Delete on sched-1 -> Add form closes, Delete confirmation opens
      fireEvent.click(screen.getByLabelText("Delete Monday schedule window"));
      expect(
        screen.queryByTestId("schedule-add-form-card")
      ).not.toBeInTheDocument();
      expect(
        screen.getByTestId("schedule-delete-confirm-sched-1")
      ).toBeInTheDocument();
    });

    it("disables row edit and delete buttons while add save is pending", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      let resolveAdd: (val: any) => void = () => {};
      const addPromise = new Promise((resolve) => {
        resolveAdd = resolve;
      });
      vi.mocked(api.createSchedule).mockReturnValue(addPromise as any);

      renderWithAuth("/facilities/demo-badminton-court", directorUser);
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));
      fireEvent.click(screen.getByTestId("submit-add-schedule-btn"));

      // Row buttons should be disabled while isSavingAdd is true
      expect(screen.getByLabelText("Edit Monday schedule window")).toBeDisabled();
      expect(screen.getByLabelText("Delete Monday schedule window")).toBeDisabled();

      // Resolve and wait for completion
      await act(async () => {
        resolveAdd({
          id: "sched-resolved",
          dayOfWeek: 2,
          startTime: "10:00:00",
          endTime: "11:00:00",
          audience: "student",
        });
      });
      await waitFor(() => {
        expect(
          screen.queryByTestId("schedule-add-form-card")
        ).not.toBeInTheDocument();
      });
    });

    it("safely handles unmount while create is in flight without throwing state errors", async () => {
      vi.mocked(api.getFacility).mockResolvedValue(completeFacility);

      let resolveAdd: (val: any) => void = () => {};
      const addPromise = new Promise((resolve) => {
        resolveAdd = resolve;
      });
      vi.mocked(api.createSchedule).mockReturnValue(addPromise as any);

      const { unmount } = renderWithAuth(
        "/facilities/demo-badminton-court",
        directorUser
      );
      await waitFor(() => {
        expect(
          screen.getByTestId("add-schedule-window-btn")
        ).toBeInTheDocument();
      });

      fireEvent.click(screen.getByTestId("add-schedule-window-btn"));
      fireEvent.click(screen.getByTestId("submit-add-schedule-btn"));

      // Unmount while create promise is in flight
      unmount();

      // Now resolve the promise - should not cause an uncaught error
      expect(() => {
        act(() => {
          resolveAdd({
            id: "sched-new",
            dayOfWeek: 1,
            startTime: "09:00:00",
            endTime: "12:00:00",
            audience: "student",
          });
        });
      }).not.toThrow();
    });
  });
});
