"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { App, Button } from "antd";
import { Download, FileUp, Plus } from "lucide-react";
import { nanoid } from "nanoid";

import { readZip } from "@/lib/zip";
import { LocalWorkspaceStatusAlert } from "@/components/local-workspace/local-workspace-status-alert";
import { deleteStoredMedia, setMediaBlob } from "@/services/file-storage";
import { deleteStoredImages, setImageBlob } from "@/services/image-storage";
import { CanvasDeleteProjectsDialog } from "./components/canvas-delete-projects-dialog";
import { CanvasProjectCard } from "./components/canvas-project-card";
import type { CanvasExportAsset, CanvasExportFile } from "./export-types";
import { useCanvasStore } from "./stores/use-canvas-store";
import { useCanvasUiStore } from "./stores/use-canvas-ui-store";
import { exportCanvasProjects } from "./utils/canvas-export";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";

export default function CanvasPage() {
    const { message } = App.useApp();
    const router = useRouter();
    const inputRef = useRef<HTMLInputElement>(null);
    const hydrated = useCanvasStore((state) => state.hydrated);
    const loading = useCanvasStore((state) => state.loading);
    const workspaceLoaded = useCanvasStore((state) => state.workspaceLoaded);
    const loadedWorkspaceId = useCanvasStore((state) => state.loadedWorkspaceId);
    const lastError = useCanvasStore((state) => state.lastError);
    const projects = useCanvasStore((state) => state.projects);
    const loadFromWorkspace = useCanvasStore((state) => state.loadFromWorkspace);
    const createProject = useCanvasStore((state) => state.createProject);
    const importProject = useCanvasStore((state) => state.importProject);
    const workspaceStatus = useLocalWorkspaceStore((state) => state.status);
    const workspaceId = useLocalWorkspaceStore((state) => state.workspace?.id || "");
    const selectedIds = useCanvasUiStore((state) => state.selectedProjectIds);
    const setDeleteIds = useCanvasUiStore((state) => state.setDeleteProjectIds);
    const ready = hydrated && workspaceStatus === "connected" && workspaceLoaded && loadedWorkspaceId === workspaceId && !loading;
    const loadingText = workspaceStatus === "connected" ? "正在加载画布..." : "请先连接本地工作区";

    useEffect(() => {
        if (!hydrated || workspaceStatus !== "connected" || loading || loadedWorkspaceId === workspaceId) return;
        void loadFromWorkspace();
    }, [hydrated, loadFromWorkspace, loadedWorkspaceId, loading, workspaceId, workspaceStatus]);

    const enterProject = (id: string) => {
        router.push(`/canvas/${id}`);
    };
    const createAndEnter = async () => {
        const id = await createProject(`无限画布 ${projects.length + 1}`);
        if (id) enterProject(id);
        else message.error(useCanvasStore.getState().lastError || "创建画布失败");
    };
    const importCanvas = async (file?: File) => {
        if (!file) return;
        const temporaryStorageKeys: string[] = [];
        try {
            const zip = await readZip(file);
            const projectFile = zip.get("projects.json");
            if (!projectFile) throw new Error("missing projects.json");
            const data = JSON.parse(await projectFile.text()) as CanvasExportFile;
            const importedProjects = await Promise.all(
                data.projects.map(async (project) => {
                    const imported = JSON.parse(JSON.stringify(project.project)) as typeof project.project;
                    await Promise.all(
                        project.files.map(async (item) => {
                            const storageKey = await importCanvasFile(zip, item, imported);
                            if (!storageKey) return;
                            temporaryStorageKeys.push(storageKey);
                            if (item.workspaceFileKey) replaceWorkspaceFileRefs(imported, item.workspaceFileKey, storageKey);
                            if (item.storageKey) replaceStorageKeyRefs(imported, item.storageKey, storageKey);
                        }),
                    );
                    return imported;
                }),
            );
            const ids = await Promise.all(importedProjects.map((item) => importProject(item)));
            message.success(`已导入 ${ids.filter(Boolean).length} 个画布`);
        } catch {
            message.error("导入失败，请选择有效的画布压缩包");
        } finally {
            await deleteTemporaryCanvasImportFiles(temporaryStorageKeys);
            if (inputRef.current) inputRef.current.value = "";
        }
    };

    const importCanvasFile = async (zip: Map<string, Blob>, item: CanvasExportAsset, project: CanvasExportFile["projects"][number]["project"]) => {
        const blob = zip.get(item.path);
        if (!blob) return "";
        const typedBlob = blob.type ? blob : blob.slice(0, blob.size, item.mimeType);
        const storageKey = `${isImageExportAsset(item, project) ? "image" : "file"}:import_${nanoid()}`;
        await (storageKey.startsWith("image:") ? setImageBlob(storageKey, typedBlob) : setMediaBlob(storageKey, typedBlob));
        return storageKey;
    };

    return (
        <main className="h-full overflow-auto bg-background text-stone-950 dark:text-stone-100">
            <div className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-6 py-10">
                <header className="flex flex-wrap items-end justify-between gap-4 border-b border-stone-200 pb-6 dark:border-stone-800">
                    <div>
                        <p className="text-xs text-stone-500">画布库</p>
                        <h1 className="mt-3 text-3xl font-semibold">无限画布</h1>
                    </div>
                    <div className="flex items-center gap-2">
                        {selectedIds.length ? (
                            <>
                                <Button disabled={!ready} icon={<Download className="size-4" />} onClick={() => void exportCanvasProjects(projects.filter((project) => selectedIds.includes(project.id)), `无限画布-${selectedIds.length}个项目`)}>
                                    导出选中
                                </Button>
                                <Button disabled={!ready} onClick={() => setDeleteIds(selectedIds)}>
                                    删除选中
                                </Button>
                            </>
                        ) : null}
                        {projects.length ? (
                            <Button disabled={!ready} onClick={() => setDeleteIds(projects.map((project) => project.id))}>
                                删除全部
                            </Button>
                        ) : null}
                        <Button disabled={!ready} icon={<FileUp className="size-4" />} onClick={() => inputRef.current?.click()}>
                            导入画布
                        </Button>
                        <Button disabled={!ready} type="primary" icon={<Plus className="size-4" />} onClick={() => void createAndEnter()}>
                            新建画布
                        </Button>
                    </div>
                </header>

                <LocalWorkspaceStatusAlert message="画布库现在以本地工作区为事实源" />

                {!ready ? (
                    <section className="flex min-h-[360px] items-center justify-center border-y border-stone-200 text-sm text-stone-500 dark:border-stone-800">{lastError || loadingText}</section>
                ) : projects.length ? (
                    <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
                        {projects.map((project) => (
                            <CanvasProjectCard key={project.id} project={project} />
                        ))}
                    </div>
                ) : (
                    <section className="flex min-h-[360px] flex-col items-center justify-center border-y border-stone-200 text-center dark:border-stone-800">
                        <h2 className="text-xl font-medium">还没有画布</h2>
                        <p className="mt-3 text-sm text-stone-500">新建一个画布后，就可以独立保存节点、连线和画布外观。</p>
                        <Button type="primary" className="mt-6" icon={<Plus className="size-4" />} onClick={() => void createAndEnter()}>
                            新建画布
                        </Button>
                    </section>
                )}
            </div>

            <input ref={inputRef} type="file" accept="application/zip,.zip" className="hidden" onChange={(event) => void importCanvas(event.target.files?.[0])} />
            <CanvasDeleteProjectsDialog />
        </main>
    );
}

