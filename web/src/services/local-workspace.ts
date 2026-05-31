"use client";

export const DEFAULT_LOCAL_WORKSPACE_BASE_URL = "http://127.0.0.1:17680";

export type LocalWorkspaceInfo = {
    id: string;
    name: string;
    schemaVersion: string;
    revision: number;
    defaultProfileId?: string;
    pathSource: string;
    directories: string[];
    runtime: {
        active: boolean;
        pid?: number;
        host?: string;
        port?: number;
        baseUrl?: string;
        tokenFile?: string;
        launchSecretFile?: string;
    };
};

export type LocalEnvelope<T> = {
    schemaVersion: string;
    kind: string;
    id: string;
    revision: number;
    createdAt: string;
    updatedAt: string;
    data: T;
};

export type LocalSecretRefSummary = {
    type: string;
    reference?: string;
    configured: boolean;
    redacted: true;
};

export type LocalSecretRefInput = {
    type: "env" | "file" | "keychain" | "cloud" | string;
    name?: string;
    service?: string;
    account?: string;
    path?: string;
    channelId?: string;
};

export type LocalProfileData = {
    name: string;
    mode?: "local" | "cloud" | "hybrid" | string;
    channels?: Array<{
        id: string;
        name?: string;
        protocol?: string;
        baseUrl?: string;
        models?: string[];
        weight?: number;
        enabled?: boolean;
        secretRef?: LocalSecretRefSummary | LocalSecretRefInput;
        metadata?: Record<string, unknown>;
    }>;
    metadata?: Record<string, unknown>;
};

export type LocalProjectData = {
    name: string;
    kind?: string;
    adapter?: string;
    rootPath?: string;
    hasRootPath?: boolean;
    rootFingerprint?: string;
    capabilities?: Record<string, boolean>;
    execution?: Record<string, unknown>;
    adapterMetadata?: Record<string, unknown>;
    credentialRefs?: Record<string, LocalSecretRefSummary>;
    metadata?: Record<string, unknown>;
};

export type LocalAssetData = {
    type: "text" | "image" | "video" | "audio" | "file" | string;
    mime?: string;
    title?: string;
    mediaType?: string;
    category?: string;
    categoryPath?: string;
    purpose?: string;
    source?: string;
    coverUrl?: string;
    description?: string;
    content?: string;
    sourceArtifactId?: string;
    privacy?: "private" | "shared" | "public" | string;
    tags?: string[];
    files?: Record<string, string>;
    metadata?: Record<string, unknown>;
};

export type LocalPromptData = {
    title: string;
    coverUrl?: string;
    kind?: string;
    privacy?: "private" | "shared" | "public" | string;
    tags?: string[];
    category?: string;
    domain?: string;
    stage?: string;
    provider?: string;
    model?: string;
    mode?: string;
    inputType?: string;
    outputType?: string;
    status?: string;
    preview?: string;
    metadata?: Record<string, unknown>;
};

export type LocalCanvasProjectData = {
    title: string;
    nodes: Array<Record<string, unknown>>;
    connections: Array<Record<string, unknown>>;
    chatSessions: Array<Record<string, unknown>>;
    activeChatId: string | null;
    backgroundMode: "dots" | "lines" | "blank" | string;
    showImageInfo: boolean;
    viewport: { x: number; y: number; k: number };
    files?: Record<string, LocalCanvasProjectFile>;
    metadata?: Record<string, unknown>;
};

export type LocalCanvasProjectFile = {
    role?: string;
    nodeId?: string;
    mime?: string;
    path?: string;
    width?: number;
    height?: number;
    bytes?: number;
    metadata?: Record<string, unknown>;
};

export type LocalCanvasProjectUploadFile = {
    key: string;
    blob: Blob;
    fileName?: string;
};

export type LocalWorkbenchLogMedia = {
    key: string;
    role?: string;
    name?: string;
    mime?: string;
    path?: string;
    width?: number;
    height?: number;
    bytes?: number;
    durationMs?: number;
    metadata?: Record<string, unknown>;
};

export type LocalWorkbenchLogData = {
    modality: "text" | "image" | "video" | string;
    title?: string;
    createdAtMillis?: number;
    status?: "success" | "error" | string;
    model?: string;
    prompt?: string;
    media?: LocalWorkbenchLogMedia[];
    payload?: Record<string, unknown>;
    metrics?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
};

