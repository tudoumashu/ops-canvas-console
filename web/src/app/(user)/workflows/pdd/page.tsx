"use client";

import { useEffect, useMemo, useState } from "react";
import type { ComponentProps } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { App, Button, Card, Input, InputNumber, Modal, Select, Space, Table, Tag, Typography, Upload } from "antd";
import type { ColumnsType } from "antd/es/table";
import { LoaderCircle, Play, RefreshCw, TerminalSquare, Upload as UploadIcon, Workflow } from "lucide-react";

import { fetchPDDRuns, fetchPDDWorkflowTemplates, startPDDWorkflowTemplateRun, type PDDRunItem, type PDDRunStatus, type WorkflowTemplate } from "@/services/api/pdd";
import { fetchLocalPDDWorkflowTemplates, startLocalPDDWorkflowTemplateRun } from "@/services/local-workflow-templates";
import { listLocalRuns, type LocalRunSummary } from "@/services/local-workspace";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";
import { useUserStore } from "@/stores/use-user-store";

type WorkflowRunRow = Omit<PDDRunItem, "status"> & {
    source: "server" | "local";
    status: PDDRunStatus | "pending" | "canceled" | string;
    templateId?: string;
    artifactCount?: number;
    latestEventSequence?: number;
};

const statusMeta: Record<string, { color: string; label: string }> = {
    idle: { color: "default", label: "idle" },
    pending: { color: "default", label: "pending" },
    running: { color: "processing", label: "running" },
    success: { color: "success", label: "success" },
    error: { color: "error", label: "error" },
    canceled: { color: "warning", label: "canceled" },
};

