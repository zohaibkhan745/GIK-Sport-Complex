export function Hero() {
  return (
    <section className="rounded-2xl border border-[var(--color-gold)] bg-[var(--color-maroon)] px-6 py-14 text-center sm:py-16">
      <img
        src="/giki-logo.png"
        alt="GIKI crest"
        className="mx-auto mb-4 h-16 w-16 object-contain"
      />
      <p className="text-xs tracking-wide text-[var(--color-gold-light)]">
        GIKI Institute
      </p>
      <h1 className="font-display mt-2 text-3xl font-semibold text-[#F3E6C4] sm:text-4xl">
        GIKI Sports Complex
      </h1>
      <p className="mx-auto mt-3 max-w-md text-sm text-[var(--color-gold-light)]">
        Where champions train, on and off the field
      </p>
      <div className="mt-6 flex justify-center gap-3">
        <a
          href="#facilities"
          className="rounded-md bg-[var(--color-gold)] px-5 py-2.5 text-sm font-semibold text-[var(--color-navy)] transition-opacity hover:opacity-90"
        >
          Book a slot
        </a>
        <a
          href="#facilities"
          className="rounded-md border border-[var(--color-gold)] px-5 py-2.5 text-sm text-[#F3E6C4] transition-colors hover:bg-white/5"
        >
          View schedule
        </a>
      </div>
    </section>
  );
}
