import localforage from "localforage";
import type { StateStorage } from "zustand/middleware";

localforage.config({
    name: "infinite-canvas",
    storeName: "app_state",
});

const LEGACY_PRIVATE_BROWSER_STATE_KEYS = [
    "infinite-canvas:ai_config_store",
    "infinite-canvas:asset_store",
    "infinite-canvas:prompt_store",
    "infinite-canvas:canvas_store",
    "text_generation_logs",
    "image_generation_logs",
    "video_generation_logs",
    "ops-canvas-workflow-folders",
] as const;

async function removeBrowserStateKey(name: string) {
    try {
        await localforage.removeItem(name);
    } catch {
        // Best effort cleanup only; storage failures should not block app startup.
    }

    try {
        window.localStorage.removeItem(name);
    } catch {
        // Best effort cleanup only.
    }
}

export async function clearLegacyPrivateBrowserState() {
    if (typeof window === "undefined") return;
    await Promise.all(LEGACY_PRIVATE_BROWSER_STATE_KEYS.map(removeBrowserStateKey));
}

export const localForageStorage: StateStorage = {
    getItem: async (name) => {
        if (typeof window === "undefined") return null;
        try {
            return (await localforage.getItem<string>(name)) || null;
        } catch {
            return window.localStorage.getItem(name);
        }
    },
    setItem: async (name, value) => {
        if (typeof window === "undefined") return;
        try {
            await localforage.setItem(name, value);
        } catch {
            window.localStorage.setItem(name, value);
        }
    },
    removeItem: async (name) => {
        if (typeof window === "undefined") return;
        try {
            await localforage.removeItem(name);
        } catch {
            window.localStorage.removeItem(name);
        }
    },
};
