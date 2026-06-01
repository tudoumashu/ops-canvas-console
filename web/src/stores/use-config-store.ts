"use client";

import { useMemo } from "react";
import { create } from "zustand";
import { persist } from "zustand/middleware";

import { apiGet } from "@/services/api/request";
import type { AdminPublicSettings } from "@/services/api/admin";
import { getLocalProfile, listLocalProfiles, saveLocalProfile, type LocalEnvelope, type LocalProfileData } from "@/services/local-workspace";

export type AiConfig = {
    channelMode: "remote" | "local";
    baseUrl: string;
    apiKey: string;
    secretRefType: "env";
    secretRefName: string;
    model: string;
    imageModel: string;
    videoModel: string;
    textModel: string;
    videoSeconds: string;
    videoReferenceMode: string;
    vquality: string;
    systemPrompt: string;
    imagePromptPrefix: string;
    textPromptPrefix: string;
    videoPromptPrefix: string;
    models: string[];
    quality: string;
    size: string;
    count: string;
};

export const CONFIG_STORE_KEY = "infinite-canvas:ai_config_store";

export const defaultConfig: AiConfig = {
    channelMode: "remote",
    baseUrl: "https://api.openai.com",
    apiKey: "",
    secretRefType: "env",
    secretRefName: "OPENAI_API_KEY",
    model: "gpt-image-2",
    imageModel: "gpt-image-2",
    videoModel: "grok-imagine-video",
    textModel: "gpt-5.5",
    videoSeconds: "6",
    videoReferenceMode: "text",
    vquality: "720",
    systemPrompt: "",
    imagePromptPrefix: "",
    textPromptPrefix: "",
    videoPromptPrefix: "",
    models: [],
    quality: "auto",
    size: "1:1",
    count: "1",
};

type ConfigStore = {
    config: AiConfig;
    publicSettings: AdminPublicSettings | null;
    isPublicSettingsLoading: boolean;
    localProfileId: string;
    localProfileRevision: number;
    isLocalProfileLoading: boolean;
    isLocalProfileSaving: boolean;
    isConfigOpen: boolean;
    shouldPromptContinue: boolean;
    updateConfig: <K extends keyof AiConfig>(key: K, value: AiConfig[K]) => void;
    loadPublicSettings: () => Promise<void>;
    loadLocalProfile: (baseUrl: string, preferredProfileId?: string) => Promise<void>;
    clearLocalProfile: () => void;
    saveLocalProfile: (baseUrl: string) => Promise<void>;
    isAiConfigReady: (config: AiConfig, model: string) => boolean;
    openConfigDialog: (shouldPromptContinue?: boolean) => void;
    setConfigDialogOpen: (isOpen: boolean) => void;
    clearPromptContinue: () => void;
};

function resolveEffectiveConfig(config: AiConfig, modelChannel: AdminPublicSettings["modelChannel"] | null) {
    const channelMode = modelChannel?.allowCustomChannel ? config.channelMode : "remote";
    if (channelMode === "local" || !modelChannel) return { ...config, channelMode };
    const models = modelChannel.availableModels;
    const fallbackModel = modelChannel.defaultModel || models[0] || "";
    return {
        ...config,
        channelMode,
        models,
        model: models.includes(config.model) ? config.model : fallbackModel,
        imageModel: models.includes(config.imageModel) ? config.imageModel : modelChannel.defaultImageModel || fallbackModel,
        videoModel: models.includes(config.videoModel) ? config.videoModel : modelChannel.defaultVideoModel || fallbackModel,
        textModel: models.includes(config.textModel) ? config.textModel : modelChannel.defaultTextModel || fallbackModel,
        systemPrompt: modelChannel.systemPrompt,
        imagePromptPrefix: modelChannel.promptInjection?.image || "",
        textPromptPrefix: modelChannel.promptInjection?.text || "",
        videoPromptPrefix: modelChannel.promptInjection?.video || "",
    };
}

function isAiConfigReady(config: AiConfig, model: string) {
    return Boolean(model.trim()) && (config.channelMode === "remote" || Boolean(config.baseUrl.trim() && config.secretRefName.trim()));
}

export const useConfigStore = create<ConfigStore>()(
    persist(
        (set, get) => ({
            config: defaultConfig,
            publicSettings: null,
            isPublicSettingsLoading: false,
            localProfileId: "",
            localProfileRevision: 0,
            isLocalProfileLoading: false,
            isLocalProfileSaving: false,
            isConfigOpen: false,
            shouldPromptContinue: false,
            updateConfig: (key, value) =>
                set((state) => ({
                    config: {
                        ...state.config,
                        [key]: value,
                    },
                })),
            loadPublicSettings: async () => {
                if (get().isPublicSettingsLoading) return;
                set({ isPublicSettingsLoading: true });
                try {
                    set({ publicSettings: await apiGet<AdminPublicSettings>("/api/settings") });
                } finally {
                    set({ isPublicSettingsLoading: false });
                }
            },
            loadLocalProfile: async (baseUrl, preferredProfileId) => {
                if (get().isLocalProfileLoading) return;
                set({ isLocalProfileLoading: true });
                try {
                    const profile = await loadBestLocalProfile(baseUrl, preferredProfileId);
                    if (!profile) {
                        set(resetLocalProfileState());
                        return;
                    }
                    set((state) => ({
                        localProfileId: profile.id,
                        localProfileRevision: profile.revision,
                        config: {
                            ...state.config,
                            ...configFromLocalProfile(profile, state.config),
                            apiKey: "",
                        },
                    }));
                } finally {
                    set({ isLocalProfileLoading: false });
                }
            },
            clearLocalProfile: () => set(resetLocalProfileState()),
            saveLocalProfile: async (baseUrl) => {
                if (get().isLocalProfileSaving) return;
                set({ isLocalProfileSaving: true });
                try {
                    const { config, localProfileId, localProfileRevision } = get();
                    const data = localProfileFromConfig(config);
                    const document = await saveLocalProfile(baseUrl, data, localProfileId ? localProfileRevision : undefined, localProfileId || undefined);
                    set({
                        localProfileId: document.id,
                        localProfileRevision: document.revision,
                        config: { ...get().config, apiKey: "" },
                    });
                } finally {
                    set({ isLocalProfileSaving: false });
                }
            },
            isAiConfigReady: (config, model) => isAiConfigReady(config, model),
            openConfigDialog: (shouldPromptContinue = false) => set({ isConfigOpen: true, shouldPromptContinue }),
            setConfigDialogOpen: (isConfigOpen) => set({ isConfigOpen }),
            clearPromptContinue: () => set({ shouldPromptContinue: false }),
        }),
        {
            name: CONFIG_STORE_KEY,
            partialize: () => ({}),
            merge: (persisted, current) => {
                void persisted;
                return current;
            },
        },
    ),
);

