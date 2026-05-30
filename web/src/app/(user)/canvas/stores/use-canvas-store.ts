"use client";

import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { localForageStorage } from "@/lib/localforage-storage";
import type { CanvasBackgroundMode } from "@/lib/canvas-theme";
import { deleteStoredMedia, getMediaBlob } from "@/services/file-storage";
import { deleteStoredImages, getImageBlob } from "@/services/image-storage";
import {
    canvasProjectFileObjectUrl,
    createLocalCanvasProject,
    deleteLocalCanvasProject,
    getLocalCanvasProject,
    listLocalCanvasProjects,
    updateLocalCanvasProject,
    type LocalCanvasProjectData,
    type LocalCanvasProjectFile,
    type LocalCanvasProjectUploadFile,
    type LocalEnvelope,
} from "@/services/local-workspace";
import { currentLocalWorkspaceConnection } from "@/stores/use-local-workspace-store";
import type { CanvasAssistantSession, CanvasConnection, CanvasNodeData, ViewportTransform } from "../types";

export type CanvasProject = {
    id: string;
    title: string;
    createdAt: string;
    updatedAt: string;
    revision?: number;
    nodes: CanvasNodeData[];
    connections: CanvasConnection[];
    chatSessions: CanvasAssistantSession[];
    files: Record<string, LocalCanvasProjectFile>;
    activeChatId: string | null;
    backgroundMode: CanvasBackgroundMode;
    showImageInfo: boolean;
    viewport: ViewportTransform;
};

type CanvasProjectPatch = Partial<Pick<CanvasProject, "nodes" | "connections" | "chatSessions" | "activeChatId" | "backgroundMode" | "showImageInfo" | "viewport">>;

type CanvasStore = {
    hydrated: boolean;
    workspaceLoaded: boolean;
    loadedWorkspaceId: string;
    loading: boolean;
    lastError: string;
    projects: CanvasProject[];
    loadFromWorkspace: () => Promise<void>;
    createProject: (title?: string) => Promise<string>;
    importProject: (project: Partial<CanvasProject>) => Promise<string>;
    openProject: (id: string) => CanvasProject | null;
    renameProject: (id: string, title: string) => Promise<void>;
    deleteProjects: (ids: string[]) => Promise<void>;
    updateProject: (id: string, patch: CanvasProjectPatch) => void;
};

const initialViewport: ViewportTransform = { x: 0, y: 0, k: 1 };
const CANVAS_STORE_KEY = "opsc:canvas_store_cache:v1";
type PersistedCanvasState = Pick<CanvasStore, "projects">;
let saveTimer: ReturnType<typeof setTimeout> | null = null;
let queuedPersistState: PersistedCanvasState | null = null;
const projectPersistTimers = new Map<string, ReturnType<typeof setTimeout>>();
const projectPersisting = new Set<string>();
const projectQueued = new Set<string>();

