"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

import { bootstrapLocalWorkspaceSession, DEFAULT_LOCAL_WORKSPACE_BASE_URL, fetchLocalWorkspaceHealth, fetchLocalWorkspaceInfo, normalizeLocalWorkspaceBaseUrl, type LocalWorkspaceInfo } from "@/services/local-workspace";

type LocalWorkspaceStatus = "idle" | "checking" | "connected" | "disconnected";

type LocalWorkspaceStore = {
    baseUrl: string;
    status: LocalWorkspaceStatus;
    serveAvailable: boolean;
    workspace: LocalWorkspaceInfo | null;
    lastError: string;
    setBaseUrl: (baseUrl: string) => void;
    refresh: () => Promise<LocalWorkspaceInfo | null>;
    connect: (baseUrl: string, launchSecret: string) => Promise<LocalWorkspaceInfo>;
    disconnect: () => void;
};

const STORE_KEY = "opsc:local_workspace_connection";
const STORE_VERSION = 1;

function persistedBaseUrl(value: unknown) {
    const candidate = value && typeof value === "object" && "baseUrl" in value ? (value as { baseUrl?: unknown }).baseUrl : "";
    if (typeof candidate === "string") {
        try {
            return normalizeLocalWorkspaceBaseUrl(candidate);
        } catch {
            return DEFAULT_LOCAL_WORKSPACE_BASE_URL;
        }
    }
    return DEFAULT_LOCAL_WORKSPACE_BASE_URL;
}

function writeSanitizedConnection(baseUrl: string) {
    if (typeof window === "undefined") return;
    try {
        window.localStorage.setItem(STORE_KEY, JSON.stringify({ state: { baseUrl: persistedBaseUrl({ baseUrl }) }, version: STORE_VERSION }));
    } catch {
        // Best effort cleanup; failed browser storage writes should not block the app.
    }
}

export const useLocalWorkspaceStore = create<LocalWorkspaceStore>()(
    persist(
        (set, get) => ({
            baseUrl: DEFAULT_LOCAL_WORKSPACE_BASE_URL,
            status: "idle",
            serveAvailable: false,
            workspace: null,
            lastError: "",
            setBaseUrl: (baseUrl) => {
                try {
                    set({ baseUrl: normalizeLocalWorkspaceBaseUrl(baseUrl), serveAvailable: false, lastError: "" });
                } catch (error) {
                    set({ lastError: error instanceof Error ? error.message : "本地工作区地址无效" });
                }
            },
            refresh: async () => {
                const baseUrl = get().baseUrl || DEFAULT_LOCAL_WORKSPACE_BASE_URL;
                set({ status: "checking", lastError: "" });
                try {
                    const normalized = normalizeLocalWorkspaceBaseUrl(baseUrl);
                    const workspace = await fetchLocalWorkspaceInfo(normalized);
                    set({ baseUrl: normalized, serveAvailable: true, workspace, status: "connected", lastError: "" });
                    return workspace;
                } catch (error) {
                    const serveAvailable = await isServeAvailable(baseUrl);
                    set({ workspace: null, serveAvailable, status: "disconnected", lastError: localWorkspaceUnavailableMessage(serveAvailable, error) });
                    return null;
                }
            },
            connect: async (baseUrl, launchSecret) => {
                const normalized = normalizeLocalWorkspaceBaseUrl(baseUrl);
                if (!launchSecret.trim()) throw new Error("请输入 launch secret");
                set({ baseUrl: normalized, status: "checking", lastError: "" });
                try {
                    await bootstrapLocalWorkspaceSession(normalized, launchSecret.trim());
                    const workspace = await fetchLocalWorkspaceInfo(normalized);
                    set({ baseUrl: normalized, serveAvailable: true, workspace, status: "connected", lastError: "" });
                    return workspace;
                } catch (error) {
                    const serveAvailable = await isServeAvailable(normalized);
                    const message = localWorkspaceUnavailableMessage(serveAvailable, error);
                    set({ workspace: null, serveAvailable, status: "disconnected", lastError: message });
                    throw new Error(message);
                }
            },
            disconnect: () => set({ workspace: null, status: "disconnected", lastError: "" }),
        }),
        {
            name: STORE_KEY,
            partialize: (state) => ({ baseUrl: state.baseUrl }),
            version: STORE_VERSION,
            migrate: (persisted) => ({ baseUrl: persistedBaseUrl(persisted) }),
            merge: (persisted, current) => {
                const baseUrl = persistedBaseUrl(persisted);
                return { ...current, baseUrl };
            },
            onRehydrateStorage: () => (state) => {
                if (state) writeSanitizedConnection(state.baseUrl);
            },
        },
    ),
);

export function currentLocalWorkspaceConnection() {
    const state = useLocalWorkspaceStore.getState();
    return state.status === "connected" && state.workspace ? { baseUrl: state.baseUrl, workspace: state.workspace } : null;
}

async function isServeAvailable(baseUrl: string) {
    try {
        await fetchLocalWorkspaceHealth(baseUrl);
        return true;
    } catch {
        return false;
    }
}

function localWorkspaceUnavailableMessage(serveAvailable: boolean, error: unknown) {
    if (serveAvailable) {
        const message = error instanceof Error ? error.message : "";
        if (message && !/(未连接|missing or invalid authentication|unauthorized|session)/i.test(message)) return message;
        return "opsc serve 已启动，但浏览器 session 未建立或已过期，请输入 launch secret 连接。";
    }
    return "未检测到 opsc serve，请先启动本地工作区服务。";
}
