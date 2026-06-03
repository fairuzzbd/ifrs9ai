import { create } from "zustand";
import { persist } from "zustand/middleware";

export interface UserProfile {
  sub: string;
  preferred_username: string;
  roles: string[];
  permissions: string[];
  tenant_id: string;
  mfa_verified: boolean;
  mfa_method?: string;
}

interface AuthState {
  token: string | null;
  user: UserProfile | null;
  setAuth: (token: string, user: UserProfile) => void;
  clearAuth: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      setAuth: (token, user) => {
        // Sync token to localStorage for baseFetch
        if (typeof window !== "undefined") {
          localStorage.setItem("blips_token", token);
        }
        set({ token, user });
      },
      clearAuth: () => {
        if (typeof window !== "undefined") {
          localStorage.removeItem("blips_token");
        }
        set({ token: null, user: null });
      },
    }),
    {
      name: "blips-auth",
      partialize: (state) => ({ token: state.token, user: state.user }),
    },
  ),
);

// ---------------------------------------------------------------------------
// Permission helpers
// ---------------------------------------------------------------------------

export function usePermissions() {
  const user = useAuthStore((s) => s.user);

  const can = (permission: string): boolean => {
    if (!user) return false;
    return user.permissions.includes(permission);
  };

  const hasRole = (role: string): boolean => {
    if (!user) return false;
    return user.roles.includes(role);
  };

  const isAuditRole = (): boolean => hasRole("ROLE-AUDIT");

  const canCreate = (entity: string) => can(`${entity}.create`);
  const canRead = (entity: string) => can(`${entity}.read`);
  const canUpdate = (entity: string) => can(`${entity}.update`);
  const canDelete = (entity: string) => can(`${entity}.delete`);
  const canReview = (entity: string) => can(`${entity}.review`);
  const canApprove = (entity: string) => can(`${entity}.approve`);
  const canSubmit = (entity: string) => can(`${entity}.submit`);
  const canExport = (entity: string) => can(`${entity}.read`);

  return {
    can,
    hasRole,
    isAuditRole,
    canCreate,
    canRead,
    canUpdate,
    canDelete,
    canReview,
    canApprove,
    canSubmit,
    canExport,
    userId: user?.sub ?? null,
    username: user?.preferred_username ?? null,
    mfaVerified: user?.mfa_verified ?? false,
  };
}
