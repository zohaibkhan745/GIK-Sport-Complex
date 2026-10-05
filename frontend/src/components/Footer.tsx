import { Mail } from "lucide-react";

export function Footer() {
  return (
    <footer className="mt-10 flex flex-col items-center justify-between gap-3 border-t border-[var(--color-border)] pt-5 text-xs text-[var(--color-navy-soft)] sm:flex-row">
      <p>GIKI Sports Complex, Topi, Khyber Pakhtunkhwa</p>
      <div className="flex items-center gap-4">
        <a href="https://instagram.com" className="hover:text-[var(--color-maroon)]">
          Instagram
        </a>
        <a href="https://facebook.com" className="hover:text-[var(--color-maroon)]">
          Facebook
        </a>
        <a
          href="mailto:sports@giki.edu.pk"
          className="flex items-center gap-1 hover:text-[var(--color-maroon)]"
        >
          <Mail size={14} /> Email
        </a>
      </div>
    </footer>
  );
}