function replaceWorkspaceFileRefs(value: unknown, workspaceFileKey: string, storageKey: string) {
    if (!value || typeof value !== "object") return;
    const object = value as Record<string, unknown>;
    if (object.workspaceFileKey === workspaceFileKey) object.storageKey = storageKey;
    Object.values(object).forEach((item) => {
        if (Array.isArray(item)) item.forEach((child) => replaceWorkspaceFileRefs(child, workspaceFileKey, storageKey));
        else replaceWorkspaceFileRefs(item, workspaceFileKey, storageKey);
    });
}

function replaceStorageKeyRefs(value: unknown, from: string, to: string) {
    if (!value || typeof value !== "object") return;
    const object = value as Record<string, unknown>;
    if (object.storageKey === from) object.storageKey = to;
    Object.values(object).forEach((item) => {
        if (Array.isArray(item)) item.forEach((child) => replaceStorageKeyRefs(child, from, to));
        else replaceStorageKeyRefs(item, from, to);
    });
}

async function deleteTemporaryCanvasImportFiles(storageKeys: string[]) {
    if (!storageKeys.length) return;
    await Promise.all([deleteStoredImages(storageKeys.filter((key) => key.startsWith("image:"))), deleteStoredMedia(storageKeys.filter((key) => !key.startsWith("image:")))]);
}

function isImageExportAsset(item: CanvasExportAsset, project: CanvasExportFile["projects"][number]["project"]) {
    if (item.mimeType.startsWith("image/")) return true;
    if (item.mimeType.startsWith("video/")) return false;
    if (item.role === "image") return true;
    if (item.role === "video") return false;
    if (!item.workspaceFileKey) return Boolean(item.storageKey?.startsWith("image:"));
    return project.files?.[item.workspaceFileKey]?.role === "image";
}
