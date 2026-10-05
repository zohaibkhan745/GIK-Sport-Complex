import { useState } from "react";
import { Link } from "react-router-dom";
import { Menu, X } from "lucide-react";

export function Navbar() {
  const [open, setOpen] = useState(false);

  const links = [
    { label: "Facilities", href: "/#facilities" },
    { label: "Achievements", href: "/#achievements" },
    { label: "Book a slot", href: "/#facilities" },
  ];

  return (
    <header className="sticky top-0 z-30 border-b border-[var(--color-border)] bg-white/90 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
        <Link to="/" className="flex items-center gap-2.5">
          <img src="/giki-logo.png" alt="GIKI crest" className="h-9 w-9 object-contain" />
          <span className="font-display text-[15px] font-semibold tracking-wide text-[var(--color-navy)]">
            GIKI Sports Complex
          </span>
        </Link>

        <nav className="hidden items-center gap-7 md:flex">
          {links.map((l) => (
            <a
              key={l.label}
              href={l.href}
              className="text-sm text-[var(--color-navy-soft)] transition-colors hover:text-[var(--color-maroon)]"
            >
              {l.label}
            </a>
          ))}
        </nav>

        <button
          className="rounded-md p-2 text-[var(--color-navy)] md:hidden"
          onClick={() => setOpen((o) => !o)}
          aria-label={open ? "Close menu" : "Open menu"}
          aria-expanded={open}
        >
          {open ? <X size={22} /> : <Menu size={22} />}
        </button>
      </div>

      {open && (
        <nav className="flex flex-col gap-1 border-t border-[var(--color-border)] px-6 py-3 md:hidden">
          {links.map((l) => (
            <a
              key={l.label}
              href={l.href}
              onClick={() => setOpen(false)}
              className="py-2 text-sm text-[var(--color-navy-soft)]"
            >
              {l.label}
            </a>
          ))}
        </nav>
      )}
    </header>
  );
}
