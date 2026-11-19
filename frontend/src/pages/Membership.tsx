import { useEffect, useState, useCallback, useRef } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  ArrowLeft,
  Dumbbell,
  Calendar,
  Clock,
  ShieldCheck,
  AlertCircle,
  CheckCircle2,
  Wallet,
  ArrowRight,
  RefreshCw,
  Sparkles,
} from "lucide-react";
import { useAuth } from "../context/AuthContext";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { api } from "../api";
import {
  type GymMembershipSummary,
  type WalletMeResponse,
  formatPaisaToPKR,
} from "../types";

export const GYM_PASS_FEE_PAISA = 300000; // PKR 3,000.00 in integer paisa

export function Membership() {
  const navigate = useNavigate();
  const { user, isLoading: isAuthLoading } = useAuth();

  const handleGoBack = useCallback(() => {
    if (window.history.state && typeof window.history.state.idx === "number" && window.history.state.idx > 0) {
      navigate(-1);
    } else if (window.history.length > 1) {
      navigate(-1);
    } else {
      navigate("/#facilities");
    }
  }, [navigate]);

  const [summary, setSummary] = useState<GymMembershipSummary | null>(null);
  const [walletData, setWalletData] = useState<WalletMeResponse | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isPurchasing, setIsPurchasing] = useState<boolean>(false);
  const [feedback, setFeedback] = useState<{ type: "success" | "error"; message: string } | null>(null);
  const [showConfirmModal, setShowConfirmModal] = useState<boolean>(false);

  const reqSeqRef = useRef<number>(0);

  const loadData = useCallback(async (signal?: AbortSignal, preserveFeedback?: boolean) => {
    if (!user) {
      setIsLoading(false);
      return;
    }

    const seq = ++reqSeqRef.current;
    setIsLoading(true);
    if (!preserveFeedback) {
      setFeedback(null);
    }

    try {
      const [sum, wal] = await Promise.all([
        api.getMyGymMembership({ signal }),
        api.getMyWallet({ signal }).catch(() => null),
      ]);

      if (reqSeqRef.current === seq) {
        setSummary(sum);
        setWalletData(wal);
      }
    } catch (err: unknown) {
      if (signal?.aborted) return;
      if (reqSeqRef.current === seq) {
        setFeedback({
          type: "error",
          message: err instanceof Error ? err.message : "Failed to load membership status",
        });
      }
    } finally {
      if (reqSeqRef.current === seq) {
        setIsLoading(false);
      }
    }
  }, [user]);

  useEffect(() => {
    if (isAuthLoading) return;
    const ac = new AbortController();
    loadData(ac.signal);
    return () => ac.abort();
  }, [isAuthLoading, loadData]);

  const handlePurchase = async () => {
    if (!user) return;
    setIsPurchasing(true);
    setFeedback(null);

    try {
      const key = typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
        ? crypto.randomUUID()
        : `gym-pass-${Date.now()}`;

      await api.purchaseGymMembership("STANDARD_30_DAY", key);
      setShowConfirmModal(false);
      // Refresh state while preserving success notification
      await loadData(undefined, true);
      setFeedback({
        type: "success",
        message: summary?.isActive
          ? "Gym membership renewed successfully! 30 days added to your coverage."
          : "30-Day Gym Membership activated successfully!",
      });
    } catch (err: unknown) {
      setFeedback({
        type: "error",
        message: err instanceof Error ? err.message : "Membership purchase failed",
      });
    } finally {
      setIsPurchasing(false);
    }
  };

  const userBalancePaisa = walletData?.wallet ? Number(walletData.wallet.balancePaisa) : 0;
  const hasSufficientFunds = userBalancePaisa >= GYM_PASS_FEE_PAISA;

  const formatDate = (dateStr: string) => {
    try {
      return new Date(dateStr).toLocaleDateString("en-PK", {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {
      return dateStr;
    }
  };

  return (
    <div className="flex min-h-screen flex-col bg-[var(--color-surface)] text-[var(--color-navy)]">
      <Navbar />

      <main className="flex-1 py-10">
        <div className="mx-auto max-w-5xl px-4 sm:px-6 lg:px-8">
          {/* Back Navigation */}
          <div className="mb-4">
            <button
              type="button"
              onClick={handleGoBack}
              className="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-maroon)] transition-opacity hover:opacity-80 cursor-pointer"
            >
              <ArrowLeft size={16} />
              Go back
            </button>
          </div>

          {/* Header */}
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4 mb-8">
            <div>
              <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-[var(--color-maroon)]">
                <Dumbbell className="h-4 w-4" />
                Fitness & Wellness Center
              </div>
              <h1 className="mt-1 text-3xl font-extrabold tracking-tight text-[var(--color-navy)]">
                Gym Membership Pass
              </h1>
              <p className="mt-1 text-sm text-[var(--color-navy-soft)]">
                Standard 30-day membership with instant digital wallet activation.
              </p>
            </div>

            {user && (
              <div className="flex items-center gap-3 bg-white p-3 rounded-xl border border-[var(--color-border)] shadow-sm">
                <div className="h-10 w-10 rounded-lg bg-[var(--color-maroon)]/10 text-[var(--color-maroon)] flex items-center justify-center">
                  <Wallet className="h-5 w-5" />
                </div>
                <div>
                  <div className="text-xs text-[var(--color-navy-soft)]">Wallet Balance</div>
                  <div className="text-sm font-bold text-[var(--color-navy)]">
                    {formatPaisaToPKR(userBalancePaisa)}
                  </div>
                </div>
                <Link
                  to="/wallet"
                  className="ml-2 text-xs font-semibold text-[var(--color-maroon)] hover:underline"
                >
                  Top Up
                </Link>
              </div>
            )}
          </div>

          {/* Feedback Banner */}
          {feedback && (
            <div
              role="alert"
              className={`mb-6 p-4 rounded-xl flex items-start gap-3 border ${
                feedback.type === "success"
                  ? "bg-emerald-50 border-emerald-200 text-emerald-900"
                  : "bg-red-50 border-red-200 text-red-900"
              }`}
            >
              {feedback.type === "success" ? (
                <CheckCircle2 className="h-5 w-5 text-emerald-600 shrink-0 mt-0.5" />
              ) : (
                <AlertCircle className="h-5 w-5 text-red-600 shrink-0 mt-0.5" />
              )}
              <div className="text-sm font-medium">{feedback.message}</div>
            </div>
          )}

          {!user ? (
            /* Unauthenticated Prompt */
            <div className="rounded-2xl border border-[var(--color-border)] bg-white p-8 text-center shadow-sm">
              <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-[var(--color-maroon)]/10 text-[var(--color-maroon)]">
                <Dumbbell className="h-7 w-7" />
              </div>
              <h2 className="mt-4 text-xl font-bold text-[var(--color-navy)]">
                Sign in to View or Purchase Membership
              </h2>
              <p className="mx-auto mt-2 max-w-md text-sm text-[var(--color-navy-soft)]">
                Active GIKI students, faculty, and staff can purchase or renew 30-day New Gym passes directly using their digital wallet. (Old Gym remains free for all students).
              </p>
              <div className="mt-6 flex justify-center gap-4">
                <Link
                  to="/login"
                  className="inline-flex items-center gap-2 rounded-lg bg-[var(--color-maroon)] px-5 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition"
                >
                  Sign In <ArrowRight className="h-4 w-4" />
                </Link>
              </div>
            </div>
          ) : isLoading ? (
            /* Loading Skeleton */
            <div className="flex flex-col items-center justify-center p-16 text-[var(--color-navy-soft)]">
              <RefreshCw className="h-8 w-8 animate-spin text-[var(--color-maroon)]" />
              <p className="mt-3 text-sm">Loading gym membership details...</p>
            </div>
          ) : (
            /* Authenticated Content */
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
              {/* Pass Status Card (Left 2 cols) */}
              <div className="lg:col-span-2 space-y-6">
                <div className="rounded-2xl border border-[var(--color-border)] bg-white p-6 shadow-sm">
                  <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-4">
                    <div className="flex items-center gap-3">
                      <div
                        className={`h-10 w-10 rounded-full flex items-center justify-center ${
                          summary?.isActive
                            ? "bg-emerald-100 text-emerald-700"
                            : "bg-slate-100 text-slate-500"
                        }`}
                      >
                        <ShieldCheck className="h-6 w-6" />
                      </div>
                      <div>
                        <h2 className="text-lg font-bold text-[var(--color-navy)]">Current Pass Status</h2>
                        <p className="text-xs text-[var(--color-navy-soft)]">
                          Physical entrance verified via GIKI card or attendant scanner
                        </p>
                      </div>
                    </div>

                    <span
                      className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-bold ${
                        summary?.isActive
                          ? "bg-emerald-100 text-emerald-800"
                          : "bg-slate-100 text-slate-700"
                      }`}
                    >
                      <span
                        className={`h-2 w-2 rounded-full ${
                          summary?.isActive ? "bg-emerald-500 animate-pulse" : "bg-slate-400"
                        }`}
                      />
                      {summary?.isActive ? "ACTIVE PASS" : "NO ACTIVE PASS"}
                    </span>
                  </div>

                  {summary?.isActive ? (
                    <div className="mt-6 space-y-6">
                      {/* Active Countdown Hero */}
                      <div className="rounded-xl bg-gradient-to-br from-emerald-50 to-teal-50 border border-emerald-100 p-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
                        <div>
                          <div className="text-xs font-bold uppercase tracking-wider text-emerald-800">
                            Validity Remaining
                          </div>
                          <div className="mt-1 text-3xl font-black text-emerald-950">
                            {summary.daysRemaining} {summary.daysRemaining === 1 ? "Day" : "Days"} Left
                          </div>
                          <p className="mt-1 text-xs text-emerald-700">
                            Valid until{" "}
                            <span className="font-semibold">
                              {summary.effectiveExpiry ? formatDate(summary.effectiveExpiry) : "N/A"}
                            </span>
                          </p>
                        </div>

                        <button
                          onClick={() => setShowConfirmModal(true)}
                          className="inline-flex items-center gap-2 rounded-lg bg-[var(--color-maroon)] px-4 py-2.5 text-xs font-bold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition self-start md:self-auto cursor-pointer"
                        >
                          <Sparkles className="h-4 w-4" /> Renew (+30 Days)
                        </button>
                      </div>

                      {/* Term Info */}
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
                        <div className="p-3 rounded-lg bg-[var(--color-surface)] border border-[var(--color-border)]">
                          <span className="text-[var(--color-navy-soft)]">Plan Tier:</span>
                          <span className="ml-1 font-bold text-[var(--color-navy)]">Standard 30-Day Pass</span>
                        </div>
                        <div className="p-3 rounded-lg bg-[var(--color-surface)] border border-[var(--color-border)]">
                          <span className="text-[var(--color-navy-soft)]">Access Hours:</span>
                          <span className="ml-1 font-bold text-[var(--color-navy)]">Daily 06:00 – 22:00 PKT</span>
                        </div>
                      </div>
                    </div>
                  ) : (
                    /* Inactive State Banner */
                    <div className="mt-6 text-center py-8">
                      <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-amber-100 text-amber-700">
                        <Clock className="h-6 w-6" />
                      </div>
                      <h3 className="mt-3 text-base font-bold text-[var(--color-navy)]">
                        No Active Gym Pass Found
                      </h3>
                      <p className="mt-1 text-xs text-[var(--color-navy-soft)] max-w-sm mx-auto">
                        Activate your 30-day gym membership to enjoy unrestricted access to the weight training area, cardio floor, and locker facilities.
                      </p>
                      <button
                        onClick={() => setShowConfirmModal(true)}
                        className="mt-5 inline-flex items-center gap-2 rounded-lg bg-[var(--color-maroon)] px-5 py-2.5 text-xs font-bold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
                      >
                        <Dumbbell className="h-4 w-4" /> Purchase 30-Day Pass (PKR 3,000)
                      </button>
                    </div>
                  )}
                </div>

                {/* Membership History */}
                <div className="rounded-2xl border border-[var(--color-border)] bg-white p-6 shadow-sm">
                  <h3 className="text-sm font-bold text-[var(--color-navy)] flex items-center gap-2 border-b border-[var(--color-border)] pb-3">
                    <Calendar className="h-4 w-4 text-[var(--color-maroon)]" />
                    Membership History
                  </h3>

                  {!summary?.history || summary.history.length === 0 ? (
                    <div className="py-6 text-center text-xs text-[var(--color-navy-soft)]">
                      No past membership passes recorded on your account.
                    </div>
                  ) : (
                    <div className="mt-4 overflow-x-auto">
                      <table className="w-full text-left text-xs">
                        <thead>
                          <tr className="border-b border-[var(--color-border)] text-[var(--color-navy-soft)] font-semibold">
                            <th className="py-2 pr-4">Term Window</th>
                            <th className="py-2 pr-4">Plan</th>
                            <th className="py-2 pr-4">Fee Paid</th>
                            <th className="py-2 pr-4">Status</th>
                            <th className="py-2">Purchased On</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-[var(--color-border)] text-[var(--color-navy)]">
                          {summary.history.map((m) => (
                            <tr key={m.id}>
                              <td className="py-3 pr-4 font-medium">
                                {formatDate(m.startTime)} – {formatDate(m.endTime)}
                              </td>
                              <td className="py-3 pr-4 font-semibold text-[var(--color-navy)]">
                                30-Day Standard
                              </td>
                              <td className="py-3 pr-4 font-bold text-[var(--color-navy)]">
                                {formatPaisaToPKR(m.pricePaisa)}
                              </td>
                              <td className="py-3 pr-4">
                                <span
                                  className={`inline-block px-2 py-0.5 rounded text-[10px] font-bold ${
                                    m.status === "ACTIVE"
                                      ? "bg-emerald-100 text-emerald-800"
                                      : "bg-slate-100 text-slate-600"
                                  }`}
                                >
                                  {m.status}
                                </span>
                              </td>
                              <td className="py-3 text-[var(--color-navy-soft)]">
                                {formatDate(m.createdAt)}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              </div>

              {/* Right Col: Plan Summary & Rules */}
              <div className="space-y-6">
                <div className="rounded-2xl border border-[var(--color-border)] bg-white p-6 shadow-sm">
                  <h3 className="text-base font-bold text-[var(--color-navy)]">New Gym 30-Day Plan</h3>
                  <div className="mt-2 text-[11px] text-emerald-800 bg-emerald-50 border border-emerald-200 rounded-lg p-2.5">
                    <strong>Note:</strong> Old Gym access remains 100% free for all students. This pass grants access to the New Gym suite.
                  </div>
                  <div className="mt-3 flex items-baseline gap-1">
                    <span className="text-3xl font-extrabold text-[var(--color-navy)]">PKR 3,000</span>
                    <span className="text-xs text-[var(--color-navy-soft)]">/ 30 days</span>
                  </div>

                  <ul className="mt-5 space-y-2.5 text-xs text-[var(--color-navy-soft)]">
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="h-4 w-4 text-emerald-600 shrink-0" />
                      Strict 30-day validity window
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="h-4 w-4 text-emerald-600 shrink-0" />
                      New Gym floor (Olympic racks, cardio suite & functional rigs)
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="h-4 w-4 text-emerald-600 shrink-0" />
                      Locker & shower facilities access
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="h-4 w-4 text-emerald-600 shrink-0" />
                      Early renewal adds 30 days without penalty
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="h-4 w-4 text-emerald-600 shrink-0" />
                      Instant activation via GIKI Wallet
                    </li>
                  </ul>

                  <div className="mt-6 border-t border-[var(--color-border)] pt-4">
                    <button
                      onClick={() => setShowConfirmModal(true)}
                      className="w-full rounded-xl bg-[var(--color-maroon)] py-3 text-xs font-bold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
                    >
                      {summary?.isActive ? "Renew Pass (+30 Days)" : "Purchase 30-Day Pass"}
                    </button>
                  </div>
                </div>

                <div className="rounded-2xl border border-[var(--color-border)] bg-white p-5 shadow-sm text-xs space-y-2">
                  <div className="font-bold text-[var(--color-navy)]">Institutional Pass Policy:</div>
                  <p className="text-[var(--color-navy-soft)]">
                    Gym passes are non-transferable and strictly assigned to your verified GIKI student/staff ID.
                  </p>
                  <p className="text-[var(--color-navy-soft)]">
                    Renewal extends your validity from the current expiry timestamp, guaranteeing zero lost days.
                  </p>
                </div>
              </div>
            </div>
          )}
        </div>
      </main>

      {/* Confirmation Modal */}
      {showConfirmModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl border border-[var(--color-border)]">
            <div className="flex items-center gap-3 border-b border-[var(--color-border)] pb-4">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-maroon)]/10 text-[var(--color-maroon)]">
                <Dumbbell className="h-5 w-5" />
              </div>
              <div>
                <h3 className="text-base font-bold text-[var(--color-navy)]">
                  {summary?.isActive ? "Confirm Early Renewal" : "Confirm Pass Purchase"}
                </h3>
                <p className="text-xs text-[var(--color-navy-soft)]">
                  Standard 30-Day Gym Membership Pass
                </p>
              </div>
            </div>

            <div className="mt-4 space-y-3 text-xs">
              <div className="flex justify-between py-1 border-b border-[var(--color-border)]">
                <span className="text-[var(--color-navy-soft)]">Membership Fee:</span>
                <span className="font-bold text-[var(--color-navy)]">PKR 3,000.00</span>
              </div>
              <div className="flex justify-between py-1 border-b border-[var(--color-border)]">
                <span className="text-[var(--color-navy-soft)]">Your Wallet Balance:</span>
                <span className={`font-bold ${hasSufficientFunds ? "text-emerald-700" : "text-red-600"}`}>
                  {formatPaisaToPKR(userBalancePaisa)}
                </span>
              </div>
              <div className="flex justify-between py-1">
                <span className="text-[var(--color-navy-soft)]">Duration:</span>
                <span className="font-semibold text-[var(--color-navy)]">30 Calendar Days (720 Hours)</span>
              </div>

              {summary?.isActive && summary.effectiveExpiry && (
                <div className="rounded-lg bg-emerald-50 border border-emerald-200 p-2.5 text-emerald-900 mt-2">
                  <div className="font-bold">Continuous Renewal:</div>
                  Your pass will be extended for 30 days starting from{" "}
                  <span className="font-semibold">{formatDate(summary.effectiveExpiry)}</span>.
                </div>
              )}

              {!hasSufficientFunds && (
                <div className="rounded-lg bg-red-50 border border-red-200 p-2.5 text-red-900 flex items-start gap-2 mt-2">
                  <AlertCircle className="h-4 w-4 shrink-0 text-red-600 mt-0.5" />
                  <div>
                    <div className="font-bold">Insufficient Wallet Balance</div>
                    You need PKR 3,000.00 to activate this pass. Please top up your wallet first.
                  </div>
                </div>
              )}
            </div>

            <div className="mt-6 flex items-center justify-end gap-3">
              <button
                type="button"
                onClick={() => setShowConfirmModal(false)}
                disabled={isPurchasing}
                className="rounded-lg border border-[var(--color-border)] px-4 py-2 text-xs font-semibold text-[var(--color-navy)] hover:bg-[var(--color-surface)] cursor-pointer"
              >
                Cancel
              </button>
              {hasSufficientFunds ? (
                <button
                  type="button"
                  onClick={handlePurchase}
                  disabled={isPurchasing}
                  className="rounded-lg bg-[var(--color-maroon)] px-4 py-2 text-xs font-bold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition disabled:opacity-50 cursor-pointer"
                >
                  {isPurchasing ? "Processing..." : "Confirm & Pay PKR 3,000"}
                </button>
              ) : (
                <Link
                  to="/wallet"
                  className="rounded-lg bg-[var(--color-maroon)] px-4 py-2 text-xs font-bold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition inline-flex items-center gap-1.5"
                >
                  Top Up Wallet <ArrowRight className="h-3.5 w-3.5" />
                </Link>
              )}
            </div>
          </div>
        </div>
      )}

      <Footer />
    </div>
  );
}