export default function PDDRunsPage() {
    const { message } = App.useApp();
    const router = useRouter();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const localWorkspaceStatus = useLocalWorkspaceStore((state) => state.status);
    const localWorkspace = useLocalWorkspaceStore((state) => state.workspace);
    const localWorkspaceBaseUrl = useLocalWorkspaceStore((state) => state.baseUrl);
    const localConnected = localWorkspaceStatus === "connected" && Boolean(localWorkspace);
    const [keyword, setKeyword] = useState("");
    const [starting, setStarting] = useState(false);
    const [startOpen, setStartOpen] = useState(false);
    const query = useQuery({
        queryKey: ["workflows", "pdd", "runs", token],
        queryFn: () => fetchPDDRuns(token),
        enabled: Boolean(token),
        refetchInterval: 15000,
    });
    const localRunsQuery = useQuery({
        queryKey: ["local-runs", localWorkspaceBaseUrl],
        queryFn: () => listLocalRuns(localWorkspaceBaseUrl),
        enabled: localConnected,
        refetchInterval: 15000,
    });

    const rows = useMemo(() => {
        const text = keyword.trim().toLowerCase();
        const localRows = (localRunsQuery.data?.runs || []).map(localRunToRow);
        const serverRows: WorkflowRunRow[] = (query.data?.items || []).map((item) => ({ ...item, source: "server" }));
        return [...localRows, ...serverRows].filter((item) => !text || item.runId.toLowerCase().includes(text) || item.recentError?.toLowerCase().includes(text) || item.templateId?.toLowerCase().includes(text));
    }, [keyword, localRunsQuery.data?.runs, query.data?.items]);

    const columns: ColumnsType<WorkflowRunRow> = [
        {
            title: "Run",
            dataIndex: "runId",
            render: (_, item) => (
                <Space size={6}>
                    <Link className="font-mono font-medium text-blue-600 dark:text-blue-300" href={`/workflows/ecommerce/${encodeURIComponent(item.runId)}`}>
                        {item.runId}
                    </Link>
                    {item.source === "local" ? <Tag>local</Tag> : null}
                </Space>
            ),
        },
        {
            title: "状态",
            dataIndex: "status",
            width: 120,
            render: (status: string) => (
                <Tag color={statusMeta[status]?.color}>
                    {status === "running" ? <LoaderCircle className="mr-1 inline size-3 animate-spin align-[-2px]" /> : null}
                    {statusMeta[status]?.label || status}
                </Tag>
            ),
        },
        {
            title: "商品",
            width: 180,
            render: (_, item) =>
                item.source === "local" ? (
                    <span className="font-mono text-xs">{item.artifactCount ?? 0} artifacts</span>
                ) : (
                    <span className="text-sm">
                        <span className="text-green-600">{item.completedProducts}</span>
                        <span className="text-stone-400"> / </span>
                        <span className="text-red-500">{item.failedProducts}</span>
                        <span className="text-stone-400"> / </span>
                        <span>{item.productTotal}</span>
                    </span>
                ),
        },
        {
            title: "日志",
            width: 90,
            render: (_, item) => <Tag color={item.hasLogs ? "green" : "orange"}>{item.source === "local" ? `${item.latestEventSequence ?? 0}` : item.hasLogs ? "完整" : "缺失"}</Tag>,
        },
        {
            title: "更新时间",
            dataIndex: "updatedAt",
            width: 210,
            render: (value: string) => <span className="font-mono text-xs">{value || "-"}</span>,
        },
        {
            title: "最近错误",
            dataIndex: "recentError",
            ellipsis: true,
            render: (value?: string) => <span className="text-xs text-red-500">{value || "-"}</span>,
        },
    ];

    const startRun = async (templateId: string, inputs: Array<Record<string, unknown>>, productConcurrency?: number, maxRetries?: number) => {
        if (!localConnected && !token) return;
        setStarting(true);
        try {
            const result = localConnected
                ? await startLocalPDDWorkflowTemplateRun(localWorkspaceBaseUrl, templateId, { inputs, productConcurrency, maxRetries })
                : await startPDDWorkflowTemplateRun(templateId, { inputs, productConcurrency, maxRetries }, token);
            message.success(`${localConnected ? "已创建本地" : "已启动"} run ${result.runId || ""}`);
            setStartOpen(false);
            if (token) await query.refetch();
            if (localConnected) await localRunsQuery.refetch();
            if (result.runId) router.push(`/workflows/ecommerce/${encodeURIComponent(result.runId)}`);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "启动失败");
        } finally {
            setStarting(false);
        }
    };

    if ((!token || !user) && !localConnected) {
        return (
            <main className="flex h-full items-center justify-center bg-background px-6 text-foreground">
                <Card className="w-full max-w-md">
                    <Typography.Title level={3}>需要登录或连接本地工作区</Typography.Title>
                    <Typography.Paragraph type="secondary">VPS run 需要管理员登录；本地 run 需要先在顶部入口连接 `opsc serve`。</Typography.Paragraph>
                    <Button type="primary" href="/login">
                        去登录
                    </Button>
                </Card>
            </main>
        );
    }

    return (
        <main className="h-full overflow-auto bg-background text-foreground">
            <div className="mx-auto flex w-full max-w-7xl flex-col gap-5 px-6 py-8">
                <header className="flex flex-wrap items-end justify-between gap-4 border-b border-stone-200 pb-5 dark:border-stone-800">
                    <div>
                        <Typography.Text type="secondary" className="text-xs">
                            {localConnected ? `local: ${localWorkspace?.name || "Workspace"}` : `root: ${query.data?.root || "/opt/pdd-workflow/runs"}`}
                        </Typography.Text>
                        <Typography.Title level={2} className="!mb-0 !mt-2">
                            电商工作流
                        </Typography.Title>
                    </div>
                    <Space wrap>
                        <Input.Search allowClear placeholder="搜索 run_id / 错误" value={keyword} onChange={(event) => setKeyword(event.target.value)} className="w-[260px]" />
                        <Button type="primary" icon={<Play className="size-4" />} loading={starting} disabled={!token && !localConnected} onClick={() => setStartOpen(true)}>
                            启动工作流
                        </Button>
                        <Button icon={<Workflow className="size-4" />} href="/workflows/ecommerce/templates">
                            工作流模板
                        </Button>
                        <Button
                            icon={<RefreshCw className="size-4" />}
                            loading={query.isFetching || localRunsQuery.isFetching}
                            onClick={() => void Promise.all([token ? query.refetch() : Promise.resolve(), localConnected ? localRunsQuery.refetch() : Promise.resolve()]).catch((error) => message.error(error instanceof Error ? error.message : "刷新失败"))}
                        >
                            刷新
                        </Button>
                        <Button icon={<TerminalSquare className="size-4" />} href={`/workflows/ecommerce/${encodeURIComponent(rows[0]?.runId || "")}`} disabled={!rows.length}>
                            打开最新
                        </Button>
                    </Space>
                </header>

                <Table rowKey={(item) => `${item.source}:${item.runId}`} columns={columns} dataSource={rows} loading={query.isLoading || localRunsQuery.isLoading} pagination={{ pageSize: 20, showSizeChanger: true }} />
                <StartRunModal
                    open={startOpen}
                    loading={starting}
                    token={token}
                    useLocalTemplates={localConnected}
                    localBaseUrl={localWorkspaceBaseUrl}
                    onCancel={() => setStartOpen(false)}
                    onStart={(templateId, inputs, productConcurrency, maxRetries) => void startRun(templateId, inputs, productConcurrency, maxRetries)}
                />
            </div>
        </main>
    );
}

