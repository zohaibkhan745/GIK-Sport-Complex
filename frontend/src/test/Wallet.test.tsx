import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "../context/AuthContext";
import { Navbar } from "../components/Navbar";
import { Wallet } from "../pages/Wallet";
import { api, ApiError } from "../api";
import type { AuthUser, WalletMeResponse } from "../types";

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
      getMyWallet: vi.fn(),
    },
  };
});

describe("My Sports Wallet Page Component (Step 11F)", () => {
  const mockStudentUser: AuthUser = {
    id: "usr-student-42",
    email: "student@giki.test",
    displayName: "Muhammad Ali",
    roles: ["student"],
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  // -------------------------------------------------------------------------
  // 1. Signed-out access
  // -------------------------------------------------------------------------
  it("shows sign-in link for signed-out visitors and does not fetch wallet data", async () => {
    // 401 indicates signed-out visitor
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(401, "unauthorized"));

    render(
      <MemoryRouter initialEntries={["/wallet"]}>
        <AuthProvider>
          <Routes>
            <Route path="/wallet" element={<Wallet />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    // Initial check in flight
    expect(screen.getByRole("status")).toHaveTextContent("Checking session...");

    // After session check completes
    await waitFor(() => {
      expect(screen.getByText("Sign In Required")).toBeInTheDocument();
      expect(
        screen.getByText(/Please sign in to view your Sports wallet balance/i)
      ).toBeInTheDocument();
      expect(screen.getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
    });

    // Verification: getMyWallet was NEVER called for anonymous visitor
    expect(api.getMyWallet).not.toHaveBeenCalled();
  });

  // -------------------------------------------------------------------------
  // 2. Session check failure vs. signed-out
  // -------------------------------------------------------------------------
  it("distinguishes session-check failure from being signed out and offers retry", async () => {
    // 500 or network failure during session check
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(500, "Database cluster unavailable"));

    render(
      <MemoryRouter initialEntries={["/wallet"]}>
        <AuthProvider>
          <Routes>
            <Route path="/wallet" element={<Wallet />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(screen.getByText("Session Verification Error")).toBeInTheDocument();
      expect(
        screen.getByText(/Unable to verify your session with the server: Database cluster unavailable/i)
      ).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Retry session check" })).toBeInTheDocument();
    });

    // Verification: getMyWallet was not called
    expect(api.getMyWallet).not.toHaveBeenCalled();
  });

  // -------------------------------------------------------------------------
  // 3. No Sports wallet yet (wallet: null)
  // -------------------------------------------------------------------------
  it("displays 'No Sports wallet yet.' with explanation and no transaction buttons", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);
    vi.mocked(api.getMyWallet).mockResolvedValueOnce({
      wallet: null,
      activity: [],
    });

    render(
      <MemoryRouter initialEntries={["/wallet"]}>
        <AuthProvider>
          <Routes>
            <Route path="/wallet" element={<Wallet />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("No Sports wallet yet.")).toBeInTheDocument();
      expect(
        screen.getByText(/A digital wallet has not been provisioned for your account yet/i)
      ).toBeInTheDocument();
      expect(
        screen.getByText("This page shows the Sports wallet balance.")
      ).toBeInTheDocument();
    });

    // Verify absence of any transaction action buttons
    expect(screen.queryByRole("button", { name: /activate/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /add funds/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /send/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /withdraw/i })).not.toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // 4. Existing zero-balance wallet
  // -------------------------------------------------------------------------
  it("displays 'PKR 0.00' and 'No recent transactions.' for a zero-balance wallet", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);
    vi.mocked(api.getMyWallet).mockResolvedValueOnce({
      wallet: {
        status: "ACTIVE",
        currency: "PKR",
        balancePaisa: "0",
      },
      activity: [],
    });

    render(
      <MemoryRouter initialEntries={["/wallet"]}>
        <AuthProvider>
          <Routes>
            <Route path="/wallet" element={<Wallet />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("PKR 0.00")).toBeInTheDocument();
      expect(screen.getByText("ACTIVE")).toBeInTheDocument();
      expect(screen.getByText("PKR")).toBeInTheDocument();
      expect(screen.getByText("No recent transactions.")).toBeInTheDocument();
      expect(
        screen.getByText(/This page shows the Sports wallet balance for booking padel courts/i)
      ).toBeInTheDocument();
    });

    // Verify absence of financial mutation buttons
    expect(screen.queryByRole("button", { name: /add funds/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /withdraw/i })).not.toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // 5. Credit and debit formatting with readable timestamps
  // -------------------------------------------------------------------------
  it("renders balance in PKR, formats debits and credits with signs, and shows readable timestamps", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);
    vi.mocked(api.getMyWallet).mockResolvedValueOnce({
      wallet: {
        status: "ACTIVE",
        currency: "PKR",
        balancePaisa: "65000", // 650.00 PKR
      },
      activity: [
        {
          entryId: "entry-1",
          transactionId: "tx-1",
          type: "PADEL_BOOKING",
          amountPaisa: "-35000", // -350.00 PKR debit
          createdAt: "2026-11-09T08:00:00Z",
        },
        {
          entryId: "entry-2",
          transactionId: "tx-2",
          type: "TOPUP",
          amountPaisa: "100000", // +1000.00 PKR credit
          createdAt: "2026-11-09T07:30:00Z",
        },
      ],
    });

    render(
      <MemoryRouter initialEntries={["/wallet"]}>
        <AuthProvider>
          <Routes>
            <Route path="/wallet" element={<Wallet />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      // Main balance
      expect(screen.getByText("PKR 650.00")).toBeInTheDocument();

      // Recent activity entries
      expect(screen.getByText("Padel Court Booking")).toBeInTheDocument();
      expect(screen.getByText("-PKR 350.00")).toBeInTheDocument();

      expect(screen.getByText("Wallet Top-Up")).toBeInTheDocument();
      expect(screen.getByText("+PKR 1,000.00")).toBeInTheDocument();

      // Count badge
      expect(screen.getByText("2 entries")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 6. Amount above JavaScript safe integer range (> 2^53 - 1)
  // -------------------------------------------------------------------------
  it("displays exact PKR for amounts exceeding JavaScript safe integer range without precision loss", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);

    // Number.MAX_SAFE_INTEGER is 9007199254740991. We pass 9007199254740992 (2^53 paisa)
    const largePaisa = "9007199254740992";

    vi.mocked(api.getMyWallet).mockResolvedValueOnce({
      wallet: {
        status: "ACTIVE",
        currency: "PKR",
        balancePaisa: largePaisa,
      },
      activity: [
        {
          entryId: "entry-large",
          transactionId: "tx-large",
          type: "TOPUP",
          amountPaisa: largePaisa,
          createdAt: "2026-11-09T08:00:00Z",
        },
      ],
    });

    render(
      <MemoryRouter initialEntries={["/wallet"]}>
        <AuthProvider>
          <Routes>
            <Route path="/wallet" element={<Wallet />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      // 9007199254740992 paisa = 90071992547409.92 PKR
      expect(screen.getByText("PKR 90,071,992,547,409.92")).toBeInTheDocument();
      expect(screen.getByText("+PKR 90,071,992,547,409.92")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 7. Retry on API failure
  // -------------------------------------------------------------------------
  it("shows error state with retry on failure and recovers when retried", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);
    // First call fails
    vi.mocked(api.getMyWallet).mockRejectedValueOnce(
      new ApiError(500, "Internal ledger service error")
    );
    // Second call succeeds
    vi.mocked(api.getMyWallet).mockResolvedValueOnce({
      wallet: {
        status: "ACTIVE",
        currency: "PKR",
        balancePaisa: "40000",
      },
      activity: [],
    });

    render(
      <MemoryRouter initialEntries={["/wallet"]}>
        <AuthProvider>
          <Routes>
            <Route path="/wallet" element={<Wallet />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    // Initial failure displayed
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(screen.getByText("Unable to load Sports wallet")).toBeInTheDocument();
      expect(screen.getByText("Internal ledger service error")).toBeInTheDocument();
    });

    // Click retry
    const retryBtn = screen.getByRole("button", { name: "Retry" });
    fireEvent.click(retryBtn);

    // Recovers successfully
    await waitFor(() => {
      expect(screen.getByText("PKR 400.00")).toBeInTheDocument();
    });

    expect(api.getMyWallet).toHaveBeenCalledTimes(2);
  });

  // -------------------------------------------------------------------------
  // 8. Late response after sign-out is discarded
  // -------------------------------------------------------------------------
  it("cancels obsolete requests and clears data on sign-out so late responses are ignored", async () => {
    let resolveWalletPromise: (val: WalletMeResponse) => void = () => {};
    const walletPromise = new Promise<WalletMeResponse>((resolve) => {
      resolveWalletPromise = resolve;
    });

    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);
    vi.mocked(api.getMyWallet).mockReturnValueOnce(walletPromise);
    vi.mocked(api.logout).mockResolvedValueOnce(undefined);

    render(
      <MemoryRouter initialEntries={["/wallet"]}>
        <AuthProvider>
          <Routes>
            <Route path="/wallet" element={<Wallet />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    // Wait for user to be authenticated in Navbar
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Sign Out" })).toBeInTheDocument();
    });

    // getMyWallet was called and is pending
    expect(api.getMyWallet).toHaveBeenCalledTimes(1);

    // User signs out via Navbar
    const signOutBtn = screen.getByRole("button", { name: "Sign Out" });
    fireEvent.click(signOutBtn);

    // Signed out state appears
    await waitFor(() => {
      expect(screen.getByText("Sign In Required")).toBeInTheDocument();
    });

    // Now the late wallet promise resolves
    resolveWalletPromise({
      wallet: {
        status: "ACTIVE",
        currency: "PKR",
        balancePaisa: "500000",
      },
      activity: [],
    });

    // Verify the late data is NEVER displayed; page remains in signed-out state
    await waitFor(() => {
      expect(screen.queryByText("PKR 5,000.00")).not.toBeInTheDocument();
      expect(screen.getByText("Sign In Required")).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 9. Navigation link in Navbar
  // -------------------------------------------------------------------------
  it("displays 'My wallet' navigation link in Navbar for signed-in users", async () => {
    vi.mocked(api.me).mockResolvedValueOnce(mockStudentUser);

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AuthProvider>
          <Navbar />
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      const walletLinks = screen.getAllByRole("link", { name: "My wallet" });
      expect(walletLinks.length).toBeGreaterThanOrEqual(1);
      expect(walletLinks[0]).toHaveAttribute("href", "/wallet");
    });
  });
});