const canvasStorage: PersistStorage<CanvasStore> = {
    getItem: async (name) => {
        const value = await localForageStorage.getItem(name);
        if (!value) return null;
        const parsed = JSON.parse(value) as StorageValue<CanvasStore>;
        queuedPersistState = parsed.state as PersistedCanvasState;
        return parsed;
    },
    setItem: (name, value) => {
        const nextState = value.state as PersistedCanvasState;
        if (queuedPersistState && queuedPersistState.projects === nextState.projects) return;
        queuedPersistState = nextState;
        if (saveTimer) clearTimeout(saveTimer);
        saveTimer = setTimeout(() => {
            saveTimer = null;
            void localForageStorage.setItem(name, JSON.stringify(value));
        }, 400);
    },
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const useCanvasStore = create<CanvasStore>()(
    persist(
        (set, get) => ({
            hydrated: false,
            workspaceLoaded: false,
            loadedWorkspaceId: "",
            loading: false,
            lastError: "",
            projects: [],
            loadFromWorkspace: async () => {
                const connection = currentLocalWorkspaceConnection();
                if (!connection) {
                    clearCanvasPersistTimers();
                    set({ projects: [], loading: false, workspaceLoaded: true, loadedWorkspaceId: "", lastError: "请先连接本地工作区" });
                    return;
                }
                clearCanvasPersistTimers();
                set({ loading: true, lastError: "" });
                try {
                    const list = await listLocalCanvasProjects(connection.baseUrl);
                    const projects = await Promise.all(
                        (list.canvasProjects || []).map(async (item) => {
                            const document = await getLocalCanvasProject(connection.baseUrl, item.id);
                            return canvasProjectFromLocalDocument(connection.baseUrl, document);
                        }),
                    );
                    set({ projects, loading: false, workspaceLoaded: true, loadedWorkspaceId: connection.workspace.id, lastError: "" });
                } catch (error) {
                    set({ loading: false, workspaceLoaded: false, loadedWorkspaceId: connection.workspace.id, lastError: error instanceof Error ? error.message : "加载本地画布失败" });
                }
            },
            createProject: async (title = "未命名画布") => {
                const connection = currentLocalWorkspaceConnection();
                if (!connection) {
                    set({ lastError: "请先连接本地工作区" });
                    return "";
                }
                const now = new Date().toISOString();
                const draft = normalizeCanvasProject({ title, createdAt: now, updatedAt: now });
                try {
                    const payload = await canvasProjectToLocalPayload(draft);
                    const document = await createLocalCanvasProject(connection.baseUrl, payload.data, payload.files);
                    const project = await canvasProjectFromLocalDocument(connection.baseUrl, document);
                    set((state) => ({ projects: [project, ...state.projects], workspaceLoaded: true, loadedWorkspaceId: connection.workspace.id, lastError: "" }));
                    cleanupCanvasBrowserStorage(payload.browserStorageKeys, useCanvasStore.getState().projects);
                    return project.id;
                } catch (error) {
                    set({ lastError: error instanceof Error ? error.message : "创建本地画布失败" });
                    return "";
                }
            },
            importProject: async (source) => {
                const connection = currentLocalWorkspaceConnection();
                if (!connection) {
                    set({ lastError: "请先连接本地工作区" });
                    return "";
                }
                const project = normalizeCanvasProject({ ...source, title: source.title || "导入画布", updatedAt: new Date().toISOString() });
                try {
                    const payload = await canvasProjectToLocalPayload(project);
                    const document = await createLocalCanvasProject(connection.baseUrl, payload.data, payload.files);
                    const saved = await canvasProjectFromLocalDocument(connection.baseUrl, document);
                    set((state) => ({ projects: [saved, ...state.projects], workspaceLoaded: true, loadedWorkspaceId: connection.workspace.id, lastError: "" }));
                    cleanupCanvasBrowserStorage(payload.browserStorageKeys, useCanvasStore.getState().projects);
                    return saved.id;
                } catch (error) {
                    set({ lastError: error instanceof Error ? error.message : "导入本地画布失败" });
                    return "";
                }
            },
            openProject: (id) => {
                return get().projects.find((item) => item.id === id) || null;
            },
            renameProject: async (id, title) => {
                const nextTitle = title.trim();
                if (!nextTitle) return;
                set((state) => ({
                    projects: state.projects.map((project) => (project.id === id ? { ...project, title: nextTitle, updatedAt: new Date().toISOString() } : project)),
                    lastError: "",
                }));
                scheduleCanvasProjectPersist(id, 0);
            },
            deleteProjects: async (ids) => {
                const connection = currentLocalWorkspaceConnection();
                if (!connection) {
                    set({ lastError: "请先连接本地工作区" });
                    return;
                }
                const existing = get().projects.filter((project) => ids.includes(project.id));
                ids.forEach(clearCanvasPersistTimer);
                set((state) => {
                    const projects = state.projects.filter((project) => !ids.includes(project.id));
                    return { projects, lastError: "" };
                });
                try {
                    await Promise.all(existing.map((project) => deleteLocalCanvasProject(connection.baseUrl, project.id)));
                    cleanupCanvasBrowserStorage(Array.from(collectCanvasStorageKeys(existing)), useCanvasStore.getState().projects);
                } catch (error) {
                    set((state) => ({ projects: [...existing, ...state.projects], lastError: error instanceof Error ? error.message : "删除本地画布失败" }));
                }
            },
            updateProject: (id, patch) => {
                set((state) => ({
                    projects: state.projects.map((project) => (project.id === id ? { ...project, ...patch, updatedAt: new Date().toISOString() } : project)),
                    lastError: "",
                }));
                scheduleCanvasProjectPersist(id);
            },
        }),
        {
            name: CANVAS_STORE_KEY,
            storage: canvasStorage,
            partialize: (state) =>
                ({
                    projects: [],
                }) as unknown as StorageValue<CanvasStore>["state"],
            onRehydrateStorage: () => () => {
                useCanvasStore.setState({ hydrated: true, projects: [] });
            },
        },
    ),
);

function normalizeCanvasProject(source: Partial<CanvasProject>): CanvasProject {
    const now = new Date().toISOString();
    return {
        id: source.id || "",
        title: source.title || "未命名画布",
        createdAt: source.createdAt || now,
        updatedAt: source.updatedAt || now,
        revision: source.revision,
        nodes: source.nodes || [],
        connections: source.connections || [],
        chatSessions: source.chatSessions || [],
        files: source.files || {},
        activeChatId: source.activeChatId || null,
        backgroundMode: source.backgroundMode || "lines",
        showImageInfo: source.showImageInfo || false,
        viewport: source.viewport || initialViewport,
    };
}

async function canvasProjectFromLocalDocument(baseUrl: string, document: LocalEnvelope<LocalCanvasProjectData>): Promise<CanvasProject> {
    const files = document.data.files || {};
    const nodes = await Promise.all(
        ((document.data.nodes || []) as unknown as CanvasNodeData[]).map(async (node) => {
            const fileKey = node.metadata?.workspaceFileKey;
            if (!fileKey || !files[fileKey]) return node;
            return {
                ...node,
                metadata: {
                    ...withoutStorageKey(node.metadata),
                    content: await canvasProjectFileObjectUrl(baseUrl, document.id, fileKey, document.revision),
                    workspaceFileKey: fileKey,
                },
            };
        }),
    );
    const chatSessions = await hydrateCanvasProjectSessionFiles(baseUrl, document.id, document.revision, (document.data.chatSessions || []) as unknown as CanvasAssistantSession[], files);
    return {
        id: document.id,
        title: document.data.title || "未命名画布",
        createdAt: document.createdAt,
        updatedAt: document.updatedAt,
        revision: document.revision,
        nodes,
        connections: (document.data.connections || []) as unknown as CanvasConnection[],
        chatSessions,
        files,
        activeChatId: document.data.activeChatId || null,
        backgroundMode: normalizeBackgroundMode(document.data.backgroundMode),
        showImageInfo: document.data.showImageInfo || false,
        viewport: document.data.viewport || initialViewport,
    };
}

type CanvasProjectLocalPayload = {
    data: LocalCanvasProjectData;
    files: LocalCanvasProjectUploadFile[];
    browserStorageKeys: string[];
};

const WORKSPACE_CANVAS_FILE_PREFIX = "workspace://canvas-file/";

async function canvasProjectToLocalPayload(project: CanvasProject): Promise<CanvasProjectLocalPayload> {
    const nodes = cloneJSON(project.nodes || []);
    const connections = cloneJSON(project.connections || []);
    const chatSessions = cloneJSON(project.chatSessions || []);
    const files = pruneCanvasProjectFiles(project.files || {});
    const uploads: LocalCanvasProjectUploadFile[] = [];
    const usedFileKeys = new Set<string>();
    const browserStorageKeys = new Set<string>();

    for (const node of nodes) {
        const metadata = node.metadata;
        if (!metadata || (node.type !== "image" && node.type !== "video")) continue;
        const fileKey = metadata.workspaceFileKey || `node_${safeCanvasFileKey(node.id)}`;
        if (metadata.storageKey) {
            const blob = await blobFromCanvasStorage(metadata.storageKey, metadata.content);
            if (blob) {
                browserStorageKeys.add(metadata.storageKey);
                usedFileKeys.add(fileKey);
                uploads.push({ key: fileKey, blob, fileName: canvasProjectFileName(node.title || node.id, blob) });
                files[fileKey] = {
                    ...files[fileKey],
                    role: node.type,
                    nodeId: node.id,
                    mime: blob.type || metadata.mimeType,
                    width: metadata.naturalWidth,
                    height: metadata.naturalHeight,
                    bytes: blob.size,
                };
                node.metadata = { ...withoutStorageKey(metadata), content: workspaceCanvasFileUrl(fileKey), workspaceFileKey: fileKey };
            } else if (metadata.workspaceFileKey && project.files?.[metadata.workspaceFileKey]) {
                usedFileKeys.add(metadata.workspaceFileKey);
                node.metadata = { ...withoutStorageKey(metadata), content: workspaceCanvasFileUrl(metadata.workspaceFileKey), workspaceFileKey: metadata.workspaceFileKey };
            }
            continue;
        }
        if (metadata.workspaceFileKey && project.files?.[metadata.workspaceFileKey]) {
            usedFileKeys.add(metadata.workspaceFileKey);
            node.metadata = { ...withoutStorageKey(metadata), content: workspaceCanvasFileUrl(metadata.workspaceFileKey), workspaceFileKey: metadata.workspaceFileKey };
            continue;
        }
        const blob = await blobFromCanvasStorage(undefined, metadata.content);
        if (!blob) continue;
        usedFileKeys.add(fileKey);
        uploads.push({ key: fileKey, blob, fileName: canvasProjectFileName(node.title || node.id, blob) });
        files[fileKey] = {
            ...files[fileKey],
            role: node.type,
            nodeId: node.id,
            mime: blob.type || metadata.mimeType,
            width: metadata.naturalWidth,
            height: metadata.naturalHeight,
            bytes: blob.size,
        };
        node.metadata = { ...withoutStorageKey(metadata), content: workspaceCanvasFileUrl(fileKey), workspaceFileKey: fileKey };
    }

    for (const session of chatSessions) {
        for (const message of session.messages || []) {
            message.references = await Promise.all((message.references || []).map((item, index) => canonicalizeAssistantMedia(item, `assistant_ref_${safeCanvasFileKey(session.id)}_${safeCanvasFileKey(message.id)}_${index}`, files, uploads, usedFileKeys, browserStorageKeys)));
            message.images = await Promise.all((message.images || []).map((item, index) => canonicalizeAssistantMedia(item, `assistant_img_${safeCanvasFileKey(session.id)}_${safeCanvasFileKey(message.id)}_${index}`, files, uploads, usedFileKeys, browserStorageKeys)));
        }
    }

    const dataFiles = Object.fromEntries(Object.entries(files).filter(([key]) => usedFileKeys.has(key)));
    return {
        data: {
            title: project.title,
            nodes: nodes as unknown as Array<Record<string, unknown>>,
            connections: connections as unknown as Array<Record<string, unknown>>,
            chatSessions: chatSessions as unknown as Array<Record<string, unknown>>,
            activeChatId: project.activeChatId,
            backgroundMode: project.backgroundMode,
            showImageInfo: project.showImageInfo,
            viewport: project.viewport,
            files: Object.keys(dataFiles).length ? dataFiles : undefined,
            metadata: { source: "web_canvas" },
        },
        files: uploads,
        browserStorageKeys: Array.from(browserStorageKeys),
    };
}

async function hydrateCanvasProjectSessionFiles(baseUrl: string, projectId: string, revision: number, sessions: CanvasAssistantSession[], files: Record<string, LocalCanvasProjectFile>) {
    const hydrateItem = async <T extends { dataUrl?: string; storageKey?: string; workspaceFileKey?: string }>(item: T) => {
        if (!item.workspaceFileKey || !files[item.workspaceFileKey]) return item;
        return { ...withoutStorageKey(item), dataUrl: await canvasProjectFileObjectUrl(baseUrl, projectId, item.workspaceFileKey, revision), workspaceFileKey: item.workspaceFileKey } as T;
    };
    return Promise.all(
        sessions.map(async (session) => ({
            ...session,
            messages: await Promise.all(
                session.messages.map(async (message) => ({
                    ...message,
                    references: await Promise.all((message.references || []).map(hydrateItem)),
                    images: await Promise.all((message.images || []).map(hydrateItem)),
                })),
            ),
        })),
    );
}

async function canonicalizeAssistantMedia<T extends { id: string; dataUrl?: string; storageKey?: string; workspaceFileKey?: string }>(
    item: T,
    fileKey: string,
    files: Record<string, LocalCanvasProjectFile>,
    uploads: LocalCanvasProjectUploadFile[],
    usedFileKeys: Set<string>,
    browserStorageKeys: Set<string>,
): Promise<T> {
    if (!item.dataUrl && !item.storageKey && !item.workspaceFileKey) return item;
    const effectiveKey = item.workspaceFileKey || fileKey;
    if (item.storageKey) {
        const blob = await blobFromCanvasStorage(item.storageKey, item.dataUrl);
        if (!blob) {
            if (item.workspaceFileKey && files[item.workspaceFileKey]) {
                usedFileKeys.add(item.workspaceFileKey);
                return { ...withoutStorageKey(item), dataUrl: workspaceCanvasFileUrl(item.workspaceFileKey), workspaceFileKey: item.workspaceFileKey } as T;
            }
            return item;
        }
        browserStorageKeys.add(item.storageKey);
        usedFileKeys.add(effectiveKey);
        uploads.push({ key: effectiveKey, blob, fileName: canvasProjectFileName(item.id, blob) });
        files[effectiveKey] = { ...files[effectiveKey], role: "assistant_media", mime: blob.type, bytes: blob.size };
        return { ...withoutStorageKey(item), dataUrl: workspaceCanvasFileUrl(effectiveKey), workspaceFileKey: effectiveKey } as T;
    }
    if (item.workspaceFileKey && files[item.workspaceFileKey]) {
        usedFileKeys.add(item.workspaceFileKey);
        return { ...withoutStorageKey(item), dataUrl: workspaceCanvasFileUrl(item.workspaceFileKey), workspaceFileKey: item.workspaceFileKey } as T;
    }
    const blob = await blobFromCanvasStorage(undefined, item.dataUrl);
    if (!blob) return item;
    usedFileKeys.add(effectiveKey);
    uploads.push({ key: effectiveKey, blob, fileName: canvasProjectFileName(item.id, blob) });
    files[effectiveKey] = { ...files[effectiveKey], role: "assistant_media", mime: blob.type, bytes: blob.size };
    return { ...withoutStorageKey(item), dataUrl: workspaceCanvasFileUrl(effectiveKey), workspaceFileKey: effectiveKey } as T;
}

function pruneCanvasProjectFiles(files: Record<string, LocalCanvasProjectFile>) {
    return Object.fromEntries(Object.entries(files).map(([key, value]) => [key, { ...value }]));
}

function withoutStorageKey<T extends { storageKey?: string } | undefined>(value: T) {
    if (!value) return {};
    const { storageKey: _storageKey, ...rest } = value;
    return rest;
}

async function blobFromCanvasStorage(storageKey?: string, url?: string) {
    if (storageKey) {
        const blob = storageKey.startsWith("image:") ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
        if (blob) return blob;
        return getImageBlob(storageKey);
    }
    if (!url || url.startsWith(WORKSPACE_CANVAS_FILE_PREFIX)) return null;
    if (!url.startsWith("data:") && !url.startsWith("blob:")) return null;
    try {
        return await (await fetch(url)).blob();
    } catch {
        return null;
    }
}

function workspaceCanvasFileUrl(fileKey: string) {
    return `${WORKSPACE_CANVAS_FILE_PREFIX}${fileKey}`;
}

function safeCanvasFileKey(value: string) {
    return value.trim().replace(/[^A-Za-z0-9_-]/g, "_").replace(/_+/g, "_").slice(0, 80) || "file";
}

function canvasProjectFileName(title: string, blob: Blob) {
    const base = safeCanvasFileKey(title || "canvas_file").slice(0, 48) || "canvas_file";
    if (blob.type.includes("jpeg")) return `${base}.jpg`;
    if (blob.type.includes("png")) return `${base}.png`;
    if (blob.type.includes("webp")) return `${base}.webp`;
    if (blob.type.includes("gif")) return `${base}.gif`;
    if (blob.type.includes("mp4")) return `${base}.mp4`;
    if (blob.type.includes("webm")) return `${base}.webm`;
    return `${base}.bin`;
}

function cloneJSON<T>(value: T): T {
    return JSON.parse(JSON.stringify(value)) as T;
}

function normalizeBackgroundMode(value?: string): CanvasBackgroundMode {
    if (value === "dots" || value === "blank") return value;
    return "lines";
}

function scheduleCanvasProjectPersist(id: string, delay = 700) {
    const project = useCanvasStore.getState().projects.find((item) => item.id === id);
    if (!project?.revision) return;
    clearCanvasPersistTimer(id);
    projectPersistTimers.set(
        id,
        setTimeout(() => {
            projectPersistTimers.delete(id);
            void persistCanvasProjectNow(id);
        }, delay),
    );
}

async function persistCanvasProjectNow(id: string) {
    if (projectPersisting.has(id)) {
        projectQueued.add(id);
        return;
    }
    const connection = currentLocalWorkspaceConnection();
    const snapshot = useCanvasStore.getState().projects.find((project) => project.id === id);
    if (!connection || !snapshot?.revision) {
        useCanvasStore.setState({ lastError: "请先连接本地工作区" });
        return;
    }
    projectPersisting.add(id);
    try {
        const payload = await canvasProjectToLocalPayload(snapshot);
        const document = await updateLocalCanvasProject(connection.baseUrl, id, snapshot.revision, payload.data, payload.files);
        const saved = await canvasProjectFromLocalDocument(connection.baseUrl, document);
        useCanvasStore.setState((state) => ({
            projects: state.projects.map((project) => {
                if (project.id !== id) return project;
                if (project.updatedAt !== snapshot.updatedAt) return { ...project, revision: document.revision, createdAt: document.createdAt, files: saved.files };
                return saved;
            }),
            lastError: "",
        }));
        cleanupCanvasBrowserStorage(payload.browserStorageKeys, useCanvasStore.getState().projects.find((project) => project.id === id));
    } catch (error) {
        useCanvasStore.setState({ lastError: error instanceof Error ? error.message : "保存本地画布失败" });
    } finally {
        projectPersisting.delete(id);
        if (projectQueued.delete(id)) scheduleCanvasProjectPersist(id, 0);
    }
}

function cleanupCanvasBrowserStorage(keys: Iterable<string>, protectedData?: unknown) {
    const protectedKeys = collectCanvasStorageKeys(protectedData);
    const removable = Array.from(new Set(keys)).filter((key) => key && !protectedKeys.has(key));
    if (!removable.length) return;
    const imageKeys = removable.filter((key) => key.startsWith("image:"));
    const mediaKeys = removable.filter((key) => !key.startsWith("image:"));
    void Promise.all([deleteStoredImages(imageKeys), deleteStoredMedia(mediaKeys)]);
}

function collectCanvasStorageKeys(value: unknown, keys = new Set<string>()) {
    if (!value || typeof value !== "object") return keys;
    if ("storageKey" in value && typeof value.storageKey === "string" && value.storageKey.includes(":")) keys.add(value.storageKey);
    Object.values(value).forEach((item) => {
        if (Array.isArray(item)) item.forEach((child) => collectCanvasStorageKeys(child, keys));
        else collectCanvasStorageKeys(item, keys);
    });
    return keys;
}

function clearCanvasPersistTimer(id: string) {
    const timer = projectPersistTimers.get(id);
    if (timer) clearTimeout(timer);
    projectPersistTimers.delete(id);
    projectQueued.delete(id);
}

function clearCanvasPersistTimers() {
    for (const id of projectPersistTimers.keys()) clearCanvasPersistTimer(id);
}