export type LocalTemplateData = {
    title: string;
    description?: string;
    workflowType?: string;
    version?: number;
    nodes?: Array<Record<string, unknown>>;
    edges?: Array<Record<string, unknown>>;
    settings?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
};

export type LocalRunData = {
    templateId?: string;
    status: "pending" | "running" | "success" | "error" | "canceled" | string;
    profileId?: string;
    projectId?: string;
    input?: Record<string, unknown>;
    output?: Record<string, unknown>;
    artifactRefs?: LocalRunArtifactRef[];
    metadata?: Record<string, unknown>;
};

export type LocalRunSummary = {
    id: string;
    status: string;
    templateId?: string;
    profileId?: string;
    projectId?: string;
    artifactCount: number;
    latestEventSequence: number;
    revision: number;
    createdAt: string;
    updatedAt: string;
};

export type LocalRunNodeStateSummary = {
    nodeId: string;
    status: string;
    startedAt?: string;
    finishedAt?: string;
    error?: string;
    output?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    revision: number;
    updatedAt: string;
};

export type LocalRunStatusSnapshot = {
    run: LocalRunSummary;
    nodes: LocalRunNodeStateSummary[];
    latestEventSequence: number;
};

export type LocalRunEvent = {
    schemaVersion: string;
    id: string;
    sequence: number;
    type: string;
    level: "debug" | "info" | "warn" | "error" | string;
    actor: { type: string; id?: string };
    subject: { kind: string; id: string };
    message: string;
    createdAt: string;
    data?: Record<string, unknown>;
};

export type LocalRunArtifactRef = {
    artifactId: string;
    role?: string;
    nodeId?: string;
    slot?: string;
    order?: number;
    metadata?: Record<string, unknown>;
};

export type LocalArtifactData = {
    type: "text" | "image" | "video" | "audio" | "file" | string;
    mime?: string;
    title?: string;
    sha256?: string;
    bytes?: number;
    width?: number;
    height?: number;
    durationSeconds?: number;
    source?: Record<string, unknown>;
    privacy?: "private" | "shared" | "public" | string;
    files?: Record<string, string>;
    metadata?: Record<string, unknown>;
};

export type LocalArtifactSummary = {
    id: string;
    type: string;
    mime?: string;
    title?: string;
    sha256?: string;
    bytes?: number;
    width?: number;
    height?: number;
    durationSeconds?: number;
    privacy?: string;
    original?: string;
    thumbnail?: string;
    revision: number;
    createdAt: string;
    updatedAt: string;
};

export type LocalRunArtifactSummary = {
    artifact: LocalArtifactSummary;
    ref: LocalRunArtifactRef;
};

export type LocalWorkflowFolder = {
    id: string;
    title: string;
    description?: string;
    href?: string;
    kind: "article" | "video" | "custom" | string;
};

export type LocalWorkspacePreferences = {
    workflowFolders: LocalWorkflowFolder[];
};

export type LocalWorkspacePreferencesSnapshot = {
    revision: number;
    preferences: LocalWorkspacePreferences;
};

export type LocalSummaryList<T extends string, V> = Record<T, V[]>;

type LocalApiResponse<T> = {
    code: number;
    data: T;
    msg: string;
};

const blobUrlCache = new Map<string, string>();

export function normalizeLocalWorkspaceBaseUrl(value?: string) {
    const raw = (value || DEFAULT_LOCAL_WORKSPACE_BASE_URL).trim().replace(/\/+$/, "");
    const url = new URL(raw);
    if (url.protocol !== "http:" && url.protocol !== "https:") throw new Error("本地工作区地址必须是 http/https");
    if (!isLoopbackHost(url.hostname)) throw new Error("本地工作区地址只允许 localhost / 127.0.0.1 / ::1");
    return url.toString().replace(/\/+$/, "");
}

export function isLoopbackHost(hostname: string) {
    return hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1" || hostname === "[::1]";
}

export async function bootstrapLocalWorkspaceSession(baseUrl: string, launchSecret: string) {
    return localWorkspaceRequest<{ authenticated: boolean; expiresAt: string }>(baseUrl, "/api/local/bootstrap/session", {
        method: "POST",
        body: JSON.stringify({ launchSecret }),
        headers: { "Content-Type": "application/json" },
    });
}

