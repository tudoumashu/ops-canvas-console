"use client";

import { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { App, Button, Card, Empty, Modal, Space, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import { ArrowLeft, FileImage, LoaderCircle, RefreshCw, Workflow } from "lucide-react";

import { fetchLocalRunEvents, fetchLocalRunStatus, listLocalRunArtifacts, localArtifactFileObjectUrl, type LocalRunArtifactSummary, type LocalRunEvent, type LocalRunNodeStateSummary } from "@/services/local-workspace";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";

const statusColor: Record<string, string> = {
    pending: "default",
    running: "processing",
    success: "success",
    error: "error",
    canceled: "warning",
};

export default function LocalRunClientPage({ runId }: { runId: string }) {
    const { message } = App.useApp();
    const localWorkspaceStatus = useLocalWorkspaceStore((state) => state.status);
    const localWorkspace = useLocalWorkspaceStore((state) => state.workspace);
    const baseUrl = useLocalWorkspaceStore((state) => state.baseUrl);
    const connected = localWorkspaceStatus === "connected" && Boolean(localWorkspace);
    const [preview, setPreview] = useState<{ title: string; url: string } | null>(null);

    const statusQuery = useQuery({
        queryKey: ["local-run-status", baseUrl, runId],
        queryFn: () => fetchLocalRunStatus(baseUrl, runId),
        enabled: connected,
        refetchInterval: (query) => {
            const status = query.state.data?.run.status;
            return status === "running" ? 3000 : false;
        },
    });
    const eventsQuery = useQuery({
        queryKey: ["local-run-events", baseUrl, runId],
        queryFn: () => fetchLocalRunEvents(baseUrl, runId),
        enabled: connected,
        refetchInterval: statusQuery.data?.run.status === "running" ? 3000 : false,
    });
    const artifactsQuery = useQuery({
        queryKey: ["local-run-artifacts", baseUrl, runId],
        queryFn: () => listLocalRunArtifacts(baseUrl, runId),
        enabled: connected,
    });

    const openArtifact = useCallback(
        async (item: LocalRunArtifactSummary) => {
            try {
                const url = await localArtifactFileObjectUrl(baseUrl, item.artifact.id, "original");
                setPreview({ title: item.artifact.title || item.artifact.id, url });
            } catch (error) {
                message.error(error instanceof Error ? error.message : "读取本地 artifact 失败");
            }
        },
        [baseUrl, message],
    );

    const nodeColumns: ColumnsType<LocalRunNodeStateSummary> = useMemo(
        () => [
            { title: "节点", dataIndex: "nodeId", render: (value: string) => <span className="font-mono text-xs">{value}</span> },
            { title: "状态", dataIndex: "status", width: 120, render: (value: string) => <StatusTag status={value} /> },
            { title: "错误", dataIndex: "error", ellipsis: true, render: (value?: string) => <span className="text-xs text-red-500">{value || "-"}</span> },
            { title: "更新时间", dataIndex: "updatedAt", width: 220, render: (value: string) => <span className="font-mono text-xs">{value || "-"}</span> },
        ],
        [],
    );
    const artifactColumns: ColumnsType<LocalRunArtifactSummary> = useMemo(
        () => [
            { title: "Artifact", render: (_, item) => <span className="font-mono text-xs">{item.artifact.id}</span> },
            { title: "标题", render: (_, item) => item.artifact.title || "-" },
            { title: "类型", render: (_, item) => <Tag>{item.artifact.type}</Tag>, width: 100 },
            { title: "节点", render: (_, item) => <span className="font-mono text-xs">{item.ref.nodeId || "-"}</span>, width: 160 },
            {
                title: "操作",
                width: 120,
                render: (_, item) => (
                    <Button size="small" icon={<FileImage className="size-3.5" />} disabled={!item.artifact.original} onClick={() => void openArtifact(item)}>
                        预览
                    </Button>
                ),
            },
        ],
        [openArtifact],
    );
    const eventColumns: ColumnsType<LocalRunEvent> = useMemo(
        () => [
            { title: "#", dataIndex: "sequence", width: 70 },
            { title: "类型", dataIndex: "type", render: (value: string) => <span className="font-mono text-xs">{value}</span> },
            { title: "级别", dataIndex: "level", width: 90, render: (value: string) => <Tag color={value === "error" ? "error" : value === "warn" ? "warning" : "default"}>{value}</Tag> },
            { title: "消息", dataIndex: "message", ellipsis: true },
            { title: "时间", dataIndex: "createdAt", width: 220, render: (value: string) => <span className="font-mono text-xs">{value}</span> },
        ],
        [],
    );

    if (!connected) {
        return (
            <main className="grid h-full place-items-center bg-background px-6 text-foreground">
                <Card className="w-full max-w-md">
                    <Typography.Title level={3}>需要连接本地工作区</Typography.Title>
                    <Typography.Paragraph type="secondary">本地 run 只保存在当前 local workspace 中，请先通过顶部入口连接 `opsc serve`。</Typography.Paragraph>
                    <Button href="/workflows/ecommerce" icon={<ArrowLeft className="size-4" />}>
                        返回工作流
                    </Button>
                </Card>
            </main>
        );
    }

    const run = statusQuery.data?.run;
    const loading = statusQuery.isLoading || eventsQuery.isLoading || artifactsQuery.isLoading;

    return (
        <main className="h-full overflow-auto bg-background text-foreground">
            <div className="mx-auto flex w-full max-w-7xl flex-col gap-5 px-6 py-8">
                <header className="flex flex-wrap items-end justify-between gap-4 border-b border-stone-200 pb-5 dark:border-stone-800">
                    <div className="min-w-0">
                        <Typography.Text type="secondary" className="text-xs">
                            Local Workspace / Run
                        </Typography.Text>
                        <Typography.Title level={2} className="!mb-0 !mt-2 truncate">
                            {runId}
                        </Typography.Title>
                    </div>
                    <Space wrap>
                        {run ? <StatusTag status={run.status} /> : null}
                        <Button href="/workflows/ecommerce" icon={<ArrowLeft className="size-4" />}>
                            返回列表
                        </Button>
                        {run?.templateId ? (
                            <Button href={`/workflows/ecommerce/templates/${encodeURIComponent(run.templateId)}`} icon={<Workflow className="size-4" />}>
                                打开模板
                            </Button>
                        ) : null}
                        <Button
                            icon={<RefreshCw className="size-4" />}
                            loading={loading}
                            onClick={() => {
                                void statusQuery.refetch();
                                void eventsQuery.refetch();
                                void artifactsQuery.refetch();
                            }}
                        >
                            刷新
                        </Button>
                    </Space>
                </header>

                {statusQuery.isError ? (
                    <Card>
                        <Empty description={statusQuery.error instanceof Error ? statusQuery.error.message : "读取本地 run 失败"} />
                    </Card>
                ) : (
                    <>
                        <Card size="small" title="运行概览">
                            <div className="grid gap-4 md:grid-cols-4">
                                <Metric label="状态" value={run?.status || "-"} />
                                <Metric label="Artifact" value={String(run?.artifactCount ?? 0)} />
                                <Metric label="事件" value={String(run?.latestEventSequence ?? 0)} />
                                <Metric label="更新时间" value={run?.updatedAt || "-"} />
                            </div>
                        </Card>
                        <Card size="small" title="节点状态">
                            <Table rowKey="nodeId" size="small" columns={nodeColumns} dataSource={statusQuery.data?.nodes || []} loading={statusQuery.isLoading} pagination={false} />
                        </Card>
                        <Card size="small" title="Artifacts">
                            <Table rowKey={(item) => item.artifact.id} size="small" columns={artifactColumns} dataSource={artifactsQuery.data?.artifacts || []} loading={artifactsQuery.isLoading} pagination={false} />
                        </Card>
                        <Card size="small" title="事件">
                            <Table rowKey="id" size="small" columns={eventColumns} dataSource={eventsQuery.data?.events || []} loading={eventsQuery.isLoading} pagination={false} />
                        </Card>
                    </>
                )}
            </div>
            <Modal title={preview?.title} open={Boolean(preview)} footer={null} onCancel={() => setPreview(null)} width={860}>
                {preview ? <img src={preview.url} alt={preview.title} className="max-h-[70vh] w-full object-contain" draggable={false} /> : null}
            </Modal>
        </main>
    );
}

function StatusTag({ status }: { status: string }) {
    return (
        <Tag color={statusColor[status] || "default"}>
            {status === "running" ? <LoaderCircle className="mr-1 inline size-3 animate-spin align-[-2px]" /> : null}
            {status}
        </Tag>
    );
}

function Metric({ label, value }: { label: string; value: string }) {
    return (
        <div>
            <div className="text-xs text-stone-500">{label}</div>
            <div className="mt-1 truncate font-mono text-sm">{value}</div>
        </div>
    );
}