function localRunToRow(item: LocalRunSummary): WorkflowRunRow {
    return {
        source: "local",
        runId: item.id,
        status: item.status,
        runDir: `local:${item.id}`,
        updatedAt: item.updatedAt,
        customWorkflow: true,
        completed: item.status === "success" || item.status === "error" || item.status === "canceled",
        hasLogs: item.latestEventSequence > 0,
        productTotal: 0,
        completedProducts: 0,
        failedProducts: item.status === "error" ? 1 : 0,
        runningProducts: item.status === "running" ? 1 : 0,
        templateId: item.templateId,
        artifactCount: item.artifactCount,
        latestEventSequence: item.latestEventSequence,
    };
}

function StartRunModal({
    open,
    loading,
    token,
    useLocalTemplates,
    localBaseUrl,
    onCancel,
    onStart,
}: {
    open: boolean;
    loading: boolean;
    token?: string;
    useLocalTemplates: boolean;
    localBaseUrl: string;
    onCancel: () => void;
    onStart: (templateId: string, inputs: Array<Record<string, unknown>>, productConcurrency?: number, maxRetries?: number) => void;
}) {
    const { message } = App.useApp();
    const [themesText, setThemesText] = useState("");
    const [templateId, setTemplateId] = useState("");
    const [productConcurrency, setProductConcurrency] = useState<number | null>(null);
    const [maxRetries, setMaxRetries] = useState<number | null>(null);
    const templateQuery = useQuery({
        queryKey: ["pdd-workflow-templates", "start", useLocalTemplates ? "local" : "server", useLocalTemplates ? localBaseUrl : token],
        queryFn: () => (useLocalTemplates ? fetchLocalPDDWorkflowTemplates(localBaseUrl) : fetchPDDWorkflowTemplates(token || "")),
        enabled: open && (useLocalTemplates || Boolean(token)),
    });
    const templates = templateQuery.data?.items || [];
    const selectedTemplate = templates.find((item) => item.id === templateId) || templates[0];

    useEffect(() => {
        if (!templates[0]?.id) return;
        if (!templateId || !templates.some((item) => item.id === templateId)) setTemplateId(templates[0].id);
    }, [templateId, templates]);

    const importJson = async (file: File) => {
        try {
            const text = await file.text();
            const payload = JSON.parse(text);
            if (Array.isArray(payload)) {
                setThemesText(JSON.stringify({ themes: payload }, null, 2));
            } else {
                setThemesText(JSON.stringify(payload, null, 2));
            }
            message.success("JSON 已导入");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "JSON 导入失败");
        }
        return false;
    };

    const submit = () => {
        try {
            const inputs = parseTemplateInputs(themesText);
            if (!templateId) throw new Error("请先选择工作流模板");
            if (!inputs.length) throw new Error("请先输入至少 1 条商品输入");
            onStart(templateId, inputs, productConcurrency || selectedTemplate?.spec.settings.productConcurrency, maxRetries || selectedTemplate?.spec.settings.maxRetries);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "任务配置不完整");
        }
    };

    return (
        <Modal
            title={useLocalTemplates ? "启动本地电商工作流" : "启动电商工作流"}
            open={open}
            onCancel={onCancel}
            width={820}
            destroyOnHidden={false}
            footer={
                <Space>
                    <Button onClick={onCancel}>取消</Button>
                    <Button type="primary" icon={<Play className="size-4" />} loading={loading} onClick={submit}>
                        启动
                    </Button>
                </Space>
            }
        >
            <div className="flex flex-col gap-4">
                <Card size="small" title="选择模板">
                    <Space orientation="vertical" className="w-full">
                        <Select
                            className="w-full"
                            loading={templateQuery.isLoading}
                            value={templateId || undefined}
                            placeholder="选择一个工作流模板"
                            options={templates.map((item: WorkflowTemplate) => ({ label: item.title, value: item.id }))}
                            onChange={setTemplateId}
                        />
                        {selectedTemplate ? (
                            <Typography.Text type="secondary" className="text-xs">
                                {selectedTemplate.description || "无模板说明"} · {selectedTemplate.spec.nodes.length} 节点 / {selectedTemplate.spec.edges.length} 连线
                            </Typography.Text>
                        ) : null}
                    </Space>
                </Card>
                <Card size="small" title="运行参数">
                    <div className="grid grid-cols-2 gap-3">
                        <LabeledNumberInput label="商品并发" min={1} max={20} value={productConcurrency ?? selectedTemplate?.spec.settings.productConcurrency ?? 2} onChange={(value) => setProductConcurrency(Number(value || 1))} />
                        <LabeledNumberInput label="重试" min={0} max={100} value={maxRetries ?? selectedTemplate?.spec.settings.maxRetries ?? 0} onChange={(value) => setMaxRetries(Number(value ?? 0))} />
                    </div>
                </Card>
                <Card
                    size="small"
                    title="输入商品"
                    extra={
                        <Upload accept=".json,application/json" showUploadList={false} beforeUpload={importJson}>
                            <Button icon={<UploadIcon className="size-4" />}>导入 JSON</Button>
                        </Upload>
                    }
                >
                    <Input.TextArea
                        value={themesText}
                        onChange={(event) => setThemesText(event.target.value)}
                        rows={9}
                        placeholder={'每行一个 JSON 对象：\n{"theme":"《原神》","character":"七七","presentation":"feminine"}\n{"theme":"《原神》","character":"丽莎","presentation":"feminine"}\n\n也可以粘贴 JSON 数组或 {"themes":[...]}'}
                    />
                </Card>
            </div>
        </Modal>
    );
}

