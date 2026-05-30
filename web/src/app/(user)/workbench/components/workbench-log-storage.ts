"use client";

import {
    createLocalWorkbenchLog,
    deleteLocalWorkbenchLog,
    getLocalWorkbenchLog,
    listLocalWorkbenchLogs,
    workbenchLogFileObjectUrl,
    type LocalEnvelope,
    type LocalWorkbenchLogData,
    type LocalWorkbenchLogMedia,
} from "@/services/local-workspace";
import { currentLocalWorkspaceConnection } from "@/stores/use-local-workspace-store";

export type WorkbenchModality = "text" | "image" | "video";
export type WorkbenchLogUpload = { key: string; blob: Blob; fileName?: string };
export type WorkbenchLogDocument<T extends object> = LocalEnvelope<LocalWorkbenchLogData> & { payload: T };

export async function listWorkbenchLogs<T extends object>(modality: WorkbenchModality) {
    const connection = currentLocalWorkspaceConnection();
    if (!connection) return [] as Array<WorkbenchLogDocument<T>>;
    const list = await listLocalWorkbenchLogs(connection.baseUrl, modality);
    const documents = await Promise.all(list.workbenchLogs.map((item) => getLocalWorkbenchLog(connection.baseUrl, item.id)));
    return documents.map((document) => ({ ...document, payload: (document.data.payload || {}) as T }));
}

export async function saveWorkbenchLog<T extends object>(modality: WorkbenchModality, data: Omit<LocalWorkbenchLogData, "modality" | "payload">, payload: T, files: WorkbenchLogUpload[] = []) {
    const connection = requireWorkbenchConnection();
    return createLocalWorkbenchLog(connection.baseUrl, { ...data, modality, payload: payload as Record<string, unknown> }, files);
}

export async function deleteWorkbenchLogs(ids: string[]) {
    const connection = requireWorkbenchConnection();
    await Promise.all(ids.map((id) => deleteLocalWorkbenchLog(connection.baseUrl, id)));
}

export async function workbenchLogFileUrl(logId: string, fileKey: string) {
    const connection = currentLocalWorkspaceConnection();
    if (!connection) return "";
    return workbenchLogFileObjectUrl(connection.baseUrl, logId, fileKey);
}

export async function blobFromUrl(value?: string) {
    if (!value) return null;
    const response = await fetch(value);
    if (!response.ok) return null;
    return response.blob();
}

export function requireWorkbenchConnection() {
    const connection = currentLocalWorkspaceConnection();
    if (!connection) throw new Error("请先连接本地工作区");
    return connection;
}

export function mediaByKey(items: LocalWorkbenchLogMedia[] | undefined, key: string) {
    return (items || []).find((item) => item.key === key);
}
