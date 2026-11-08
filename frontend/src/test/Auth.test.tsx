import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "../context/AuthContext";
import { Navbar } from "../components/Navbar";
import { Login } from "../pages/Login";
import { api, ApiError } from "../api";
import type { AuthUser } from "../types";

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
    },
  };
});

describe("Frontend Authentication Flow", () => {
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

  beforeEach(() => {
    vi.clearAllMocks();
  });

  // -------------------------------------------------------------------------
  // 1. Session Restore on Startup
  // -------------------------------------------------------------------------
  it("restores active session on app startup and updates navigation", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    // Initial check in flight
    expect(api.me).toHaveBeenCalledTimes(1);

    // After /me resolves, navigation displays user display name and sign out action
    await waitFor(() => {
      expect(screen.getByText("Sports Director (Dev)")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Sign Out" })).toBeInTheDocument();
      expect(screen.queryByRole("link", { name: "Sign In" })).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 2. Signed-Out Startup
  // -------------------------------------------------------------------------
  it("recognizes 401 from /me as signed out without showing error banner", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(
      new ApiError(401, "unauthorized: missing session cookie")
    );

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByRole("link", { name: "Sign In" })).toBeInTheDocument();
      expect(screen.queryByText("Sports Director (Dev)")).not.toBeInTheDocument();
      expect(screen.queryByText(/Session check failed/i)).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 3. Startup Failure and Retry
  // -------------------------------------------------------------------------
  it("shows retry button on non-401 startup failure and recovers when retried", async () => {
    // First call fails with 500 internal server error
    vi.mocked(api.me).mockRejectedValueOnce(
      new ApiError(500, "internal server error")
    );

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    // Session check failure should be reported without blocking public navigation
    await waitFor(() => {
      expect(screen.getAllByText(/Session check failed/i)[0]).toBeInTheDocument();
      expect(screen.getByRole("link", { name: "Facilities" })).toBeInTheDocument();
    });

    // When user clicks Retry, second call succeeds
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);
    const retryButtons = screen.getAllByRole("button", { name: "Retry" });
    fireEvent.click(retryButtons[0]);

    await waitFor(() => {
      expect(screen.getByText("Student (Dev)")).toBeInTheDocument();
      expect(screen.queryByText(/Session check failed/i)).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 4. Login Form Validation & Input Boundaries
  // -------------------------------------------------------------------------
  it("validates required fields and preserves password whitespace exactly", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(401, "unauthorized"));
    vi.mocked(api.login).mockResolvedValueOnce(mockDirectorUser);

    render(
      <MemoryRouter initialEntries={["/login"]}>
        <AuthProvider>
          <Login />
        </AuthProvider>
      </MemoryRouter>
    );

    const emailInput = screen.getByLabelText(/Email address/i);
    const passwordInput = screen.getByLabelText(/Password/i);
    const submitBtn = screen.getByRole("button", { name: "Sign In" });

    // 1. Submit empty form
    fireEvent.click(submitBtn);
    expect(screen.getByText("Email address is required")).toBeInTheDocument();
    expect(screen.getByText("Password is required")).toBeInTheDocument();
    expect(api.login).not.toHaveBeenCalled();

    // 2. Fill email and password with leading/trailing spaces
    // The password MUST be preserved with spaces intact (no trimming)
    const exactPassword = "  untrimmed sample test pass  ";
    fireEvent.change(emailInput, { target: { value: "  director@example.test  " } });
    fireEvent.change(passwordInput, { target: { value: exactPassword } });

    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(api.login).toHaveBeenCalledWith({
        email: "director@example.test", // trimmed email
        password: exactPassword,       // exact password preserved
      });
    });
  });

  // -------------------------------------------------------------------------
  // 5. Login Success Flow
  // -------------------------------------------------------------------------
  it("authenticates user and redirects to home page on login success", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(401, "unauthorized"));
    vi.mocked(api.login).mockResolvedValueOnce(mockDirectorUser);

    render(
      <MemoryRouter initialEntries={["/login"]}>
        <AuthProvider>
          <Navbar />
          <Routes>
            <Route path="/" element={<div>Home Page Content</div>} />
            <Route path="/login" element={<Login />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    fireEvent.change(screen.getByLabelText(/Email address/i), {
      target: { value: "director@example.test" },
    });
    fireEvent.change(screen.getByLabelText(/Password/i), {
      target: { value: "validPassword" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    await waitFor(() => {
      // Redirected to home page
      expect(screen.getByText("Home Page Content")).toBeInTheDocument();
      // Navigation shows authenticated user
      expect(screen.getByText("Sports Director (Dev)")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 6. Login Failure Error Mapping
  // -------------------------------------------------------------------------
  it("displays helpful error messages for 401, 429, and 503 errors", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(401, "unauthorized"));
    vi.mocked(api.login).mockRejectedValueOnce(
      new ApiError(401, "invalid email or password")
    );

    render(
      <MemoryRouter initialEntries={["/login"]}>
        <AuthProvider>
          <Login />
        </AuthProvider>
      </MemoryRouter>
    );

    const emailInput = screen.getByLabelText(/Email address/i);
    const passwordInput = screen.getByLabelText(/Password/i);
    const submitBtn = screen.getByRole("button", { name: "Sign In" });

    // 1. Invalid credentials (401)
    fireEvent.change(emailInput, { target: { value: "wrong@example.test" } });
    fireEvent.change(passwordInput, { target: { value: "wrongpass" } });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(
        screen.getByText("Invalid email or password. Please check your credentials.")
      ).toBeInTheDocument();
    });

    // 2. Throttled (429)
    vi.mocked(api.login).mockRejectedValueOnce(
      new ApiError(429, "too many login attempts")
    );
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(
        screen.getByText("Too many login attempts. Please wait a moment before trying again.")
      ).toBeInTheDocument();
    });

    // 3. Busy / unavailable (503)
    vi.mocked(api.login).mockRejectedValueOnce(
      new ApiError(503, "authentication service is busy")
    );
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(
        screen.getByText("Authentication service is busy or temporarily unavailable. Please retry shortly.")
      ).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 7. Duplicate Submission Prevention
  // -------------------------------------------------------------------------
  it("disables submit button and shows pending state while login request is in flight", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(401, "unauthorized"));
    let resolveLogin: (user: AuthUser) => void;
    vi.mocked(api.login).mockReturnValueOnce(
      new Promise<AuthUser>((resolve) => {
        resolveLogin = resolve;
      })
    );

    render(
      <MemoryRouter initialEntries={["/login"]}>
        <AuthProvider>
          <Login />
        </AuthProvider>
      </MemoryRouter>
    );

    fireEvent.change(screen.getByLabelText(/Email address/i), {
      target: { value: "director@example.test" },
    });
    fireEvent.change(screen.getByLabelText(/Password/i), {
      target: { value: "validPassword" },
    });

    const submitBtn = screen.getByRole("button", { name: "Sign In" });
    fireEvent.click(submitBtn);

    // In flight: button shows "Signing in..." and is disabled
    expect(screen.getByRole("button", { name: "Signing in..." })).toBeDisabled();

    // Clicking again while disabled should not trigger additional api.login calls
    fireEvent.click(submitBtn);
    expect(api.login).toHaveBeenCalledTimes(1);

    // Complete login
    resolveLogin!(mockDirectorUser);
    await waitFor(() => {
      expect(api.login).toHaveBeenCalledTimes(1);
    });
  });

  // -------------------------------------------------------------------------
  // 8. Logout Success
  // -------------------------------------------------------------------------
  it("clears user state and shows Sign In button on successful logout", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    vi.mocked(api.logout).mockResolvedValueOnce(undefined);

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Sports Director (Dev)")).toBeInTheDocument();
    });

    const signOutBtn = screen.getByRole("button", { name: "Sign Out" });
    fireEvent.click(signOutBtn);

    await waitFor(() => {
      expect(api.logout).toHaveBeenCalledTimes(1);
      expect(screen.getByRole("link", { name: "Sign In" })).toBeInTheDocument();
      expect(screen.queryByText("Sports Director (Dev)")).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 9. Logout Failure and Retry
  // -------------------------------------------------------------------------
  it("preserves user session and displays retry when logout call fails", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockDirectorUser);
    // Logout fails with 500
    vi.mocked(api.logout).mockRejectedValueOnce(
      new ApiError(500, "session revocation failed")
    );

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Sports Director (Dev)")).toBeInTheDocument();
    });

    const signOutBtn = screen.getByRole("button", { name: "Sign Out" });
    fireEvent.click(signOutBtn);

    await waitFor(() => {
      // Must NOT claim session was revoked; user remains in memory
      expect(screen.getByText("Sports Director (Dev)")).toBeInTheDocument();
      expect(screen.getByText(/Sign out failed: session revocation failed/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Retry Sign Out" })).toBeInTheDocument();
    });

    // When user retries and logout succeeds
    vi.mocked(api.logout).mockResolvedValueOnce(undefined);
    fireEvent.click(screen.getByRole("button", { name: "Retry Sign Out" }));

    await waitFor(() => {
      expect(screen.getByRole("link", { name: "Sign In" })).toBeInTheDocument();
      expect(screen.queryByText("Sports Director (Dev)")).not.toBeInTheDocument();
      expect(screen.queryByText(/Sign out failed/i)).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 10. Stale /me Response Guard
  // -------------------------------------------------------------------------
  it("discards a late /me response to prevent overwriting a newer login", async () => {
    let resolveMe: (user: AuthUser) => void;
    // Initial /me is slow
    vi.mocked(api.me).mockReturnValueOnce(
      new Promise<AuthUser>((resolve) => {
        resolveMe = resolve;
      })
    );
    // User signs in as Director in the meantime
    vi.mocked(api.login).mockResolvedValueOnce(mockDirectorUser);

    render(
      <MemoryRouter initialEntries={["/login"]}>
        <AuthProvider>
          <Login />
        </AuthProvider>
      </MemoryRouter>
    );

    // Perform login while /me is still pending
    fireEvent.change(screen.getByLabelText(/Email address/i), {
      target: { value: "director@example.test" },
    });
    fireEvent.change(screen.getByLabelText(/Password/i), {
      target: { value: "validPassword" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    await waitFor(() => {
      expect(screen.getAllByText("Sports Director (Dev)").length).toBeGreaterThan(0);
    });

    // The slow /me now resolves with an older Student profile
    resolveMe!(mockStudentUser);

    // The newer Director state MUST be preserved!
    await waitFor(() => {
      expect(screen.getAllByText("Sports Director (Dev)").length).toBeGreaterThan(0);
      expect(screen.queryByText("Student (Dev)")).not.toBeInTheDocument();
    });
  });
});
