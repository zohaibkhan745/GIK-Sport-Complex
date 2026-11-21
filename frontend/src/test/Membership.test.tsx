import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "../context/AuthContext";
import { Membership } from "../pages/Membership";
import { api, ApiError } from "../api";
import type { AuthUser, GymMembershipSummary, WalletMeResponse } from "../types";

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
      me: vi.fn(),
      logout: vi.fn(),
      getMyGymMembership: vi.fn(),
      purchaseGymMembership: vi.fn(),
      getMyWallet: vi.fn(),
      getGymMembershipPlan: vi.fn(),
      updateGymMembershipPlan: vi.fn(),
    },
  };
});

describe("Gym Membership Page Component (Phase 7 / Step 7C)", () => {
  const mockStudentUser: AuthUser = {
    id: "usr-student-42",
    email: "student@giki.test",
    displayName: "Muhammad Ali",
    roles: ["student"],
  };

  const mockSufficientWallet: WalletMeResponse = {
    wallet: {
      status: "ACTIVE",
      currency: "PKR",
      balancePaisa: "500000", // PKR 5,000.00
    },
    activity: [],
  };

  const mockLowWallet: WalletMeResponse = {
    wallet: {
      status: "ACTIVE",
      currency: "PKR",
      balancePaisa: "100000", // PKR 1,000.00 (< PKR 3,000.00)
    },
    activity: [],
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getGymMembershipPlan).mockResolvedValue({
      id: "plan-default",
      planCode: "STANDARD_30_DAY",
      name: "Standard Gym Membership",
      durationDays: 30,
      pricePaisa: 300000,
      isActive: true,
      description: "Olympic racks, specialized free weights, cardio machines, and functional rigs.",
    });
  });

  // 1. Signed-out access
  it("shows sign-in prompt for signed-out visitors", async () => {
    vi.mocked(api.me).mockRejectedValueOnce(new ApiError(401, "unauthorized"));

    render(
      <MemoryRouter initialEntries={["/membership"]}>
        <AuthProvider>
          <Routes>
            <Route path="/membership" element={<Membership />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Sign in to View or Purchase Membership/i)).toBeInTheDocument();
    });
    expect(screen.getAllByRole("link", { name: /Sign In/i }).length).toBeGreaterThanOrEqual(1);
  });

  // 2. Active pass state
  it("renders active pass badge and days remaining countdown when user has an active pass", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);

    const activeSummary: GymMembershipSummary = {
      isActive: true,
      currentMembership: {
        id: "mbr-active-1",
        userId: mockStudentUser.id,
        planType: "STANDARD_30_DAY",
        status: "ACTIVE",
        startTime: "2026-11-10T00:00:00Z",
        endTime: "2026-12-10T00:00:00Z",
        pricePaisa: 300000,
        debitTransactionId: "tx-1",
        idempotencyKey: "idem-1",
        createdAt: "2026-11-10T00:00:00Z",
      },
      effectiveExpiry: "2026-12-10T00:00:00Z",
      daysRemaining: 26,
      secondsRemaining: 26 * 86400,
      history: [],
    };

    vi.mocked(api.getMyGymMembership).mockResolvedValueOnce(activeSummary);
    vi.mocked(api.getMyWallet).mockResolvedValueOnce(mockSufficientWallet);

    render(
      <MemoryRouter initialEntries={["/membership"]}>
        <AuthProvider>
          <Routes>
            <Route path="/membership" element={<Membership />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("ACTIVE PASS")).toBeInTheDocument();
    });

    expect(screen.getByText("26 Days Left")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Renew \(\+30 Days\)/i })).toBeInTheDocument();
  });

  // 3. Inactive pass state & purchase modal with sufficient funds
  it("allows purchasing 30-day pass when wallet has sufficient funds", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);

    const inactiveSummary: GymMembershipSummary = {
      isActive: false,
      daysRemaining: 0,
      secondsRemaining: 0,
      history: [],
    };

    vi.mocked(api.getMyGymMembership).mockResolvedValueOnce(inactiveSummary);
    vi.mocked(api.getMyWallet).mockResolvedValueOnce(mockSufficientWallet);
    vi.mocked(api.purchaseGymMembership).mockResolvedValueOnce({
      id: "mbr-new-1",
      userId: mockStudentUser.id,
      planType: "STANDARD_30_DAY",
      status: "ACTIVE",
      startTime: "2026-11-14T10:00:00Z",
      endTime: "2026-12-14T10:00:00Z",
      pricePaisa: 300000,
      debitTransactionId: "tx-new",
      idempotencyKey: "idem-new",
      createdAt: "2026-11-14T10:00:00Z",
    });

    render(
      <MemoryRouter initialEntries={["/membership"]}>
        <AuthProvider>
          <Routes>
            <Route path="/membership" element={<Membership />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("NO ACTIVE PASS")).toBeInTheDocument();
    });

    // Click Purchase 30-Day Pass button
    const purchaseBtn = screen.getByRole("button", { name: /Purchase 30-Day Pass \(PKR 3,000\)/i });
    fireEvent.click(purchaseBtn);

    // Modal opens
    await waitFor(() => {
      expect(screen.getByText("Confirm Pass Purchase")).toBeInTheDocument();
    });

    expect(screen.getAllByText("PKR 5,000.00").length).toBeGreaterThanOrEqual(1);

    const confirmBtn = screen.getByRole("button", { name: /Confirm & Pay PKR 3,000/i });
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(api.purchaseGymMembership).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(screen.getByText(/30-Day Gym Membership activated successfully!/i)).toBeInTheDocument();
    });
  });

  // 4. Insufficient wallet balance shows alert and Top Up link
  it("displays insufficient funds alert in modal when wallet has less than PKR 3,000", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);

    const inactiveSummary: GymMembershipSummary = {
      isActive: false,
      daysRemaining: 0,
      secondsRemaining: 0,
      history: [],
    };

    vi.mocked(api.getMyGymMembership).mockResolvedValueOnce(inactiveSummary);
    vi.mocked(api.getMyWallet).mockResolvedValueOnce(mockLowWallet); // Only PKR 1,000.00

    render(
      <MemoryRouter initialEntries={["/membership"]}>
        <AuthProvider>
          <Routes>
            <Route path="/membership" element={<Membership />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("NO ACTIVE PASS")).toBeInTheDocument();
    });

    const purchaseBtn = screen.getByRole("button", { name: /Purchase 30-Day Pass \(PKR 3,000\)/i });
    fireEvent.click(purchaseBtn);

    await waitFor(() => {
      expect(screen.getByText("Insufficient Wallet Balance")).toBeInTheDocument();
    });

    expect(screen.getByRole("link", { name: /Top Up Wallet/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Confirm & Pay/i })).not.toBeInTheDocument();
  });

  // 5. Renders history table
  it("renders past membership passes in the history table", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);

    const summaryWithHistory: GymMembershipSummary = {
      isActive: false,
      daysRemaining: 0,
      secondsRemaining: 0,
      history: [
        {
          id: "mbr-past-1",
          userId: mockStudentUser.id,
          planType: "STANDARD_30_DAY",
          status: "EXPIRED",
          startTime: "2026-10-01T00:00:00Z",
          endTime: "2026-10-31T00:00:00Z",
          pricePaisa: 300000,
          debitTransactionId: "tx-old-1",
          idempotencyKey: "idem-old-1",
          createdAt: "2026-10-01T00:00:00Z",
        },
      ],
    };

    vi.mocked(api.getMyGymMembership).mockResolvedValueOnce(summaryWithHistory);
    vi.mocked(api.getMyWallet).mockResolvedValueOnce(mockSufficientWallet);

    render(
      <MemoryRouter initialEntries={["/membership"]}>
        <AuthProvider>
          <Routes>
            <Route path="/membership" element={<Membership />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("EXPIRED")).toBeInTheDocument();
    });

    expect(screen.getByText("30-Day Standard")).toBeInTheDocument();
  });

  // 6. Sports Director Plan Editing
  it("allows Sports Director to open edit modal and update pass fee and duration", async () => {
    const mockDirectorUser: AuthUser = {
      id: "usr-director-1",
      email: "director@giki.test",
      displayName: "Sports Director",
      roles: ["sports_director"],
    };

    vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
    vi.mocked(api.getMyGymMembership).mockResolvedValue({
      isActive: false,
      daysRemaining: 0,
      secondsRemaining: 0,
      history: [],
    });
    vi.mocked(api.getMyWallet).mockResolvedValue(mockSufficientWallet);
    vi.mocked(api.getGymMembershipPlan).mockResolvedValue({
      id: "plan-1",
      planCode: "STANDARD_30_DAY",
      name: "Standard Gym Membership",
      durationDays: 30,
      pricePaisa: 300000,
      isActive: true,
      description: "Olympic racks and specialized free weights.",
    });

    render(
      <MemoryRouter initialEntries={["/membership"]}>
        <AuthProvider>
          <Routes>
            <Route path="/membership" element={<Membership />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    // Verify edit button is visible for director
    const editBtn = await screen.findByTestId("edit-plan-btn");
    expect(editBtn).toBeInTheDocument();
    fireEvent.click(editBtn);

    // Verify modal opened
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Edit Gym Pass Settings")).toBeInTheDocument();

    const nameInput = screen.getByLabelText(/Pass Title \/ Name/i);
    const feeInput = screen.getByLabelText(/Membership Fee/i);
    const durationInput = screen.getByLabelText(/Pass Validity Duration/i);

    fireEvent.change(nameInput, { target: { value: "Premium Gym Pass" } });
    fireEvent.change(feeInput, { target: { value: "3500" } });
    fireEvent.change(durationInput, { target: { value: "45" } });

    vi.mocked(api.updateGymMembershipPlan).mockResolvedValueOnce({
      id: "plan-1",
      planCode: "STANDARD_30_DAY",
      name: "Premium Gym Pass",
      durationDays: 45,
      pricePaisa: 350000,
      isActive: true,
      description: "Olympic racks and specialized free weights.",
    });

    const saveBtn = screen.getByRole("button", { name: /Save Pass Settings/i });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(api.updateGymMembershipPlan).toHaveBeenCalledWith({
        name: "Premium Gym Pass",
        durationDays: 45,
        pricePaisa: 350000,
        description: "Olympic racks and specialized free weights.",
      });
    });

    // Check updated pricing displayed
    await waitFor(() => {
      expect(screen.getByText(/Gym pass settings updated/i)).toBeInTheDocument();
      expect(screen.getAllByText(/45 days/i).length).toBeGreaterThanOrEqual(1);
    });
  });

  // 7. Student cannot see edit button
  it("does not display edit button for regular students", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);
    vi.mocked(api.getMyGymMembership).mockResolvedValue({
      isActive: false,
      daysRemaining: 0,
      secondsRemaining: 0,
      history: [],
    });

    render(
      <MemoryRouter initialEntries={["/membership"]}>
        <AuthProvider>
          <Routes>
            <Route path="/membership" element={<Membership />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.queryByTestId("edit-plan-btn")).not.toBeInTheDocument();
    });
  });
});
