"use client";

import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";
import { nanoid } from "nanoid";

import { localForageStorage } from "@/lib/localforage-storage";

export type MyPrompt = {
    id: string;
    title: string;
    prompt: string;
    coverUrl?: string;
    tags: string[];
    domain: "image" | "text" | "video" | "general" | string;
    stage: string;
    source: string;
    note?: string;
    metadata?: Record<string, unknown>;
    createdAt: string;
    updatedAt: string;
};

type PromptStore = {
    prompts: MyPrompt[];
    addPrompt: (prompt: Omit<MyPrompt, "id" | "createdAt" | "updatedAt">) => string;
    updatePrompt: (id: string, patch: Partial<Omit<MyPrompt, "id" | "createdAt">>) => void;
    removePrompt: (id: string) => void;
};

const PROMPT_STORE_KEY = "infinite-canvas:prompt_store";

const promptStorage: PersistStorage<PromptStore> = {
    getItem: async (name) => {
        const value = await localForageStorage.getItem(name);
        return value ? (JSON.parse(value) as StorageValue<PromptStore>) : null;
    },
    setItem: (name, value) => localForageStorage.setItem(name, JSON.stringify(value)),
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const usePromptStore = create<PromptStore>()(
    persist(
        (set) => ({
            prompts: [],
            addPrompt: (prompt) => {
                const now = new Date().toISOString();
                const id = nanoid();
                set((state) => ({ prompts: [{ ...prompt, id, createdAt: now, updatedAt: now }, ...state.prompts] }));
                return id;
            },
            updatePrompt: (id, patch) =>
                set((state) => ({
                    prompts: state.prompts.map((prompt) => (prompt.id === id ? { ...prompt, ...patch, updatedAt: new Date().toISOString() } : prompt)),
                })),
            removePrompt: (id) => set((state) => ({ prompts: state.prompts.filter((prompt) => prompt.id !== id) })),
        }),
        {
            name: PROMPT_STORE_KEY,
            storage: promptStorage,
            partialize: (state) => ({ prompts: state.prompts }) as StorageValue<PromptStore>["state"],
        },
    ),
);
