import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import {
  ArrowLeft,
  Calendar,
  Clock,
  Ticket,
  CheckCircle2,
  AlertCircle,
  X,
  CreditCard,
  RefreshCw,
  ArrowRight,
  UserCheck,
  Ban,
  Check,
} from "lucide-react";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { api } from "../api";
import { useAuth } from "../context/AuthContext";
import {
  type PadelFacility,
  type PadelSlot,
  type PadelCheckoutResponse,
  type PadelVerifyTicketResponse,
  type PadelCancelResponse,
  formatPaisaToPKR,
} from "../types";

function getTodayString(): string {
  const d = new Date();
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function addDays(dateStr: string, days: number): string {
  const parts = dateStr.split("-").map(Number);
  const d = new Date(parts[0], parts[1] - 1, parts[2]);
  d.setDate(d.getDate() + days);
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function formatSlotTime(isoString: string): string {
  const d = new Date(isoString);
  if (isNaN(d.getTime())) return isoString;
  return d.toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: true,
  });
}

function formatSlotDate(isoDate: string): string {
  const parts = isoDate.split("-").map(Number);
  const d = new Date(parts[0], parts[1] - 1, parts[2]);
  return d.toLocaleDateString("en-US", {
    weekday: "long",
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function PadelBooking() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const [searchParams] = useSearchParams();

  const handleGoBack = useCallback(() => {
    if (window.history.state && typeof window.history.state.idx === "number" && window.history.state.idx > 0) {
      navigate(-1);
    } else if (window.history.length > 1) {
      navigate(-1);
    } else {
      navigate("/#facilities");
    }
  }, [navigate]);

  // Facilities & selection
  const [facilities, setFacilities] = useState<PadelFacility[]>([]);
  const [selectedFacilityId, setSelectedFacilityId] = useState<string>("");
  const [selectedDate, setSelectedDate] = useState<string>(getTodayString());

  // Inventory & state
  const [slots, setSlots] = useState<PadelSlot[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  // Active Hold tracking
  const [heldSlot, setHeldSlot] = useState<PadelSlot | null>(null);
  const [remainingHoldSeconds, setRemainingHoldSeconds] = useState<number>(0);
  const [isHolding, setIsHolding] = useState<boolean>(false);
  const [isReleasing, setIsReleasing] = useState<boolean>(false);
  const [actionError, setActionError] = useState<string | null>(null);

  // User Wallet
  const [userWalletBalancePaisa, setUserWalletBalancePaisa] = useState<string | null>(null);
  const [isFetchingWallet, setIsFetchingWallet] = useState<boolean>(false);

  // Checkout modal
  const [checkoutSlot, setCheckoutSlot] = useState<PadelSlot | null>(null);
  const [isCheckingOut, setIsCheckingOut] = useState<boolean>(false);
  const [checkoutError, setCheckoutError] = useState<string | null>(null);

  // Confirmed ticket modal
  const [confirmedTicket, setConfirmedTicket] = useState<PadelCheckoutResponse | null>(null);

  // Attendant / Admin tools
  const isStaffOrAdmin = Boolean(
    user && (user.roles.includes("sports_director") || user.roles.includes("super_admin"))
  );
  const [adminTab, setAdminTab] = useState<"book" | "verify" | "cancel" | "generate">("book");

  // Ticket verification state
  const [verifyCode, setVerifyCode] = useState<string>("");
  const [isVerifying, setIsVerifying] = useState<boolean>(false);
  const [verifiedTicket, setVerifiedTicket] = useState<PadelVerifyTicketResponse | null>(null);
  const [verifyError, setVerifyError] = useState<string | null>(null);

  // Admin cancellation state
  const [cancelBookingId, setCancelBookingId] = useState<string>("");
  const [cancelReason, setCancelReason] = useState<string>("");
  const [isCancelling, setIsCancelling] = useState<boolean>(false);
  const [cancelResult, setCancelResult] = useState<PadelCancelResponse | null>(null);
  const [cancelError, setCancelError] = useState<string | null>(null);

  // Slot generator state
  const [genDate, setGenDate] = useState<string>(getTodayString());
  const [genPrice, setGenPrice] = useState<string>("100000");
  const [isGenerating, setIsGenerating] = useState<boolean>(false);
  const [genResult, setGenResult] = useState<string | null>(null);
  const [genError, setGenError] = useState<string | null>(null);

  // Modal accessibility ref
  const checkoutCancelBtnRef = useRef<HTMLButtonElement | null>(null);
  const isMountedRef = useRef<boolean>(true);

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  // Fetch facilities once (strictly limited to Padel court facilities)
  useEffect(() => {
    api
      .getPadelFacilities()
      .then((data) => {
        if (!isMountedRef.current) return;
        const padelOnly = data.filter((f) => {
          const s = (f.slug || "").toLowerCase();
          const n = (f.name || "").toLowerCase();
          return s.includes("padel") || n.includes("padel");
        });
        setFacilities(padelOnly);
        const queryFac = searchParams.get("facility");
        if (queryFac && padelOnly.some((f) => f.id === queryFac || f.slug === queryFac)) {
          const match = padelOnly.find((f) => f.id === queryFac || f.slug === queryFac);
          if (match) setSelectedFacilityId(match.id);
        } else if (padelOnly.length > 0) {
          setSelectedFacilityId(padelOnly[0].id);
        }
      })
      .catch((err) => {
        if (!isMountedRef.current) return;
        setError(err instanceof Error ? err.message : "Failed to load facilities");
      });
  }, [searchParams]);

  // Fetch user wallet balance when logged in
  const fetchWallet = useCallback(() => {
    if (!user) {
      setUserWalletBalancePaisa(null);
      return;
    }
    setIsFetchingWallet(true);
    api
      .getMyWallet()
      .then((res) => {
        if (!isMountedRef.current) return;
        if (res.wallet) {
          setUserWalletBalancePaisa(res.wallet.balancePaisa);
        } else {
          setUserWalletBalancePaisa("0");
        }
      })
      .catch(() => {
        if (!isMountedRef.current) return;
        setUserWalletBalancePaisa(null);
      })
      .finally(() => {
        if (isMountedRef.current) setIsFetchingWallet(false);
      });
  }, [user]);

  useEffect(() => {
    fetchWallet();
  }, [fetchWallet]);

  // Fetch slots for selected date & facility
  const fetchSlots = useCallback(() => {
    if (!selectedFacilityId || !selectedDate) return;
    setIsLoading(true);
    setError(null);
    setActionError(null);

    api
      .getPadelSlots(selectedDate, selectedFacilityId)
      .then((res) => {
        if (!isMountedRef.current) return;
        setSlots(res.slots);

        // Check if caller currently holds any slot
        const callerHold = res.slots.find((s) => s.status === "HELD" && s.heldByCaller);
        if (callerHold) {
          setHeldSlot(callerHold);
          if (callerHold.holdExpiresAt) {
            const exp = new Date(callerHold.holdExpiresAt).getTime();
            const now = Date.now();
            const secs = Math.max(0, Math.floor((exp - now) / 1000));
            setRemainingHoldSeconds(secs);
          }
        } else {
          setHeldSlot(null);
          setRemainingHoldSeconds(0);
        }
      })
      .catch((err) => {
        if (!isMountedRef.current) return;
        setError(err instanceof Error ? err.message : "Failed to load slots");
      })
      .finally(() => {
        if (isMountedRef.current) setIsLoading(false);
      });
  }, [selectedFacilityId, selectedDate]);

  useEffect(() => {
    fetchSlots();
  }, [fetchSlots]);

  // Real-time 5-minute countdown ticker (QR-08)
  useEffect(() => {
    if (!heldSlot || remainingHoldSeconds <= 0) return;

    const timer = setInterval(() => {
      setRemainingHoldSeconds((prev) => {
        if (prev <= 1) {
          clearInterval(timer);
          // Hold expired
          setHeldSlot(null);
          fetchSlots();
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(timer);
  }, [heldSlot, remainingHoldSeconds, fetchSlots]);

  // Focus trap / keyboard handler for Checkout Modal (QR-09)
  useEffect(() => {
    if (checkoutSlot) {
      checkoutCancelBtnRef.current?.focus();
    }
  }, [checkoutSlot]);

  // Hold slot handler
  const handleHoldSlot = async (slot: PadelSlot) => {
    if (!user) {
      setActionError("Please sign in to hold and book a court slot.");
      return;
    }
    setIsHolding(true);
    setActionError(null);

    try {
      const res = await api.holdPadelSlot(slot.id);
      if (!isMountedRef.current) return;

      const updatedSlot: PadelSlot = {
        ...slot,
        status: "HELD",
        heldByCaller: true,
        holdExpiresAt: res.holdExpiresAt,
      };
      setHeldSlot(updatedSlot);
      if (res.holdExpiresAt) {
        const exp = new Date(res.holdExpiresAt).getTime();
        const secs = Math.max(0, Math.floor((exp - Date.now()) / 1000));
        setRemainingHoldSeconds(secs);
      }
      // Update slot in matrix
      setSlots((prev) => prev.map((s) => (s.id === slot.id ? updatedSlot : s)));
    } catch (err) {
      if (!isMountedRef.current) return;
      setActionError(err instanceof Error ? err.message : "Failed to hold slot");
      fetchSlots();
    } finally {
      if (isMountedRef.current) setIsHolding(false);
    }
  };

  // Release hold handler
  const handleReleaseHold = async () => {
    if (!heldSlot) return;
    setIsReleasing(true);
    setActionError(null);

    try {
      await api.releasePadelHold(heldSlot.id);
      if (!isMountedRef.current) return;
      setHeldSlot(null);
      setRemainingHoldSeconds(0);
      fetchSlots();
    } catch (err) {
      if (!isMountedRef.current) return;
      setActionError(err instanceof Error ? err.message : "Failed to release hold");
    } finally {
      if (isMountedRef.current) setIsReleasing(false);
    }
  };

  // Open Checkout Modal
  const openCheckout = (slot: PadelSlot) => {
    setCheckoutSlot(slot);
    setCheckoutError(null);
    fetchWallet();
  };

  // Close Checkout Modal
  const closeCheckout = () => {
    setCheckoutSlot(null);
    setCheckoutError(null);
  };

  // Confirm Checkout Handler
  const handleConfirmCheckout = async () => {
    if (!checkoutSlot || !user) return;
    setIsCheckingOut(true);
    setCheckoutError(null);

    const idempotencyKey = `padel-checkout-${checkoutSlot.id}-${Date.now()}`;

    try {
      const res = await api.checkoutPadelSlot({
        slotId: checkoutSlot.id,
        idempotencyKey,
      });

      if (!isMountedRef.current) return;
      setCheckoutSlot(null);
      setHeldSlot(null);
      setRemainingHoldSeconds(0);
      setConfirmedTicket(res);
      fetchWallet();
      fetchSlots();
    } catch (err) {
      if (!isMountedRef.current) return;
      setCheckoutError(err instanceof Error ? err.message : "Checkout failed");
    } finally {
      if (isMountedRef.current) setIsCheckingOut(false);
    }
  };

  // Attendant ticket verification handler
  const handleVerifyTicket = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!verifyCode.trim()) return;

    setIsVerifying(true);
    setVerifyError(null);
    setVerifiedTicket(null);

    try {
      const res = await api.verifyPadelTicket(verifyCode.trim());
      if (!isMountedRef.current) return;
      setVerifiedTicket(res);
    } catch (err) {
      if (!isMountedRef.current) return;
      setVerifyError(err instanceof Error ? err.message : "Ticket verification failed");
    } finally {
      if (isMountedRef.current) setIsVerifying(false);
    }
  };

  // Admin cancellation handler
  const handleAdminCancel = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!cancelBookingId.trim()) return;

    setIsCancelling(true);
    setCancelError(null);
    setCancelResult(null);

    try {
      const res = await api.cancelPadelBooking(cancelBookingId.trim(), cancelReason.trim());
      if (!isMountedRef.current) return;
      setCancelResult(res);
      fetchSlots();
    } catch (err) {
      if (!isMountedRef.current) return;
      setCancelError(err instanceof Error ? err.message : "Cancellation failed");
    } finally {
      if (isMountedRef.current) setIsCancelling(false);
    }
  };

  // Admin slot generation handler
  const handleGenerateSlots = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedFacilityId || !genDate.trim()) return;

    setIsGenerating(true);
    setGenError(null);
    setGenResult(null);

    try {
      await api.generatePadelSlots({
        facility_id: selectedFacilityId,
        date: genDate.trim(),
        start_hour: 8,
        end_hour: 23,
        slot_duration_minutes: 60,
        price_paisa: genPrice.trim() || "100000",
      });
      if (!isMountedRef.current) return;
      setGenResult(`Successfully generated slots for ${genDate}.`);
      setSelectedDate(genDate);
      fetchSlots();
    } catch (err) {
      if (!isMountedRef.current) return;
      setGenError(err instanceof Error ? err.message : "Failed to generate slots");
    } finally {
      if (isMountedRef.current) setIsGenerating(false);
    }
  };

  const selectedFacility = facilities.find((f) => f.id === selectedFacilityId);

  // Check funds calculation
  const slotPricePaisa = checkoutSlot ? BigInt(checkoutSlot.pricePaisa) : 0n;
  const userBalancePaisa = userWalletBalancePaisa ? BigInt(userWalletBalancePaisa) : 0n;
  const hasSufficientFunds = userBalancePaisa >= slotPricePaisa;

  const quickDates = [
    { label: "Today", date: getTodayString() },
    { label: "Tomorrow", date: addDays(getTodayString(), 1) },
    { label: "In 2 Days", date: addDays(getTodayString(), 2) },
    { label: "In 3 Days", date: addDays(getTodayString(), 3) },
  ];

  return (
    <div className="min-h-screen bg-white flex flex-col justify-between">
      <Navbar />

      <main className="mx-auto w-full max-w-6xl px-4 py-8 sm:px-6">
        {/* Back & Breadcrumb Navigation */}
        <div className="mb-6 flex items-center justify-between">
          <button
            type="button"
            onClick={handleGoBack}
            className="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--color-maroon)] transition-opacity hover:opacity-80 cursor-pointer"
          >
            <ArrowLeft size={14} />
            Go back
          </button>
          <div className="flex items-center gap-2 text-xs text-[var(--color-navy-soft)]">
            <Link to="/" className="hover:text-[var(--color-maroon)]">
              Home
            </Link>
            <span>/</span>
            <span className="font-semibold text-[var(--color-navy)]">Padel Court Reservations</span>
          </div>
        </div>

        {/* Hero Header */}
        <div className="mb-8 rounded-2xl bg-gradient-to-br from-[var(--color-navy)] to-[#152a48] p-6 sm:p-8 text-white shadow-sm">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-6">
            <div>
              <div className="inline-flex items-center gap-2 rounded-full bg-white/10 px-3 py-1 text-xs font-medium text-[var(--color-gold)] backdrop-blur">
                <Ticket size={14} />
                Official Sports Complex Reservations
              </div>
              <h1 className="mt-3 font-display text-2xl font-bold tracking-tight sm:text-3xl text-white">
                Padel Court Booking
              </h1>
              <p className="mt-2 max-w-xl text-sm text-gray-300">
                Reserve championship-grade glass padel courts with guaranteed 5-minute temporary holds,
                instant wallet ledger checkout, and entrance ticket verification.
              </p>
            </div>

            {/* Quick Wallet Summary Pill */}
            <div className="flex flex-col items-start md:items-end gap-2 bg-white/5 p-4 rounded-xl border border-white/10">
              <span className="text-xs uppercase tracking-wider text-gray-400">Your Wallet Balance</span>
              <div className="flex items-center gap-2">
                <CreditCard size={18} className="text-[var(--color-gold)]" />
                <span className="font-mono text-lg font-bold text-white">
                  {user ? (
                    userWalletBalancePaisa !== null ? (
                      formatPaisaToPKR(userWalletBalancePaisa)
                    ) : (
                      "Loading..."
                    )
                  ) : (
                    <Link to="/login" className="text-xs underline text-[var(--color-gold)]">
                      Sign in to view balance
                    </Link>
                  )}
                </span>
              </div>
              {user && (
                <Link
                  to="/wallet"
                  className="text-xs text-[var(--color-gold)] hover:underline inline-flex items-center gap-1 font-medium"
                >
                  Manage wallet & top up <ArrowRight size={12} />
                </Link>
              )}
            </div>
          </div>
        </div>

        {/* Navigation Tabs for Admins / Staff */}
        {isStaffOrAdmin && (
          <div className="mb-6 flex flex-wrap gap-2 border-b border-[var(--color-border)] pb-3">
            <button
              type="button"
              onClick={() => setAdminTab("book")}
              className={`rounded-lg px-3.5 py-1.5 text-xs font-semibold transition cursor-pointer ${
                adminTab === "book"
                  ? "bg-[var(--color-navy)] text-white"
                  : "bg-[var(--color-surface)] text-[var(--color-navy)] hover:bg-gray-200"
              }`}
            >
              Court Booking Grid
            </button>
            <button
              type="button"
              onClick={() => setAdminTab("verify")}
              className={`rounded-lg px-3.5 py-1.5 text-xs font-semibold transition cursor-pointer ${
                adminTab === "verify"
                  ? "bg-[var(--color-navy)] text-white"
                  : "bg-[var(--color-surface)] text-[var(--color-navy)] hover:bg-gray-200"
              }`}
            >
              Attendant Ticket Check-In (FR-03)
            </button>
            <button
              type="button"
              onClick={() => setAdminTab("cancel")}
              className={`rounded-lg px-3.5 py-1.5 text-xs font-semibold transition cursor-pointer ${
                adminTab === "cancel"
                  ? "bg-[var(--color-navy)] text-white"
                  : "bg-[var(--color-surface)] text-[var(--color-navy)] hover:bg-gray-200"
              }`}
            >
              Admin Cancellation & 100% Refund (FR-06)
            </button>
            <button
              type="button"
              onClick={() => setAdminTab("generate")}
              className={`rounded-lg px-3.5 py-1.5 text-xs font-semibold transition cursor-pointer ${
                adminTab === "generate"
                  ? "bg-[var(--color-navy)] text-white"
                  : "bg-[var(--color-surface)] text-[var(--color-navy)] hover:bg-gray-200"
              }`}
            >
              Generate Daily Slots
            </button>
          </div>
        )}

        {/* STAFF TAB: Ticket Check-in Verification (FR-03, TC-FR-03-03) */}
        {isStaffOrAdmin && adminTab === "verify" && (
          <section className="mb-8 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
            <div className="flex items-center gap-2 mb-4">
              <UserCheck className="text-[var(--color-maroon)]" size={20} />
              <h2 className="font-display text-lg font-bold text-[var(--color-navy)]">
                Attendant Ticket Verification Gate (Entrance)
              </h2>
            </div>
            <p className="text-xs text-[var(--color-navy-soft)] mb-4">
              Enter the 4-digit check-in code presented by the player to verify attendance and unlock court entry.
            </p>

            <form onSubmit={handleVerifyTicket} className="flex flex-col sm:flex-row gap-3 max-w-md">
              <input
                type="text"
                maxLength={4}
                value={verifyCode}
                onChange={(e) => setVerifyCode(e.target.value.replace(/\D/g, ""))}
                placeholder="4-digit code (e.g. 4821)"
                className="rounded-lg border border-[var(--color-border)] bg-white px-4 py-2 font-mono text-base tracking-widest text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
              />
              <button
                type="submit"
                disabled={isVerifying || verifyCode.trim().length !== 4}
                className="inline-flex items-center justify-center rounded-lg bg-[var(--color-maroon)] px-5 py-2 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50 cursor-pointer"
              >
                {isVerifying ? "Verifying..." : "Verify Ticket"}
              </button>
            </form>

            {verifyError && (
              <div role="alert" className="mt-4 rounded-lg bg-red-100 p-3 text-xs text-red-800">
                {verifyError}
              </div>
            )}

            {verifiedTicket && (
              <div className="mt-4 rounded-xl border border-green-200 bg-green-50 p-4">
                <div className="flex items-center gap-2 text-green-800 font-bold text-sm">
                  <CheckCircle2 size={18} />
                  Ticket Verified Successfully — CHECKED_IN
                </div>
                <div className="mt-2 grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs text-green-900">
                  <div><strong>Booking ID:</strong> {verifiedTicket.bookingId}</div>
                  <div><strong>Check-In Code:</strong> <span className="font-mono">{verifiedTicket.checkInCode}</span></div>
                  <div><strong>Player Name:</strong> {verifiedTicket.userName || "Verified Student"}</div>
                  <div><strong>Player Email:</strong> {verifiedTicket.userEmail || "N/A"}</div>
                  <div><strong>Court:</strong> {verifiedTicket.facilityName || "Padel Courts"}</div>
                  <div><strong>Status:</strong> <span className="font-bold uppercase text-green-700">{verifiedTicket.status}</span></div>
                </div>
              </div>
            )}
          </section>
        )}

        {/* STAFF TAB: Admin Cancellation & 100% Refund (FR-06, TC-FR-06-02) */}
        {isStaffOrAdmin && adminTab === "cancel" && (
          <section className="mb-8 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
            <div className="flex items-center gap-2 mb-4">
              <Ban className="text-red-600" size={20} />
              <h2 className="font-display text-lg font-bold text-[var(--color-navy)]">
                Administrative Cancellation & 100% Single Refund
              </h2>
            </div>
            <p className="text-xs text-[var(--color-navy-soft)] mb-4">
              Authorized sports directors may cancel confirmed bookings and automatically credit a 100% full refund
              without deduction to the student&apos;s wallet ledger.
            </p>

            <form onSubmit={handleAdminCancel} className="space-y-3 max-w-lg">
              <div>
                <label className="block text-xs font-semibold text-[var(--color-navy)] mb-1">
                  Booking UUID
                </label>
                <input
                  type="text"
                  value={cancelBookingId}
                  onChange={(e) => setCancelBookingId(e.target.value)}
                  placeholder="e.g. c0000000-0000-0000-0000-000000000101"
                  className="w-full rounded-lg border border-[var(--color-border)] bg-white px-3 py-2 text-xs font-mono text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-[var(--color-navy)] mb-1">
                  Cancellation Reason / Rationale
                </label>
                <input
                  type="text"
                  value={cancelReason}
                  onChange={(e) => setCancelReason(e.target.value)}
                  placeholder="e.g. Heavy rain floodlight failure"
                  className="w-full rounded-lg border border-[var(--color-border)] bg-white px-3 py-2 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                />
              </div>
              <button
                type="submit"
                disabled={isCancelling || !cancelBookingId.trim()}
                className="inline-flex items-center justify-center rounded-lg bg-red-600 px-5 py-2 text-xs font-semibold text-white hover:bg-red-700 disabled:opacity-50 cursor-pointer"
              >
                {isCancelling ? "Processing Refund..." : "Cancel Booking & Issue 100% Refund"}
              </button>
            </form>

            {cancelError && (
              <div role="alert" className="mt-4 rounded-lg bg-red-100 p-3 text-xs text-red-800">
                {cancelError}
              </div>
            )}

            {cancelResult && (
              <div className="mt-4 rounded-xl border border-blue-200 bg-blue-50 p-4">
                <div className="flex items-center gap-2 text-blue-800 font-bold text-sm">
                  <CheckCircle2 size={18} />
                  Booking Cancelled & 100% Refund Issued
                </div>
                <div className="mt-2 grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs text-blue-900">
                  <div><strong>Booking ID:</strong> {cancelResult.bookingId}</div>
                  <div><strong>Refund Amount:</strong> {formatPaisaToPKR(cancelResult.feePaisa)}</div>
                  <div><strong>Refund Ledger Tx ID:</strong> <span className="font-mono">{cancelResult.refundTransactionId}</span></div>
                  <div><strong>Status:</strong> <span className="font-bold uppercase text-red-600">{cancelResult.status}</span></div>
                  <div><strong>Reason:</strong> {cancelResult.reason || "Administrative cancellation"}</div>
                </div>
              </div>
            )}
          </section>
        )}

        {/* STAFF TAB: Slot Generation */}
        {isStaffOrAdmin && adminTab === "generate" && (
          <section className="mb-8 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
            <h2 className="font-display text-lg font-bold text-[var(--color-navy)] mb-2">
              Generate Daily Court Slots
            </h2>
            <p className="text-xs text-[var(--color-navy-soft)] mb-4">
              Generate contiguous 60-minute slots (08:00 to 23:00 PKT) for any court and date.
            </p>

            <form onSubmit={handleGenerateSlots} className="flex flex-wrap items-end gap-3">
              <div>
                <label className="block text-xs font-semibold text-[var(--color-navy)] mb-1">Target Date</label>
                <input
                  type="date"
                  value={genDate}
                  onChange={(e) => setGenDate(e.target.value)}
                  className="rounded-lg border border-[var(--color-border)] bg-white px-3 py-1.5 text-xs text-[var(--color-navy)]"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-[var(--color-navy)] mb-1">Price (paisa)</label>
                <input
                  type="text"
                  value={genPrice}
                  onChange={(e) => setGenPrice(e.target.value)}
                  className="rounded-lg border border-[var(--color-border)] bg-white px-3 py-1.5 text-xs text-[var(--color-navy)]"
                />
              </div>
              <button
                type="submit"
                disabled={isGenerating}
                className="rounded-lg bg-[var(--color-maroon)] px-4 py-2 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50 cursor-pointer"
              >
                {isGenerating ? "Generating..." : "Generate Slots"}
              </button>
            </form>

            {genError && <div className="mt-3 text-xs text-red-600">{genError}</div>}
            {genResult && <div className="mt-3 text-xs text-green-700 font-semibold">{genResult}</div>}
          </section>
        )}

        {/* MAIN BOOKING VIEW */}
        {(adminTab === "book" || !isStaffOrAdmin) && (
          <>
            {/* Active Hold Countdown Banner (FR-03, QR-08) */}
            {heldSlot && (
              <div
                role="region"
                aria-label="Active court hold timer"
                className="mb-8 rounded-xl border-2 border-[var(--color-gold)] bg-amber-50/80 p-4 sm:p-5 shadow-xs"
              >
                <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
                  <div className="flex items-start gap-3">
                    <div className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-[var(--color-gold)]/20 text-amber-800">
                      <Clock size={20} />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-bold uppercase tracking-wider text-amber-900">
                          Slot Reserved For You (5-Minute Temporary Hold)
                        </span>
                        <span className="inline-flex items-center rounded-full bg-amber-200 px-2 py-0.5 text-xs font-bold text-amber-900">
                          {Math.floor(remainingHoldSeconds / 60)}:
                          {String(remainingHoldSeconds % 60).padStart(2, "0")} left
                        </span>
                      </div>
                      <p className="mt-1 text-xs text-amber-800">
                        {selectedFacility?.name || "Padel Courts"} &bull; {formatSlotDate(selectedDate)} &bull;{" "}
                        {formatSlotTime(heldSlot.startTime)} &ndash; {formatSlotTime(heldSlot.endTime)} &bull;{" "}
                        <strong className="font-semibold">{formatPaisaToPKR(heldSlot.pricePaisa)}</strong>
                      </p>
                    </div>
                  </div>

                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      disabled={isReleasing}
                      onClick={handleReleaseHold}
                      className="rounded-lg border border-amber-300 bg-white px-3 py-1.5 text-xs font-semibold text-amber-900 hover:bg-amber-100 disabled:opacity-50 cursor-pointer"
                    >
                      {isReleasing ? "Releasing..." : "Release Hold"}
                    </button>
                    <button
                      type="button"
                      onClick={() => openCheckout(heldSlot)}
                      className="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-maroon)] px-4 py-1.5 text-xs font-semibold text-white hover:opacity-90 cursor-pointer shadow-xs"
                    >
                      Proceed to Checkout <ArrowRight size={14} />
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Error notifications */}
            {actionError && (
              <div
                role="alert"
                className="mb-6 rounded-xl border border-red-200 bg-red-50 p-4 text-xs text-red-800 flex items-center justify-between"
              >
                <span>{actionError}</span>
                <button
                  type="button"
                  onClick={() => setActionError(null)}
                  className="text-red-700 hover:text-red-900 font-bold"
                >
                  &times;
                </button>
              </div>
            )}

            {/* Controls: Court selector & Date Picker */}
            <div className="mb-6 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
              <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
                {/* Court / Facility Switcher (Restricted exclusively to Padel courts) */}
                <div>
                  <label htmlFor="court-select" className="block text-xs font-semibold text-[var(--color-navy)] mb-1">
                    Padel Court Facility
                  </label>
                  {facilities.length > 1 ? (
                    <select
                      id="court-select"
                      value={selectedFacilityId}
                      onChange={(e) => setSelectedFacilityId(e.target.value)}
                      className="rounded-lg border border-[var(--color-border)] bg-white px-3 py-2 text-xs font-semibold text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                    >
                      {facilities.map((fac) => (
                        <option key={fac.id} value={fac.id}>
                          {fac.name}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <div className="inline-flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-white px-3 py-2 text-xs font-semibold text-[var(--color-navy)] shadow-2xs">
                      <span className="h-2 w-2 rounded-full bg-emerald-500" />
                      <span>{selectedFacility?.name || "GIKI Glass Padel Courts"}</span>
                      <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-bold text-emerald-800">
                        Bookable
                      </span>
                    </div>
                  )}
                </div>

                {/* Date Picker Controls */}
                <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3">
                  {/* Quick date pills */}
                  <div className="flex items-center gap-1.5">
                    {quickDates.map((qd) => (
                      <button
                        key={qd.date}
                        type="button"
                        onClick={() => setSelectedDate(qd.date)}
                        className={`rounded-lg px-2.5 py-1 text-xs font-medium transition cursor-pointer ${
                          selectedDate === qd.date
                            ? "bg-[var(--color-maroon)] text-white"
                            : "bg-white text-[var(--color-navy)] border border-[var(--color-border)] hover:bg-gray-50"
                        }`}
                      >
                        {qd.label}
                      </button>
                    ))}
                  </div>

                  {/* Standard date picker */}
                  <div className="flex items-center gap-2">
                    <Calendar size={16} className="text-[var(--color-navy-soft)]" />
                    <input
                      type="date"
                      value={selectedDate}
                      onChange={(e) => setSelectedDate(e.target.value)}
                      className="rounded-lg border border-[var(--color-border)] bg-white px-2.5 py-1 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                    />
                  </div>
                </div>
              </div>
            </div>

            {/* Slots Grid */}
            <section aria-labelledby="slots-heading">
              <div className="flex items-center justify-between mb-4">
                <div>
                  <h2 id="slots-heading" className="font-display text-lg font-bold text-[var(--color-navy)]">
                    Available Time Slots
                  </h2>
                  <p className="text-xs text-[var(--color-navy-soft)]">
                    {selectedFacility?.name || "Padel Courts"} &bull; {formatSlotDate(selectedDate)}
                  </p>
                </div>

                <button
                  type="button"
                  onClick={fetchSlots}
                  disabled={isLoading}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-white px-3 py-1.5 text-xs font-semibold text-[var(--color-navy)] hover:bg-[var(--color-surface)] disabled:opacity-50 cursor-pointer"
                >
                  <RefreshCw size={13} className={isLoading ? "animate-spin" : ""} />
                  Refresh
                </button>
              </div>

              {isLoading ? (
                <div className="flex items-center justify-center p-12 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
                  <div className="inline-block h-6 w-6 animate-spin rounded-full border-2 border-[var(--color-maroon)] border-t-transparent" />
                  <span className="ml-3 text-xs font-medium text-[var(--color-navy-soft)]">
                    Loading court slot availability...
                  </span>
                </div>
              ) : error ? (
                <div className="p-8 text-center rounded-xl border border-red-200 bg-red-50 text-red-800">
                  <AlertCircle className="mx-auto mb-2 text-red-600" size={24} />
                  <p className="text-sm font-semibold">{error}</p>
                  <button
                    type="button"
                    onClick={fetchSlots}
                    className="mt-3 inline-flex items-center gap-1 text-xs font-semibold underline text-red-900 cursor-pointer"
                  >
                    Retry loading slots
                  </button>
                </div>
              ) : slots.length === 0 ? (
                <div className="p-12 text-center rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]">
                  <Calendar className="mx-auto mb-2 text-[var(--color-navy-soft)]" size={32} />
                  <p className="text-sm font-semibold text-[var(--color-navy)]">
                    No time slots scheduled for this date.
                  </p>
                  <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
                    Please select another date or check back later.
                  </p>
                  {isStaffOrAdmin && (
                    <button
                      type="button"
                      onClick={() => setAdminTab("generate")}
                      className="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-maroon)] px-4 py-2 text-xs font-semibold text-white hover:opacity-90 cursor-pointer"
                    >
                      Generate Slots for {selectedDate}
                    </button>
                  )}
                </div>
              ) : (
                <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                  {slots.map((slot) => {
                    const isAvailable = slot.status === "AVAILABLE";
                    const isHeldByMe = slot.status === "HELD" && slot.heldByCaller;
                    const isHeldByOther = slot.status === "HELD" && !slot.heldByCaller;
                    const isBooked = slot.status === "BOOKED";
                    const isBlocked = slot.status === "BLOCKED";

                    return (
                      <div
                        key={slot.id}
                        data-testid={`padel-slot-${slot.id}`}
                        className={`rounded-xl border p-4 transition flex flex-col justify-between ${
                          isHeldByMe
                            ? "border-[var(--color-gold)] bg-amber-50/60 shadow-xs"
                            : isAvailable
                            ? "border-[var(--color-border)] bg-white hover:border-[var(--color-maroon)] hover:shadow-xs"
                            : "border-gray-200 bg-gray-50 opacity-75"
                        }`}
                      >
                        <div>
                          <div className="flex items-center justify-between mb-2">
                            <span className="font-mono text-xs font-bold text-[var(--color-navy)]">
                              {formatSlotTime(slot.startTime)} &ndash; {formatSlotTime(slot.endTime)}
                            </span>
                            {isAvailable && (
                              <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-bold text-emerald-800">
                                Available
                              </span>
                            )}
                            {isHeldByMe && (
                              <span className="rounded-full bg-amber-200 px-2 py-0.5 text-[10px] font-bold text-amber-900">
                                Held by you
                              </span>
                            )}
                            {isHeldByOther && (
                              <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium text-amber-800">
                                Held
                              </span>
                            )}
                            {isBooked && (
                              <span className="rounded-full bg-gray-200 px-2 py-0.5 text-[10px] font-medium text-gray-700">
                                Booked
                              </span>
                            )}
                            {isBlocked && (
                              <span className="rounded-full bg-gray-300 px-2 py-0.5 text-[10px] font-medium text-gray-800">
                                Closed
                              </span>
                            )}
                          </div>

                          <div className="mt-1 flex items-baseline justify-between text-xs text-[var(--color-navy-soft)]">
                            <span>Duration: 60 min</span>
                            <span className="font-bold text-[var(--color-navy)]">
                              {formatPaisaToPKR(slot.pricePaisa)}
                            </span>
                          </div>
                        </div>

                        <div className="mt-4 pt-3 border-t border-[var(--color-border)]/60">
                          {isAvailable ? (
                            <button
                              type="button"
                              disabled={isHolding || Boolean(heldSlot)}
                              onClick={() => handleHoldSlot(slot)}
                              className="w-full rounded-lg bg-[var(--color-navy)] py-1.5 text-xs font-semibold text-white hover:bg-[var(--color-maroon)] disabled:opacity-40 transition cursor-pointer"
                            >
                              Hold Slot (5 min)
                            </button>
                          ) : isHeldByMe ? (
                            <button
                              type="button"
                              onClick={() => openCheckout(slot)}
                              className="w-full rounded-lg bg-[var(--color-maroon)] py-1.5 text-xs font-semibold text-white hover:opacity-90 shadow-xs transition cursor-pointer"
                            >
                              Checkout & Pay
                            </button>
                          ) : (
                            <div className="py-1.5 text-center text-[11px] font-medium text-gray-500">
                              {isHeldByOther
                                ? "Temporarily reserved"
                                : isBooked
                                ? "Session reserved"
                                : "Unavailable"}
                            </div>
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </section>
          </>
        )}

        {/* CHECKOUT MODAL (QR-03, QR-08, QR-09) */}
        {checkoutSlot && (
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="checkout-title"
            onKeyDown={(e) => {
              if (e.key === "Escape") closeCheckout();
            }}
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-xs"
          >
            <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl border border-[var(--color-border)] animate-in fade-in zoom-in-95 duration-150">
              <div className="flex items-center justify-between pb-3 border-b border-[var(--color-border)]">
                <div className="flex items-center gap-2 text-[var(--color-navy)]">
                  <CreditCard size={18} className="text-[var(--color-maroon)]" />
                  <h3 id="checkout-title" className="font-display text-base font-bold text-[var(--color-navy)]">
                    Confirm Padel Reservation
                  </h3>
                </div>
                <button
                  type="button"
                  ref={checkoutCancelBtnRef}
                  onClick={closeCheckout}
                  aria-label="Close checkout modal"
                  className="rounded-lg p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-800"
                >
                  <X size={18} />
                </button>
              </div>

              {/* Booking Summary Breakdown */}
              <div className="mt-4 space-y-2.5 text-xs">
                <div className="flex justify-between py-1 border-b border-gray-100">
                  <span className="text-[var(--color-navy-soft)]">Facility / Court:</span>
                  <span className="font-semibold text-[var(--color-navy)]">
                    {selectedFacility?.name || "Padel Courts"}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-gray-100">
                  <span className="text-[var(--color-navy-soft)]">Date:</span>
                  <span className="font-semibold text-[var(--color-navy)]">
                    {formatSlotDate(selectedDate)}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-gray-100">
                  <span className="text-[var(--color-navy-soft)]">Time Window (PKT):</span>
                  <span className="font-mono font-semibold text-[var(--color-navy)]">
                    {formatSlotTime(checkoutSlot.startTime)} &ndash; {formatSlotTime(checkoutSlot.endTime)}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-gray-100">
                  <span className="text-[var(--color-navy-soft)]">Duration:</span>
                  <span className="font-semibold text-[var(--color-navy)]">60 minutes</span>
                </div>
                <div className="flex justify-between py-1 border-b border-gray-100">
                  <span className="text-[var(--color-navy-soft)]">Reservation Fee:</span>
                  <span className="font-mono font-bold text-[var(--color-maroon)] text-sm">
                    {formatPaisaToPKR(checkoutSlot.pricePaisa)}
                  </span>
                </div>

                {/* Wallet Balance Comparison */}
                <div className="mt-4 rounded-xl bg-[var(--color-surface)] p-3 border border-[var(--color-border)]">
                  <div className="flex justify-between text-xs mb-1">
                    <span className="text-[var(--color-navy-soft)]">Your Current Wallet Balance:</span>
                    <span className="font-mono font-bold text-[var(--color-navy)]">
                      {userWalletBalancePaisa !== null
                        ? formatPaisaToPKR(userWalletBalancePaisa)
                        : "Checking..."}
                    </span>
                  </div>

                  {hasSufficientFunds ? (
                    <div className="mt-2 flex items-center gap-1.5 text-[11px] font-semibold text-emerald-700">
                      <Check size={14} />
                      Sufficient funds. Payment debits directly from your Sports Wallet.
                    </div>
                  ) : (
                    <div className="mt-2 text-[11px] font-semibold text-red-700">
                      <div className="flex items-center gap-1 mb-1">
                        <AlertCircle size={14} />
                        Insufficient funds in your Sports Wallet.
                      </div>
                      <Link
                        to="/wallet"
                        className="underline text-[var(--color-maroon)] hover:opacity-80 font-bold inline-flex items-center gap-1"
                      >
                        Top up your wallet now <ArrowRight size={11} />
                      </Link>
                    </div>
                  )}
                </div>
              </div>

              {checkoutError && (
                <div role="alert" className="mt-4 rounded-lg bg-red-100 p-2.5 text-xs text-red-800">
                  {checkoutError}
                </div>
              )}

              {/* Action Buttons */}
              <div className="mt-6 flex items-center justify-end gap-3 pt-3 border-t border-[var(--color-border)]">
                <button
                  type="button"
                  disabled={isCheckingOut}
                  onClick={closeCheckout}
                  className="rounded-lg border border-gray-300 bg-white px-4 py-2 text-xs font-semibold text-gray-700 hover:bg-gray-50 disabled:opacity-50 cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={isCheckingOut || !hasSufficientFunds || isFetchingWallet}
                  onClick={handleConfirmCheckout}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-maroon)] px-5 py-2 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50 shadow-xs cursor-pointer"
                >
                  {isCheckingOut ? (
                    <>
                      <span className="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-white border-t-transparent" />
                      Debiting & Confirming...
                    </>
                  ) : (
                    "Confirm & Pay"
                  )}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* DIGITAL TICKET CONFIRMATION MODAL (FR-03, TC-FR-03-03) */}
        {confirmedTicket && (
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="ticket-title"
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-xs"
          >
            <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl border border-emerald-200 text-center animate-in fade-in zoom-in-95 duration-150">
              <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-emerald-100 text-emerald-600 mb-4">
                <CheckCircle2 size={32} />
              </div>

              <h3 id="ticket-title" className="font-display text-xl font-bold text-[var(--color-navy)]">
                Booking Confirmed!
              </h3>
              <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
                Your court reservation and wallet payment have been settled atomically.
              </p>

              {/* 4-Digit Check-In Ticket Code */}
              <div className="my-6 rounded-xl border-2 border-dashed border-[var(--color-gold)] bg-amber-50/70 p-5">
                <span className="text-[11px] font-bold uppercase tracking-wider text-amber-900 block mb-2">
                  Court Entrance Check-In Code
                </span>
                <div className="flex items-center justify-center gap-3">
                  {confirmedTicket.checkInCode.split("").map((digit, i) => (
                    <span
                      key={i}
                      className="flex h-12 w-12 items-center justify-center rounded-xl bg-white text-2xl font-black font-mono text-[var(--color-navy)] shadow-xs border border-amber-200"
                    >
                      {digit}
                    </span>
                  ))}
                </div>
                <p className="mt-3 text-[11px] text-amber-800">
                  Present this 4-digit numeric code to the complex gate attendant at the court entrance.
                </p>
              </div>

              {/* Details table */}
              <div className="rounded-xl bg-[var(--color-surface)] p-3 text-left text-xs space-y-1.5 border border-[var(--color-border)] mb-6">
                <div className="flex justify-between">
                  <span className="text-[var(--color-navy-soft)]">Booking Ref:</span>
                  <span className="font-mono text-[var(--color-navy)] text-[10px]">
                    {confirmedTicket.bookingId}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--color-navy-soft)]">Court Facility:</span>
                  <span className="font-semibold text-[var(--color-navy)]">
                    {confirmedTicket.facilityName || selectedFacility?.name || "Padel Courts"}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--color-navy-soft)]">Time Window:</span>
                  <span className="font-mono font-semibold text-[var(--color-navy)]">
                    {formatSlotTime(confirmedTicket.startTime)} &ndash; {formatSlotTime(confirmedTicket.endTime)}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--color-navy-soft)]">Amount Debited:</span>
                  <span className="font-bold text-[var(--color-maroon)]">
                    {formatPaisaToPKR(confirmedTicket.feePaisa)}
                  </span>
                </div>
              </div>

              <div className="flex items-center justify-center gap-3">
                <button
                  type="button"
                  onClick={() => setConfirmedTicket(null)}
                  className="rounded-lg bg-[var(--color-navy)] px-5 py-2 text-xs font-semibold text-white hover:bg-[var(--color-maroon)] transition cursor-pointer"
                >
                  Done
                </button>
                <Link
                  to="/wallet"
                  className="rounded-lg border border-[var(--color-border)] bg-white px-5 py-2 text-xs font-semibold text-[var(--color-navy)] hover:bg-[var(--color-surface)] transition inline-flex items-center gap-1"
                >
                  View in Wallet <ArrowRight size={12} />
                </Link>
              </div>
            </div>
          </div>
        )}
      </main>

      <div className="mx-auto max-w-6xl w-full px-4 sm:px-6 pb-8">
        <Footer />
      </div>
    </div>
  );
}