export async function fetchLocalWorkspaceHealth(baseUrl: string) {
    const response = await fetch(`${normalizeLocalWorkspaceBaseUrl(baseUrl)}/api/health`, {
        method: "GET",
        credentials: "omit",
    });
    const text = await response.text().catch(() => "");
    if (!response.ok || text.trim() !== "ok") throw new Error("未检测到可用的 opsc serve");
    return { ok: true };
}

export async function fetchLocalWorkspaceInfo(baseUrl: string) {
    return localWorkspaceRequest<LocalWorkspaceInfo>(baseUrl, "/api/local/workspace");
}

export async function fetchLocalWorkspacePreferences(baseUrl: string) {
    return localWorkspaceRequest<LocalWorkspacePreferencesSnapshot>(baseUrl, "/api/local/workspace/preferences");
}

export async function updateLocalWorkspacePreferences(baseUrl: string, revision: number, preferences: LocalWorkspacePreferences) {
    return localWorkspaceRequest<LocalWorkspacePreferencesSnapshot>(baseUrl, "/api/local/workspace/preferences", jsonRequest("PUT", { revision, preferences }));
}

export async function listLocalProfiles(baseUrl: string) {
    return localWorkspaceRequest<LocalSummaryList<"profiles", { id: string; name: string; mode?: string; channelCount: number; revision: number; createdAt: string; updatedAt: string }>>(baseUrl, "/api/local/profiles");
}

export async function getLocalProfile(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalProfileData>>(baseUrl, `/api/local/profiles/${encodeURIComponent(id)}`);
}

export async function saveLocalProfile(baseUrl: string, data: LocalProfileData, revision?: number, id?: string) {
    if (id) return localWorkspaceRequest<LocalEnvelope<LocalProfileData>>(baseUrl, `/api/local/profiles/${encodeURIComponent(id)}`, jsonRequest("PUT", { revision, data }));
    return localWorkspaceRequest<LocalEnvelope<LocalProfileData>>(baseUrl, "/api/local/profiles", jsonRequest("POST", { data }));
}

