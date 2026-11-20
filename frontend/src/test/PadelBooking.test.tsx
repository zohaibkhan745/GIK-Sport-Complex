import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "../context/AuthContext";
import { PadelBooking } from "../pages/PadelBooking";
import { api } from "../api";
import type {
  AuthUser,
  PadelFacility,
  PadelSlot,
  PadelSlotsResponse,
  PadelCheckoutResponse,
  PadelVerifyTicketResponse,
  PadelCancelResponse,
} from "../types";

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
      getFacilities: vi.fn().mockResolvedValue([]),
      getPadelFacilities: vi.fn(),
      getPadelSlots: vi.fn(),
      holdPadelSlot: vi.fn(),
      releasePadelHold: vi.fn(),
      checkoutPadelSlot: vi.fn(),
      verifyPadelTicket: vi.fn(),
      cancelPadelBooking: vi.fn(),
      generatePadelSlots: vi.fn(),
      getMyWallet: vi.fn(),
    },
  };
});

describe("Padel Court Booking Client (Step 12I)", () => {
  const mockStudentUser: AuthUser = {
    id: "usr-student-alice",
    email: "alice@giki.test",
    displayName: "Alice Student",
    roles: ["student"],
  };

  const mockDirectorUser: AuthUser = {
    id: "usr-director-bob",
    email: "director@giki.test",
    displayName: "Bob Director",
    roles: ["sports_director"],
  };

  const mockFacilities: PadelFacility[] = [
    {
      id: "fac-padel-1",
      slug: "padel-court-1",
      name: "Glass Padel Court 1",
      status: "active",
    },
    {
      id: "fac-padel-2",
      slug: "padel-court-2",
      name: "Glass Padel Court 2",
      status: "active",
    },
  ];

  const mockSlots: PadelSlot[] = [
    {
      id: "slot-0800",
      facilityId: "fac-padel-1",
      startTime: "2026-11-14T08:00:00Z",
      endTime: "2026-11-14T09:00:00Z",
      status: "AVAILABLE",
      pricePaisa: "100000", // PKR 1,000.00
      isHoldExpired: false,
      holdExpiresAt: null,
      heldByCaller: false,
    },
    {
      id: "slot-0900",
      facilityId: "fac-padel-1",
      startTime: "2026-11-14T09:00:00Z",
      endTime: "2026-11-14T10:00:00Z",
      status: "BOOKED",
      pricePaisa: "100000",
      isHoldExpired: false,
      holdExpiresAt: null,
      heldByCaller: false,
    },
    {
      id: "slot-1000",
      facilityId: "fac-padel-1",
      startTime: "2026-11-14T10:00:00Z",
      endTime: "2026-11-14T11:00:00Z",
      status: "HELD",
      pricePaisa: "100000",
      isHoldExpired: false,
      holdExpiresAt: "2026-11-14T10:05:00Z",
      heldByCaller: false,
    },
  ];

  const mockSlotsResponse: PadelSlotsResponse = {
    date: "2026-11-14",
    facility: mockFacilities[0],
    slots: mockSlots,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getPadelFacilities).mockResolvedValue(mockFacilities);
    vi.mocked(api.getPadelSlots).mockResolvedValue(mockSlotsResponse);
  });

  // -------------------------------------------------------------------------
  // 1. Initial Render & Slot Matrix Inspection
  // -------------------------------------------------------------------------
  it("renders page header, facility selector, and daily slot matrix with availability badges", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);
    vi.mocked(api.getMyWallet).mockResolvedValue({
      wallet: { status: "ACTIVE", currency: "PKR", balancePaisa: "300000" },
      activity: [],
    });

    render(
      <MemoryRouter initialEntries={["/padel"]}>
        <AuthProvider>
          <Routes>
            <Route path="/padel" element={<PadelBooking />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    // Header & title
    expect(await screen.findByText("Padel Court Booking")).toBeInTheDocument();
    expect(screen.getByText("Padel Court Reservations")).toBeInTheDocument();

    // Quick wallet balance in hero pill (PKR 3,000.00)
    expect(await screen.findByText("PKR 3,000.00")).toBeInTheDocument();

    // Slot matrix items
    expect(await screen.findByText("Available Time Slots")).toBeInTheDocument();
    expect(screen.getByText("Available")).toBeInTheDocument();
    expect(screen.getByText("Booked")).toBeInTheDocument();
    expect(screen.getByText("Held")).toBeInTheDocument();

    // Available slot has 'Hold Slot (5 min)' action button
    expect(screen.getByRole("button", { name: "Hold Slot (5 min)" })).toBeInTheDocument();
  });

  it("strictly filters facility list so only padel facilities have booking options", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);
    vi.mocked(api.getPadelFacilities).mockResolvedValue([
      { id: "fac-padel-1", slug: "glass-padel-courts", name: "Padel Court", status: "active" },
      { id: "fac-badminton", slug: "badminton-hall", name: "Badminton Hall", status: "active" },
      { id: "fac-gym", slug: "central-fitness-gym", name: "Gym", status: "active" },
      { id: "fac-pool", slug: "swimming-pool", name: "Swimming Pool", status: "active" },
    ]);

    render(
      <MemoryRouter initialEntries={["/padel"]}>
        <AuthProvider>
          <Routes>
            <Route path="/padel" element={<PadelBooking />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    // Only Padel Court should be rendered, not Badminton, Gym, or Swimming Pool
    expect(await screen.findByText("Padel Court")).toBeInTheDocument();
    expect(screen.queryByText("Badminton Hall")).not.toBeInTheDocument();
    expect(screen.queryByText("Gym")).not.toBeInTheDocument();
    expect(screen.queryByText("Swimming Pool")).not.toBeInTheDocument();
    // Since only 1 padel court is in list, it renders the clean indicator with Bookable badge
    expect(screen.getByText("Bookable")).toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // 2. Active 5-Minute Hold & Release Flow (FR-03, QR-08)
  // -------------------------------------------------------------------------
  it("allows holding an available slot, displaying the 5-minute countdown banner and releasing hold", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);
    vi.mocked(api.getMyWallet).mockResolvedValue({
      wallet: { status: "ACTIVE", currency: "PKR", balancePaisa: "300000" },
      activity: [],
    });

    // Mock successful hold response
    const holdExpiry = new Date(Date.now() + 300 * 1000).toISOString();
    vi.mocked(api.holdPadelSlot).mockResolvedValueOnce({
      id: "slot-0800",
      status: "HELD",
      holdExpiresAt: holdExpiry,
      heldByCaller: true,
    });

    render(
      <MemoryRouter initialEntries={["/padel"]}>
        <AuthProvider>
          <Routes>
            <Route path="/padel" element={<PadelBooking />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    const holdBtn = await screen.findByRole("button", { name: "Hold Slot (5 min)" });
    fireEvent.click(holdBtn);

    // Verify API called with slot ID
    await waitFor(() => {
      expect(api.holdPadelSlot).toHaveBeenCalledWith("slot-0800");
    });

    // Active hold countdown banner appears (FR-03)
    expect(
      await screen.findByText(/Slot Reserved For You \(5-Minute Temporary Hold\)/i)
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Proceed to Checkout" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Release Hold" })).toBeInTheDocument();

    // Release hold
    vi.mocked(api.releasePadelHold).mockResolvedValueOnce({
      id: "slot-0800",
      status: "AVAILABLE",
      holdExpiresAt: null,
      heldByCaller: false,
    });

    fireEvent.click(screen.getByRole("button", { name: "Release Hold" }));

    await waitFor(() => {
      expect(api.releasePadelHold).toHaveBeenCalledWith("slot-0800");
    });
  });

  // -------------------------------------------------------------------------
  // 3. Checkout Modal & Insufficient Funds Warning (QR-03, QR-09)
  // -------------------------------------------------------------------------
  it("opens checkout modal, warns when wallet funds are insufficient, and disables Confirm & Pay", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);
    // Student only has PKR 500.00 (50,000 paisa), but slot fee is PKR 1,000.00 (100,000 paisa)
    vi.mocked(api.getMyWallet).mockResolvedValue({
      wallet: { status: "ACTIVE", currency: "PKR", balancePaisa: "50000" },
      activity: [],
    });

    // Slot already held by caller
    const heldSlotsResponse: PadelSlotsResponse = {
      date: "2026-11-14",
      facility: mockFacilities[0],
      slots: [
        {
          ...mockSlots[0],
          status: "HELD",
          heldByCaller: true,
          holdExpiresAt: new Date(Date.now() + 250 * 1000).toISOString(),
        },
      ],
    };
    vi.mocked(api.getPadelSlots).mockResolvedValue(heldSlotsResponse);

    render(
      <MemoryRouter initialEntries={["/padel"]}>
        <AuthProvider>
          <Routes>
            <Route path="/padel" element={<PadelBooking />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    // Click Proceed to Checkout in banner
    const checkoutBtn = await screen.findByRole("button", { name: "Proceed to Checkout" });
    fireEvent.click(checkoutBtn);

    // Modal opens (accessible dialog)
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Confirm Padel Reservation")).toBeInTheDocument();

    // Breakdown details
    expect(screen.getByText("Reservation Fee:")).toBeInTheDocument();
    expect(screen.getByText("Your Current Wallet Balance:")).toBeInTheDocument();
    expect(screen.getAllByText("PKR 500.00").length).toBeGreaterThanOrEqual(1);

    // Insufficient funds alert
    expect(screen.getByText(/Insufficient funds in your Sports Wallet/i)).toBeInTheDocument();

    // Confirm & Pay button is disabled
    const confirmBtn = screen.getByRole("button", { name: "Confirm & Pay" });
    expect(confirmBtn).toBeDisabled();

    // Dismiss modal with Escape key (QR-09: zero keyboard traps)
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape", code: "Escape" });
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 4. Successful Atomic Checkout & Digital Ticket Confirmation (FR-03, TC-FR-03-03)
  // -------------------------------------------------------------------------
  it("executes atomic checkout with sufficient funds and displays 4-digit digital check-in ticket", async () => {
    vi.mocked(api.me).mockResolvedValue(mockStudentUser);
    vi.mocked(api.getMyWallet).mockResolvedValue({
      wallet: { status: "ACTIVE", currency: "PKR", balancePaisa: "300000" },
      activity: [],
    });

    const heldSlotsResponse: PadelSlotsResponse = {
      date: "2026-11-14",
      facility: mockFacilities[0],
      slots: [
        {
          ...mockSlots[0],
          status: "HELD",
          heldByCaller: true,
          holdExpiresAt: new Date(Date.now() + 250 * 1000).toISOString(),
        },
      ],
    };
    vi.mocked(api.getPadelSlots).mockResolvedValue(heldSlotsResponse);

    const mockConfirm: PadelCheckoutResponse = {
      bookingId: "b0000000-0000-0000-0000-000000000999",
      slotId: "slot-0800",
      facilityId: "fac-padel-1",
      facilityName: "Glass Padel Court 1",
      startTime: "2026-11-14T08:00:00Z",
      endTime: "2026-11-14T09:00:00Z",
      status: "CONFIRMED",
      feePaisa: "100000",
      checkInCode: "7419",
      debitTransactionId: "tx-debit-123",
      createdAt: new Date().toISOString(),
      alreadyBooked: false,
    };
    vi.mocked(api.checkoutPadelSlot).mockResolvedValueOnce(mockConfirm);

    render(
      <MemoryRouter initialEntries={["/padel"]}>
        <AuthProvider>
          <Routes>
            <Route path="/padel" element={<PadelBooking />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    fireEvent.click(await screen.findByRole("button", { name: "Proceed to Checkout" }));

    // Sufficient balance alert
    expect(
      await screen.findByText(/Sufficient funds\. Payment debits directly from your Sports Wallet\./i)
    ).toBeInTheDocument();

    const confirmBtn = screen.getByRole("button", { name: "Confirm & Pay" });
    expect(confirmBtn).not.toBeDisabled();
    fireEvent.click(confirmBtn);

    // Digital ticket confirmation appears
    expect(await screen.findByText("Booking Confirmed!")).toBeInTheDocument();
    expect(screen.getByText("Court Entrance Check-In Code")).toBeInTheDocument();

    // 4-digit code rendered in individual boxes
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText("9")).toBeInTheDocument();

    expect(screen.getByRole("button", { name: "Done" })).toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // 5. Attendant Ticket Verification Tool (FR-03, TC-FR-03-03)
  // -------------------------------------------------------------------------
  it("renders staff ticket verification gate for Sports Director and verifies 4-digit ticket", async () => {
    vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
    vi.mocked(api.getMyWallet).mockResolvedValue({
      wallet: null,
      activity: [],
    });

    const mockVerifyRes: PadelVerifyTicketResponse = {
      bookingId: "b0000000-0000-0000-0000-000000000999",
      slotId: "slot-0800",
      facilityId: "fac-padel-1",
      facilityName: "Glass Padel Court 1",
      userId: "usr-student-alice",
      userName: "Alice Student",
      userEmail: "alice@giki.test",
      startTime: "2026-11-14T08:00:00Z",
      endTime: "2026-11-14T09:00:00Z",
      status: "CHECKED_IN",
      feePaisa: "100000",
      checkInCode: "7419",
      checkedInAt: new Date().toISOString(),
      checkedInBy: mockDirectorUser.id,
    };
    vi.mocked(api.verifyPadelTicket).mockResolvedValueOnce(mockVerifyRes);

    render(
      <MemoryRouter initialEntries={["/padel"]}>
        <AuthProvider>
          <Routes>
            <Route path="/padel" element={<PadelBooking />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    // Director tabs visible
    const verifyTabBtn = await screen.findByRole("button", {
      name: "Attendant Ticket Check-In (FR-03)",
    });
    fireEvent.click(verifyTabBtn);

    expect(
      screen.getByText("Attendant Ticket Verification Gate (Entrance)")
    ).toBeInTheDocument();

    const input = screen.getByPlaceholderText(/4-digit code/i);
    fireEvent.change(input, { target: { value: "7419" } });

    const submitBtn = screen.getByRole("button", { name: "Verify Ticket" });
    expect(submitBtn).not.toBeDisabled();
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(api.verifyPadelTicket).toHaveBeenCalledWith("7419");
    });

    expect(
      await screen.findByText(/Ticket Verified Successfully — CHECKED_IN/i)
    ).toBeInTheDocument();
    expect(screen.getByText("Alice Student")).toBeInTheDocument();
  });

  // -------------------------------------------------------------------------
  // 6. Admin Booking Cancellation & 100% Refund Tool (FR-06, TC-FR-06-02)
  // -------------------------------------------------------------------------
  it("renders admin cancellation tab for Sports Director and executes 100% refund", async () => {
    vi.mocked(api.me).mockResolvedValue(mockDirectorUser);
    vi.mocked(api.getMyWallet).mockResolvedValue({
      wallet: null,
      activity: [],
    });

    const mockCancelRes: PadelCancelResponse = {
      bookingId: "b0000000-0000-0000-0000-000000000999",
      slotId: "slot-0800",
      userId: "usr-student-alice",
      status: "CANCELLED",
      feePaisa: "100000",
      refundTransactionId: "tx-refund-456",
      cancelledAt: new Date().toISOString(),
      cancelledBy: mockDirectorUser.id,
      reason: "Glass panel replacement",
      message: "Booking successfully cancelled and 100% refund credited to user wallet",
    };
    vi.mocked(api.cancelPadelBooking).mockResolvedValueOnce(mockCancelRes);

    render(
      <MemoryRouter initialEntries={["/padel"]}>
        <AuthProvider>
          <Routes>
            <Route path="/padel" element={<PadelBooking />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    );

    const cancelTabBtn = await screen.findByRole("button", {
      name: "Admin Cancellation & 100% Refund (FR-06)",
    });
    fireEvent.click(cancelTabBtn);

    expect(
      screen.getByText("Administrative Cancellation & 100% Single Refund")
    ).toBeInTheDocument();

    const uuidInput = screen.getByPlaceholderText(/e\.g\. c0000000/i);
    fireEvent.change(uuidInput, {
      target: { value: "b0000000-0000-0000-0000-000000000999" },
    });

    const reasonInput = screen.getByPlaceholderText(/Heavy rain/i);
    fireEvent.change(reasonInput, {
      target: { value: "Glass panel replacement" },
    });

    const cancelSubmitBtn = screen.getByRole("button", {
      name: "Cancel Booking & Issue 100% Refund",
    });
    fireEvent.click(cancelSubmitBtn);

    await waitFor(() => {
      expect(api.cancelPadelBooking).toHaveBeenCalledWith(
        "b0000000-0000-0000-0000-000000000999",
        "Glass panel replacement"
      );
    });

    expect(
      await screen.findByText("Booking Cancelled & 100% Refund Issued")
    ).toBeInTheDocument();
    expect(screen.getByText("tx-refund-456")).toBeInTheDocument();
  });
});
