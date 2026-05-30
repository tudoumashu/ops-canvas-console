"use client";

import { App, Button, Checkbox, Empty, Input, Modal, Popconfirm, Space, Spin, Tag, Tooltip, Typography } from "antd";
import { FolderGit2, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { deleteLocalProject, getLocalProject, listLocalProjects, saveLocalProject, type LocalProjectData } from "@/services/local-workspace";

type ProjectSummary = {
    id: string;
    name: string;
    kind?: string;
    adapter?: string;
    hasRootPath: boolean;
    rootFingerprint?: string;
    capabilities?: Record<string, boolean>;
    revision: number;
    createdAt: string;
    updatedAt: string;
};

type ProjectDraft = {
    id?: string;
    revision?: number;
    name: string;
    kind: string;
    adapter: string;
    rootPath: string;
    capabilityKeys: string[];
    execution?: Record<string, unknown>;
    adapterMetadata?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    hasCredentialRefs: boolean;
};

const PROJECT_CAPABILITIES = [
    { label: "读取文件", value: "fs.read" },
    { label: "写入文件", value: "fs.write" },
    { label: "执行命令", value: "process.exec" },
    { label: "本地网络", value: "network.local" },
    { label: "写入产物", value: "artifact.write" },
];

const DEFAULT_DENY_GLOBS = ["**/.env", "**/.env.*", "**/.git/**", "**/node_modules/**"];

function createEmptyDraft(): ProjectDraft {
    return {
        name: "",
        kind: "code",
        adapter: "generic",
        rootPath: "",
        capabilityKeys: ["fs.read"],
        hasCredentialRefs: false,
    };
}

function capabilityKeysFrom(data?: Record<string, boolean>) {
    if (!data) return ["fs.read"];
    return PROJECT_CAPABILITIES.filter((capability) => data[capability.value]).map((capability) => capability.value);
}

function capabilityRecord(keys: string[]) {
    return Object.fromEntries(PROJECT_CAPABILITIES.map((capability) => [capability.value, keys.includes(capability.value)]));
}

function displayCapabilities(data?: Record<string, boolean>) {
    const enabled = PROJECT_CAPABILITIES.filter((capability) => data?.[capability.value]);
    if (enabled.length === 0) return <Tag>无权限</Tag>;
    return enabled.map((capability) => (
        <Tag key={capability.value} className="!m-0">
            {capability.value}
        </Tag>
    ));
}

export function LocalProjectsPanel({ baseUrl }: { baseUrl: string }) {
    const { message } = App.useApp();
    const [projects, setProjects] = useState<ProjectSummary[]>([]);
    const [loading, setLoading] = useState(false);
    const [editorOpen, setEditorOpen] = useState(false);
    const [draft, setDraft] = useState<ProjectDraft>(() => createEmptyDraft());
    const [loadingProjectId, setLoadingProjectId] = useState("");
    const [saving, setSaving] = useState(false);
    const [deletingId, setDeletingId] = useState("");

    const refreshProjects = useCallback(async () => {
        setLoading(true);
        try {
            const data = await listLocalProjects(baseUrl);
            setProjects(data.projects);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取本地项目失败");
        } finally {
            setLoading(false);
        }
    }, [baseUrl, message]);

    useEffect(() => {
        void refreshProjects();
    }, [refreshProjects]);

    const openCreate = () => {
        setDraft(createEmptyDraft());
        setEditorOpen(true);
    };

    const openEdit = async (project: ProjectSummary) => {
        setLoadingProjectId(project.id);
        try {
            const document = await getLocalProject(baseUrl, project.id, true);
            setDraft({
                id: document.id,
                revision: document.revision,
                name: document.data.name || "",
                kind: document.data.kind || "code",
                adapter: document.data.adapter || "generic",
                rootPath: document.data.rootPath || "",
                capabilityKeys: capabilityKeysFrom(document.data.capabilities),
                execution: document.data.execution,
                adapterMetadata: document.data.adapterMetadata,
                metadata: document.data.metadata,
                hasCredentialRefs: Boolean(document.data.credentialRefs && Object.keys(document.data.credentialRefs).length > 0),
            });
            setEditorOpen(true);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取本地项目失败");
        } finally {
            setLoadingProjectId("");
        }
    };

    const handleSave = async () => {
        const name = draft.name.trim();
        if (!name) {
            message.warning("项目名称不能为空");
            return;
        }
        if (draft.hasCredentialRefs) {
            message.error("该项目包含 credentialRef，请用 CLI 修改，避免 Web UI 写回脱敏信息");
            return;
        }
        setSaving(true);
        try {
            const data: LocalProjectData = {
                name,
                kind: draft.kind.trim() || "code",
                adapter: draft.adapter.trim() || "generic",
                capabilities: capabilityRecord(draft.capabilityKeys),
                execution: draft.execution || { denyGlobs: DEFAULT_DENY_GLOBS },
                adapterMetadata: draft.adapterMetadata,
                metadata: { ...(draft.metadata || {}), source: "web_local_workspace" },
            };
            const rootPath = draft.rootPath.trim();
            if (rootPath) data.rootPath = rootPath;
            await saveLocalProject(baseUrl, data, draft.revision, draft.id);
            setEditorOpen(false);
            await refreshProjects();
            message.success(draft.id ? "本地项目已更新" : "本地项目已创建");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存本地项目失败");
        } finally {
            setSaving(false);
        }
    };

    const handleDelete = async (project: ProjectSummary) => {
        setDeletingId(project.id);
        try {
            await deleteLocalProject(baseUrl, project.id);
            await refreshProjects();
            message.success("本地项目已删除");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "删除本地项目失败");
        } finally {
            setDeletingId("");
        }
    };

    return (
        <div className="space-y-3 rounded-md border border-stone-200 p-3 dark:border-stone-700">
            <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                    <div className="flex items-center gap-2">
                        <FolderGit2 className="size-4 text-stone-500" />
                        <Typography.Text strong>本地项目</Typography.Text>
                    </div>
                    <Typography.Text type="secondary" className="block !text-xs">
                        列表只展示 project id、权限和 fingerprint；编辑时才读取本机路径。
                    </Typography.Text>
                </div>
                <Space>
                    <Button icon={<RefreshCw className="size-4" />} onClick={() => void refreshProjects()} loading={loading}>
                        刷新
                    </Button>
                    <Button type="primary" icon={<Plus className="size-4" />} onClick={openCreate}>
                        新建
                    </Button>
                </Space>
            </div>

            <Spin spinning={loading}>
                {projects.length === 0 ? (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无本地项目引用" />
                ) : (
                    <div className="space-y-2">
                        {projects.map((project) => (
                            <div key={project.id} className="rounded-md border border-stone-200 bg-white/70 p-3 dark:border-stone-700 dark:bg-stone-900/60">
                                <div className="flex items-start justify-between gap-3">
                                    <div className="min-w-0 space-y-2">
                                        <div className="flex flex-wrap items-center gap-2">
                                            <Typography.Text strong className="min-w-0 max-w-64 truncate">
                                                {project.name}
                                            </Typography.Text>
                                            <Tag>{project.kind || "code"}</Tag>
                                            <Tag>{project.adapter || "generic"}</Tag>
                                            {project.hasRootPath ? <Tag color="blue">root linked</Tag> : <Tag>no root</Tag>}
                                        </div>
                                        <div className="flex flex-wrap gap-1">{displayCapabilities(project.capabilities)}</div>
                                        <Typography.Text type="secondary" className="block !text-xs">
                                            id: <Typography.Text code>{project.id}</Typography.Text>
                                            {project.rootFingerprint ? (
                                                <>
                                                    {" "}
                                                    fingerprint: <Typography.Text code>{project.rootFingerprint}</Typography.Text>
                                                </>
                                            ) : null}
                                        </Typography.Text>
                                    </div>
                                    <Space>
                                        <Tooltip title="编辑">
                                            <Button size="small" icon={<Pencil className="size-4" />} loading={loadingProjectId === project.id} onClick={() => void openEdit(project)} />
                                        </Tooltip>
                                        <Popconfirm title="删除本地项目引用？" description="只删除 workspace 中的项目引用，不删除外部项目文件。" okText="删除" cancelText="取消" onConfirm={() => void handleDelete(project)}>
                                            <Tooltip title="删除">
                                                <Button danger size="small" icon={<Trash2 className="size-4" />} loading={deletingId === project.id} />
                                            </Tooltip>
                                        </Popconfirm>
                                    </Space>
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </Spin>

            <Modal
                title={draft.id ? "编辑本地项目" : "新建本地项目"}
                open={editorOpen}
                onCancel={() => setEditorOpen(false)}
                onOk={() => void handleSave()}
                confirmLoading={saving}
                okText="保存"
                cancelText="取消"
                destroyOnHidden
            >
                <div className="space-y-4">
                    {draft.hasCredentialRefs ? (
                        <div className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-100">
                            该项目包含 credentialRef。当前 Web UI 只拿到脱敏摘要，不会写回保存；请使用 CLI 修改含密钥引用的项目。
                        </div>
                    ) : null}
                    <div className="space-y-2">
                        <Typography.Text strong>项目名称</Typography.Text>
                        <Input value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-2">
                            <Typography.Text strong>类型</Typography.Text>
                            <Input value={draft.kind} onChange={(event) => setDraft((current) => ({ ...current, kind: event.target.value }))} />
                        </div>
                        <div className="space-y-2">
                            <Typography.Text strong>Adapter</Typography.Text>
                            <Input value={draft.adapter} onChange={(event) => setDraft((current) => ({ ...current, adapter: event.target.value }))} />
                        </div>
                    </div>
                    <div className="space-y-2">
                        <Typography.Text strong>本机项目路径</Typography.Text>
                        <Input placeholder="/abs/path/to/project" value={draft.rootPath} onChange={(event) => setDraft((current) => ({ ...current, rootPath: event.target.value }))} />
                        <Typography.Text type="secondary" className="block !text-xs">
                            保存时后端会校验绝对路径、计算 fingerprint，并阻止路径逃逸。
                        </Typography.Text>
                    </div>
                    <div className="space-y-2">
                        <Typography.Text strong>能力</Typography.Text>
                        <Checkbox.Group options={PROJECT_CAPABILITIES} value={draft.capabilityKeys} onChange={(keys) => setDraft((current) => ({ ...current, capabilityKeys: keys.map(String) }))} />
                    </div>
                </div>
            </Modal>
        </div>
    );
}
