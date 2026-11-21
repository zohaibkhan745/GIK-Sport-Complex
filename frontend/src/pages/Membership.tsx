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
  Pencil,
  X,
} from "lucide-react";
import { useAuth } from "../context/AuthContext";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { api } from "../api";
import {
  type GymMembershipSummary,
  type GymMembershipPlan,
  type WalletMeResponse,
  formatPaisaToPKR,
} from "../types";

export const GYM_PASS_FEE_PAISA = 300000; // PKR 3,000.00 fallback in integer paisa

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
  const [plan, setPlan] = useState<GymMembershipPlan | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isPurchasing, setIsPurchasing] = useState<boolean>(false);
  const [feedback, setFeedback] = useState<{ type: "success" | "error"; message: string } | null>(null);
  const [showConfirmModal, setShowConfirmModal] = useState<boolean>(false);

  // Director Plan Editing state
  const [showEditPlanModal, setShowEditPlanModal] = useState<boolean>(false);
  const [editName, setEditName] = useState<string>("Standard Gym Membership");
  const [editFeePKR, setEditFeePKR] = useState<string>("3000");
  const [editDurationDays, setEditDurationDays] = useState<number>(30);
  const [editDescription, setEditDescription] = useState<string>("");
  const [isSavingPlan, setIsSavingPlan] = useState<boolean>(false);
  const [planSaveError, setPlanSaveError] = useState<string | null>(null);

  const isDirectorOrAdmin = Boolean(
    user && (user.roles.includes("sports_director") || user.roles.includes("super_admin"))
  );

  const reqSeqRef = useRef<number>(0);

  const loadData = useCallback(async (signal?: AbortSignal, preserveFeedback?: boolean) => {
    const seq = ++reqSeqRef.current;
    setIsLoading(true);
    if (!preserveFeedback) {
      setFeedback(null);
    }

    try {
      const [sum, wal, planRes] = await Promise.all([
        user ? api.getMyGymMembership({ signal }) : Promise.resolve(null),
        user ? api.getMyWallet({ signal }).catch(() => null) : Promise.resolve(null),
        api.getGymMembershipPlan({ signal }).catch(() => null),
      ]);

      if (reqSeqRef.current === seq) {
        setSummary(sum);
        setWalletData(wal);
        if (planRes) {
          setPlan(planRes);
        }
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

  const currentFeePaisa = plan ? plan.pricePaisa : GYM_PASS_FEE_PAISA;
  const currentDurationDays = plan ? plan.durationDays : 30;
  const userBalancePaisa = walletData?.wallet ? Number(walletData.wallet.balancePaisa) : 0;
  const hasSufficientFunds = userBalancePaisa >= currentFeePaisa;

  const openEditPlanModal = () => {
    setEditName(plan?.name || "Standard Gym Membership");
    const feePkr = plan ? Math.round(plan.pricePaisa / 100) : 3000;
    setEditFeePKR(String(feePkr));
    setEditDurationDays(plan ? plan.durationDays : 30);
    setEditDescription(plan?.description || "");
    setPlanSaveError(null);
    setShowEditPlanModal(true);
  };

  const handleSavePlan = async (e: React.FormEvent) => {
    e.preventDefault();
    const fee = Number(editFeePKR);
    if (isNaN(fee) || fee < 0) {
      setPlanSaveError("Fee must be a non-negative number.");
      return;
    }
    const days = Number(editDurationDays);
    if (isNaN(days) || days <= 0) {
      setPlanSaveError("Duration must be at least 1 day.");
      return;
    }

    setIsSavingPlan(true);
    setPlanSaveError(null);
    try {
      const updated = await api.updateGymMembershipPlan({
        name: editName.trim(),
        durationDays: days,
        pricePaisa: Math.round(fee * 100),
        description: editDescription.trim(),
      });
      setPlan(updated);
      setShowEditPlanModal(false);
      setFeedback({
        type: "success",
        message: `Gym pass settings updated: ${updated.name || "Pass"} (PKR ${fee.toLocaleString()} for ${days} days).`,
      });
    } catch (err: unknown) {
      setPlanSaveError(err instanceof Error ? err.message : "Failed to update membership plan.");
    } finally {
      setIsSavingPlan(false);
    }
  };

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
          ? `Gym membership renewed successfully! ${currentDurationDays} days added to your coverage.`
          : `${currentDurationDays}-Day Gym Membership activated successfully!`,
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
                {plan?.name || "Gym Membership Pass"}
              </h1>
              <p className="mt-1 text-sm text-[var(--color-navy-soft)]">
                Digital gym pass with instant wallet activation.
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
                          <Sparkles className="h-4 w-4" /> Renew (+{currentDurationDays} Days)
                        </button>
                      </div>

                      {/* Term Info */}
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
                        <div className="p-3 rounded-lg bg-[var(--color-surface)] border border-[var(--color-border)]">
                          <span className="text-[var(--color-navy-soft)]">Plan Tier:</span>
                          <span className="ml-1 font-bold text-[var(--color-navy)]">{plan?.name || "Standard Pass"}</span>
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
                        Activate your gym membership pass to enjoy access to the fitness facility.
                      </p>
                      <button
                        onClick={() => setShowConfirmModal(true)}
                        className="mt-5 inline-flex items-center gap-2 rounded-lg bg-[var(--color-maroon)] px-5 py-2.5 text-xs font-bold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
                      >
                        <Dumbbell className="h-4 w-4" /> Purchase {currentDurationDays}-Day Pass (PKR {Math.round(currentFeePaisa / 100).toLocaleString()})
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

              {/* Right Col: Plan Summary */}
              <div className="space-y-6">
                <div className="rounded-2xl border border-[var(--color-border)] bg-white p-6 shadow-sm">
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="text-base font-bold text-[var(--color-navy)]">
                      {plan?.name || "Gym Pass"}
                    </h3>
                    {isDirectorOrAdmin && (
                      <button
                        type="button"
                        data-testid="edit-plan-btn"
                        onClick={openEditPlanModal}
                        className="inline-flex items-center gap-1 rounded-lg border border-[var(--color-maroon)]/30 bg-[var(--color-maroon)]/5 px-2.5 py-1 text-xs font-semibold text-[var(--color-maroon)] hover:bg-[var(--color-maroon)] hover:text-white transition cursor-pointer"
                      >
                        <Pencil size={12} />
                        Edit Pass Details
                      </button>
                    )}
                  </div>

                  <div className="mt-3 flex items-baseline gap-1">
                    <span className="text-3xl font-extrabold text-[var(--color-navy)]">
                      {formatPaisaToPKR(currentFeePaisa)}
                    </span>
                    <span className="text-xs text-[var(--color-navy-soft)]">/ {currentDurationDays} days</span>
                  </div>

                  {plan?.description && (
                    <p className="mt-4 text-xs text-[var(--color-navy-soft)] leading-relaxed whitespace-pre-line border-t border-[var(--color-border)] pt-3">
                      {plan.description}
                    </p>
                  )}

                  <div className="mt-6 border-t border-[var(--color-border)] pt-4">
                    <button
                      onClick={() => setShowConfirmModal(true)}
                      className="w-full rounded-xl bg-[var(--color-maroon)] py-3 text-xs font-bold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
                    >
                      {summary?.isActive ? `Renew Pass (+${currentDurationDays} Days)` : `Purchase Pass`}
                    </button>
                  </div>
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
                  Standard {currentDurationDays}-Day Gym Membership Pass
                </p>
              </div>
            </div>

            <div className="mt-4 space-y-3 text-xs">
              <div className="flex justify-between py-1 border-b border-[var(--color-border)]">
                <span className="text-[var(--color-navy-soft)]">Membership Fee:</span>
                <span className="font-bold text-[var(--color-navy)]">{formatPaisaToPKR(currentFeePaisa)}</span>
              </div>
              <div className="flex justify-between py-1 border-b border-[var(--color-border)]">
                <span className="text-[var(--color-navy-soft)]">Your Wallet Balance:</span>
                <span className={`font-bold ${hasSufficientFunds ? "text-emerald-700" : "text-red-600"}`}>
                  {formatPaisaToPKR(userBalancePaisa)}
                </span>
              </div>
              <div className="flex justify-between py-1">
                <span className="text-[var(--color-navy-soft)]">Duration:</span>
                <span className="font-semibold text-[var(--color-navy)]">
                  {currentDurationDays} Calendar Days ({currentDurationDays * 24} Hours)
                </span>
              </div>

              {summary?.isActive && summary.effectiveExpiry && (
                <div className="rounded-lg bg-emerald-50 border border-emerald-200 p-2.5 text-emerald-900 mt-2">
                  <div className="font-bold">Continuous Renewal:</div>
                  Your pass will be extended for {currentDurationDays} days starting from{" "}
                  <span className="font-semibold">{formatDate(summary.effectiveExpiry)}</span>.
                </div>
              )}

              {!hasSufficientFunds && (
                <div className="rounded-lg bg-red-50 border border-red-200 p-2.5 text-red-900 flex items-start gap-2 mt-2">
                  <AlertCircle className="h-4 w-4 shrink-0 text-red-600 mt-0.5" />
                  <div>
                    <div className="font-bold">Insufficient Wallet Balance</div>
                    You need {formatPaisaToPKR(currentFeePaisa)} to activate this pass. Please top up your wallet first.
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
                  {isPurchasing ? "Processing..." : `Confirm & Pay ${formatPaisaToPKR(currentFeePaisa)}`}
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

      {/* Director Edit Plan Modal */}
      {showEditPlanModal && (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="edit-plan-title"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm"
        >
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl border border-[var(--color-border)] animate-in fade-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
              <div className="flex items-center gap-2 text-[var(--color-navy)]">
                <Dumbbell className="h-5 w-5 text-[var(--color-maroon)]" />
                <h3 id="edit-plan-title" className="font-display text-base font-bold text-[var(--color-navy)]">
                  Edit Gym Pass Settings
                </h3>
              </div>
              <button
                type="button"
                onClick={() => setShowEditPlanModal(false)}
                className="rounded-lg p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 cursor-pointer"
              >
                <X size={18} />
              </button>
            </div>

            <form onSubmit={handleSavePlan} className="mt-4 space-y-4">
              {planSaveError && (
                <div role="alert" className="rounded-lg bg-red-50 border border-red-200 p-3 text-xs text-red-800">
                  {planSaveError}
                </div>
              )}

              <div>
                <label htmlFor="plan-name" className="block text-xs font-semibold text-[var(--color-navy)] mb-1">
                  Pass Title / Name
                </label>
                <input
                  id="plan-name"
                  type="text"
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  placeholder="e.g. Standard Gym Pass"
                  required
                  className="w-full rounded-lg border border-[var(--color-border)] bg-white px-3 py-2 text-xs font-bold text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                />
              </div>

              <div>
                <label htmlFor="plan-fee" className="block text-xs font-semibold text-[var(--color-navy)] mb-1">
                  Membership Fee (PKR)
                </label>
                <div className="relative">
                  <span className="absolute left-3 top-2 text-xs font-bold text-gray-500">PKR</span>
                  <input
                    id="plan-fee"
                    type="number"
                    min="0"
                    step="50"
                    value={editFeePKR}
                    onChange={(e) => setEditFeePKR(e.target.value)}
                    required
                    className="w-full rounded-lg border border-[var(--color-border)] bg-white pl-12 pr-3 py-2 text-xs font-bold text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                  />
                </div>
                <p className="mt-1 text-[11px] text-[var(--color-navy-soft)]">
                  Stored as {Math.round(Number(editFeePKR || 0) * 100).toLocaleString()} paisa in double-entry ledger.
                </p>
              </div>

              <div>
                <label htmlFor="plan-duration" className="block text-xs font-semibold text-[var(--color-navy)] mb-1">
                  Pass Validity Duration (Days)
                </label>
                <input
                  id="plan-duration"
                  type="number"
                  min="1"
                  max="365"
                  value={editDurationDays}
                  onChange={(e) => setEditDurationDays(Number(e.target.value))}
                  required
                  className="w-full rounded-lg border border-[var(--color-border)] bg-white px-3 py-2 text-xs font-bold text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                />
                <p className="mt-1 text-[11px] text-[var(--color-navy-soft)]">
                  Number of days the pass remains valid from purchase or continuous renewal.
                </p>
              </div>

              <div>
                <label htmlFor="plan-desc" className="block text-xs font-semibold text-[var(--color-navy)] mb-1">
                  Plan Description & Perks
                </label>
                <textarea
                  id="plan-desc"
                  rows={3}
                  value={editDescription}
                  onChange={(e) => setEditDescription(e.target.value)}
                  placeholder="e.g. Olympic racks, specialized free weights, cardio machines, and functional rigs."
                  className="w-full rounded-lg border border-[var(--color-border)] bg-white px-3 py-2 text-xs text-[var(--color-navy)] focus:border-[var(--color-maroon)] focus:outline-none"
                />
              </div>

              <div className="flex items-center justify-end gap-3 pt-3 border-t border-[var(--color-border)]">
                <button
                  type="button"
                  onClick={() => setShowEditPlanModal(false)}
                  disabled={isSavingPlan}
                  className="rounded-lg border border-[var(--color-border)] px-4 py-2 text-xs font-semibold text-[var(--color-navy)] hover:bg-gray-50 cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isSavingPlan}
                  className="rounded-lg bg-[var(--color-maroon)] px-5 py-2 text-xs font-bold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] disabled:opacity-50 transition cursor-pointer"
                >
                  {isSavingPlan ? "Saving Changes..." : "Save Pass Settings"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      <Footer />
    </div>
  );
}
