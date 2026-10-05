import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, Image as ImageIcon } from "lucide-react";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";
import { FACILITY_ICONS, META_ICONS } from "../components/icons";
import { getFacilityBySlug } from "../data/facilities";
import { api } from "../api";
import type { Facility } from "../types";
import { NotFound } from "./NotFound";

export function FacilityDetail() {
  const { slug } = useParams<{ slug: string }>();
  const seed = slug ? getFacilityBySlug(slug) : undefined;
  const [facility, setFacility] = useState<Facility | undefined>(seed);

  useEffect(() => {
    if (!slug) return;
    api
      .getFacility(slug)
      .then((data) => setFacility(data))
      .catch(() => {
        // Backend not reachable yet — keep showing the local seed data.
      });
  }, [slug]);

  if (!facility) return <NotFound />;

  const Icon = FACILITY_ICONS[facility.icon];

  return (
    <div className="min-h-screen bg-white">
      <Navbar />
      <main className="mx-auto max-w-3xl px-6 py-8">
        <Link
          to="/#facilities"
          className="flex items-center gap-1.5 text-sm text-[var(--color-maroon)]"
        >
          <ArrowLeft size={16} />
          Back to facilities
        </Link>

        <div className="mt-4 flex items-center gap-4 rounded-2xl border border-[var(--color-gold)] bg-[var(--color-maroon)] p-6">
          <span className="flex h-13 w-13 flex-shrink-0 items-center justify-center rounded-xl bg-[var(--color-navy)] p-3">
            <Icon size={24} className="text-[var(--color-gold-light)]" />
          </span>
          <div>
            <h1 className="font-display text-xl font-semibold text-[#F3E6C4]">
              {facility.name}
            </h1>
            <p className="mt-0.5 text-sm text-[var(--color-gold-light)]">
              {facility.description}
            </p>
          </div>
        </div>

        <div className="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-3">
          {facility.meta.map((m) => {
            const MetaIcon = META_ICONS[m.icon];
            return (
              <div
                key={m.label}
                className="rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-3.5 text-center"
              >
                <MetaIcon size={16} className="mx-auto text-[var(--color-maroon)]" />
                <p className="mt-1.5 text-xs text-[var(--color-navy-soft)]">
                  {m.label}
                </p>
              </div>
            );
          })}
        </div>

        <section className="mt-6">
          <h2 className="font-display text-sm font-semibold text-[var(--color-navy)]">
            Weekly schedule
          </h2>
          <div className="mt-2.5 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-1">
            {facility.schedule.map((row, i) => (
              <div
                key={row.days}
                className={`flex items-center justify-between py-2.5 text-sm ${
                  i < facility.schedule.length - 1
                    ? "border-b border-[var(--color-border)]"
                    : ""
                }`}
              >
                <span className="text-[var(--color-navy)]">{row.days}</span>
                <span className="text-[var(--color-navy-soft)]">{row.activity}</span>
              </div>
            ))}
          </div>
        </section>

        {facility.coach && (
          <section className="mt-6">
            <h2 className="font-display text-sm font-semibold text-[var(--color-navy)]">
              Coach
            </h2>
            <div className="mt-2.5 flex items-center gap-3 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
              <span className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-[var(--color-maroon)] text-sm font-semibold text-[var(--color-gold-light)]">
                {facility.coach.initials}
              </span>
              <div>
                <p className="text-sm font-semibold text-[var(--color-navy)]">
                  {facility.coach.name}
                </p>
                <p className="text-xs text-[var(--color-navy-soft)]">
                  {facility.coach.title}
                </p>
              </div>
            </div>
          </section>
        )}

        <section className="mt-6">
          <h2 className="font-display text-sm font-semibold text-[var(--color-navy)]">
            Gallery
          </h2>
          <div className="mt-2.5 grid grid-cols-3 gap-2">
            {Array.from({ length: facility.galleryCount }).map((_, i) => (
              <div
                key={i}
                className="flex aspect-square items-center justify-center rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]"
              >
                <ImageIcon size={20} className="text-[var(--color-border)]" />
              </div>
            ))}
          </div>
        </section>

        <button
          type="button"
          className="mt-6 w-full rounded-md bg-[var(--color-gold)] py-3 text-sm font-semibold text-[var(--color-navy)] transition-opacity hover:opacity-90"
        >
          Book a session
        </button>
      </main>
      <div className="mx-auto max-w-3xl px-6 pb-8">
        <Footer />
      </div>
    </div>
  );
}
