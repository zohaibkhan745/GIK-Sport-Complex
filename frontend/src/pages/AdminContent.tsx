import { useEffect, useRef, useState, useCallback } from "react";
import { Link, useLocation } from "react-router-dom";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { useAuth } from "../context/AuthContext";
import { api } from "../api";
import type { DraftFilterKind, DraftSummary } from "../types";

function formatDraftDate(isoString: string): string {
  try {
    const d = new Date(isoString);
    if (isNaN(d.getTime())) return isoString;
    return d.toLocaleString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
      hour12: true,
    });
  } catch {
    return isoString;
  }
}

export function AdminContent() {
  const location = useLocation();
  const { user, isLoading, checkError, retryCheckSession } = useAuth();

  const [publishedBanner, setPublishedBanner] = useState<string | null>(
    (location.state as { publishedSuccess?: string } | null)?.publishedSuccess ?? null
  );

  const [filter, setFilter] = useState<DraftFilterKind>("all");
  const [drafts, setDrafts] = useState<DraftSummary[]>([]);
  const [draftsLoading, setDraftsLoading] = useState<boolean>(false);
  const [hasFetched, setHasFetched] = useState<boolean>(false);
  const [draftsError, setDraftsError] = useState<string | null>(null);

  // Monotonic sequence counter and AbortController to cancel obsolete requests
  // and prevent stale / slow responses from an earlier filter replacing newer results.
  const reqSeqRef = useRef<number>(0);
  const abortControllerRef = useRef<AbortController | null>(null);

  const canManageContent = Boolean(
    user &&
      (user.roles.includes("sports_director") ||
        user.roles.includes("super_admin"))
  );

  const fetchDrafts = useCallback(
    async (selectedFilter: DraftFilterKind) => {
      // Cancel previous in-flight request
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
      const controller = new AbortController();
      abortControllerRef.current = controller;

      const seq = ++reqSeqRef.current;
      setDraftsLoading(true);
      setDraftsError(null);

      try {
        const data = await api.listDrafts(selectedFilter, {
          signal: controller.signal,
        });
        if (seq === reqSeqRef.current) {
          setDrafts(data);
          setDraftsError(null);
          setHasFetched(true);
        }
      } catch (err: unknown) {
        // Ignore aborted requests cleanly
        if (
          (err instanceof DOMException && err.name === "AbortError") ||
          (err instanceof Error && err.name === "AbortError")
        ) {
          return;
        }
        if (seq === reqSeqRef.current) {
          setDraftsError(
            err instanceof Error ? err.message : "Failed to load drafts"
          );
          setHasFetched(true);
        }
      } finally {
        if (seq === reqSeqRef.current) {
          setDraftsLoading(false);
        }
      }
    },
    []
  );

  useEffect(() => {
    // Only fetch drafts once the session check has completed and user has allowed role
    if (!isLoading && canManageContent) {
      fetchDrafts(filter);
    }
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, [isLoading, canManageContent, filter, fetchDrafts]);

  const handleFilterChange = (newFilter: DraftFilterKind) => {
    if (newFilter === filter) return;
    setFilter(newFilter);
  };

  const handleRetry = () => {
    fetchDrafts(filter);
  };

  return (
    <div className="flex min-h-screen flex-col bg-white">
      <Navbar />

      <main className="flex-1 py-10 px-6 max-w-6xl mx-auto w-full">
        {/* State 1: Initial Session Check in progress */}
        {isLoading && (
          <div
            role="status"
            aria-live="polite"
            className="flex flex-col items-center justify-center py-20 space-y-3"
          >
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--color-maroon)]" />
            <p className="text-sm text-[var(--color-navy-soft)]">
              Checking authorization...
            </p>
          </div>
        )}

        {/* State 2: Session check outage (server or network failure) */}
        {!isLoading && checkError && !user && (
          <div
            role="alert"
            className="max-w-xl mx-auto my-12 rounded-xl border border-amber-300 bg-amber-50 p-6 text-center"
          >
            <h2 className="text-lg font-bold text-amber-900">
              Session Check Unavailable
            </h2>
            <p className="mt-2 text-sm text-amber-800">
              We could not verify your session due to a server or network outage:{" "}
              <span className="font-medium">{checkError}</span>
            </p>
            <button
              onClick={() => retryCheckSession()}
              className="mt-4 inline-flex items-center px-4 py-2 rounded-md bg-[var(--color-maroon)] text-xs font-semibold text-white hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
            >
              Retry Session Check
            </button>
          </div>
        )}

        {/* State 3: Signed-out visitor */}
        {!isLoading && !checkError && !user && (
          <div
            role="region"
            aria-label="Sign In Required"
            className="max-w-xl mx-auto my-12 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center"
          >
            <h2 className="text-xl font-bold text-[var(--color-navy)]">
              Sign In Required
            </h2>
            <p className="mt-2 text-sm text-[var(--color-navy-soft)]">
              You must be signed in with a Sports Director or Administrator account
              to manage content drafts.
            </p>
            <div className="mt-6">
              <Link
                to="/login"
                className="inline-block px-5 py-2.5 rounded-md bg-[var(--color-maroon)] text-white text-xs font-semibold hover:bg-[var(--color-maroon-dark)] shadow-sm transition"
              >
                Sign in
              </Link>
            </div>
          </div>
        )}

        {/* State 4: Signed-in user without allowed role */}
        {!isLoading && user && !canManageContent && (
          <div
            role="alert"
            className="max-w-xl mx-auto my-12 rounded-xl border border-red-200 bg-red-50 p-8 text-center"
          >
            <h2 className="text-xl font-bold text-red-900">Access Denied</h2>
            <p className="mt-2 text-sm text-red-800">
              Your account ({user.displayName} &lt;{user.email}&gt;) does not
              have permission to view or manage content drafts. This area is
              reserved for Sports Directors and Super Administrators.
            </p>
            <div className="mt-6">
              <Link
                to="/"
                className="inline-block px-4 py-2 rounded-md border border-[var(--color-border)] bg-white text-xs font-semibold text-[var(--color-navy)] hover:bg-[var(--color-surface)]"
              >
                Return to Home
              </Link>
            </div>
          </div>
        )}

        {/* State 5: Authorized director or super admin */}
        {!isLoading && canManageContent && (
          <div className="space-y-6">
            {/* Published Success Announcement Banner */}
            {publishedBanner && (
              <div
                role="status"
                className="rounded-lg border border-emerald-300 bg-emerald-50 px-4 py-3 text-xs font-semibold text-emerald-800 flex items-center justify-between"
              >
                <span>{publishedBanner}</span>
                <button
                  type="button"
                  onClick={() => setPublishedBanner(null)}
                  className="text-emerald-700 hover:text-emerald-950 font-bold ml-4 cursor-pointer"
                  aria-label="Dismiss banner"
                >
                  &times;
                </button>
              </div>
            )}

            {/* Header with Title and 50-Item Cap Notice */}
            <div className="border-b border-[var(--color-border)] pb-5">
              <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
                <div>
                  <h1 className="text-2xl font-bold tracking-tight text-[var(--color-navy)] sm:text-3xl font-display">
                    Content Drafts
                  </h1>
                  <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
                    Showing up to 50 most recently updated drafts. Older drafts
                    or pagination are not supported yet.
                  </p>
                </div>
                <div>
                  <Link
                    to="/admin/content/new"
                    className="inline-flex items-center gap-1.5 px-4 py-2 rounded-md bg-[var(--color-maroon)] text-white text-xs font-semibold shadow-sm hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
                  >
                    + New draft
                  </Link>
                </div>
              </div>

              {/* Filter Tabs (All, Announcements, Achievements) */}
              <div className="mt-6 flex items-center gap-2 border-b border-[var(--color-border)] pb-2 overflow-x-auto">
                <button
                  type="button"
                  onClick={() => handleFilterChange("all")}
                  aria-pressed={filter === "all"}
                  className={`px-3.5 py-1.5 text-xs font-semibold rounded-md transition cursor-pointer ${
                    filter === "all"
                      ? "bg-[var(--color-maroon)] text-white shadow-sm"
                      : "text-[var(--color-navy-soft)] hover:bg-[var(--color-surface)] hover:text-[var(--color-navy)]"
                  }`}
                >
                  All
                </button>
                <button
                  type="button"
                  onClick={() => handleFilterChange("announcement")}
                  aria-pressed={filter === "announcement"}
                  className={`px-3.5 py-1.5 text-xs font-semibold rounded-md transition cursor-pointer ${
                    filter === "announcement"
                      ? "bg-[var(--color-maroon)] text-white shadow-sm"
                      : "text-[var(--color-navy-soft)] hover:bg-[var(--color-surface)] hover:text-[var(--color-navy)]"
                  }`}
                >
                  Announcements
                </button>
                <button
                  type="button"
                  onClick={() => handleFilterChange("achievement")}
                  aria-pressed={filter === "achievement"}
                  className={`px-3.5 py-1.5 text-xs font-semibold rounded-md transition cursor-pointer ${
                    filter === "achievement"
                      ? "bg-[var(--color-maroon)] text-white shadow-sm"
                      : "text-[var(--color-navy-soft)] hover:bg-[var(--color-surface)] hover:text-[var(--color-navy)]"
                  }`}
                >
                  Achievements
                </button>
              </div>
            </div>

            {/* Loading State for Drafts */}
            {(!hasFetched || draftsLoading) && !draftsError && (
              <div
                role="status"
                aria-live="polite"
                className="flex flex-col items-center justify-center py-16 space-y-3"
              >
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--color-maroon)]" />
                <p className="text-sm text-[var(--color-navy-soft)]">
                  Loading drafts...
                </p>
              </div>
            )}

            {/* Error State for Drafts */}
            {!draftsLoading && draftsError && (
              <div
                role="alert"
                className="rounded-lg border border-red-200 bg-red-50 p-6 text-center"
              >
                <p className="text-sm font-semibold text-red-800">
                  {draftsError}
                </p>
                <div className="mt-4">
                  <button
                    onClick={handleRetry}
                    className="px-4 py-2 bg-[var(--color-maroon)] text-white rounded-md text-xs font-semibold hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
                  >
                    Retry
                  </button>
                </div>
              </div>
            )}

            {/* Empty State */}
            {hasFetched && !draftsLoading && !draftsError && drafts.length === 0 && (
              <div
                data-testid="empty-drafts"
                className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-12 text-center"
              >
                <p className="text-base font-medium text-[var(--color-navy)]">
                  No drafts found.
                </p>
                <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
                  {filter === "all"
                    ? "There are currently no saved drafts."
                    : `There are currently no saved ${filter} drafts.`}
                </p>
              </div>
            )}

            {/* Populated Drafts Table / List */}
            {hasFetched && !draftsLoading && !draftsError && drafts.length > 0 && (
              <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-white shadow-xs">
                <table className="min-w-full divide-y divide-[var(--color-border)]">
                  <thead className="bg-[var(--color-surface)]">
                    <tr>
                      <th
                        scope="col"
                        className="py-3.5 pl-6 pr-3 text-left text-xs font-semibold text-[var(--color-navy)]"
                      >
                        Title
                      </th>
                      <th
                        scope="col"
                        className="px-3 py-3.5 text-left text-xs font-semibold text-[var(--color-navy)]"
                      >
                        Kind
                      </th>
                      <th
                        scope="col"
                        className="py-3.5 pl-3 pr-4 text-left text-xs font-semibold text-[var(--color-navy)]"
                      >
                        Last Updated
                      </th>
                      <th
                        scope="col"
                        className="py-3.5 pl-2 pr-6 text-right text-xs font-semibold text-[var(--color-navy)]"
                      >
                        <span className="sr-only">Actions</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[var(--color-border)]">
                    {drafts.map((draft) => {
                      const displayTitle =
                        draft.title && draft.title.trim().length > 0
                          ? draft.title
                          : "Untitled draft";
                      const isUntitled =
                        !draft.title || draft.title.trim().length === 0;

                      return (
                        <tr key={draft.id} className="hover:bg-neutral-50/50">
                          <td className="py-4 pl-6 pr-3 text-sm text-[var(--color-navy)]">
                            <Link
                              to={`/admin/content/${draft.id}`}
                              className="group block"
                            >
                              <span
                                className={
                                  isUntitled
                                    ? "italic text-neutral-400 group-hover:text-[var(--color-maroon)]"
                                    : "font-semibold text-[var(--color-navy)] group-hover:text-[var(--color-maroon)] group-hover:underline"
                                }
                              >
                                {displayTitle}
                              </span>
                            </Link>
                          </td>
                          <td className="px-3 py-4 text-xs whitespace-nowrap">
                            <span
                              className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium border ${
                                draft.kind === "announcement"
                                  ? "bg-blue-50 text-blue-700 border-blue-200"
                                  : "bg-amber-50 text-amber-800 border-amber-200"
                              }`}
                            >
                              {draft.kind === "announcement"
                                ? "Announcement"
                                : "Achievement"}
                            </span>
                          </td>
                          <td className="py-4 pl-3 pr-4 text-xs text-[var(--color-navy-soft)] whitespace-nowrap">
                            <time dateTime={draft.updatedAt}>
                              {formatDraftDate(draft.updatedAt)}
                            </time>
                          </td>
                          <td className="py-4 pl-2 pr-6 text-right text-xs whitespace-nowrap">
                            <Link
                              to={`/admin/content/${draft.id}`}
                              className="font-semibold text-[var(--color-maroon)] hover:underline"
                              aria-label={`Open draft ${displayTitle}`}
                            >
                              Open
                            </Link>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </main>

      <Footer />
    </div>
  );
}
