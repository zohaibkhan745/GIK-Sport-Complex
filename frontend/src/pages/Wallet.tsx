import { useEffect, useRef, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import { Wallet as WalletIcon, ArrowDownLeft, ArrowUpRight } from "lucide-react";
import { useAuth } from "../context/AuthContext";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { api } from "../api";
import {
  type WalletMeResponse,
  formatPaisaToPKR,
  formatTransactionTimestamp,
  formatTransactionType,
} from "../types";

export function Wallet() {
  const { user, isLoading: isAuthLoading, checkError, retryCheckSession } = useAuth();

  const [walletData, setWalletData] = useState<WalletMeResponse | null>(null);
  const [isLoadingWallet, setIsLoadingWallet] = useState<boolean>(false);
  const [walletError, setWalletError] = useState<string | null>(null);

  // Monotonic sequence counter to guard against late responses when account changes or signs out
  const requestSeqRef = useRef<number>(0);

  const fetchWallet = useCallback(async (signal?: AbortSignal) => {
    if (!user) return;
    const currentSeq = ++requestSeqRef.current;
    setIsLoadingWallet(true);
    setWalletError(null);

    try {
      const data = await api.getMyWallet({ signal });
      if (requestSeqRef.current === currentSeq) {
        setWalletData(data);
        setWalletError(null);
      }
    } catch (err: unknown) {
      if (signal?.aborted) {
        return;
      }
      if (requestSeqRef.current === currentSeq) {
        setWalletError(
          err instanceof Error ? err.message : "Failed to load Sports wallet"
        );
      }
    } finally {
      if (requestSeqRef.current === currentSeq) {
        setIsLoadingWallet(false);
      }
    }
  }, [user]);

  useEffect(() => {
    // 1. Wait for session restore
    if (isAuthLoading) {
      return;
    }

    // 2. Clear state on sign-out, session error, or account change
    if (checkError || !user) {
      requestSeqRef.current++;
      setWalletData(null);
      setWalletError(null);
      setIsLoadingWallet(false);
      return;
    }

    // 3. User is signed in: fetch wallet data with abort controller
    setWalletData(null);
    setWalletError(null);

    const controller = new AbortController();
    fetchWallet(controller.signal);

    return () => {
      controller.abort();
    };
  }, [user?.id, isAuthLoading, checkError, fetchWallet]);

  const handleRetryWallet = () => {
    fetchWallet();
  };

  return (
    <div className="flex min-h-screen flex-col bg-[var(--color-bg)]">
      <Navbar />

      <main className="flex-1 py-10 px-4 sm:px-6">
        <div className="mx-auto max-w-4xl space-y-6">
          {/* Header */}
          <div>
            <h1 className="text-2xl font-bold tracking-tight text-[var(--color-navy)] sm:text-3xl">
              My Sports Wallet
            </h1>
            <p className="mt-1 text-sm text-[var(--color-navy-soft)]">
              Personal ledger balance for GIKI Sports Complex court bookings and facility services.
            </p>
          </div>

          {/* State 1: Session or Wallet Loading */}
          {(isAuthLoading || (user && isLoadingWallet)) && (
            <div
              role="status"
              aria-live="polite"
              className="rounded-2xl border border-[var(--color-border)] bg-white p-12 text-center shadow-sm"
            >
              <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-[var(--color-maroon)] border-r-transparent align-[-0.125em]" />
              <p className="mt-4 text-sm font-medium text-[var(--color-navy-soft)]">
                {isAuthLoading
                  ? "Checking session..."
                  : "Loading Sports wallet balance and recent activity..."}
              </p>
            </div>
          )}

          {/* State 2: Session Check Failure (Distinguished from Signed Out) */}
          {!isAuthLoading && checkError && (
            <div
              role="alert"
              className="rounded-2xl border border-amber-200 bg-amber-50 p-8 text-center shadow-sm"
            >
              <h2 className="text-lg font-bold text-amber-900">Session Verification Error</h2>
              <p className="mt-2 text-sm text-amber-800">
                Unable to verify your session with the server: {checkError}
              </p>
              <button
                onClick={() => retryCheckSession()}
                className="mt-4 inline-flex items-center rounded-md bg-[var(--color-maroon)] px-4 py-2 text-xs font-semibold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] cursor-pointer"
              >
                Retry session check
              </button>
            </div>
          )}

          {/* State 3: Signed-out Visitor */}
          {!isAuthLoading && !user && !checkError && (
            <div className="rounded-2xl border border-[var(--color-border)] bg-white p-10 text-center shadow-sm">
              <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--color-surface)] text-[var(--color-navy)] mb-4">
                <WalletIcon className="h-7 w-7 text-[var(--color-navy)]" />
              </div>
              <h2 className="text-xl font-bold text-[var(--color-navy)]">Sign In Required</h2>
              <p className="mt-2 text-sm text-[var(--color-navy-soft)] max-w-md mx-auto">
                Please sign in to view your Sports wallet balance and transaction history.
              </p>
              <div className="mt-6">
                <Link
                  to="/login"
                  className="inline-flex items-center rounded-md bg-[var(--color-maroon)] px-5 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] transition"
                >
                  Sign in
                </Link>
              </div>
            </div>
          )}

          {/* State 4: Wallet API Error with Retry */}
          {!isAuthLoading && user && !isLoadingWallet && walletError && (
            <div
              role="alert"
              className="rounded-2xl border border-red-200 bg-red-50 p-8 text-center shadow-sm"
            >
              <h2 className="text-lg font-bold text-red-900">Unable to load Sports wallet</h2>
              <p className="mt-2 text-sm text-red-800">{walletError}</p>
              <button
                onClick={handleRetryWallet}
                className="mt-4 inline-flex items-center rounded-md bg-[var(--color-maroon)] px-4 py-2 text-xs font-semibold text-white shadow-sm hover:bg-[var(--color-maroon-dark)] cursor-pointer"
              >
                Retry
              </button>
            </div>
          )}

          {/* State 5: No Sports Wallet Yet */}
          {!isAuthLoading && user && !isLoadingWallet && !walletError && walletData && walletData.wallet === null && (
            <div className="rounded-2xl border border-[var(--color-border)] bg-white p-10 text-center shadow-sm space-y-4">
              <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--color-surface)] text-[var(--color-navy-soft)]">
                <WalletIcon className="h-7 w-7 text-[var(--color-navy)]" />
              </div>
              <div>
                <h2 className="text-2xl font-bold text-[var(--color-navy)]">No Sports wallet yet.</h2>
                <p className="mt-2 text-sm text-[var(--color-navy-soft)] max-w-lg mx-auto">
                  A digital wallet has not been provisioned for your account yet. When court bookings, payments, or funds are registered, your balance and transaction history will appear here.
                </p>
              </div>
              <div className="mx-auto max-w-md rounded-lg bg-[var(--color-surface)] p-3 text-xs text-[var(--color-navy-soft)]">
                This page shows the Sports wallet balance.
              </div>
            </div>
          )}

          {/* State 6: Active / Existing Sports Wallet */}
          {!isAuthLoading && user && !isLoadingWallet && !walletError && walletData && walletData.wallet !== null && (
            <div className="space-y-6">
              {/* Balance Card */}
              <div className="relative overflow-hidden rounded-3xl bg-[var(--color-navy)] p-6 sm:p-8 text-white shadow-xl shadow-slate-200">
                {/* Background decorative soft blur elements */}
                <div className="pointer-events-none absolute -top-16 -right-16 h-56 w-56 rounded-full bg-white/5 blur-2xl" />
                <div className="pointer-events-none absolute -bottom-16 -left-16 h-48 w-48 rounded-full bg-[var(--color-gold)]/10 blur-2xl" />

                <div className="relative space-y-6">
                  {/* Top row */}
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="rounded-xl bg-white/10 p-2.5 backdrop-blur-md">
                        <WalletIcon className="h-6 w-6 text-[var(--color-gold)]" />
                      </div>
                      <div>
                        <span className="text-xs font-semibold uppercase tracking-wider text-slate-300">
                          Sports Digital Wallet
                        </span>
                        <p className="text-sm font-medium text-white">{user.displayName}</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="rounded-full bg-white/15 px-3 py-1 text-xs font-bold uppercase tracking-wider text-white">
                        {walletData.wallet.currency}
                      </span>
                      <span className="rounded-full bg-emerald-500/20 px-3 py-1 text-xs font-bold uppercase tracking-wider text-emerald-300">
                        {walletData.wallet.status}
                      </span>
                    </div>
                  </div>

                  {/* Balance Display */}
                  <div>
                    <p className="text-xs font-medium uppercase tracking-wider text-slate-300">
                      Total Sports Balance
                    </p>
                    <div className="mt-1 flex items-baseline gap-2">
                      <span className="font-display text-4xl sm:text-5xl font-extrabold tracking-tight text-white">
                        {walletData.wallet.balancePaisa === "0"
                          ? "PKR 0.00"
                          : formatPaisaToPKR(walletData.wallet.balancePaisa)}
                      </span>
                    </div>
                  </div>

                  {/* Explanatory Footer */}
                  <div className="border-t border-white/10 pt-4 text-xs text-slate-300">
                    This page shows the Sports wallet balance for booking padel courts and complex facilities.
                  </div>
                </div>
              </div>

              {/* Recent Activity Section */}
              <div className="rounded-2xl border border-[var(--color-border)] bg-white p-6 shadow-sm space-y-4">
                <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
                  <div>
                    <h2 className="text-lg font-bold text-[var(--color-navy)]">Recent Activity</h2>
                    <p className="text-xs text-[var(--color-navy-soft)]">
                      Showing up to 50 latest ledger entries on your personal Sports wallet.
                    </p>
                  </div>
                  {walletData.activity.length > 0 && (
                    <span className="text-xs font-semibold text-[var(--color-navy-soft)]">
                      {walletData.activity.length} {walletData.activity.length === 1 ? "entry" : "entries"}
                    </span>
                  )}
                </div>

                {walletData.activity.length === 0 ? (
                  <div className="py-12 text-center text-sm text-[var(--color-navy-soft)]">
                    No recent transactions.
                  </div>
                ) : (
                  <div className="divide-y divide-[var(--color-border)]">
                    {walletData.activity.map((entry) => {
                      const isNegative = entry.amountPaisa.startsWith("-");
                      return (
                        <div
                          key={entry.entryId}
                          className="flex items-center justify-between py-3.5 transition hover:bg-[var(--color-surface)]/50 px-2 rounded-lg"
                        >
                          {/* Left: Icon + Type + Date */}
                          <div className="flex items-center gap-3 min-w-0">
                            <div
                              className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${
                                isNegative
                                  ? "bg-rose-50 text-rose-700"
                                  : "bg-emerald-50 text-emerald-700"
                              }`}
                            >
                              {isNegative ? (
                                <ArrowUpRight className="h-5 w-5" />
                              ) : (
                                <ArrowDownLeft className="h-5 w-5" />
                              )}
                            </div>
                            <div className="min-w-0">
                              <p className="truncate text-sm font-bold text-[var(--color-navy)]">
                                {formatTransactionType(entry.type)}
                              </p>
                              <p className="text-xs text-[var(--color-navy-soft)]">
                                {formatTransactionTimestamp(entry.createdAt)}
                              </p>
                            </div>
                          </div>

                          {/* Right: Amount */}
                          <div className="text-right pl-4 shrink-0">
                            <span
                              className={`font-semibold tracking-tight text-sm sm:text-base ${
                                isNegative ? "text-slate-900" : "text-emerald-700"
                              }`}
                            >
                              {formatPaisaToPKR(entry.amountPaisa, {
                                showExplicitPositiveSign: true,
                              })}
                            </span>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </main>

      <Footer />
    </div>
  );
}

