"use client";

import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";
import { nanoid } from "nanoid";

import { localForageStorage } from "@/lib/localforage-storage";
import { createLocalPrompt, deleteLocalPrompt, getLocalPrompt, getLocalPromptContent, listLocalPrompts, updateLocalPrompt, type LocalEnvelope, type LocalPromptData } from "@/services/local-workspace";
import { currentLocalWorkspaceConnection } from "@/stores/use-local-workspace-store";

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
    revision?: number;
    createdAt: string;
    updatedAt: string;
};

type PromptStore = {
    prompts: MyPrompt[];
    workspaceLoaded: boolean;
    loadedWorkspaceId: string;
    loading: boolean;
    lastError: string;
    loadFromWorkspace: () => Promise<void>;
    addPrompt: (prompt: Omit<MyPrompt, "id" | "createdAt" | "updatedAt">) => Promise<string>;
    updatePrompt: (id: string, patch: Partial<Omit<MyPrompt, "id" | "createdAt">>) => Promise<void>;
    removePrompt: (id: string) => Promise<void>;
};

const PROMPT_STORE_KEY = "opsc:prompt_store_cache:v1";

const promptStorage: PersistStorage<PromptStore> = {
    getItem: async (name) => {
        const value = await localForageStorage.getItem(name);
        if (!value) return null;
        const parsed = JSON.parse(value) as StorageValue<PromptStore>;
        parsed.state.prompts = [];
        parsed.state.workspaceLoaded = false;
        parsed.state.loadedWorkspaceId = "";
        parsed.state.loading = false;
        parsed.state.lastError = "";
        return parsed;
    },
    setItem: (name, value) => localForageStorage.setItem(name, JSON.stringify(value)),
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const usePromptStore = create<PromptStore>()(
    persist(
        (set, get) => ({
            prompts: [],
            workspaceLoaded: false,
            loadedWorkspaceId: "",
            loading: false,
            lastError: "",
            loadFromWorkspace: async () => {
                const connection = currentLocalWorkspaceConnection();
                if (!connection) {
                    set({ prompts: [], workspaceLoaded: true, loadedWorkspaceId: "", loading: false, lastError: "请先连接本地工作区" });
                    return;
                }
                const workspaceId = connection.workspace.id;
                set((state) => ({ prompts: state.loadedWorkspaceId === workspaceId ? state.prompts : [], workspaceLoaded: false, loadedWorkspaceId: workspaceId, loading: true, lastError: "" }));
                try {
                    const list = await listLocalPrompts(connection.baseUrl);
                    const prompts = await Promise.all(
                        (list.prompts || []).map(async (item) => {
                            const document = await getLocalPrompt(connection.baseUrl, item.id);
                            const content = item.hasContent ? await getLocalPromptContent(connection.baseUrl, item.id) : "";
                            return promptFromLocalDocument(document, content);
                        }),
                    );
                    set({ prompts, workspaceLoaded: true, loadedWorkspaceId: workspaceId, loading: false, lastError: "" });
                } catch (error) {
                    set({ prompts: [], workspaceLoaded: false, loadedWorkspaceId: workspaceId, loading: false, lastError: error instanceof Error ? error.message : "加载本地提示词失败" });
                }
            },
            addPrompt: async (prompt) => {
                const connection = currentLocalWorkspaceConnection();
                if (!connection) {
                    set({ lastError: "请先连接本地工作区" });
                    return "";
                }
                const now = new Date().toISOString();
                const workspaceId = connection.workspace.id;
                const optimistic: MyPrompt = { ...prompt, id: `pending_${nanoid()}`, createdAt: now, updatedAt: now };
                set((state) => ({ prompts: [optimistic, ...(state.loadedWorkspaceId === workspaceId ? state.prompts : [])], workspaceLoaded: true, loadedWorkspaceId: workspaceId, lastError: "" }));
                try {
                    const document = await createLocalPrompt(connection.baseUrl, promptToLocalData(prompt), prompt.prompt);
                    const saved = promptFromLocalDocument(document, prompt.prompt);
                    set((state) => ({ prompts: [saved, ...state.prompts.filter((item) => item.id !== optimistic.id)], workspaceLoaded: true, loadedWorkspaceId: workspaceId }));
                    return saved.id;
                } catch (error) {
                    set((state) => ({ prompts: state.prompts.filter((item) => item.id !== optimistic.id), lastError: error instanceof Error ? error.message : "保存本地提示词失败" }));
                    return "";
                }
            },
            updatePrompt: async (id, patch) => {
                const connection = currentLocalWorkspaceConnection();
                const existing = get().prompts.find((prompt) => prompt.id === id);
                if (!connection || !existing?.revision || get().loadedWorkspaceId !== connection.workspace.id) {
                    set({ lastError: "请先连接本地工作区" });
                    return;
                }
                const next = { ...existing, ...patch, updatedAt: new Date().toISOString() };
                set((state) => ({ prompts: state.prompts.map((prompt) => (prompt.id === id ? next : prompt)), lastError: "" }));
                try {
                    const document = await updateLocalPrompt(connection.baseUrl, id, existing.revision, promptToLocalData(next), next.prompt);
                    const saved = promptFromLocalDocument(document, next.prompt);
                    set((state) => ({ prompts: state.prompts.map((prompt) => (prompt.id === id ? saved : prompt)) }));
                } catch (error) {
                    set((state) => ({ prompts: state.prompts.map((prompt) => (prompt.id === id ? existing : prompt)), lastError: error instanceof Error ? error.message : "更新本地提示词失败" }));
                }
            },
            removePrompt: async (id) => {
                const connection = currentLocalWorkspaceConnection();
                const existing = get().prompts.find((prompt) => prompt.id === id);
                if (!connection || !existing || get().loadedWorkspaceId !== connection.workspace.id) {
                    set({ lastError: "请先连接本地工作区" });
                    return;
                }
                set((state) => ({ prompts: state.prompts.filter((prompt) => prompt.id !== id), lastError: "" }));
                try {
                    await deleteLocalPrompt(connection.baseUrl, id);
                } catch (error) {
                    set((state) => ({ prompts: [existing, ...state.prompts], lastError: error instanceof Error ? error.message : "删除本地提示词失败" }));
                }
            },
        }),
        {
            name: PROMPT_STORE_KEY,
            storage: promptStorage,
            partialize: () => ({ prompts: [], workspaceLoaded: false, loadedWorkspaceId: "" }) as unknown as StorageValue<PromptStore>["state"],
        },
    ),
);

function promptToLocalData(prompt: Omit<MyPrompt, "id" | "createdAt" | "updatedAt"> | MyPrompt): LocalPromptData {
    return {
        title: prompt.title,
        coverUrl: prompt.coverUrl,
        kind: "user",
        privacy: "private",
        tags: prompt.tags || [],
        category: prompt.source,
        domain: prompt.domain,
        stage: prompt.stage,
        provider: String(prompt.metadata?.provider || "local"),
        model: String(prompt.metadata?.model || ""),
        mode: String(prompt.metadata?.mode || "general"),
        inputType: "text",
        outputType: prompt.domain,
        status: "local",
        preview: prompt.prompt.slice(0, 180),
        metadata: { ...(prompt.metadata || {}), note: prompt.note },
    };
}

function promptFromLocalDocument(document: LocalEnvelope<LocalPromptData>, content: string): MyPrompt {
    return {
        id: document.id,
        title: document.data.title,
        prompt: content,
        coverUrl: document.data.coverUrl,
        tags: document.data.tags || [],
        domain: document.data.domain || "image",
        stage: document.data.stage || "general",
        source: document.data.category || "本地工作区",
        note: typeof document.data.metadata?.note === "string" ? document.data.metadata.note : undefined,
        metadata: document.data.metadata || {},
        revision: document.revision,
        createdAt: document.createdAt,
        updatedAt: document.updatedAt,
    };
}