function LabeledNumberInput({ label, ...props }: ComponentProps<typeof InputNumber> & { label: string }) {
    return (
        <Space.Compact className="w-full">
            <span className="inline-flex h-8 shrink-0 items-center rounded-l-md border border-r-0 border-stone-300 bg-stone-50 px-3 text-sm text-stone-600 dark:border-stone-700 dark:bg-stone-900 dark:text-stone-300">{label}</span>
            <InputNumber {...props} className="!w-full" />
        </Space.Compact>
    );
}

function parseTemplateInputs(text: string) {
    const value = text.trim();
    if (!value) return [];
    if (value.startsWith("{") || value.startsWith("[")) {
        try {
            return parsedInputItems(JSON.parse(value)).map(normalizeInput);
        } catch (error) {
            if (!value.includes("\n")) throw error;
        }
    }
    const items: Array<Record<string, unknown>> = [];
    value
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter(Boolean)
        .forEach((line, index) => {
            if (line.startsWith("{") || line.startsWith("[")) {
                try {
                    items.push(...parsedInputItems(JSON.parse(line)).map(normalizeInput));
                } catch (error) {
                    const reason = error instanceof Error ? error.message : "JSON 格式不正确";
                    throw new Error(`第 ${index + 1} 行 JSON 格式不正确：${reason}`);
                }
                return;
            }
            items.push({ theme: line });
        });
    return items;
}

function parsedInputItems(parsed: unknown) {
    if (Array.isArray(parsed)) return parsed;
    if (parsed && typeof parsed === "object") {
        const record = parsed as { items?: unknown; themes?: unknown };
        if (Array.isArray(record.items)) return record.items;
        if (Array.isArray(record.themes)) return record.themes;
    }
    return [parsed];
}

function normalizeInput(value: unknown) {
    if (typeof value === "string") return { theme: value };
    if (value && typeof value === "object") return value as Record<string, unknown>;
    return { theme: String(value || "") };
}
