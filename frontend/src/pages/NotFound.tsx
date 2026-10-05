import { Link } from "react-router-dom";

export function NotFound() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-white px-6 text-center">
      <p className="font-display text-xl font-semibold text-[var(--color-navy)]">
        Page not found
      </p>
      <p className="text-sm text-[var(--color-navy-soft)]">
        That facility doesn't exist, or the link is out of date.
      </p>
      <Link
        to="/"
        className="mt-2 rounded-md bg-[var(--color-gold)] px-4 py-2 text-sm font-semibold text-[var(--color-navy)]"
      >
        Back to home
      </Link>
    </div>
  );
}