export async function deleteLocalProfile(baseUrl: string, id: string) {
    return localWorkspaceRequest<{ deleted: boolean; id: string }>(baseUrl, `/api/local/profiles/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listLocalProjects(baseUrl: string) {
    return localWorkspaceRequest<LocalSummaryList<"projects", { id: string; name: string; kind?: string; adapter?: string; hasRootPath: boolean; rootFingerprint?: string; capabilities?: Record<string, boolean>; revision: number; createdAt: string; updatedAt: string }>>(baseUrl, "/api/local/projects");
}

export async function getLocalProject(baseUrl: string, id: string, showPaths = false) {
    return localWorkspaceRequest<LocalEnvelope<LocalProjectData>>(baseUrl, `/api/local/projects/${encodeURIComponent(id)}${showPaths ? "?showPaths=1" : ""}`);
}

export async function saveLocalProject(baseUrl: string, data: LocalProjectData, revision?: number, id?: string) {
    if (id) return localWorkspaceRequest<LocalEnvelope<LocalProjectData>>(baseUrl, `/api/local/projects/${encodeURIComponent(id)}`, jsonRequest("PUT", { revision, data }));
    return localWorkspaceRequest<LocalEnvelope<LocalProjectData>>(baseUrl, "/api/local/projects", jsonRequest("POST", { data }));
}

export async function deleteLocalProject(baseUrl: string, id: string) {
    return localWorkspaceRequest<{ deleted: boolean; id: string }>(baseUrl, `/api/local/projects/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listLocalAssets(baseUrl: string) {
    return localWorkspaceRequest<LocalSummaryList<"assets", { id: string; type: string; mime?: string; title?: string; mediaType?: string; categoryPath?: string; purpose?: string; source?: string; privacy?: string; tags?: string[]; original?: string; thumbnail?: string; revision: number; createdAt: string; updatedAt: string }>>(baseUrl, "/api/local/assets");
}

export async function getLocalAsset(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalAssetData>>(baseUrl, `/api/local/assets/${encodeURIComponent(id)}`);
}

export async function createLocalAsset(baseUrl: string, data: LocalAssetData) {
    return localWorkspaceRequest<LocalEnvelope<LocalAssetData>>(baseUrl, "/api/local/assets", jsonRequest("POST", { data }));
}

export async function updateLocalAsset(baseUrl: string, id: string, revision: number, data: LocalAssetData) {
    return localWorkspaceRequest<LocalEnvelope<LocalAssetData>>(baseUrl, `/api/local/assets/${encodeURIComponent(id)}`, jsonRequest("PUT", { revision, data }));
}

export async function importLocalAssetFile(baseUrl: string, data: LocalAssetData, file: Blob, options: { id?: string; revision?: number; fileKey?: string; fileName?: string } = {}) {
    const form = new FormData();
    form.set("data", JSON.stringify(data));
    form.set("fileKey", options.fileKey || "original");
    if (options.revision) form.set("revision", String(options.revision));
    form.set("file", file, options.fileName || defaultFileName(data, file));
    const path = options.id ? `/api/local/assets/${encodeURIComponent(options.id)}/import` : "/api/local/assets/import";
    return localWorkspaceRequest<LocalEnvelope<LocalAssetData>>(baseUrl, path, { method: options.id ? "PUT" : "POST", body: form });
}

export async function deleteLocalAsset(baseUrl: string, id: string) {
    return localWorkspaceRequest<{ deleted: boolean; id: string }>(baseUrl, `/api/local/assets/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function assetFileObjectUrl(baseUrl: string, assetId: string, fileKey = "original") {
    return localWorkspaceBlobUrl(baseUrl, `/api/local/assets/${encodeURIComponent(assetId)}/files/${encodeURIComponent(fileKey)}`);
}

export async function assetFileBlob(baseUrl: string, assetId: string, fileKey = "original") {
    return fetchLocalWorkspaceBlob(baseUrl, `/api/local/assets/${encodeURIComponent(assetId)}/files/${encodeURIComponent(fileKey)}`);
}

export async function listLocalPrompts(baseUrl: string) {
    return localWorkspaceRequest<LocalSummaryList<"prompts", { id: string; title: string; kind?: string; category?: string; domain?: string; stage?: string; privacy?: string; tags?: string[]; hasContent: boolean; revision: number; createdAt: string; updatedAt: string }>>(baseUrl, "/api/local/prompts");
}

export async function getLocalPrompt(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalPromptData>>(baseUrl, `/api/local/prompts/${encodeURIComponent(id)}`);
}

export async function getLocalPromptContent(baseUrl: string, id: string) {
    const response = await fetch(`${normalizeLocalWorkspaceBaseUrl(baseUrl)}/api/local/prompts/${encodeURIComponent(id)}/content`, {
        method: "GET",
        credentials: "include",
    });
    if (!response.ok) throw new Error("读取本地提示词内容失败");
    return response.text();
}

export async function createLocalPrompt(baseUrl: string, data: LocalPromptData, content: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalPromptData>>(baseUrl, "/api/local/prompts", jsonRequest("POST", { data, content }));
}

export async function updateLocalPrompt(baseUrl: string, id: string, revision: number, data: LocalPromptData, content: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalPromptData>>(baseUrl, `/api/local/prompts/${encodeURIComponent(id)}`, jsonRequest("PUT", { revision, data, content }));
}

export async function deleteLocalPrompt(baseUrl: string, id: string) {
    return localWorkspaceRequest<{ deleted: boolean; id: string }>(baseUrl, `/api/local/prompts/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listLocalCanvasProjects(baseUrl: string) {
    return localWorkspaceRequest<
        LocalSummaryList<
            "canvasProjects",
            {
                id: string;
                title: string;
                nodeCount: number;
                connectionCount: number;
                fileCount: number;
                revision: number;
                createdAt: string;
                updatedAt: string;
            }
        >
    >(baseUrl, "/api/local/canvas-projects");
}

export async function getLocalCanvasProject(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalCanvasProjectData>>(baseUrl, `/api/local/canvas-projects/${encodeURIComponent(id)}`);
}

export async function createLocalCanvasProject(baseUrl: string, data: LocalCanvasProjectData, files: LocalCanvasProjectUploadFile[] = []) {
    if (files.length) {
        const form = canvasProjectFormData(data, files);
        return localWorkspaceRequest<LocalEnvelope<LocalCanvasProjectData>>(baseUrl, "/api/local/canvas-projects", { method: "POST", body: form });
    }
    return localWorkspaceRequest<LocalEnvelope<LocalCanvasProjectData>>(baseUrl, "/api/local/canvas-projects", jsonRequest("POST", { data }));
}

export async function updateLocalCanvasProject(baseUrl: string, id: string, revision: number, data: LocalCanvasProjectData, files: LocalCanvasProjectUploadFile[] = []) {
    if (files.length) {
        const form = canvasProjectFormData(data, files);
        form.set("revision", String(revision));
        return localWorkspaceRequest<LocalEnvelope<LocalCanvasProjectData>>(baseUrl, `/api/local/canvas-projects/${encodeURIComponent(id)}`, { method: "PUT", body: form });
    }
    return localWorkspaceRequest<LocalEnvelope<LocalCanvasProjectData>>(baseUrl, `/api/local/canvas-projects/${encodeURIComponent(id)}`, jsonRequest("PUT", { revision, data }));
}

export async function deleteLocalCanvasProject(baseUrl: string, id: string) {
    return localWorkspaceRequest<{ deleted: boolean; id: string }>(baseUrl, `/api/local/canvas-projects/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function canvasProjectFileObjectUrl(baseUrl: string, canvasProjectId: string, fileKey: string, version?: number | string) {
    const query = version ? `?v=${encodeURIComponent(String(version))}` : "";
    return localWorkspaceBlobUrl(baseUrl, `/api/local/canvas-projects/${encodeURIComponent(canvasProjectId)}/files/${encodeURIComponent(fileKey)}${query}`);
}

export async function canvasProjectFileBlob(baseUrl: string, canvasProjectId: string, fileKey: string, version?: number | string) {
    const query = version ? `?v=${encodeURIComponent(String(version))}` : "";
    return fetchLocalWorkspaceBlob(baseUrl, `/api/local/canvas-projects/${encodeURIComponent(canvasProjectId)}/files/${encodeURIComponent(fileKey)}${query}`);
}

export async function listLocalTemplates(baseUrl: string) {
    return localWorkspaceRequest<LocalSummaryList<"templates", { id: string; title: string; description?: string; workflowType?: string; version?: number; revision: number; createdAt: string; updatedAt: string }>>(baseUrl, "/api/local/templates");
}

export async function getLocalTemplate(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalTemplateData>>(baseUrl, `/api/local/templates/${encodeURIComponent(id)}`);
}

export async function createLocalTemplate(baseUrl: string, data: LocalTemplateData) {
    return localWorkspaceRequest<LocalEnvelope<LocalTemplateData>>(baseUrl, "/api/local/templates", jsonRequest("POST", { data }));
}

export async function updateLocalTemplate(baseUrl: string, id: string, revision: number, data: LocalTemplateData) {
    return localWorkspaceRequest<LocalEnvelope<LocalTemplateData>>(baseUrl, `/api/local/templates/${encodeURIComponent(id)}`, jsonRequest("PUT", { revision, data }));
}

export async function deleteLocalTemplate(baseUrl: string, id: string) {
    return localWorkspaceRequest<{ deleted: boolean; id: string }>(baseUrl, `/api/local/templates/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listLocalRuns(baseUrl: string) {
    return localWorkspaceRequest<LocalSummaryList<"runs", LocalRunSummary>>(baseUrl, "/api/local/runs");
}

export async function getLocalRun(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalRunData>>(baseUrl, `/api/local/runs/${encodeURIComponent(id)}`);
}

export async function createLocalRun(baseUrl: string, data: LocalRunData) {
    return localWorkspaceRequest<LocalEnvelope<LocalRunData>>(baseUrl, "/api/local/runs", jsonRequest("POST", { data }));
}

export async function updateLocalRun(baseUrl: string, id: string, revision: number, data: LocalRunData) {
    return localWorkspaceRequest<LocalEnvelope<LocalRunData>>(baseUrl, `/api/local/runs/${encodeURIComponent(id)}`, jsonRequest("PUT", { revision, data }));
}

export async function fetchLocalRunStatus(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalRunStatusSnapshot>(baseUrl, `/api/local/runs/${encodeURIComponent(id)}/status`);
}

export async function fetchLocalRunEvents(baseUrl: string, id: string, after = 0) {
    const query = after > 0 ? `?after=${encodeURIComponent(String(after))}` : "";
    return localWorkspaceRequest<{ runId: string; events: LocalRunEvent[] }>(baseUrl, `/api/local/runs/${encodeURIComponent(id)}/events${query}`);
}

export async function appendLocalRunEvent(baseUrl: string, id: string, event: { type: string; level?: string; actor?: { type: string; id?: string }; message?: string; data?: Record<string, unknown> }) {
    return localWorkspaceRequest<LocalRunEvent>(baseUrl, `/api/local/runs/${encodeURIComponent(id)}/events`, jsonRequest("POST", { event }));
}

export async function writeLocalRunNodeState(
    baseUrl: string,
    runId: string,
    nodeId: string,
    data: { nodeId?: string; status: string; startedAt?: string; finishedAt?: string; error?: string; output?: Record<string, unknown>; metadata?: Record<string, unknown> },
    revision?: number,
) {
    return localWorkspaceRequest<LocalEnvelope<Record<string, unknown>>>(baseUrl, `/api/local/runs/${encodeURIComponent(runId)}/nodes/${encodeURIComponent(nodeId)}`, jsonRequest(revision ? "PUT" : "POST", { revision, data }));
}

export async function listLocalRunArtifacts(baseUrl: string, runId: string) {
    return localWorkspaceRequest<{ runId: string; artifacts: LocalRunArtifactSummary[] }>(baseUrl, `/api/local/runs/${encodeURIComponent(runId)}/artifacts`);
}

export async function attachLocalRunArtifact(baseUrl: string, runId: string, data: LocalRunArtifactRef, revision?: number) {
    return localWorkspaceRequest<LocalEnvelope<LocalRunArtifactRef>>(baseUrl, `/api/local/runs/${encodeURIComponent(runId)}/artifacts`, jsonRequest("POST", { revision, data }));
}

export async function listLocalArtifacts(baseUrl: string) {
    return localWorkspaceRequest<LocalSummaryList<"artifacts", LocalArtifactSummary>>(baseUrl, "/api/local/artifacts");
}

export async function getLocalArtifact(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalArtifactData>>(baseUrl, `/api/local/artifacts/${encodeURIComponent(id)}`);
}

export async function createLocalArtifact(baseUrl: string, data: LocalArtifactData) {
    return localWorkspaceRequest<LocalEnvelope<LocalArtifactData>>(baseUrl, "/api/local/artifacts", jsonRequest("POST", { data }));
}

export async function updateLocalArtifact(baseUrl: string, id: string, revision: number, data: LocalArtifactData) {
    return localWorkspaceRequest<LocalEnvelope<LocalArtifactData>>(baseUrl, `/api/local/artifacts/${encodeURIComponent(id)}`, jsonRequest("PUT", { revision, data }));
}

export async function importLocalArtifactFile(baseUrl: string, data: LocalArtifactData, file: Blob, options: { id?: string; revision?: number; fileKey?: string; fileName?: string } = {}) {
    const form = new FormData();
    form.set("data", JSON.stringify(data));
    form.set("fileKey", options.fileKey || "original");
    if (options.revision) form.set("revision", String(options.revision));
    form.set("file", file, options.fileName || defaultArtifactFileName(data, file));
    const path = options.id ? `/api/local/artifacts/${encodeURIComponent(options.id)}/import` : "/api/local/artifacts/import";
    return localWorkspaceRequest<LocalEnvelope<LocalArtifactData>>(baseUrl, path, { method: options.id ? "PUT" : "POST", body: form });
}

export async function localArtifactFileObjectUrl(baseUrl: string, artifactId: string, fileKey = "original") {
    return localWorkspaceBlobUrl(baseUrl, `/api/local/artifacts/${encodeURIComponent(artifactId)}/files/${encodeURIComponent(fileKey)}`);
}

export async function listLocalWorkbenchLogs(baseUrl: string, modality?: string) {
    const query = modality ? `?modality=${encodeURIComponent(modality)}` : "";
    return localWorkspaceRequest<
        LocalSummaryList<
            "workbenchLogs",
            {
                id: string;
                modality: string;
                title?: string;
                status?: string;
                model?: string;
                createdAtMillis?: number;
                mediaCount: number;
                revision: number;
                createdAt: string;
                updatedAt: string;
            }
        >
    >(baseUrl, `/api/local/workbench-logs${query}`);
}

export async function getLocalWorkbenchLog(baseUrl: string, id: string) {
    return localWorkspaceRequest<LocalEnvelope<LocalWorkbenchLogData>>(baseUrl, `/api/local/workbench-logs/${encodeURIComponent(id)}`);
}

export async function createLocalWorkbenchLog(baseUrl: string, data: LocalWorkbenchLogData, files: Array<{ key: string; blob: Blob; fileName?: string }> = []) {
    if (!files.length) {
        return localWorkspaceRequest<LocalEnvelope<LocalWorkbenchLogData>>(baseUrl, "/api/local/workbench-logs", jsonRequest("POST", { data }));
    }
    const form = new FormData();
    form.set("data", JSON.stringify(data));
    files.forEach((file) => {
        form.set(`file:${file.key}`, file.blob, file.fileName || defaultWorkbenchFileName(file.key, file.blob));
    });
    return localWorkspaceRequest<LocalEnvelope<LocalWorkbenchLogData>>(baseUrl, "/api/local/workbench-logs", { method: "POST", body: form });
}

export async function deleteLocalWorkbenchLog(baseUrl: string, id: string) {
    return localWorkspaceRequest<{ deleted: boolean; id: string }>(baseUrl, `/api/local/workbench-logs/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function workbenchLogFileObjectUrl(baseUrl: string, logId: string, fileKey: string) {
    return localWorkspaceBlobUrl(baseUrl, `/api/local/workbench-logs/${encodeURIComponent(logId)}/files/${encodeURIComponent(fileKey)}`);
}

async function localWorkspaceBlobUrl(baseUrl: string, path: string) {
    const key = `${normalizeLocalWorkspaceBaseUrl(baseUrl)}${path}`;
    const cached = blobUrlCache.get(key);
    if (cached) return cached;
    const url = URL.createObjectURL(await fetchLocalWorkspaceBlob(baseUrl, path));
    blobUrlCache.set(key, url);
    return url;
}

async function fetchLocalWorkspaceBlob(baseUrl: string, path: string) {
    const response = await fetch(`${normalizeLocalWorkspaceBaseUrl(baseUrl)}${path}`, { method: "GET", credentials: "include" });
    if (!response.ok) throw new Error("读取本地工作区文件失败");
    return response.blob();
}

async function localWorkspaceRequest<T>(baseUrl: string, path: string, init: RequestInit = {}) {
    const response = await fetch(`${normalizeLocalWorkspaceBaseUrl(baseUrl)}${path}`, {
        ...init,
        credentials: "include",
        headers: init.headers,
    });
    let result: LocalApiResponse<T> | null = null;
    try {
        result = (await response.json()) as LocalApiResponse<T>;
    } catch {
        throw new Error(response.status === 401 ? "本地工作区未连接" : "本地工作区响应异常");
    }
    if (!response.ok || !result || result.code !== 0) throw new Error(result?.msg || "本地工作区请求失败");
    return result.data;
}

function jsonRequest(method: "POST" | "PUT", body: unknown): RequestInit {
    return {
        method,
        body: JSON.stringify(body),
        headers: { "Content-Type": "application/json" },
    };
}

function defaultFileName(data: LocalAssetData, file: Blob) {
    const type = (data.type || file.type || "file").split("/")[0] || "file";
    const ext = mimeExtension(file.type || data.mime || "");
    return `${type}${ext}`;
}

function defaultWorkbenchFileName(key: string, file: Blob) {
    return `${key}${mimeExtension(file.type || "")}`;
}

function defaultArtifactFileName(data: LocalArtifactData, file: Blob) {
    const type = (data.type || file.type || "artifact").split("/")[0] || "artifact";
    return `${type}${mimeExtension(file.type || data.mime || "")}`;
}

function canvasProjectFormData(data: LocalCanvasProjectData, files: LocalCanvasProjectUploadFile[]) {
    const form = new FormData();
    form.set("data", JSON.stringify(data));
    files.forEach((file) => {
        form.set(`file:${file.key}`, file.blob, file.fileName || `${file.key}${mimeExtension(file.blob.type || "")}`);
    });
    return form;
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
