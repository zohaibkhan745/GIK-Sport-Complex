import { useEffect, useState, useCallback, useRef } from "react";
import { api } from "../api";
import type { PublicContentItem } from "../types";

function formatContentDate(isoString: string): string {
  try {
    const d = new Date(isoString);
    if (isNaN(d.getTime())) return isoString;
    return d.toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  } catch {
    return isoString;
  }
}

export function AnnouncementsSection() {
  const [items, setItems] = useState<PublicContentItem[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  const loadAnnouncements = useCallback(async () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    const controller = new AbortController();
    abortControllerRef.current = controller;

    setIsLoading(true);
    setError(null);

    try {
      const data = await api.getAnnouncements({ signal: controller.signal });
      setItems(data);
      setError(null);
    } catch (err: unknown) {
      if (
        (err instanceof DOMException && err.name === "AbortError") ||
        (err instanceof Error && err.name === "AbortError")
      ) {
        return;
      }
      setError(
        err instanceof Error
          ? err.message
          : "Failed to load announcements from server"
      );
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadAnnouncements();
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, [loadAnnouncements]);

  return (
    <section id="announcements" aria-labelledby="announcements-heading" className="space-y-4">
      <div className="border-b border-[var(--color-border)] pb-3">
        <h2
          id="announcements-heading"
          className="font-display text-xl font-bold tracking-tight text-[var(--color-navy)] sm:text-2xl"
        >
          Campus Announcements
        </h2>
        <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
          Official notices, court maintenance schedules, and operational updates.
        </p>
      </div>

      {/* Loading state */}
      {isLoading && (
        <div
          role="status"
          aria-live="polite"
          className="flex items-center justify-center py-10 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]"
        >
          <div className="flex items-center gap-3">
            <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-[var(--color-maroon)] border-t-transparent" />
            <span className="text-xs font-medium text-[var(--color-navy-soft)]">
              Loading announcements...
            </span>
          </div>
        </div>
      )}

      {/* Error state with retry */}
      {!isLoading && error && (
        <div
          role="alert"
          className="rounded-xl border border-red-200 bg-red-50 p-6 text-center space-y-3"
        >
          <p className="text-xs font-semibold text-red-800">{error}</p>
          <div>
            <button
              type="button"
              onClick={loadAnnouncements}
              className="px-3.5 py-1.5 rounded-md bg-[var(--color-maroon)] text-white text-xs font-semibold hover:bg-[var(--color-maroon-dark)] transition cursor-pointer"
            >
              Retry
            </button>
          </div>
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && items.length === 0 && (
        <div
          role="region"
          aria-label="No announcements"
          className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center"
        >
          <p className="text-xs text-[var(--color-navy-soft)]">
            No announcements published yet. Check back soon for campus sports updates.
          </p>
        </div>
      )}

      {/* Populated items */}
      {!isLoading && !error && items.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2">
          {items.map((item) => (
            <article
              key={item.id}
              className="rounded-xl border border-[var(--color-border)] bg-white p-5 shadow-xs flex flex-col justify-between"
            >
              <div>
                <div className="flex items-center justify-between gap-2 mb-2">
                  <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-semibold bg-blue-50 text-blue-700 border border-blue-200">
                    Announcement
                  </span>
                  <time
                    dateTime={item.publishedAt}
                    className="text-[11px] text-[var(--color-navy-soft)] font-medium"
                  >
                    {formatContentDate(item.publishedAt)}
                  </time>
                </div>
                <h3 className="font-display font-semibold text-base text-[var(--color-navy)] leading-snug">
                  {item.title}
                </h3>
                <p className="mt-2 text-xs leading-relaxed text-[var(--color-navy-soft)] whitespace-pre-line">
                  {item.content}
                </p>
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}

