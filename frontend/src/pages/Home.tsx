import { useEffect, useState } from "react";
import { Navbar } from "../components/Navbar";
import { Hero } from "../components/Hero";
import { StatsStrip } from "../components/StatsStrip";
import { AchievementsFeed } from "../components/AchievementsFeed";
import { FacilitiesGrid } from "../components/FacilitiesGrid";
import { Footer } from "../components/Footer";
import { FACILITIES } from "../data/facilities";
import { api } from "../api";
import type { Facility } from "../types";

export function Home() {
  // Starts with the local seed data so the page is never empty, then
  // swaps in live data from the Go backend if/when it responds.
  const [facilities, setFacilities] = useState<Facility[]>(FACILITIES);

  useEffect(() => {
    let cancelled = false;
    api
      .getFacilities()
      .then((data) => {
        if (!cancelled && data.length > 0) setFacilities(data);
      })
      .catch(() => {
        // Backend not reachable yet — keep showing the local seed data.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="min-h-screen bg-white">
      <Navbar />
      <main className="mx-auto flex max-w-6xl flex-col gap-10 px-6 py-8">
        <Hero />
        <StatsStrip />
        <AchievementsFeed />
        <FacilitiesGrid facilities={facilities} />
      </main>
      <div className="mx-auto max-w-6xl px-6 pb-8">
        <Footer />
      </div>
    </div>
  );
}
