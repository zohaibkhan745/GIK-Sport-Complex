import { useState } from "react";
import { Link } from "react-router-dom";
import { Menu, X } from "lucide-react";
import { useAuth } from "../context/AuthContext";

export function Navbar() {
  const [open, setOpen] = useState(false);
  const [isLoggingOut, setIsLoggingOut] = useState(false);
  const {
    user,
    checkError,
    logoutError,
    logout,
    retryCheckSession,
    clearLogoutError,
  } = useAuth();

  const links = [
    { label: "Facilities", href: "/#facilities" },
    { label: "Achievements", href: "/#achievements" },
    { label: "Book a slot", href: "/#facilities" },
  ];

  const handleLogout = async () => {
    setIsLoggingOut(true);
    clearLogoutError();
    try {
      await logout();
    } catch {
      // logoutError is recorded in AuthContext and displayed
    } finally {
      setIsLoggingOut(false);
    }
  };

  return (
    <header className="sticky top-0 z-30 border-b border-[var(--color-border)] bg-white/90 backdrop-blur">
      {logoutError && (
        <div className="bg-red-50 border-b border-red-200 px-6 py-2 text-xs text-red-800 flex items-center justify-between">
          <span>Sign out failed: {logoutError}</span>
          <div className="flex items-center gap-3">
            <button
              onClick={handleLogout}
              disabled={isLoggingOut}
              className="font-semibold text-red-900 underline cursor-pointer hover:text-red-950"
            >
              {isLoggingOut ? "Retrying..." : "Retry Sign Out"}
            </button>
            <button
              onClick={clearLogoutError}
              className="text-red-700 hover:text-red-900 font-bold"
              aria-label="Dismiss sign out error"
            >
              &times;
            </button>
          </div>
        </div>
      )}

      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
        <Link to="/" className="flex items-center gap-2.5">
          <img src="/giki-logo.png" alt="GIKI crest" className="h-9 w-9 object-contain" />
          <span className="font-display text-[15px] font-semibold tracking-wide text-[var(--color-navy)]">
            GIKI Sports Complex
          </span>
        </Link>

        {/* Desktop Navigation */}
        <nav className="hidden items-center gap-6 md:flex">
          {links.map((l) => (
            <a
              key={l.label}
              href={l.href}
              className="text-sm text-[var(--color-navy-soft)] transition-colors hover:text-[var(--color-maroon)]"
            >
              {l.label}
            </a>
          ))}

          {/* Session check retry badge when server or network is unreachable */}
          {checkError && (
            <div className="flex items-center gap-1.5 rounded bg-amber-50 px-2.5 py-1 text-xs text-amber-800 border border-amber-200">
              <span>Session check failed</span>
              <button
                onClick={() => retryCheckSession()}
                className="font-medium text-[var(--color-maroon)] underline cursor-pointer hover:text-[var(--color-maroon-dark)]"
              >
                Retry
              </button>
            </div>
          )}

          {/* User authentication actions */}
          {user ? (
            <div className="flex items-center gap-3 pl-2 border-l border-[var(--color-border)]">
              <span className="text-sm font-semibold text-[var(--color-navy)]">
                {user.displayName}
              </span>
              <button
                onClick={handleLogout}
                disabled={isLoggingOut}
                className="rounded-md border border-[var(--color-border)] bg-white px-3 py-1.5 text-xs font-semibold text-[var(--color-navy)] transition hover:bg-[var(--color-surface)] hover:text-[var(--color-maroon)] focus-visible:outline-2 focus-visible:outline-[var(--color-gold)] disabled:opacity-50 cursor-pointer"
              >
                {isLoggingOut ? "Signing out..." : "Sign Out"}
              </button>
            </div>
          ) : (
            <Link
              to="/login"
              className="rounded-md bg-[var(--color-maroon)] px-3.5 py-1.5 text-xs font-semibold text-white shadow-sm transition hover:bg-[var(--color-maroon-dark)] focus-visible:outline-2 focus-visible:outline-[var(--color-gold)]"
            >
              Sign In
            </Link>
          )}
        </nav>

        {/* Mobile menu trigger */}
        <button
          className="rounded-md p-2 text-[var(--color-navy)] md:hidden cursor-pointer"
          onClick={() => setOpen((o) => !o)}
          aria-label={open ? "Close menu" : "Open menu"}
          aria-expanded={open}
        >
          {open ? <X size={22} /> : <Menu size={22} />}
        </button>
      </div>

      {/* Mobile Navigation */}
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

          <div className="border-t border-[var(--color-border)] pt-3 mt-1 flex flex-col gap-2">
            {checkError && (
              <div className="flex items-center justify-between py-1 text-xs text-amber-800">
                <span>Session check failed</span>
                <button
                  onClick={() => retryCheckSession()}
                  className="font-medium text-[var(--color-maroon)] underline"
                >
                  Retry
                </button>
              </div>
            )}

            {user ? (
              <div className="flex flex-col gap-2">
                <span className="text-sm font-semibold text-[var(--color-navy)]">
                  {user.displayName}
                </span>
                <button
                  onClick={async () => {
                    setOpen(false);
                    await handleLogout();
                  }}
                  disabled={isLoggingOut}
                  className="text-left text-xs font-semibold text-[var(--color-maroon)] hover:underline"
                >
                  {isLoggingOut ? "Signing out..." : "Sign Out"}
                </button>
              </div>
            ) : (
              <Link
                to="/login"
                onClick={() => setOpen(false)}
                className="inline-block py-1 text-sm font-semibold text-[var(--color-maroon)]"
              >
                Sign In
              </Link>
            )}
          </div>
        </nav>
      )}
    </header>
  );
}