export function useEffectiveConfig() {
    const config = useConfigStore((state) => state.config);
    const modelChannel = useConfigStore((state) => state.publicSettings?.modelChannel || null);
    return useMemo(() => resolveEffectiveConfig(config, modelChannel), [config, modelChannel]);
}

export function buildApiUrl(baseUrl: string, path: string) {
    const normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, "");
    const apiBaseUrl = normalizedBaseUrl.endsWith("/v1") ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`;
    return `${apiBaseUrl}${path}`;
}

async function loadBestLocalProfile(baseUrl: string, preferredProfileId?: string) {
    const summaries = await listLocalProfiles(baseUrl);
    const profileIds = summaries.profiles.map((profile) => profile.id);
    const orderedIds = preferredProfileId && profileIds.includes(preferredProfileId) ? [preferredProfileId, ...profileIds.filter((id) => id !== preferredProfileId)] : profileIds;
    const documents = await Promise.all(orderedIds.map((id) => getLocalProfile(baseUrl, id).catch(() => null)));
    return (
        documents.find((document): document is LocalEnvelope<LocalProfileData> => Boolean(document && document.data.metadata?.client === "ops-canvas-web")) ||
        documents.find((document): document is LocalEnvelope<LocalProfileData> => Boolean(document && document.data.mode === "local")) ||
        documents.find((document): document is LocalEnvelope<LocalProfileData> => Boolean(document)) ||
        null
    );
}

function resetLocalProfileState() {
    return {
        localProfileId: "",
        localProfileRevision: 0,
        config: { ...defaultConfig, models: [...defaultConfig.models] },
    };
}

function configFromLocalProfile(profile: LocalEnvelope<LocalProfileData>, fallback: AiConfig): Partial<AiConfig> {
    const channel = profile.data.channels?.find((item) => item.enabled !== false && item.baseUrl) || profile.data.channels?.[0];
    const metadata = (profile.data.metadata || {}) as Record<string, unknown>;
    return {
        channelMode: profile.data.mode === "cloud" ? "remote" : "local",
        baseUrl: channel?.baseUrl || fallback.baseUrl,
        models: channel?.models || fallback.models,
        secretRefType: "env",
        secretRefName: envSecretRefName(channel?.secretRef, fallback.secretRefName),
        model: stringMetadata(metadata.defaultModel) || fallback.model,
        imageModel: stringMetadata(metadata.defaultImageModel) || fallback.imageModel,
        videoModel: stringMetadata(metadata.defaultVideoModel) || fallback.videoModel,
        textModel: stringMetadata(metadata.defaultTextModel) || fallback.textModel,
        systemPrompt: stringMetadata(metadata.systemPrompt) || fallback.systemPrompt,
        imagePromptPrefix: stringMetadata(metadata.imagePromptPrefix) || fallback.imagePromptPrefix,
        textPromptPrefix: stringMetadata(metadata.textPromptPrefix) || fallback.textPromptPrefix,
        videoPromptPrefix: stringMetadata(metadata.videoPromptPrefix) || fallback.videoPromptPrefix,
    };
}

function envSecretRefName(secretRef: NonNullable<LocalProfileData["channels"]>[number]["secretRef"] | undefined, fallback: string) {
    if (!secretRef || secretRef.type !== "env") return fallback;
    if ("name" in secretRef && typeof secretRef.name === "string" && secretRef.name.trim()) return secretRef.name.trim();
    if ("reference" in secretRef && typeof secretRef.reference === "string" && secretRef.reference.trim()) return secretRef.reference.trim();
    return fallback;
}

function localProfileFromConfig(config: AiConfig): LocalProfileData {
    return {
        name: "Web UI Local AI",
        mode: "local",
        channels: [
            {
                id: "openai",
                name: "OpenAI compatible",
                protocol: "openai",
                baseUrl: config.baseUrl.trim(),
                models: config.models,
                weight: 1,
                enabled: true,
                secretRef: {
                    type: "env",
                    name: config.secretRefName.trim() || "OPENAI_API_KEY",
                },
            },
        ],
        metadata: {
            client: "ops-canvas-web",
            defaultModel: config.model,
            defaultImageModel: config.imageModel,
            defaultVideoModel: config.videoModel,
            defaultTextModel: config.textModel,
            systemPrompt: config.systemPrompt,
            imagePromptPrefix: config.imagePromptPrefix,
            textPromptPrefix: config.textPromptPrefix,
            videoPromptPrefix: config.videoPromptPrefix,
        },
    };
}

function stringMetadata(value: unknown) {
    return typeof value === "string" ? value : "";
}
