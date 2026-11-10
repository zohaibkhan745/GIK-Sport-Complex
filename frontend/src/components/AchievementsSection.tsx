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

export function AchievementsSection() {
  const [items, setItems] = useState<PublicContentItem[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  const loadAchievements = useCallback(async () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    const controller = new AbortController();
    abortControllerRef.current = controller;

    setIsLoading(true);
    setError(null);

    try {
      const data = await api.getAchievementsFeed({ signal: controller.signal });
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
          : "Failed to load achievements from server"
      );
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadAchievements();
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, [loadAchievements]);

  return (
    <section id="achievements" aria-labelledby="achievements-heading" className="space-y-4">
      <div className="border-b border-[var(--color-border)] pb-3">
        <h2
          id="achievements-heading"
          className="font-display text-xl font-bold tracking-tight text-[var(--color-navy)] sm:text-2xl"
        >
          Sports Achievements
        </h2>
        <p className="mt-1 text-xs text-[var(--color-navy-soft)]">
          Recent tournament victories, championship trophies, and campus athletic honors.
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
              Loading achievements...
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
              onClick={loadAchievements}
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
          aria-label="No achievements"
          className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center"
        >
          <p className="text-xs text-[var(--color-navy-soft)]">
            No sports achievements published yet. Recent match achievements will appear here.
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
                  <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-semibold bg-amber-50 text-amber-800 border border-amber-200">
                    Achievement
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

