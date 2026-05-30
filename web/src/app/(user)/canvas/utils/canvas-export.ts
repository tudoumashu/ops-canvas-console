import { saveAs } from "file-saver";

import { createZip } from "@/lib/zip";
import { getMediaBlob } from "@/services/file-storage";
import { getImageBlob } from "@/services/image-storage";
import { canvasProjectFileBlob } from "@/services/local-workspace";
import { currentLocalWorkspaceConnection } from "@/stores/use-local-workspace-store";
import type { CanvasExportAsset, CanvasExportFile } from "../export-types";
import type { CanvasProject } from "../stores/use-canvas-store";

export async function exportCanvasProjects(projects: CanvasProject[], fileName = "无限画布") {
    const connection = currentLocalWorkspaceConnection();
    const zipFiles: { name: string; data: BlobPart }[] = [];
    const exportedProjects = await Promise.all(
        projects.map(async (project) => {
            const files: CanvasExportAsset[] = [];
            await Promise.all([
                ...collectStorageKeys(project).map(async (storageKey) => {
                    const blob = storageKey.startsWith("image:") ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
                    if (!blob) return;
                    const path = `projects/${project.id}/files/browser/${safeFileName(storageKey)}.${fileExtension(blob.type, storageKey)}`;
                    files.push({ storageKey, path, mimeType: blob.type || "application/octet-stream", bytes: blob.size });
                    zipFiles.push({ name: path, data: blob });
                }),
                ...collectWorkspaceFileKeys(project).map(async (workspaceFileKey) => {
                    if (!connection || !project.files?.[workspaceFileKey]) return;
                    const meta = project.files[workspaceFileKey];
                    const blob = await canvasProjectFileBlob(connection.baseUrl, project.id, workspaceFileKey, project.revision);
                    const mimeType = blob.type || meta.mime || "application/octet-stream";
                    const path = `projects/${project.id}/files/workspace/${safeFileName(workspaceFileKey)}.${fileExtension(mimeType, workspaceFileKey)}`;
                    files.push({ workspaceFileKey, role: meta.role, path, mimeType, bytes: blob.size || meta.bytes || 0 });
                    zipFiles.push({ name: path, data: blob });
                }),
            ]);
            return { project, files };
        }),
    );

    const data: CanvasExportFile = { app: "infinite-canvas", version: 3, exportedAt: new Date().toISOString(), projects: exportedProjects };
    const zip = await createZip([{ name: "projects.json", data: JSON.stringify(data, null, 2) }, ...zipFiles]);
    saveAs(zip, `${safeFileName(fileName)}.zip`);
}

function collectStorageKeys(value: unknown, keys = new Set<string>()) {
    if (!value || typeof value !== "object") return [...keys];
    if ("storageKey" in value && typeof value.storageKey === "string" && value.storageKey.includes(":")) keys.add(value.storageKey);
    Object.values(value).forEach((item) => (Array.isArray(item) ? item.forEach((child) => collectStorageKeys(child, keys)) : collectStorageKeys(item, keys)));
    return [...keys];
}

function collectWorkspaceFileKeys(project: CanvasProject) {
    const referenced = collectWorkspaceFileRefs(project);
    return [...referenced].filter((key) => Boolean(project.files?.[key]));
}

function collectWorkspaceFileRefs(value: unknown, keys = new Set<string>()) {
    if (!value || typeof value !== "object") return keys;
    if ("workspaceFileKey" in value && typeof value.workspaceFileKey === "string" && value.workspaceFileKey) keys.add(value.workspaceFileKey);
    Object.values(value).forEach((item) => (Array.isArray(item) ? item.forEach((child) => collectWorkspaceFileRefs(child, keys)) : collectWorkspaceFileRefs(item, keys)));
    return keys;
}

function safeFileName(value: string) {
    return value.replace(/[\\/:*?"<>|]/g, "_");
}

function fileExtension(mimeType: string, storageKey: string) {
    if (mimeType.includes("png")) return "png";
    if (mimeType.includes("jpeg")) return "jpg";
    if (mimeType.includes("webp")) return "webp";
    if (mimeType.includes("gif")) return "gif";
    if (mimeType.includes("mp4")) return "mp4";
    if (mimeType.includes("webm")) return "webm";
    return storageKey.startsWith("image:") ? "png" : "bin";
}
