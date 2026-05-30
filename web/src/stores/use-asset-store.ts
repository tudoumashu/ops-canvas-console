"use client";

import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { nanoid } from "nanoid";
import { localForageStorage } from "@/lib/localforage-storage";
import { cleanupUnusedImages } from "@/services/image-storage";
import { cleanupUnusedMedia } from "@/services/file-storage";
import { assetFileObjectUrl, createLocalAsset, deleteLocalAsset, getLocalAsset, importLocalAssetFile, listLocalAssets, updateLocalAsset, type LocalAssetData, type LocalEnvelope } from "@/services/local-workspace";
import { currentLocalWorkspaceConnection } from "@/stores/use-local-workspace-store";

export type AssetKind = "text" | "image" | "video";
export type TextAsset = AssetBase<"text"> & { data: { content: string } };
export type ImageAsset = AssetBase<"image"> & { data: { dataUrl: string; storageKey?: string; width: number; height: number; bytes: number; mimeType: string } };
export type VideoAsset = AssetBase<"video"> & { data: { url: string; storageKey?: string; width: number; height: number; bytes: number; mimeType: string } };
export type Asset = TextAsset | ImageAsset | VideoAsset;

type AssetBase<T extends AssetKind> = {
    id: string;
    kind: T;
    title: string;
    coverUrl: string;
    tags: string[];
    source?: string;
    note?: string;
    createdAt: string;
    updatedAt: string;
    metadata?: Record<string, unknown>;
    revision?: number;
};

type AssetStore = {
    assets: Asset[];
    workspaceLoaded: boolean;
    loadedWorkspaceId: string;
    loading: boolean;
    lastError: string;
    loadFromWorkspace: () => Promise<void>;
    addAsset: (asset: Omit<Asset, "id" | "createdAt" | "updatedAt">) => Promise<string>;
    updateAsset: (id: string, patch: Partial<Omit<Asset, "id" | "createdAt">>) => Promise<void>;
    removeAsset: (id: string) => Promise<void>;
    cleanupImages: (extra?: unknown) => void;
};

const ASSET_STORE_KEY = "opsc:asset_store_cache:v1";

