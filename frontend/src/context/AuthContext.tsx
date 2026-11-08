import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError, type LoginInput } from "../api";
import type { AuthUser } from "../types";

export interface AuthContextType {
  user: AuthUser | null;
  isLoading: boolean;
  checkError: string | null;
  logoutError: string | null;
  login: (input: LoginInput) => Promise<AuthUser>;
  logout: () => Promise<void>;
  retryCheckSession: () => Promise<void>;
  clearLogoutError: () => void;
}

const defaultAuthContext: AuthContextType = {
  user: null,
  isLoading: false,
  checkError: null,
  logoutError: null,
  login: async () => {
    throw new Error("AuthProvider not found in component tree");
  },
  logout: async () => {
    throw new Error("AuthProvider not found in component tree");
  },
  retryCheckSession: async () => {},
  clearLogoutError: () => {},
};

export const AuthContext = createContext<AuthContextType>(defaultAuthContext);

export interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [checkError, setCheckError] = useState<string | null>(null);
  const [logoutError, setLogoutError] = useState<string | null>(null);

  // Monotonic action sequence counter to guard against late /me responses
  // overwriting newer login, logout, or retry actions.
  const actionSeqRef = useRef<number>(0);

  const checkSession = useCallback(async () => {
    const currentSeq = ++actionSeqRef.current;
    setIsLoading(true);
    setCheckError(null);

    try {
      const authUser = await api.me();
      if (actionSeqRef.current !== currentSeq) {
        // Discard stale response if a newer action (login, logout, or retry) has occurred
        return;
      }
      setUser(authUser);
      setCheckError(null);
    } catch (err) {
      if (actionSeqRef.current !== currentSeq) {
        return;
      }
      if (err instanceof ApiError && err.status === 401) {
        // 401 Unauthorized strictly means signed out (no valid session cookie)
        setUser(null);
        setCheckError(null);
      } else {
        // Network or server failure: session could not be verified.
        // Keep public pages unblocked, record error for explicit retry.
        setUser(null);
        setCheckError(
          err instanceof Error
            ? err.message
            : "Unable to verify session with server."
        );
      }
    } finally {
      if (actionSeqRef.current === currentSeq) {
        setIsLoading(false);
      }
    }
  }, []);

  // Restore session on app startup
  useEffect(() => {
    checkSession();
  }, [checkSession]);

  const login = useCallback(async (input: LoginInput): Promise<AuthUser> => {
    const currentSeq = ++actionSeqRef.current;
    setCheckError(null);
    setLogoutError(null);

    try {
      const authenticatedUser = await api.login(input);
      if (actionSeqRef.current === currentSeq) {
        setUser(authenticatedUser);
        setIsLoading(false);
      }
      return authenticatedUser;
    } catch (err) {
      throw err;
    }
  }, []);

  const logout = useCallback(async (): Promise<void> => {
    const currentSeq = ++actionSeqRef.current;
    setLogoutError(null);

    try {
      await api.logout();
      if (actionSeqRef.current === currentSeq) {
        setUser(null);
        setLogoutError(null);
      }
    } catch (err) {
      // Logout failure: do NOT clear the user or claim the session was revoked.
      const errorMsg =
        err instanceof Error
          ? err.message
          : "Failed to sign out from server.";
      if (actionSeqRef.current === currentSeq) {
        setLogoutError(errorMsg);
      }
      throw err;
    }
  }, []);

  const clearLogoutError = useCallback(() => {
    setLogoutError(null);
  }, []);

  const value: AuthContextType = {
    user,
    isLoading,
    checkError,
    logoutError,
    login,
    logout,
    retryCheckSession: checkSession,
    clearLogoutError,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextType {
  return useContext(AuthContext);
}

