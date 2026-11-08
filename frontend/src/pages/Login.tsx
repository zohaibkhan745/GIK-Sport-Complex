import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ArrowLeft, AlertCircle, ShieldCheck } from "lucide-react";
import { useAuth } from "../context/AuthContext";
import { ApiError } from "../api";
import { Navbar } from "../components/Navbar";
import { Footer } from "../components/Footer";

export function Login() {
  const navigate = useNavigate();
  const { user, login, logout } = useAuth();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [validationErrors, setValidationErrors] = useState<{
    email?: string;
    password?: string;
  }>({});
  const [serverError, setServerError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (isSubmitting) return;

    const errors: { email?: string; password?: string } = {};
    const trimmedEmail = email.trim();

    // Check actual API input limits
    if (!trimmedEmail) {
      errors.email = "Email address is required";
    } else if (trimmedEmail.length > 255) {
      errors.email = "Email address exceeds maximum allowable length (255 characters)";
    }

    // Preserve password bytes exactly as entered — DO NOT TRIM
    if (!password) {
      errors.password = "Password is required";
    } else if (password.length > 4096) {
      errors.password = "Password exceeds maximum allowable length";
    }

    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors);
      return;
    }

    setValidationErrors({});
    setServerError(null);
    setIsSubmitting(true);

    try {
      await login({
        email: trimmedEmail,
        password: password,
      });
      // Redirect to public homepage upon successful sign in
      navigate("/", { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          setServerError("Invalid email or password. Please check your credentials.");
        } else if (err.status === 429) {
          setServerError("Too many login attempts. Please wait a moment before trying again.");
        } else if (err.status === 503) {
          setServerError("Authentication service is busy or temporarily unavailable. Please retry shortly.");
        } else {
          setServerError(err.message || "Sign in failed due to a server error. Please try again.");
        }
      } else {
        setServerError("Network error: unable to reach the server. Please check your connection.");
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-white flex flex-col justify-between">
      <Navbar />

      <main className="mx-auto flex w-full max-w-md flex-col px-6 py-12">
        <div className="rounded-2xl border border-[var(--color-border)] bg-white p-6 sm:p-8 shadow-sm">
          {/* Header Branding */}
          <div className="text-center mb-6">
            <img
              src="/giki-logo.png"
              alt="GIKI crest"
              className="h-12 w-12 object-contain mx-auto mb-3"
            />
            <h1 className="font-display text-2xl font-bold tracking-wide text-[var(--color-navy)]">
              Sign In
            </h1>
            <p className="text-xs text-[var(--color-navy-soft)] mt-1">
              GIKI Sports Complex Management System
            </p>
          </div>

          {user ? (
            /* Already authenticated state */
            <div className="space-y-4">
              <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
                <ShieldCheck className="h-6 w-6 text-[var(--color-maroon)] mx-auto mb-1.5" />
                <p className="text-sm font-semibold text-[var(--color-navy)]">
                  Already Signed In
                </p>
                <p className="text-xs text-[var(--color-navy-soft)] mt-0.5">
                  Signed in as <span className="font-medium text-[var(--color-navy)]">{user.displayName}</span> ({user.email})
                </p>
                <div className="mt-2 flex flex-wrap justify-center gap-1">
                  {user.roles.map((r) => (
                    <span
                      key={r}
                      className="rounded bg-white px-2 py-0.5 text-[11px] font-medium text-[var(--color-navy-soft)] border border-[var(--color-border)]"
                    >
                      {r}
                    </span>
                  ))}
                </div>
              </div>

              <div className="flex gap-2">
                <Link
                  to="/"
                  className="flex-1 rounded-lg bg-[var(--color-maroon)] py-2 text-center text-xs font-semibold text-white transition hover:bg-[var(--color-maroon-dark)]"
                >
                  Return to Home
                </Link>
                <button
                  type="button"
                  onClick={() => logout()}
                  className="rounded-lg border border-[var(--color-border)] bg-white px-3 py-2 text-xs font-semibold text-[var(--color-navy)] transition hover:bg-gray-50 hover:text-[var(--color-maroon)] cursor-pointer"
                >
                  Sign Out
                </button>
              </div>
            </div>
          ) : (
            /* Sign in form */
            <form onSubmit={handleSubmit} noValidate className="space-y-4">
              {serverError && (
                <div
                  className="rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-800 flex items-start gap-2"
                  role="alert"
                >
                  <AlertCircle className="h-4 w-4 text-red-600 shrink-0 mt-0.5" />
                  <span>{serverError}</span>
                </div>
              )}

              {/* Email Field */}
              <div>
                <label
                  htmlFor="login-email"
                  className="block text-xs font-semibold uppercase tracking-wider text-[var(--color-navy-soft)] mb-1"
                >
                  Email address
                </label>
                <input
                  id="login-email"
                  name="email"
                  type="email"
                  autoComplete="username"
                  placeholder="e.g. director@example.test"
                  required
                  maxLength={255}
                  value={email}
                  onChange={(e) => {
                    setEmail(e.target.value);
                    if (validationErrors.email) {
                      setValidationErrors((prev) => ({ ...prev, email: undefined }));
                    }
                  }}
                  disabled={isSubmitting}
                  className="w-full rounded-lg border border-[var(--color-border)] bg-white px-3.5 py-2.5 text-sm text-[var(--color-navy)] placeholder:text-gray-400 focus:border-[var(--color-gold)] focus:outline-none focus:ring-1 focus:ring-[var(--color-gold)] disabled:bg-gray-50 disabled:text-gray-500"
                />
                {validationErrors.email && (
                  <p className="mt-1 text-xs text-red-600" role="alert">
                    {validationErrors.email}
                  </p>
                )}
              </div>

              {/* Password Field */}
              <div>
                <label
                  htmlFor="login-password"
                  className="block text-xs font-semibold uppercase tracking-wider text-[var(--color-navy-soft)] mb-1"
                >
                  Password
                </label>
                <input
                  id="login-password"
                  name="password"
                  type="password"
                  autoComplete="current-password"
                  placeholder="Enter your password"
                  required
                  maxLength={4096}
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value);
                    if (validationErrors.password) {
                      setValidationErrors((prev) => ({ ...prev, password: undefined }));
                    }
                  }}
                  disabled={isSubmitting}
                  className="w-full rounded-lg border border-[var(--color-border)] bg-white px-3.5 py-2.5 text-sm text-[var(--color-navy)] placeholder:text-gray-400 focus:border-[var(--color-gold)] focus:outline-none focus:ring-1 focus:ring-[var(--color-gold)] disabled:bg-gray-50 disabled:text-gray-500"
                />
                {validationErrors.password && (
                  <p className="mt-1 text-xs text-red-600" role="alert">
                    {validationErrors.password}
                  </p>
                )}
              </div>

              {/* Submit Button */}
              <button
                type="submit"
                disabled={isSubmitting}
                className="w-full rounded-lg bg-[var(--color-maroon)] px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-[var(--color-maroon-dark)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--color-gold)] disabled:cursor-not-allowed disabled:opacity-60 cursor-pointer"
              >
                {isSubmitting ? "Signing in..." : "Sign In"}
              </button>
            </form>
          )}

          {/* Back Navigation */}
          <div className="text-center mt-6 pt-4 border-t border-[var(--color-border)]">
            <Link
              to="/"
              className="text-xs text-[var(--color-navy-soft)] hover:text-[var(--color-maroon)] hover:underline inline-flex items-center gap-1 font-medium"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              Back to Sports Complex Home
            </Link>
          </div>
        </div>
      </main>

      <div className="mx-auto max-w-6xl px-6 pb-8 w-full">
        <Footer />
      </div>
    </div>
  );
}