const assetStorage: PersistStorage<AssetStore> = {
    getItem: async (name) => {
        const value = await localForageStorage.getItem(name);
        if (!value) return null;
        const parsed = JSON.parse(value) as StorageValue<AssetStore>;
        parsed.state.assets = [];
        parsed.state.workspaceLoaded = false;
        parsed.state.loadedWorkspaceId = "";
        parsed.state.loading = false;
        parsed.state.lastError = "";
        return parsed;
    },
    setItem: (name, value) => localForageStorage.setItem(name, JSON.stringify(value)),
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const useAssetStore = create<AssetStore>()(
    persist(
        (set, get) => ({
            assets: [],
            workspaceLoaded: false,
            loadedWorkspaceId: "",
            loading: false,
            lastError: "",
            loadFromWorkspace: async () => {
                const connection = currentLocalWorkspaceConnection();
                if (!connection) {
                    set({ assets: [], workspaceLoaded: true, loadedWorkspaceId: "", loading: false, lastError: "请先连接本地工作区" });
                    return;
                }
                const workspaceId = connection.workspace.id;
                set((state) => ({ assets: state.loadedWorkspaceId === workspaceId ? state.assets : [], workspaceLoaded: false, loadedWorkspaceId: workspaceId, loading: true, lastError: "" }));
                try {
                    const list = await listLocalAssets(connection.baseUrl);
                    const assets = await Promise.all(
                        (list.assets || []).map(async (item) => {
                            const document = await getLocalAsset(connection.baseUrl, item.id);
                            return assetFromLocalDocument(connection.baseUrl, document);
                        }),
                    );
                    set({ assets, workspaceLoaded: true, loadedWorkspaceId: workspaceId, loading: false, lastError: "" });
                } catch (error) {
                    set({ assets: [], workspaceLoaded: false, loadedWorkspaceId: workspaceId, loading: false, lastError: error instanceof Error ? error.message : "加载本地素材失败" });
                }
            },
            addAsset: async (asset) => {
                const connection = currentLocalWorkspaceConnection();
                if (!connection) {
                    set({ lastError: "请先连接本地工作区" });
                    return "";
                }
                const now = new Date().toISOString();
                const workspaceId = connection.workspace.id;
                const optimistic = { ...asset, id: `pending_${nanoid()}`, createdAt: now, updatedAt: now } as Asset;
                set((state) => ({ assets: [optimistic, ...(state.loadedWorkspaceId === workspaceId ? state.assets : [])], workspaceLoaded: true, loadedWorkspaceId: workspaceId, lastError: "" }));
                try {
                    const document = await saveAssetToWorkspace(connection.baseUrl, optimistic);
                    const saved = await assetFromLocalDocument(connection.baseUrl, document);
                    set((state) => ({ assets: [saved, ...state.assets.filter((item) => item.id !== optimistic.id)], workspaceLoaded: true, loadedWorkspaceId: workspaceId }));
                    get().cleanupImages();
                    return saved.id;
                } catch (error) {
                    set((state) => ({ assets: state.assets.filter((item) => item.id !== optimistic.id), lastError: error instanceof Error ? error.message : "保存本地素材失败" }));
                    return "";
                }
            },
            updateAsset: async (id, patch) => {
                const connection = currentLocalWorkspaceConnection();
                const existing = get().assets.find((asset) => asset.id === id);
                if (!connection || !existing?.revision || get().loadedWorkspaceId !== connection.workspace.id) {
                    set({ lastError: "请先连接本地工作区" });
                    return;
                }
                const next = { ...existing, ...patch, updatedAt: new Date().toISOString() } as Asset;
                set((state) => ({ assets: state.assets.map((asset) => (asset.id === id ? next : asset)), lastError: "" }));
                try {
                    const document = await saveAssetToWorkspace(connection.baseUrl, next, id, existing.revision);
                    const saved = await assetFromLocalDocument(connection.baseUrl, document);
                    set((state) => ({ assets: state.assets.map((asset) => (asset.id === id ? saved : asset)) }));
                    get().cleanupImages();
                } catch (error) {
                    set((state) => ({ assets: state.assets.map((asset) => (asset.id === id ? existing : asset)), lastError: error instanceof Error ? error.message : "更新本地素材失败" }));
                }
            },
            removeAsset: async (id) => {
                const connection = currentLocalWorkspaceConnection();
                const existing = get().assets.find((asset) => asset.id === id);
                if (!connection || !existing || get().loadedWorkspaceId !== connection.workspace.id) {
                    set({ lastError: "请先连接本地工作区" });
                    return;
                }
                set((state) => {
                    const assets = state.assets.filter((asset) => asset.id !== id);
                    return { assets, lastError: "" };
                });
                try {
                    await deleteLocalAsset(connection.baseUrl, id);
                    get().cleanupImages();
                } catch (error) {
                    set((state) => ({ assets: [existing, ...state.assets], lastError: error instanceof Error ? error.message : "删除本地素材失败" }));
                }
            },
            cleanupImages: (extra) => {
                window.setTimeout(async () => {
                    const { useCanvasStore } = await import("@/app/(user)/canvas/stores/use-canvas-store");
                    await cleanupUnusedImages({ assets: get().assets, projects: useCanvasStore.getState().projects, extra });
                    await cleanupUnusedMedia({ assets: get().assets, projects: useCanvasStore.getState().projects, extra });
                }, 0);
            },
        }),
        {
            name: ASSET_STORE_KEY,
            storage: assetStorage,
            partialize: () => ({ assets: [], workspaceLoaded: false, loadedWorkspaceId: "" }) as unknown as StorageValue<AssetStore>["state"],
        },
    ),
);

async function saveAssetToWorkspace(baseUrl: string, asset: Asset, id?: string, revision?: number) {
    const data = assetToLocalData(asset);
    const blob = await assetBlobForWorkspace(asset);
    if (blob) return importLocalAssetFile(baseUrl, data, blob, { id, revision, fileKey: "original", fileName: `${asset.title || "asset"}${mimeExtension(blob.type || assetMime(asset))}` });
    if (id && revision) return updateLocalAsset(baseUrl, id, revision, data);
    return createLocalAsset(baseUrl, data);
}

function assetToLocalData(asset: Asset): LocalAssetData {
    const metadata = {
        ...(asset.metadata || {}),
        note: asset.note,
        width: asset.kind === "text" ? undefined : asset.data.width,
        height: asset.kind === "text" ? undefined : asset.data.height,
        bytes: asset.kind === "text" ? undefined : asset.data.bytes,
        mimeType: asset.kind === "text" ? undefined : asset.data.mimeType,
    };
    return {
        type: asset.kind,
        mime: assetMime(asset),
        title: asset.title,
        mediaType: asset.kind,
        categoryPath: String(asset.metadata?.categoryPath || ""),
        purpose: String(asset.metadata?.purpose || "generic"),
        source: String(asset.metadata?.source || asset.source || "local_upload"),
        coverUrl: isExternalUrl(asset.coverUrl) ? asset.coverUrl : "",
        description: asset.note,
        content: asset.kind === "text" ? asset.data.content : externalAssetUrl(asset),
        privacy: "private",
        tags: asset.tags || [],
        metadata,
    };
}

async function assetFromLocalDocument(baseUrl: string, document: LocalEnvelope<LocalAssetData>): Promise<Asset> {
    const kind = normalizeAssetKind(document.data.type || document.data.mediaType);
    const metadata = document.data.metadata || {};
    const base = {
        id: document.id,
        kind,
        title: document.data.title || "未命名素材",
        coverUrl: document.data.coverUrl || "",
        tags: document.data.tags || [],
        source: document.data.source || "本地工作区",
        note: document.data.description || (typeof metadata.note === "string" ? metadata.note : undefined),
        metadata,
        revision: document.revision,
        createdAt: document.createdAt,
        updatedAt: document.updatedAt,
    };
    if (kind === "text") return { ...base, kind: "text", data: { content: document.data.content || "" } };
    const fileUrl = document.data.files?.original ? await assetFileObjectUrl(baseUrl, document.id, "original") : document.data.content || document.data.coverUrl || "";
    const media = {
        width: numberFromMetadata(metadata.width),
        height: numberFromMetadata(metadata.height),
        bytes: numberFromMetadata(metadata.bytes),
        mimeType: document.data.mime || String(metadata.mimeType || (kind === "video" ? "video/mp4" : "image/png")),
    };
    if (kind === "video") return { ...base, kind: "video", coverUrl: base.coverUrl || fileUrl, data: { url: fileUrl, ...media } };
    return { ...base, kind: "image", coverUrl: base.coverUrl || fileUrl, data: { dataUrl: fileUrl, ...media } };
}

async function assetBlobForWorkspace(asset: Asset) {
    if (asset.kind === "text") return null;
    const url = asset.kind === "video" ? asset.data.url : asset.data.dataUrl;
    if (!url || isExternalUrl(url)) return null;
    return (await fetch(url)).blob();
}

function assetMime(asset: Asset) {
    if (asset.kind === "text") return "text/plain";
    return asset.data.mimeType;
}

function externalAssetUrl(asset: Asset) {
    if (asset.kind === "text") return "";
    const url = asset.kind === "video" ? asset.data.url : asset.data.dataUrl;
    return isExternalUrl(url) ? url : "";
}

function isExternalUrl(value?: string) {
    return Boolean(value && /^https?:\/\//i.test(value));
}

function normalizeAssetKind(value?: string): AssetKind {
    if (value === "video") return "video";
    if (value === "image") return "image";
    return "text";
}

function numberFromMetadata(value: unknown) {
    return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function mimeExtension(mimeType: string) {
    if (mimeType.includes("jpeg")) return ".jpg";
    if (mimeType.includes("png")) return ".png";
    if (mimeType.includes("webp")) return ".webp";
    if (mimeType.includes("gif")) return ".gif";
    if (mimeType.includes("mp4")) return ".mp4";
    if (mimeType.includes("webm")) return ".webm";
    return ".bin";
}
