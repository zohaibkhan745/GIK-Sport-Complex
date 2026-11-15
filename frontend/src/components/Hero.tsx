import { Link } from "react-router-dom";
import { ArrowDown, Calendar, Dumbbell, Activity } from "lucide-react";

export function Hero() {
  return (
    <section className="relative overflow-hidden rounded-2xl border border-[var(--color-gold)] bg-radial-[at_top] from-[#8B1E2D] to-[var(--color-maroon)] px-6 py-12 text-center sm:py-16 shadow-sm">
      {/* Subtle background ambient pattern */}
      <div
        className="pointer-events-none absolute inset-0 opacity-10"
        style={{
          backgroundImage:
            "radial-gradient(#F3E6C4 1px, transparent 1px), radial-gradient(#F3E6C4 1px, transparent 1px)",
          backgroundSize: "24px 24px",
          backgroundPosition: "0 0, 12px 12px",
        }}
        aria-hidden="true"
      />

      <div className="relative z-10 mx-auto max-w-2xl">
        <img
          src="/giki-logo.png"
          alt="GIKI crest"
          className="mx-auto mb-3.5 h-16 w-16 object-contain drop-shadow-sm"
        />
        <p className="text-xs font-medium uppercase tracking-widest text-[var(--color-gold-light)]">
          Ghulam Ishaq Khan Institute
        </p>
        <h1 className="font-display mt-2 text-3xl font-bold tracking-tight text-[#F3E6C4] sm:text-4xl">
          GIKI Sports Complex
        </h1>
        <p className="mx-auto mt-2 text-sm text-[var(--color-gold-light)]">
          Campus athletics, fitness facilities, and weekly timetables.
        </p>

        {/* Primary Action Button */}
        <div className="mt-5 flex items-center justify-center">
          <a
            href="#facilities"
            className="inline-flex items-center gap-2 rounded-xl bg-[var(--color-gold)] px-5 py-2 text-xs font-semibold text-[var(--color-navy)] shadow-sm transition-all hover:brightness-105 active:scale-98"
          >
            Explore Facilities
            <ArrowDown size={13} />
          </a>
        </div>

        {/* Quick Facility Access Links */}
        <div className="mt-7 border-t border-[var(--color-gold)]/20 pt-4">
          <div className="flex flex-wrap items-center justify-center gap-2">
            <Link
              to="/padel"
              className="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-gold)]/30 bg-black/20 px-3 py-1 text-xs font-medium text-[#F3E6C4] transition-colors hover:bg-black/35 hover:border-[var(--color-gold)]"
            >
              <Activity size={12} className="text-[var(--color-gold)]" />
              <span>Padel Booking</span>
            </Link>
            <Link
              to="/membership"
              className="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-gold)]/30 bg-black/20 px-3 py-1 text-xs font-medium text-[#F3E6C4] transition-colors hover:bg-black/35 hover:border-[var(--color-gold)]"
            >
              <Dumbbell size={12} className="text-[var(--color-gold)]" />
              <span>Gym Pass</span>
            </Link>
            <a
              href="#facilities"
              className="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-gold)]/30 bg-black/20 px-3 py-1 text-xs font-medium text-[#F3E6C4] transition-colors hover:bg-black/35 hover:border-[var(--color-gold)]"
            >
              <Calendar size={12} className="text-[var(--color-gold)]" />
              <span>Timetables</span>
            </a>
          </div>
        </div>
      </div>
    </section>
  );
}
