"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { usePathname } from "next/navigation";

import { clearLegacyPrivateBrowserState } from "@/lib/localforage-storage";
import { useConfigStore } from "@/stores/use-config-store";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";
import { useUserStore } from "@/stores/use-user-store";

export function ClientRootInit({ children }: { children: ReactNode }) {
    const pathname = usePathname();
    const hydrateUser = useUserStore((state) => state.hydrateUser);
    const loadPublicSettings = useConfigStore((state) => state.loadPublicSettings);
    const loadLocalProfile = useConfigStore((state) => state.loadLocalProfile);
    const clearLocalProfile = useConfigStore((state) => state.clearLocalProfile);
    const refreshLocalWorkspace = useLocalWorkspaceStore((state) => state.refresh);
    const isLoginPage = pathname === "/login" || pathname === "/admin/login";

    useEffect(() => {
        void loadPublicSettings();
    }, [loadPublicSettings]);

    useEffect(() => {
        void clearLegacyPrivateBrowserState();
    }, []);

    useEffect(() => {
        if (!isLoginPage) void hydrateUser();
    }, [hydrateUser, isLoginPage]);

    useEffect(() => {
        if (!isLoginPage) {
            void (async () => {
                try {
                    const workspace = await refreshLocalWorkspace();
                    const baseUrl = useLocalWorkspaceStore.getState().baseUrl;
                    if (workspace) await loadLocalProfile(baseUrl, workspace.defaultProfileId);
                    else clearLocalProfile();
                } catch {
                    clearLocalProfile();
                }
            })();
        }
    }, [refreshLocalWorkspace, loadLocalProfile, clearLocalProfile, isLoginPage]);

    return <>{children}</>;
}
