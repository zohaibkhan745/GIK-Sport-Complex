import { useCallback, useEffect, useState } from "react";
import { Navbar } from "../components/Navbar";
import { Hero } from "../components/Hero";
import { StatsStrip } from "../components/StatsStrip";
import { AchievementsFeed } from "../components/AchievementsFeed";
import { FacilitiesGrid } from "../components/FacilitiesGrid";
import { Footer } from "../components/Footer";
import { api } from "../api";
import type { Facility } from "../types";

export function Home() {
  const [facilities, setFacilities] = useState<Facility[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  const loadFacilities = useCallback(() => {
    setIsLoading(true);
    setError(null);

    const controller = new AbortController();

    api
      .getFacilities({ signal: controller.signal })
      .then((data) => {
        setFacilities(data);
        setIsLoading(false);
      })
      .catch((err) => {
        if (err.name === "AbortError") return;
        setError(
          err instanceof Error
            ? err.message
            : "Failed to load facilities from server"
        );
        setIsLoading(false);
      });

    return () => {
      controller.abort();
    };
  }, []);

  useEffect(() => {
    const cancel = loadFacilities();
    return cancel;
  }, [loadFacilities]);

  return (
    <div className="min-h-screen bg-white">
      <Navbar />
      <main className="mx-auto flex max-w-6xl flex-col gap-10 px-6 py-8">
        <Hero />
        <StatsStrip
          facilityCount={!isLoading && !error ? facilities.length : undefined}
        />
        <AchievementsFeed />
        <FacilitiesGrid
          facilities={facilities}
          isLoading={isLoading}
          error={error}
          onRetry={loadFacilities}
        />
      </main>
      <div className="mx-auto max-w-6xl px-6 pb-8">
        <Footer />
      </div>
    </div>
  );
}
