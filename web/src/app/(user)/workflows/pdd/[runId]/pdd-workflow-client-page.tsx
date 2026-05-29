"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ChangeEvent as ReactChangeEvent, ReactNode, PointerEvent as ReactPointerEvent } from "react";
import Link from "next/link";
import { saveAs } from "file-saver";
import { nanoid } from "nanoid";
import { useQuery } from "@tanstack/react-query";
import { App, Button, Card, Drawer, Dropdown, Empty, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tabs, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { MenuProps } from "antd";
import { Activity, ArrowLeft, Boxes, CircleStop, ExternalLink, FileJson, FileText, Focus, Image as ImageIcon, Info, Layers3, List, LoaderCircle, Play, RefreshCw, ServerCog, WandSparkles, Wrench, X, ZoomIn, ZoomOut } from "lucide-react";

import { InfiniteCanvas } from "@/app/(user)/canvas/components/infinite-canvas";
import { Minimap } from "@/app/(user)/canvas/components/canvas-mini-map";
import { ActiveConnectionPath, ConnectionPath } from "@/app/(user)/canvas/components/canvas-connections";
import { CanvasNode } from "@/app/(user)/canvas/components/canvas-node";
import { CanvasZoomControls } from "@/app/(user)/canvas/components/canvas-zoom-controls";
import { CanvasToolbar } from "@/app/(user)/canvas/components/canvas-toolbar";
import { CanvasNodeHoverToolbar, CanvasNodeInfoModal } from "@/app/(user)/canvas/components/canvas-node-hover-toolbar";
import { CanvasConfigNodePanel } from "@/app/(user)/canvas/components/canvas-config-node-panel";
import { buildNodeChatMessages, buildNodeGenerationContext, buildNodeGenerationInputs, hydrateNodeGenerationContext, type NodeGenerationInput } from "@/app/(user)/canvas/components/canvas-node-generation";
import { CanvasNodeAngleDialog, type CanvasImageAngleParams } from "@/app/(user)/canvas/components/canvas-node-angle-dialog";
import { CanvasNodeCropDialog, type CanvasImageCropRect } from "@/app/(user)/canvas/components/canvas-node-crop-dialog";
import { CanvasNodePromptPanel, type CanvasNodeGenerationMode } from "@/app/(user)/canvas/components/canvas-node-prompt-panel";
import { NODE_DEFAULT_SIZE, getNodeSpec } from "@/app/(user)/canvas/constants";
import { CanvasNodeType, type CanvasConnection, type CanvasImageGenerationType, type CanvasNodeData, type CanvasNodeMetadata, type ConnectionHandle, type Position, type ViewportTransform } from "@/app/(user)/canvas/types";
import { cropDataUrl } from "@/app/(user)/canvas/utils/canvas-image-data";
import { fitNodeSize, nodeSizeFromRatio } from "@/app/(user)/canvas/utils/canvas-node-size";
import { canvasThemes, type CanvasBackgroundMode } from "@/lib/canvas-theme";
import { requestEdit, requestGeneration, requestImageQuestion } from "@/services/api/image";
import {
    applyPDDCreativeCanvasOutput,
    createPDDManualEdit,
    fetchPDDCreativeCanvas,
    fetchPDDProductDetail,
    fetchPDDRunOverview,
    runPDDAction,
    savePDDCreativeCanvas,
    uploadPDDCreativeCanvasAsset,
    withPDDFileToken,
    type PDDActionRequest,
    type PDDArtifact,
    type PDDCreativeCanvas,
    type PDDDetailFile,
    type PDDGraphEdge,
    type PDDGraphNode,
    type PDDProductDetail,
    type PDDProductSummary,
    type PDDRunOverview,
    type PDDRunStatus,
    type PDDStageNode,
} from "@/services/api/pdd";
import { requestVideoGeneration } from "@/services/api/video";
import { uploadMediaFile } from "@/services/file-storage";
import { uploadImage } from "@/services/image-storage";
import { defaultConfig, type AiConfig, useConfigStore, useEffectiveConfig } from "@/stores/use-config-store";
import { useAssetStore } from "@/stores/use-asset-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";
import type { ReferenceImage } from "@/types/image";

type DetailTarget = { kind: "stage"; stage: PDDStageNode } | { kind: "product-node"; node: PDDGraphNode; product: PDDProductDetail } | { kind: "artifact"; artifact: PDDArtifact; product: PDDProductDetail } | { kind: "file"; file: PDDDetailFile };

type RunViewMode = "overview" | "product" | "creative";
type DrawerKind = "products" | "detail" | "log" | null;
type DetailTabKey = "summary" | "files" | "log";
type LightboxImage = { title: string; src: string; path: string; artifact?: PDDArtifact; node?: PDDGraphNode; product?: PDDProductDetail };
type ManualEditTarget = { artifact: PDDArtifact; node: PDDGraphNode; product: PDDProductDetail };

const stagePositions = {
    y: 40,
    width: 230,
    height: 150,
    gap: 285,
};
const creativeVideoNodeMaxWidth = 420;
const creativeVideoNodeMaxHeight = 420;

const statusMeta: Record<PDDRunStatus, { label: string; color: string; className: string }> = {
    idle: { label: "idle", color: "default", className: "border-stone-300 bg-stone-50 dark:border-stone-700 dark:bg-stone-900" },
    running: { label: "running", color: "processing", className: "border-blue-400 bg-blue-50 dark:border-blue-500/70 dark:bg-blue-950/40" },
    success: { label: "success", color: "success", className: "border-green-400 bg-green-50 dark:border-green-500/70 dark:bg-green-950/35" },
    error: { label: "error", color: "error", className: "border-red-400 bg-red-50 dark:border-red-500/70 dark:bg-red-950/35" },
};

export default function PDDRunClientPage({ runId }: { runId: string }) {
    const { message, modal } = App.useApp();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const [viewMode, setViewMode] = useState<RunViewMode>("overview");
    const [drawer, setDrawer] = useState<DrawerKind>(null);
    const [detailTab, setDetailTab] = useState<DetailTabKey>("summary");
    const [selectedProductKey, setSelectedProductKey] = useState("");
    const [selectedTarget, setSelectedTarget] = useState<DetailTarget | null>(null);
    const [keyword, setKeyword] = useState("");
    const [statusFilter, setStatusFilter] = useState<string>("");
    const [actionOutput, setActionOutput] = useState("");
    const [viewport, setViewport] = useState<ViewportTransform>({ x: 80, y: 92, k: 0.78 });
    const [lightbox, setLightbox] = useState<LightboxImage | null>(null);
    const [manualEditTarget, setManualEditTarget] = useState<ManualEditTarget | null>(null);
    const [manualEditSubmitting, setManualEditSubmitting] = useState(false);
    const [manualMaskDataUrl, setManualMaskDataUrl] = useState("");
    const [manualEditForm] = Form.useForm();

    const overviewQuery = useQuery({
        queryKey: ["pdd-run-overview", runId, token],
        queryFn: () => fetchPDDRunOverview(runId, token),
        enabled: Boolean(token && runId),
        refetchInterval: 3000,
    });

    const runStatus = effectiveRunStatus(overviewQuery.data);
    const running = runStatus === "running";

    const detailQuery = useQuery({
        queryKey: ["pdd-product-detail", runId, selectedProductKey, token],
        queryFn: () => fetchPDDProductDetail(runId, selectedProductKey, token),
        enabled: Boolean(token && selectedProductKey),
        refetchInterval: selectedProductKey ? 3000 : false,
    });

    const rows = useMemo(() => {
        const text = keyword.trim().toLowerCase();
        return (overviewQuery.data?.products || []).filter((item) => {
            if (statusFilter && item.status !== statusFilter && item.rawStatus !== statusFilter) return false;
            if (!text) return true;
            return `${item.sourceProduct} ${item.product} ${item.themeName} ${item.error || ""}`.toLowerCase().includes(text);
        });
    }, [keyword, overviewQuery.data?.products, statusFilter]);

    const selectedProduct = useMemo(() => rows.find((item) => item.key === selectedProductKey) || null, [rows, selectedProductKey]);

    useEffect(() => {
        if (!rows.length) return;
        if (!selectedProductKey || !rows.some((item) => item.key === selectedProductKey)) setSelectedProductKey(rows[0].key);
    }, [rows, selectedProductKey]);

    useEffect(() => {
        if (!detailQuery.data || !selectedTarget) return;
        if (selectedTarget.kind === "product-node") {
            const node = detailQuery.data.nodes.find((item) => item.id === selectedTarget.node.id);
            if (node && node !== selectedTarget.node) setSelectedTarget({ kind: "product-node", node, product: detailQuery.data });
        }
        if (selectedTarget.kind === "artifact") {
            const artifact = detailQuery.data.nodes.flatMap((node) => node.artifacts || []).find((item) => item.id === selectedTarget.artifact.id);
            if (artifact && artifact !== selectedTarget.artifact) setSelectedTarget({ kind: "artifact", artifact, product: detailQuery.data });
        }
    }, [detailQuery.data, selectedTarget]);

    useEffect(() => {
        if (!selectedProductKey || !overviewQuery.data?.run.updatedAt) return;
        void detailQuery.refetch();
    }, [overviewQuery.data?.run.updatedAt, selectedProductKey]);

    if (!token || !user) {
        return (
            <main className="flex h-full items-center justify-center bg-background px-6 text-foreground">
                <Card className="w-full max-w-md">
                    <Typography.Title level={3}>需要登录</Typography.Title>
                    <Typography.Paragraph type="secondary">请先登录管理员账号后查看电商工作流。</Typography.Paragraph>
                    <Button type="primary" href="/login">
                        去登录
                    </Button>
                </Card>
            </main>
        );
    }

    const runAction = async (payload: PDDActionRequest) => {
        try {
            const result = await runPDDAction(payload, token);
            setActionOutput(result.output || "ok");
            message.success("动作已执行");
            await overviewQuery.refetch();
            if (selectedProductKey) await detailQuery.refetch();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "动作执行失败");
        }
    };

    const confirmServiceAction = (payload: PDDActionRequest, title: string) => {
        modal.confirm({
            title,
            content: "该动作会在 VPS 上执行受控命令，请确认当前没有关键任务被误中断。",
            okText: "执行",
            cancelText: "取消",
            onOk: () => runAction(payload),
        });
    };

    const selectProduct = (product: PDDProductSummary) => {
        setSelectedProductKey(product.key);
        setViewMode("product");
        setSelectedTarget(null);
        setDetailTab("summary");
        setDrawer(null);
        setViewport({ x: 120, y: 140, k: 0.72 });
    };

    const refreshAll = async () => {
        await overviewQuery.refetch();
        if (selectedProductKey) await detailQuery.refetch();
    };

    const resetViewport = () => setViewport(defaultRunViewport(viewMode));
    const openManualEdit = (target: ManualEditTarget) => {
        setManualEditTarget(target);
        setLightbox(null);
        manualEditForm.setFieldsValue({
            prompt: "",
            model: configString(target.node.config?.model) || "gpt-image-2",
            count: 1,
            size: configString(target.node.config?.size) || "1:1",
            quality: configString(target.node.config?.quality) || "high",
            apply: true,
            rerunDownstream: true,
            useMask: false,
        });
        setManualMaskDataUrl("");
    };
    const submitManualEdit = async () => {
        if (!manualEditTarget) return;
        try {
            const values = await manualEditForm.validateFields();
            if (values.useMask && !manualMaskDataUrl) {
                message.warning("请先在图片上涂抹需要局部修改的区域");
                return;
            }
            setManualEditSubmitting(true);
            const result = await createPDDManualEdit(
                runId,
                {
                    productKey: manualEditTarget.product.product.key,
                    nodeId: manualEditTarget.node.id,
                    artifactPath: manualEditTarget.artifact.path,
                    prompt: values.prompt,
                    model: values.model,
                    count: values.count,
                    size: values.size,
                    quality: values.quality,
                    maskDataUrl: values.useMask ? manualMaskDataUrl : undefined,
                    apply: Boolean(values.apply),
                    rerunDownstream: Boolean(values.apply && values.rerunDownstream),
                },
                token,
            );
            setActionOutput(result.output || `人工编辑副本已生成：${result.editId}`);
            message.success(result.output || "人工编辑副本已生成");
            setManualEditTarget(null);
            await refreshAll();
            const first = result.artifacts?.[0];
            if (first && !result.rerunDownstream) setLightbox({ title: first.title, src: withPDDFileToken(first.url, token), path: first.path });
        } catch (error) {
            message.error(error instanceof Error ? error.message : "人工编辑副本失败");
        } finally {
            setManualEditSubmitting(false);
        }
    };

    return (
        <main className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] bg-background text-foreground">
            <header className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-200 px-5 py-3 dark:border-stone-800">
                <Space wrap>
                    <Button href="/workflows/ecommerce" icon={<ArrowLeft className="size-4" />}>
                        工作流列表
                    </Button>
                    <div>
                        <Typography.Text type="secondary" className="text-xs">
                            当前 Run
                        </Typography.Text>
                        <Typography.Title level={4} className="!m-0 font-mono">
                            {runId}
                        </Typography.Title>
                    </div>
                    {overviewQuery.data?.run ? <StatusTag status={runStatus} /> : null}
                    {running ? (
                        <Typography.Text type="secondary" className="font-mono text-xs">
                            运行 {formatElapsed(overviewQuery.data?.run.startedAt)}
                        </Typography.Text>
                    ) : null}
                </Space>
                <Typography.Text type="secondary" className="max-w-[520px] truncate text-xs">
                    {viewMode === "creative" ? "创作画布" : viewMode === "product" ? selectedProduct?.sourceProduct || "商品流程" : "总览视图"}
                </Typography.Text>
            </header>

            <section className="relative min-h-0 overflow-hidden">
                <RunWorkflowCanvas
                    runId={runId}
                    mode={viewMode}
                    overview={overviewQuery.data}
                    detail={detailQuery.data}
                    selectedProductKey={selectedProductKey}
                    loading={overviewQuery.isLoading || (viewMode === "product" && detailQuery.isLoading && !detailQuery.data)}
                    token={token}
                    selectedTarget={selectedTarget}
                    viewport={viewport}
                    onViewportChange={setViewport}
                    onSelectStage={(stage) => {
                        setSelectedTarget({ kind: "stage", stage });
                        setDetailTab("summary");
                        setDrawer("detail");
                    }}
                    onSelectNode={(node, product) => {
                        setSelectedTarget({ kind: "product-node", node, product });
                        setDetailTab("summary");
                        setDrawer("detail");
                    }}
                    onOpenArtifact={(artifact, node, product) => {
                        setDrawer(null);
                        setLightbox({ title: artifact.title, src: withPDDFileToken(artifact.url, token), path: artifact.path, artifact, node, product });
                    }}
                    onOpenImage={(image) => {
                        setDrawer(null);
                        setLightbox(image);
                    }}
                />

                <RunBottomToolbar
                    runId={runId}
                    mode={viewMode}
                    hasProduct={Boolean(selectedProductKey)}
                    refreshing={overviewQuery.isFetching || detailQuery.isFetching}
                    onModeChange={(mode) => {
                        setViewMode(mode);
                        setSelectedTarget(null);
                        setDetailTab("summary");
                        setViewport(defaultRunViewport(mode));
                    }}
                    onOpenProducts={() => setDrawer("products")}
                    onOpenDetail={() => {
                        setDetailTab("summary");
                        setDrawer("detail");
                    }}
                    onOpenLog={() => {
                        setDetailTab("log");
                        setDrawer("log");
                    }}
                    onRefresh={() => void refreshAll()}
                    onResetViewport={resetViewport}
                    onRun={() => void runAction({ action: "run", runId })}
                    onStop={() => confirmServiceAction({ action: "stop", runId }, "停止当前 run？")}
                    onServiceAction={confirmServiceAction}
                />

                <Drawer title="商品列表" placement="right" width={520} open={drawer === "products"} onClose={() => setDrawer(null)}>
                    <ProductTable
                        rows={rows}
                        loading={overviewQuery.isLoading}
                        isCustomWorkflow={Boolean(overviewQuery.data?.run.customWorkflow)}
                        selectedKey={selectedProductKey}
                        keyword={keyword}
                        statusFilter={statusFilter}
                        onKeywordChange={setKeyword}
                        onStatusFilterChange={setStatusFilter}
                        onSelect={selectProduct}
                    />
                </Drawer>

                <Drawer title={detailTitle(selectedTarget)} placement="right" width={520} open={drawer === "detail" || drawer === "log"} onClose={() => setDrawer(null)}>
                    <DetailPanel
                        target={selectedTarget}
                        runId={runId}
                        token={token}
                        actionOutput={actionOutput}
                        recentErrors={overviewQuery.data?.recentErrors || []}
                        activeTab={detailTab}
                        onTabChange={setDetailTab}
                        onOpenImage={(image) => setLightbox(image)}
                    />
                </Drawer>
                <ImageLightbox image={lightbox} onClose={() => setLightbox(null)} />
                <Modal title="人工编辑副本" open={Boolean(manualEditTarget)} okText={manualEditForm.getFieldValue("rerunDownstream") ? "生成并重跑后续" : "生成副本"} cancelText="取消" confirmLoading={manualEditSubmitting} onOk={submitManualEdit} onCancel={() => setManualEditTarget(null)} destroyOnClose>
                    <Form form={manualEditForm} layout="vertical" preserve={false}>
                        <Typography.Paragraph type="secondary" className="!mb-3 text-xs">
                            当前图片会作为参考图生成一个人工编辑副本；勾选应用后，会用副本接管当前节点输出并重跑当前商品的后续节点。
                        </Typography.Paragraph>
	                        <Form.Item name="prompt" label="编辑要求" rules={[{ required: true, message: "请输入人工编辑要求" }]}>
	                            <Input.TextArea rows={5} placeholder="描述希望如何修改这张图片" />
	                        </Form.Item>
	                        <Form.Item name="useMask" valuePropName="checked" extra="只支持 gpt-image-2 图片编辑。涂抹区域会作为需要修改的局部，其余区域尽量保持不变。">
	                            <Switch checkedChildren="启用局部蒙版" unCheckedChildren="整图编辑" />
	                        </Form.Item>
	                        <Form.Item shouldUpdate noStyle>
	                            {({ getFieldValue }) =>
	                                getFieldValue("useMask") && manualEditTarget ? (
	                                    <ManualMaskCanvas
	                                        imageUrl={withPDDFileToken(manualEditTarget.artifact.url, token)}
	                                        onChange={setManualMaskDataUrl}
	                                        onClear={() => setManualMaskDataUrl("")}
	                                    />
	                                ) : null
	                            }
	                        </Form.Item>
	                        <div className="grid grid-cols-2 gap-3">
                            <Form.Item name="model" label="模型" rules={[{ required: true, message: "请输入模型" }]}>
                                <Input />
                            </Form.Item>
                            <Form.Item name="count" label="数量">
                                <InputNumber min={1} max={4} className="w-full" />
                            </Form.Item>
                            <Form.Item name="size" label="尺寸">
                                <Input placeholder="1:1 / 1024x1024" />
                            </Form.Item>
                            <Form.Item name="quality" label="质量">
                                <Select
                                    options={[
                                        { value: "high", label: "high" },
                                        { value: "medium", label: "medium" },
                                        { value: "low", label: "low" },
                                        { value: "auto", label: "auto" },
                                    ]}
                                />
                            </Form.Item>
                        </div>
                        <Form.Item name="apply" valuePropName="checked">
                            <Switch checkedChildren="应用副本" unCheckedChildren="仅保存" />
                        </Form.Item>
                        <Form.Item shouldUpdate noStyle>
                            {({ getFieldValue }) => (
                                <Form.Item name="rerunDownstream" valuePropName="checked">
                                    <Switch disabled={!getFieldValue("apply")} checkedChildren="重跑后续流程" unCheckedChildren="不重跑" />
                                </Form.Item>
                            )}
                        </Form.Item>
                    </Form>
                </Modal>
            </section>
        </main>
    );
}

function detailTitle(target: DetailTarget | null) {
    if (!target) return "Run 运行信息";
    if (target.kind === "stage") return target.stage.title;
    if (target.kind === "product-node") return target.node.title;
    if (target.kind === "artifact") return target.artifact.title;
    return target.file.title;
}

function effectiveRunStatus(overview?: PDDRunOverview): PDDRunStatus {
    const status = overview?.run.status || "idle";
    if (!overview || status !== "running") return status;
    if (overview.run.completed) return "success";
    const stagesDone = overview.stages.length > 0 && overview.stages.every((stage) => stage.status === "success");
    const productsDone = overview.products.length > 0 && overview.products.every((product) => product.status === "success");
    if (stagesDone && productsDone) return "success";
    return status;
}

function defaultRunViewport(mode: RunViewMode): ViewportTransform {
    if (mode === "overview") return { x: 80, y: 92, k: 0.78 };
    if (mode === "creative") return { x: 120, y: 120, k: 0.82 };
    return { x: 120, y: 140, k: 0.72 };
}

function RunBottomToolbar({
    runId,
    mode,
    hasProduct,
    refreshing,
    onModeChange,
    onOpenProducts,
    onOpenDetail,
    onOpenLog,
    onRefresh,
    onResetViewport,
    onRun,
    onStop,
    onServiceAction,
}: {
    runId: string;
    mode: RunViewMode;
    hasProduct: boolean;
    refreshing: boolean;
    onModeChange: (mode: RunViewMode) => void;
    onOpenProducts: () => void;
    onOpenDetail: () => void;
    onOpenLog: () => void;
    onRefresh: () => void;
    onResetViewport: () => void;
    onRun: () => void;
    onStop: () => void;
    onServiceAction: (payload: PDDActionRequest, title: string) => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const dockStyle = { background: theme.toolbar.panel, borderColor: theme.toolbar.border, color: theme.toolbar.item, boxShadow: "0 18px 45px rgba(0,0,0,.26)" };
    const opsItems: MenuProps["items"] = [
        { key: "health_check", icon: <Activity className="size-4" />, label: "健康检查" },
        { key: "docker_status", icon: <ServerCog className="size-4" />, label: "Docker 状态" },
        { type: "divider" },
        { key: "restart_chatgpt2api", icon: <Wrench className="size-4" />, label: "重启 chatgpt2api" },
        { key: "restart_sub2api", icon: <Wrench className="size-4" />, label: "重启 sub2api" },
        { key: "restart_cli_proxy", icon: <Wrench className="size-4" />, label: "重启 cli-proxy" },
        { key: "warp_reconnect", icon: <Wrench className="size-4" />, label: "WARP 重连" },
    ];
    const opsTitles: Record<string, string> = {
        health_check: "执行健康检查？",
        docker_status: "查看 Docker 状态？",
        restart_chatgpt2api: "重启 chatgpt2api？",
        restart_sub2api: "重启 sub2api？",
        restart_cli_proxy: "重启 cli-proxy-api-vps？",
        warp_reconnect: "执行 WARP 重连？",
    };

    return (
        <div className="pointer-events-none fixed bottom-5 left-1/2 z-50 flex max-w-[calc(100vw-2rem)] -translate-x-1/2 justify-center">
            <div className="thin-scrollbar pointer-events-auto flex h-14 max-w-full items-center gap-1 overflow-x-auto rounded-xl border px-2 shadow-lg backdrop-blur [&>*]:shrink-0" style={dockStyle} data-canvas-no-zoom>
                <RunToolbarButton label="总览视图" active={mode === "overview"} onClick={() => onModeChange("overview")}>
                    <Layers3 className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarButton label="商品流程" active={mode === "product"} disabled={!hasProduct} onClick={() => onModeChange("product")}>
                    <Boxes className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarButton label="创作画布" active={mode === "creative"} disabled={!hasProduct} onClick={() => onModeChange("creative")}>
                    <ImageIcon className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarDivider />
                <RunToolbarButton label="商品列表" onClick={onOpenProducts}>
                    <List className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarButton label="详情" onClick={onOpenDetail}>
                    <Info className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarButton label="实时日志" onClick={onOpenLog}>
                    <FileText className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarDivider />
                <RunToolbarButton label="重置视图" onClick={onResetViewport}>
                    <Focus className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarButton label="刷新" loading={refreshing} onClick={onRefresh}>
                    <RefreshCw className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarDivider />
                <RunToolbarButton label="续跑" onClick={onRun}>
                    <Play className="size-4.5" />
                </RunToolbarButton>
                <RunToolbarButton label="停止" danger onClick={onStop}>
                    <CircleStop className="size-4.5" />
                </RunToolbarButton>
                <Dropdown
                    trigger={["click"]}
                    menu={{
                        items: opsItems,
                        onClick: ({ key }) => onServiceAction({ action: key as PDDActionRequest["action"], runId }, opsTitles[key] || "执行 VPS 运维动作？"),
                    }}
                >
                    <span>
                        <RunToolbarButton label="VPS 运维">
                            <ServerCog className="size-4.5" />
                        </RunToolbarButton>
                    </span>
                </Dropdown>
            </div>
        </div>
    );
}

function RunToolbarButton({ label, active, disabled, danger, loading, onClick, children }: { label: string; active?: boolean; disabled?: boolean; danger?: boolean; loading?: boolean; onClick?: () => void; children: ReactNode }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const style = active ? { background: theme.toolbar.activeBg, color: theme.toolbar.activeText } : { color: danger ? "#f87171" : theme.toolbar.item, opacity: disabled ? 0.35 : 1 };
    return (
        <Tooltip title={label}>
            <Button type="text" aria-label={label} className="!h-8 !w-8 !min-w-8 !p-0" disabled={disabled} loading={loading} style={style} onClick={onClick}>
                {children}
            </Button>
        </Tooltip>
    );
}

function RunToolbarDivider() {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return <span className="mx-1 h-6 w-px" style={{ background: theme.toolbar.border }} />;
}

function RunWorkflowCanvas({
    runId,
    mode,
    overview,
    detail,
    selectedProductKey,
    loading,
    token,
    selectedTarget,
    viewport,
    onViewportChange,
    onSelectStage,
    onSelectNode,
    onOpenArtifact,
    onOpenImage,
}: {
    runId: string;
    mode: RunViewMode;
    overview?: PDDRunOverview;
    detail?: PDDProductDetail;
    selectedProductKey: string;
    loading: boolean;
    token: string;
    selectedTarget: DetailTarget | null;
    viewport: ViewportTransform;
    onViewportChange: (viewport: ViewportTransform) => void;
    onSelectStage: (stage: PDDStageNode) => void;
    onSelectNode: (node: PDDGraphNode, product: PDDProductDetail) => void;
    onOpenArtifact: (artifact: PDDArtifact, node: PDDGraphNode, product: PDDProductDetail) => void;
    onOpenImage: (image: { title: string; src: string; path: string }) => void;
}) {
    const containerRef = useRef<HTMLDivElement>(null);
    const [size, setSize] = useState({ width: 1200, height: 720 });
    const [isMiniMapOpen, setIsMiniMapOpen] = useState(false);

    useEffect(() => {
        const updateSize = () => {
            const box = containerRef.current?.getBoundingClientRect();
            if (box) setSize({ width: box.width, height: box.height });
        };
        updateSize();
        const observer = new ResizeObserver(updateSize);
        if (containerRef.current) observer.observe(containerRef.current);
        window.addEventListener("resize", updateSize);
        return () => {
            observer.disconnect();
            window.removeEventListener("resize", updateSize);
        };
    }, [mode]);

    const setZoomScale = (scale: number) => {
        const nextScale = Math.min(Math.max(scale, 0.05), 5);
        onViewportChange({
            x: size.width / 2 - ((size.width / 2 - viewport.x) / viewport.k) * nextScale,
            y: size.height / 2 - ((size.height / 2 - viewport.y) / viewport.k) * nextScale,
            k: nextScale,
        });
    };
    const resetZoomViewport = () => onViewportChange({ x: size.width / 2, y: size.height / 2, k: 1 });

    if (loading) return <div className="flex h-full items-center justify-center text-sm text-stone-500">正在加载 Run 数据...</div>;

    if (mode === "creative") {
        if (!selectedProductKey) return <Empty description="打开商品列表，选择一个商品进入创作画布" className="mt-16" />;
        return <RunCreativeCanvas runId={runId} productKey={selectedProductKey} token={token} viewport={viewport} onViewportChange={onViewportChange} onOpenImage={onOpenImage} />;
    }

    if (mode === "product") {
        if (!detail) return <Empty description="打开商品列表，选择一个商品查看节点详情" className="mt-16" />;
        const byId = Object.fromEntries(detail.nodes.map((node) => [node.id, node]));
        const minimapNodes = detail.nodes.map(toMinimapNode);
        return (
            <div className="relative h-full min-h-0">
                <InfiniteCanvas containerRef={containerRef} viewport={viewport} backgroundMode="lines" onViewportChange={onViewportChange}>
                    <GraphEdges edges={detail.edges} byId={byId} />
                    {detail.nodes.map((node) => (
                        <ProductGraphNode key={node.id} node={node} token={token} product={detail} selected={selectedTarget?.kind === "product-node" && selectedTarget.node.id === node.id} onSelectNode={onSelectNode} onOpenArtifact={onOpenArtifact} />
                    ))}
                </InfiniteCanvas>
                {isMiniMapOpen ? <Minimap nodes={minimapNodes} viewport={viewport} viewportSize={size} onViewportChange={onViewportChange} /> : null}
                <CanvasZoomControls scale={viewport.k} onScaleChange={setZoomScale} onReset={resetZoomViewport} isMiniMapOpen={isMiniMapOpen} onToggleMiniMap={() => setIsMiniMapOpen((value) => !value)} />
            </div>
        );
    }

    const stages = overview?.stages || [];
    const useTemplateLayout = Boolean(overview?.run?.customWorkflow);
    const nodes = stages.map((stage, index) => ({
        ...stage,
        x: useTemplateLayout ? (stage.x ?? index * stagePositions.gap) : index * stagePositions.gap,
        y: useTemplateLayout ? (stage.y ?? stagePositions.y) : stagePositions.y,
        width: stage.width || stagePositions.width,
        height: stage.height || stagePositions.height,
    }));
    const byId = Object.fromEntries(nodes.map((node) => [node.id, node]));

    if (!nodes.length) return <Empty description="没有可展示的阶段数据" className="mt-16" />;

    const minimapNodes = nodes.map(toMinimapNode);
    return (
        <div className="relative h-full min-h-0">
            <InfiniteCanvas containerRef={containerRef} viewport={viewport} backgroundMode="dots" onViewportChange={onViewportChange}>
                <GraphEdges edges={overview?.edges || []} byId={byId} />
                {nodes.map((node) => (
                    <OverviewGraphNode key={node.id} node={node} selected={selectedTarget?.kind === "stage" && selectedTarget.stage.id === node.id} onSelectStage={onSelectStage} />
                ))}
            </InfiniteCanvas>
            {isMiniMapOpen ? <Minimap nodes={minimapNodes} viewport={viewport} viewportSize={size} onViewportChange={onViewportChange} /> : null}
            <CanvasZoomControls scale={viewport.k} onScaleChange={setZoomScale} onReset={resetZoomViewport} isMiniMapOpen={isMiniMapOpen} onToggleMiniMap={() => setIsMiniMapOpen((value) => !value)} />
        </div>
    );
}

function RunCreativeCanvas({
    runId,
    productKey,
    token,
    viewport,
    onViewportChange,
    onOpenImage,
}: {
    runId: string;
    productKey: string;
    token: string;
    viewport: ViewportTransform;
    onViewportChange: (viewport: ViewportTransform) => void;
    onOpenImage: (image: { title: string; src: string; path: string }) => void;
}) {
    const { message, modal } = App.useApp();
    const effectiveConfig = useEffectiveConfig();
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const addAsset = useAssetStore((state) => state.addAsset);
    const containerRef = useRef<HTMLDivElement>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const uploadTargetRef = useRef<{ nodeId?: string; kind: CanvasNodeType.Image | CanvasNodeType.Video }>({ kind: CanvasNodeType.Image });
    const mediaSizeProbeRef = useRef<Set<string>>(new Set());
    const historyRef = useRef<{ undo: Array<{ nodes: CanvasNodeData[]; connections: CanvasConnection[] }>; redo: Array<{ nodes: CanvasNodeData[]; connections: CanvasConnection[] }> }>({ undo: [], redo: [] });
    const loadedRef = useRef("");
    const serverMergeRef = useRef("");
    const savingRef = useRef(false);
    const saveAgainRef = useRef(false);
    const suppressNextSaveRef = useRef(false);
    const toolbarHideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const dragRef = useRef<{ nodeId: string; startX: number; startY: number; originX: number; originY: number } | null>(null);
    const nodesRef = useRef<CanvasNodeData[]>([]);
    const connectionsRef = useRef<CanvasConnection[]>([]);
    const viewportRef = useRef(viewport);
    const backgroundModeRef = useRef<CanvasBackgroundMode>("lines");
    const showImageInfoRef = useRef(true);
    const [size, setSize] = useState({ width: 1200, height: 720 });
    const [isMiniMapOpen, setIsMiniMapOpen] = useState(true);
    const [nodes, setNodes] = useState<CanvasNodeData[]>([]);
    const [connections, setConnections] = useState<CanvasConnection[]>([]);
    const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set());
    const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
    const [toolbarNodeId, setToolbarNodeId] = useState<string | null>(null);
    const [connecting, setConnecting] = useState<ConnectionHandle | null>(null);
    const [mouseWorld, setMouseWorld] = useState<Position>({ x: 0, y: 0 });
    const [selectedConnectionId, setSelectedConnectionId] = useState("");
    const [backgroundMode, setBackgroundMode] = useState<CanvasBackgroundMode>("lines");
    const [showImageInfo, setShowImageInfo] = useState(true);
    const [fileInputAccept, setFileInputAccept] = useState("image/*");
    const [infoNodeId, setInfoNodeId] = useState("");
    const [dialogNodeId, setDialogNodeId] = useState("");
    const [editingNodeId, setEditingNodeId] = useState("");
    const [editRequestNonce, setEditRequestNonce] = useState(0);
    const [runningNodeId, setRunningNodeId] = useState("");
    const [cropNodeId, setCropNodeId] = useState<string | null>(null);
    const [angleNodeId, setAngleNodeId] = useState<string | null>(null);
    const [cropSubmitting, setCropSubmitting] = useState(false);
    const [angleSubmitting, setAngleSubmitting] = useState(false);
    const [nodeImageSettingsOpen, setNodeImageSettingsOpen] = useState(false);
    const [applyingNodeId, setApplyingNodeId] = useState("");
    const [historyVersion, setHistoryVersion] = useState(0);

    const canvasQuery = useQuery({
        queryKey: ["pdd-creative-canvas", runId, productKey, token],
        queryFn: () => fetchPDDCreativeCanvas(runId, productKey, token),
        enabled: Boolean(token && runId && productKey),
        refetchOnWindowFocus: false,
        refetchInterval: runningNodeId || applyingNodeId || cropSubmitting || angleSubmitting ? false : 3000,
    });

    const nodeById = useMemo(() => Object.fromEntries(nodes.map((node) => [node.id, node])), [nodes]);
    const selectedNodeId = Array.from(selectedNodeIds)[0] || "";
    const toolbarNode = useMemo(() => nodes.find((node) => node.id === toolbarNodeId) || nodes.find((node) => node.id === hoveredNodeId) || nodes.find((node) => node.id === selectedNodeId) || null, [hoveredNodeId, nodes, selectedNodeId, toolbarNodeId]);
    const infoNode = useMemo(() => nodes.find((node) => node.id === infoNodeId) || null, [infoNodeId, nodes]);
    const cropNode = useMemo(() => nodes.find((node) => node.id === cropNodeId) || null, [cropNodeId, nodes]);
    const angleNode = useMemo(() => nodes.find((node) => node.id === angleNodeId) || null, [angleNodeId, nodes]);
    const canUndo = historyVersion >= 0 && historyRef.current.undo.length > 0;
    const canRedo = historyVersion >= 0 && historyRef.current.redo.length > 0;

    useEffect(() => {
        nodesRef.current = nodes;
    }, [nodes]);

    useEffect(() => {
        connectionsRef.current = connections;
    }, [connections]);

    useEffect(() => {
        viewportRef.current = viewport;
    }, [viewport]);

    useEffect(() => {
        backgroundModeRef.current = backgroundMode;
    }, [backgroundMode]);

    useEffect(() => {
        showImageInfoRef.current = showImageInfo;
    }, [showImageInfo]);

    useEffect(() => {
        return () => {
            if (toolbarHideTimerRef.current) clearTimeout(toolbarHideTimerRef.current);
        };
    }, []);

    const recordHistory = useCallback(() => {
        historyRef.current.undo.push({ nodes: cloneCreativeCanvasNodes(nodes), connections: cloneCreativeConnections(connections) });
        if (historyRef.current.undo.length > 80) historyRef.current.undo.shift();
        historyRef.current.redo = [];
        setHistoryVersion((value) => value + 1);
    }, [connections, nodes]);

    const restoreSnapshot = useCallback((snapshot: { nodes: CanvasNodeData[]; connections: CanvasConnection[] }) => {
        setNodes(cloneCreativeCanvasNodes(snapshot.nodes));
        setConnections(cloneCreativeConnections(snapshot.connections));
        setSelectedNodeIds(new Set());
        setSelectedConnectionId("");
    }, []);

    const undo = useCallback(() => {
        const snapshot = historyRef.current.undo.pop();
        if (!snapshot) return;
        historyRef.current.redo.push({ nodes: cloneCreativeCanvasNodes(nodes), connections: cloneCreativeConnections(connections) });
        restoreSnapshot(snapshot);
        setHistoryVersion((value) => value + 1);
    }, [connections, nodes, restoreSnapshot]);

    const redo = useCallback(() => {
        const snapshot = historyRef.current.redo.pop();
        if (!snapshot) return;
        historyRef.current.undo.push({ nodes: cloneCreativeCanvasNodes(nodes), connections: cloneCreativeConnections(connections) });
        restoreSnapshot(snapshot);
        setHistoryVersion((value) => value + 1);
    }, [connections, nodes, restoreSnapshot]);

    const keepNodeToolbar = useCallback(
        (nodeId: string) => {
            if (dragRef.current || nodeImageSettingsOpen) return;
            if (toolbarHideTimerRef.current) {
                clearTimeout(toolbarHideTimerRef.current);
                toolbarHideTimerRef.current = null;
            }
            setToolbarNodeId(nodeId);
        },
        [nodeImageSettingsOpen],
    );

    const hideNodeToolbar = useCallback(() => {
        if (toolbarHideTimerRef.current) clearTimeout(toolbarHideTimerRef.current);
        toolbarHideTimerRef.current = setTimeout(() => {
            setToolbarNodeId(null);
            toolbarHideTimerRef.current = null;
        }, 420);
    }, []);

    const addNode = useCallback(
        (type: CanvasNodeType, position?: Position) => {
            recordHistory();
            const node = createCreativeCanvasNode(type, position || canvasCenterPosition(size, viewport, type));
            setNodes((current) => [...current, node]);
            setSelectedNodeIds(new Set([node.id]));
            setSelectedConnectionId("");
            return node;
        },
        [recordHistory, size, viewport],
    );

    const deleteSelected = useCallback(() => {
        if (!selectedNodeIds.size && !selectedConnectionId) return;
        recordHistory();
        const selected = new Set(selectedNodeIds);
        setNodes((current) => current.filter((node) => !selected.has(node.id)));
        setConnections((current) => current.filter((connection) => !selected.has(connection.fromNodeId) && !selected.has(connection.toNodeId) && connection.id !== selectedConnectionId));
        setSelectedNodeIds(new Set());
        setSelectedConnectionId("");
    }, [recordHistory, selectedConnectionId, selectedNodeIds]);

    const clearCanvas = useCallback(() => {
        if (!nodes.length) return;
        Modal.confirm({
            title: "清空创作画布",
            content: "仅清空当前 run 的创作画布视图，不会删除原始 run 产物文件。",
            okText: "清空",
            okButtonProps: { danger: true },
            cancelText: "取消",
            onOk: () => {
                recordHistory();
                setNodes([]);
                setConnections([]);
                setSelectedNodeIds(new Set());
                setSelectedConnectionId("");
            },
        });
    }, [nodes.length, recordHistory]);

    const startUpload = useCallback((kind: CanvasNodeType.Image | CanvasNodeType.Video, nodeId?: string) => {
        uploadTargetRef.current = { kind, nodeId };
        setFileInputAccept(kind === CanvasNodeType.Video ? "video/*" : "image/*");
        window.setTimeout(() => fileInputRef.current?.click(), 0);
    }, []);

    const downloadNode = useCallback((node: CanvasNodeData) => {
        const content = typeof node.metadata?.content === "string" ? node.metadata.content : "";
        if (!content) {
            message.warning("当前节点没有可下载的内容");
            return;
        }
        const suffix = node.type === CanvasNodeType.Video ? "mp4" : node.type === CanvasNodeType.Text ? "txt" : "png";
        if (node.type === CanvasNodeType.Text) {
            saveAs(new Blob([content], { type: "text/plain;charset=utf-8" }), `${safeDownloadName(node.title || node.id)}.${suffix}`);
            return;
        }
        saveAs(content, `${safeDownloadName(node.title || node.id)}.${suffix}`);
    }, [message]);

    const openNodePreview = useCallback(
        (node: CanvasNodeData) => {
            const content = typeof node.metadata?.content === "string" ? node.metadata.content : "";
            if (!content) return;
            if (node.type === CanvasNodeType.Image) {
                const meta = (node.metadata || {}) as Record<string, unknown>;
                onOpenImage({ title: node.title, src: content, path: String(meta.artifactPath || meta.storageKey || node.title) });
                return;
            }
            if (node.type === CanvasNodeType.Video) window.open(content, "_blank", "noopener,noreferrer");
        },
        [onOpenImage],
    );

    const saveNodeAsset = useCallback(
        async (node: CanvasNodeData) => {
            const content = creativeNodeContent(node);
            if (node.type === CanvasNodeType.Text) {
                const text = content.trim();
                if (!text) {
                    message.warning("当前文本节点没有可保存的内容");
                    return;
                }
                addAsset({ kind: "text", title: node.title || "创作画布文本", coverUrl: "", tags: [], source: "PDD 创作画布", data: { content: text }, metadata: { source: "pdd_creative_canvas", runId, productKey, nodeId: node.id } });
                message.success("已加入我的素材");
                return;
            }
            if (!content) {
                message.warning("当前节点没有可保存的内容");
                return;
            }
            if (node.type === CanvasNodeType.Video) {
                const uploaded = await uploadMediaFile(await (await fetch(content)).blob(), "video");
                addAsset({
                    kind: "video",
                    title: node.title || "创作画布视频",
                    coverUrl: "",
                    tags: [],
                    source: "PDD 创作画布",
                    data: { url: uploaded.url, storageKey: uploaded.storageKey, width: uploaded.width || node.width, height: uploaded.height || node.height, bytes: uploaded.bytes, mimeType: uploaded.mimeType },
                    metadata: { source: "pdd_creative_canvas", runId, productKey, nodeId: node.id, prompt: node.metadata?.prompt },
                });
                message.success("已加入我的素材");
                return;
            }
            const uploaded = await uploadImage(await (await fetch(content)).blob());
            addAsset({
                kind: "image",
                title: node.title || "创作画布图片",
                coverUrl: uploaded.url,
                tags: [],
                source: "PDD 创作画布",
                data: { dataUrl: uploaded.url, storageKey: uploaded.storageKey, width: uploaded.width, height: uploaded.height, bytes: uploaded.bytes, mimeType: uploaded.mimeType },
                metadata: { source: "pdd_creative_canvas", runId, productKey, nodeId: node.id, prompt: node.metadata?.prompt },
            });
            message.success("已加入我的素材");
        },
        [addAsset, message, productKey, runId],
    );

    const uploadCreativeDataUrl = useCallback(
        async (nodeId: string, content: string | Blob, fileName: string, mimeType?: string) => {
            const dataUrl = await ensureCreativeDataUrl(content, mimeType);
            const asset = await uploadPDDCreativeCanvasAsset(runId, productKey, { nodeId, fileName, mimeType, content: dataUrl }, token);
            return {
                content: withPDDFileToken(asset.url, token),
                artifactPath: asset.path,
                storageKey: asset.path,
                mimeType: asset.mimeType,
                bytes: asset.bytes,
                naturalWidth: asset.width,
                naturalHeight: asset.height,
            };
        },
        [productKey, runId, token],
    );

    const insertConnectedNode = useCallback(
        (sourceNode: CanvasNodeData, node: CanvasNodeData) => {
            const position = findFreeCreativePosition(nodesRef.current, sourceNode, node.width, node.height);
            const nextNode = { ...node, position };
            recordHistory();
            setNodes((current) => [...current, nextNode]);
            setConnections((current) => [...current, { id: `creative-${sourceNode.id}-${nextNode.id}-${nanoid(4)}`, fromNodeId: sourceNode.id, toNodeId: nextNode.id }]);
            setSelectedNodeIds(new Set([nextNode.id]));
            setSelectedConnectionId("");
            setDialogNodeId(nextNode.id);
            keepNodeToolbar(nextNode.id);
            return nextNode;
        },
        [keepNodeToolbar, recordHistory],
    );

    const createGeneratedNodeMetadata = useCallback(
        (sourceNode: CanvasNodeData, prompt: string, patch?: CanvasNodeMetadata): CanvasNodeMetadata => ({
            prompt,
            status: "loading",
            parentNodeId: sourceNode.id,
            originWorkflowNodeId: creativeOriginWorkflowNodeId(sourceNode),
            generationKind: "creative_canvas",
            source: "creative_generated",
            ...patch,
        }),
        [],
    );

    const handleNodePromptChange = useCallback((nodeId: string, prompt: string) => {
        setNodes((current) => current.map((node) => (node.id === nodeId ? { ...node, metadata: { ...node.metadata, prompt } } : node)));
    }, []);

    const handleConfigNodeChange = useCallback((nodeId: string, patch: Partial<CanvasNodeData["metadata"]>) => {
        setNodes((current) => current.map((node) => (node.id === nodeId ? applyCreativeNodeConfigPatch(node, patch) : node)));
    }, []);

    const toggleCreativeNodeFreeResize = useCallback(
        (nodeId: string) => {
            recordHistory();
            setNodes((current) =>
                current.map((node) => {
                    if (node.id !== nodeId) return node;
                    const freeResize = !node.metadata?.freeResize;
                    if (freeResize || node.type !== CanvasNodeType.Image) return { ...node, metadata: { ...node.metadata, freeResize } };
                    const ratio = (node.metadata?.naturalWidth || node.width) / (node.metadata?.naturalHeight || node.height || 1);
                    const height = node.width / ratio;
                    return { ...node, height, position: { x: node.position.x, y: node.position.y + node.height / 2 - height / 2 }, metadata: { ...node.metadata, freeResize } };
                }),
            );
        },
        [recordHistory],
    );

    const handleGenerateNode = useCallback(
        async (nodeId: string, mode: CanvasNodeGenerationMode, prompt: string) => {
            const sourceNode = nodesRef.current.find((node) => node.id === nodeId);
            if (!sourceNode) return;
            const generationConfig = buildCreativeGenerationConfig(effectiveConfig, sourceNode, mode);
            if (!isCreativeAiConfigReady(generationConfig, generationConfig.model)) {
                openConfigDialog(true);
                return;
            }
            const sourceTextContent = sourceNode.type === CanvasNodeType.Text ? sourceNode.metadata?.content?.trim() || "" : "";
            const editingTextNode = mode === "text" && Boolean(sourceTextContent);
            const context = await hydrateNodeGenerationContext(buildNodeGenerationContext(nodeId, nodesRef.current, connectionsRef.current, editingTextNode ? `请根据要求修改以下文本。\n\n原文：\n${sourceTextContent}\n\n修改要求：\n${prompt}` : prompt));
            const effectivePrompt = context.prompt.trim();
            if (!effectivePrompt) {
                message.warning("缺少提示词，无法生成");
                return;
            }
            setRunningNodeId(nodeId);
            try {
                if (mode === "image") {
                    const sourceReference = sourceNode.type === CanvasNodeType.Image && sourceNode.metadata?.content ? sourceNodeReferenceImages(sourceNode) : [];
                    const referenceImages = sourceReference.length ? sourceReference : context.referenceImages;
                    const count = getGenerationCount(generationConfig.count);
                    const imageSpec = NODE_DEFAULT_SIZE[CanvasNodeType.Image];
                    const reservedNodes = [...nodesRef.current];
                    const childNodes = Array.from({ length: count }, () => {
                        const id = `creative-image-${nanoid(8)}`;
                        const position = findFreeCreativePosition(reservedNodes, sourceNode, imageSpec.width, imageSpec.height);
                        const node: CanvasNodeData = {
                            id,
                            type: CanvasNodeType.Image,
                            title: effectivePrompt.slice(0, 32) || "Generated Image",
                            position,
                            width: imageSpec.width,
                            height: imageSpec.height,
                            metadata: {
                                ...createGeneratedNodeMetadata(sourceNode, effectivePrompt, buildCreativeImageGenerationMetadata(referenceImages.length ? "edit" : "generation", generationConfig, count, referenceImages)),
                                batchRootId: count > 1 ? undefined : undefined,
                            },
                        };
                        reservedNodes.push(node);
                        return node;
                    });
                    recordHistory();
                    setNodes((current) => [...current, ...childNodes]);
                    setConnections((current) => [...current, ...childNodes.map((node) => ({ id: `creative-${sourceNode.id}-${node.id}-${nanoid(4)}`, fromNodeId: sourceNode.id, toNodeId: node.id }))]);
                    setSelectedNodeIds(new Set([childNodes[0]?.id].filter(Boolean) as string[]));
                    await Promise.all(
                        childNodes.map(async (child) => {
                            try {
                                const image = referenceImages.length ? await requestEdit({ ...generationConfig, count: "1" }, effectivePrompt, referenceImages).then((items) => items[0]) : await requestGeneration({ ...generationConfig, count: "1" }, effectivePrompt).then((items) => items[0]);
                                const asset = await uploadCreativeDataUrl(child.id, image.dataUrl, `${child.id}.png`, "image/png");
                                const size = asset.naturalWidth && asset.naturalHeight ? fitCreativeMediaNodeSize(CanvasNodeType.Image, asset.naturalWidth, asset.naturalHeight) : imageSpec;
                                setNodes((current) =>
                                    current.map((node) =>
                                        node.id === child.id
                                            ? {
                                                  ...node,
                                                  width: size.width,
                                                  height: size.height,
                                                  metadata: { ...node.metadata, ...asset, status: "success" },
                                              }
                                            : node,
                                    ),
                                );
                            } catch (error) {
                                const errorDetails = error instanceof Error ? error.message : "生成失败";
                                setNodes((current) => current.map((node) => (node.id === child.id ? { ...node, metadata: { ...node.metadata, status: "error", errorDetails } } : node)));
                            }
                        }),
                    );
                    return;
                }
                if (mode === "video") {
                    const spec = nodeSizeFromRatio(generationConfig.size, NODE_DEFAULT_SIZE[CanvasNodeType.Video].width, NODE_DEFAULT_SIZE[CanvasNodeType.Video].height) || NODE_DEFAULT_SIZE[CanvasNodeType.Video];
                    const child = insertConnectedNode(sourceNode, {
                        id: `creative-video-${nanoid(8)}`,
                        type: CanvasNodeType.Video,
                        title: effectivePrompt.slice(0, 32) || "Generated Video",
                        position: sourceNode.position,
                        width: spec.width,
                        height: spec.height,
                        metadata: createGeneratedNodeMetadata(sourceNode, effectivePrompt, { model: generationConfig.model, size: generationConfig.size, seconds: generationConfig.videoSeconds, vquality: generationConfig.vquality, videoReferenceMode: generationConfig.videoReferenceMode, references: context.referenceImages.map(referenceUrl).filter((url): url is string => Boolean(url)) }),
                    });
                    const video = await requestVideoGeneration(generationConfig, effectivePrompt, context.referenceImages);
                    const asset = await uploadCreativeDataUrl(child.id, video, `${child.id}.mp4`, video.type || "video/mp4");
                    const size = asset.naturalWidth && asset.naturalHeight ? fitCreativeMediaNodeSize(CanvasNodeType.Video, asset.naturalWidth, asset.naturalHeight) : spec;
                    setNodes((current) => current.map((node) => (node.id === child.id ? { ...node, width: size.width, height: size.height, metadata: { ...node.metadata, ...asset, status: "success" } } : node)));
                    return;
                }
                let streamed = "";
                const child = insertConnectedNode(sourceNode, {
                    id: `creative-text-${nanoid(8)}`,
                    type: CanvasNodeType.Text,
                    title: effectivePrompt.slice(0, 32) || "Generated Text",
                    position: sourceNode.position,
                    width: NODE_DEFAULT_SIZE[CanvasNodeType.Text].width,
                    height: NODE_DEFAULT_SIZE[CanvasNodeType.Text].height,
                    metadata: createGeneratedNodeMetadata(sourceNode, effectivePrompt, { fontSize: 14 }),
                });
                const answer = await requestImageQuestion(generationConfig, buildNodeChatMessages({ ...context, prompt: effectivePrompt }), (text) => {
                    streamed = text;
                    setNodes((current) => current.map((node) => (node.id === child.id ? { ...node, metadata: { ...node.metadata, content: text, status: "loading" } } : node)));
                });
                setNodes((current) => current.map((node) => (node.id === child.id ? { ...node, metadata: { ...node.metadata, content: answer || streamed, status: "success" } } : node)));
            } catch (error) {
                message.error(error instanceof Error ? error.message : "生成失败");
            } finally {
                setRunningNodeId("");
            }
        },
        [createGeneratedNodeMetadata, effectiveConfig, insertConnectedNode, message, openConfigDialog, recordHistory, uploadCreativeDataUrl],
    );

    const generateImageFromTextNode = useCallback(
        (node: CanvasNodeData) => {
            const prompt = (node.metadata?.content || node.metadata?.prompt || "").trim();
            if (!prompt) {
                message.warning("文本节点为空，无法生图");
                return;
            }
            const sourceNode = nodesRef.current.find((item) => item.id === node.id);
            if (!sourceNode) return;
            const spec = getNodeSpec(CanvasNodeType.Config);
            const position = findFreeCreativePosition(nodesRef.current, sourceNode, spec.width, spec.height);
            const configNode = createCreativeCanvasNode(CanvasNodeType.Config, position, {
                prompt: "",
                model: effectiveConfig.imageModel || effectiveConfig.model,
                size: effectiveConfig.size,
                count: 3,
                generationMode: "image",
                parentNodeId: sourceNode.id,
                originWorkflowNodeId: creativeOriginWorkflowNodeId(sourceNode),
                source: "creative_config",
            });
            recordHistory();
            setNodes((current) => current.map((item) => (item.id === sourceNode.id ? { ...item, metadata: { ...item.metadata, content: prompt, prompt, status: "success" as const } } : item)).concat(configNode));
            setConnections((current) => [...current, { id: `creative-${sourceNode.id}-${configNode.id}-${nanoid(4)}`, fromNodeId: sourceNode.id, toNodeId: configNode.id }]);
            setSelectedNodeIds(new Set([configNode.id]));
            setSelectedConnectionId("");
            setDialogNodeId(configNode.id);
            keepNodeToolbar(configNode.id);
        },
        [effectiveConfig.imageModel, effectiveConfig.model, effectiveConfig.size, keepNodeToolbar, message, recordHistory],
    );

    const cropImageNode = useCallback(
        async (node: CanvasNodeData, crop: CanvasImageCropRect) => {
            const content = creativeNodeContent(node);
            if (!content) return;
            const childId = `creative-crop-${nanoid(8)}`;
            setCropSubmitting(true);
            try {
                const source = await ensureCreativeDataUrl(content, node.metadata?.mimeType || "image/png");
                const cropped = await cropDataUrl(source, crop);
                const previewWidth = Math.max(1, Math.round((node.metadata?.naturalWidth || node.width) * crop.width));
                const previewHeight = Math.max(1, Math.round((node.metadata?.naturalHeight || node.height) * crop.height));
                const previewSize = fitCreativeMediaNodeSize(CanvasNodeType.Image, previewWidth, previewHeight);
                const child = insertConnectedNode(node, {
                    id: childId,
                    type: CanvasNodeType.Image,
                    title: "Cropped Image",
                    position: node.position,
                    width: previewSize.width,
                    height: previewSize.height,
                    metadata: {
                        ...createGeneratedNodeMetadata(node, node.metadata?.prompt || "", {
                            content: cropped,
                            mimeType: "image/png",
                            naturalWidth: previewWidth,
                            naturalHeight: previewHeight,
                            status: "loading",
                            source: "creative_crop",
                        }),
                    },
                });
                setCropNodeId(null);
                setRunningNodeId(childId);
                const asset = await uploadCreativeDataUrl(childId, cropped, `${childId}.png`, "image/png");
                const size = asset.naturalWidth && asset.naturalHeight ? fitCreativeMediaNodeSize(CanvasNodeType.Image, asset.naturalWidth, asset.naturalHeight) : NODE_DEFAULT_SIZE[CanvasNodeType.Image];
                setNodes((current) => current.map((item) => (item.id === child.id ? { ...item, width: size.width, height: size.height, metadata: { ...item.metadata, ...asset, status: "success" } } : item)));
            } catch (error) {
                setNodes((current) => current.map((item) => (item.id === childId ? { ...item, metadata: { ...item.metadata, status: "error", errorDetails: error instanceof Error ? error.message : "裁剪失败" } } : item)));
                message.error(error instanceof Error ? error.message : "裁剪失败");
            } finally {
                setCropSubmitting(false);
                setRunningNodeId((current) => (current === childId ? "" : current));
            }
        },
        [createGeneratedNodeMetadata, insertConnectedNode, message, uploadCreativeDataUrl],
    );

    const generateAngleNode = useCallback(
        async (node: CanvasNodeData, params: CanvasImageAngleParams) => {
            const content = creativeNodeContent(node);
            if (!content) return;
            const generationConfig = { ...buildCreativeGenerationConfig(effectiveConfig, node, "image"), count: "1" };
            if (!isCreativeAiConfigReady(generationConfig, generationConfig.model)) {
                openConfigDialog(true);
                return;
            }
            const title = buildAngleLabel(params);
            const prompt = buildAnglePrompt(params);
            const childId = `creative-angle-${nanoid(8)}`;
            const imageSpec = NODE_DEFAULT_SIZE[CanvasNodeType.Image];
            setAngleSubmitting(true);
            let child: CanvasNodeData | null = null;
            try {
                const sourceDataUrl = await ensureCreativeDataUrl(content, node.metadata?.mimeType || "image/png");
                const references: ReferenceImage[] = [{ id: node.id, name: `${node.title || node.id}.png`, type: dataUrlMimeType(sourceDataUrl, node.metadata?.mimeType || "image/png"), dataUrl: sourceDataUrl, storageKey: node.metadata?.storageKey }];
                const generationMetadata = buildCreativeImageGenerationMetadata("edit", generationConfig, 1, references);
                setAngleNodeId(null);
                setRunningNodeId(childId);
                const nextChild = insertConnectedNode(node, {
                    id: childId,
                    type: CanvasNodeType.Image,
                    title,
                    position: node.position,
                    width: imageSpec.width,
                    height: imageSpec.height,
                    metadata: createGeneratedNodeMetadata(node, prompt, { ...generationMetadata, status: "loading", source: "creative_angle" }),
                });
                child = nextChild;
                const image = await requestEdit(generationConfig, prompt, references).then((items) => items[0]);
                const asset = await uploadCreativeDataUrl(nextChild.id, image.dataUrl, `${nextChild.id}.png`, "image/png");
                const size = asset.naturalWidth && asset.naturalHeight ? fitCreativeMediaNodeSize(CanvasNodeType.Image, asset.naturalWidth, asset.naturalHeight) : imageSpec;
                setNodes((current) => current.map((item) => (item.id === nextChild.id ? { ...item, width: size.width, height: size.height, metadata: { ...item.metadata, ...asset, prompt, ...generationMetadata, status: "success" } } : item)));
            } catch (error) {
                const errorDetails = error instanceof Error ? error.message : "生成失败";
                if (child) setNodes((current) => current.map((item) => (item.id === child?.id ? { ...item, metadata: { ...item.metadata, status: "error", errorDetails } } : item)));
                message.error(errorDetails);
            } finally {
                setAngleSubmitting(false);
                setRunningNodeId((current) => (current === childId ? "" : current));
            }
        },
        [createGeneratedNodeMetadata, effectiveConfig, insertConnectedNode, message, openConfigDialog, uploadCreativeDataUrl],
    );

    const handleRetryNode = useCallback(
        async (node: CanvasNodeData) => {
            const sourceNode = findCreativeRetrySourceNode(node.id, nodesRef.current, connectionsRef.current) || node;
            const savedImageMetadata = node.type === CanvasNodeType.Image ? node.metadata : undefined;
            const hasSavedImageMetadata = Boolean(savedImageMetadata?.generationType);
            const mode: CanvasNodeGenerationMode = node.type === CanvasNodeType.Text ? "text" : node.type === CanvasNodeType.Video ? "video" : "image";
            const generationConfig = hasSavedImageMetadata
                ? {
                      ...effectiveConfig,
                      model: savedImageMetadata?.model || effectiveConfig.imageModel || effectiveConfig.model,
                      quality: savedImageMetadata?.quality || effectiveConfig.quality,
                      size: savedImageMetadata?.size || effectiveConfig.size,
                      count: "1",
                  }
                : { ...buildCreativeGenerationConfig(effectiveConfig, sourceNode, mode), count: "1" };
            if (!isCreativeAiConfigReady(generationConfig, generationConfig.model)) {
                openConfigDialog(true);
                return;
            }
            const context = hasSavedImageMetadata ? null : await hydrateNodeGenerationContext(buildNodeGenerationContext(sourceNode.id, nodesRef.current, connectionsRef.current, sourceNode.metadata?.prompt || node.metadata?.prompt || ""));
            const prompt = (savedImageMetadata?.prompt || context?.prompt || node.metadata?.prompt || sourceNode.metadata?.prompt || "").trim();
            if (!prompt) {
                message.warning("找不到提示词，无法重试");
                return;
            }
            const references =
                hasSavedImageMetadata && savedImageMetadata
                    ? await resolveCreativeMetadataReferences(runId, token, savedImageMetadata)
                    : context?.referenceImages.length
                      ? context.referenceImages
                      : sourceNodeReferenceImages(sourceNode);
            if (references === null) {
                message.error("参考图片已丢失，无法继续重试");
                setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, status: "error", errorDetails: "参考图片已丢失，无法继续重试" } } : item)));
                return;
            }

            setRunningNodeId(node.id);
            setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, status: "loading", errorDetails: undefined } } : item)));
            try {
                if (node.type === CanvasNodeType.Text) {
                    const textContext = context || (await hydrateNodeGenerationContext(buildNodeGenerationContext(sourceNode.id, nodesRef.current, connectionsRef.current, prompt)));
                    let streamed = "";
                    const answer = await requestImageQuestion(generationConfig, buildNodeChatMessages({ ...textContext, prompt }), (text) => {
                        streamed = text;
                        setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, type: CanvasNodeType.Text, metadata: { ...item.metadata, content: text, status: "loading" } } : item)));
                    });
                    setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, type: CanvasNodeType.Text, metadata: { ...item.metadata, content: answer || streamed, prompt, status: "success" } } : item)));
                    return;
                }
                if (node.type === CanvasNodeType.Video) {
                    const video = await requestVideoGeneration(generationConfig, prompt, references || []);
                    const asset = await uploadCreativeDataUrl(node.id, video, `${node.id}.mp4`, video.type || "video/mp4");
                    setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, ...asset, prompt, model: generationConfig.model, size: generationConfig.size, seconds: generationConfig.videoSeconds, vquality: generationConfig.vquality, videoReferenceMode: generationConfig.videoReferenceMode, status: "success" } } : item)));
                    return;
                }
                const referenceImages = references || [];
                const image = referenceImages.length ? await requestEdit(generationConfig, prompt, referenceImages).then((items) => items[0]) : await requestGeneration(generationConfig, prompt).then((items) => items[0]);
                const asset = await uploadCreativeDataUrl(node.id, image.dataUrl, `${node.id}.png`, "image/png");
                const size = asset.naturalWidth && asset.naturalHeight ? fitCreativeMediaNodeSize(CanvasNodeType.Image, asset.naturalWidth, asset.naturalHeight) : NODE_DEFAULT_SIZE[CanvasNodeType.Image];
                const generationMetadata = savedImageMetadata?.generationType ? { generationType: savedImageMetadata.generationType, model: generationConfig.model, size: generationConfig.size, quality: generationConfig.quality, count: savedImageMetadata.count || 1, references: savedImageMetadata.references } : buildCreativeImageGenerationMetadata(referenceImages.length ? "edit" : "generation", generationConfig, 1, referenceImages);
                setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, type: CanvasNodeType.Image, width: size.width, height: size.height, metadata: { ...item.metadata, ...asset, prompt, ...generationMetadata, status: "success" } } : item)));
            } catch (error) {
                const errorDetails = error instanceof Error ? error.message : "生成失败";
                setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, status: "error", errorDetails } } : item)));
                message.error(errorDetails);
            } finally {
                setRunningNodeId("");
            }
        },
        [effectiveConfig, message, openConfigDialog, runId, token, uploadCreativeDataUrl],
    );

    const applyCreativeNodeToWorkflow = useCallback(
        (node: CanvasNodeData) => {
            const targetNodeId = creativeOriginWorkflowNodeId(node);
            if (!targetNodeId) {
                message.warning("找不到对应的工作流节点，无法应用重跑");
                return;
            }
            modal.confirm({
                title: "应用副本并重跑下游？",
                content: "会用当前节点内容覆盖对应工作流节点输出，并按原工作流模板重跑后续节点。",
                okText: "应用并重跑",
                cancelText: "取消",
                onOk: async () => {
                    const content = creativeNodeContent(node);
                    const storageKey = metadataString(node, "storageKey");
                    const artifactPath = metadataString(node, "artifactPath") || (storageKey && !storageKey.startsWith("image:") && !storageKey.includes(":") ? storageKey : "");
                    if (!artifactPath && !content.trim()) {
                        message.warning("当前节点没有可应用的内容");
                        return;
                    }
                    setApplyingNodeId(node.id);
                    try {
                        const payloadContent = artifactPath ? undefined : node.type === CanvasNodeType.Text ? content : await ensureCreativeDataUrl(content, node.metadata?.mimeType);
                        const result = await applyPDDCreativeCanvasOutput(
                            runId,
                            productKey,
                            {
                                sourceNodeId: node.id,
                                targetNodeId,
                                artifactPath: artifactPath || undefined,
                                content: payloadContent,
                                mimeType: node.type === CanvasNodeType.Text ? "text/plain" : node.metadata?.mimeType,
                                rerunDownstream: true,
                            },
                            token,
                        );
                        setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, appliedAt: new Date().toISOString(), status: "success" } } : item)));
                        message.success(result.output || "已应用副本并启动后续重跑");
                        await canvasQuery.refetch();
                    } catch (error) {
                        message.error(error instanceof Error ? error.message : "应用副本失败");
                    } finally {
                        setApplyingNodeId("");
                    }
                },
            });
        },
        [canvasQuery, message, modal, productKey, runId, token],
    );

    const handleFileInput = useCallback(
        async (event: ReactChangeEvent<HTMLInputElement>) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (!file) return;
            const target = uploadTargetRef.current;
            const kind = file.type.startsWith("video/") ? CanvasNodeType.Video : CanvasNodeType.Image;
            const nodeId = target.nodeId || `creative-${kind}-${Date.now().toString(36)}`;
            try {
                const dataUrl = await readCreativeFileAsDataURL(file);
                const asset = await uploadPDDCreativeCanvasAsset(runId, productKey, { nodeId, fileName: file.name, mimeType: file.type || undefined, content: dataUrl }, token);
                const content = withPDDFileToken(asset.url, token);
                const title = file.name.replace(/\.[^.]+$/, "") || file.name;
                recordHistory();
                if (target.nodeId) {
                    setNodes((current) =>
                        current.map((node) =>
                            node.id === target.nodeId
                                ? {
                                      ...node,
                                      type: kind,
                                      title,
                                      ...(asset.width && asset.height ? creativeMediaNodeSizePatch(node, kind, asset.width, asset.height) : {}),
                                      metadata: {
                                          ...node.metadata,
                                          content,
                                          status: "success",
                                          errorDetails: undefined,
                                          artifactPath: asset.path,
                                          storageKey: asset.path,
                                          mimeType: asset.mimeType,
                                          bytes: asset.bytes,
                                          naturalWidth: asset.width,
                                          naturalHeight: asset.height,
                                          source: "user_upload",
                                          freeResize: false,
                                          isBatchRoot: undefined,
                                          batchRootId: undefined,
                                          batchChildIds: undefined,
                                          batchUsesReferenceImages: undefined,
                                          generationType: undefined,
                                          model: undefined,
                                          size: undefined,
                                          quality: undefined,
                                          count: undefined,
                                          references: undefined,
                                          primaryImageId: undefined,
                                          imageBatchExpanded: undefined,
                                      },
                                  }
                                : node,
                        ),
                    );
                    setSelectedNodeIds(new Set([target.nodeId]));
                    setSelectedConnectionId("");
                    setDialogNodeId(target.nodeId);
                    return;
                }
                const node = createCreativeCanvasNode(kind, canvasCenterPosition(size, viewport, kind), {
                    content,
                    status: "success",
                    artifactPath: asset.path,
                    storageKey: asset.path,
                    mimeType: asset.mimeType,
                    bytes: asset.bytes,
                    naturalWidth: asset.width,
                    naturalHeight: asset.height,
                    source: "user_upload",
                });
                node.id = nodeId;
                node.title = title || node.title;
                if (asset.width && asset.height) Object.assign(node, creativeMediaNodeSizePatch(node, kind, asset.width, asset.height));
                setNodes((current) => [...current, node]);
                setSelectedNodeIds(new Set([node.id]));
                setSelectedConnectionId("");
            } catch (error) {
                message.error(error instanceof Error ? error.message : "上传到创作画布失败");
            } finally {
                uploadTargetRef.current = { kind: CanvasNodeType.Image };
            }
        },
        [message, productKey, recordHistory, runId, size, token, viewport],
    );

    useEffect(() => {
        const updateSize = () => {
            const box = containerRef.current?.getBoundingClientRect();
            if (box) setSize({ width: box.width, height: box.height });
        };
        updateSize();
        const observer = new ResizeObserver(updateSize);
        if (containerRef.current) observer.observe(containerRef.current);
        window.addEventListener("resize", updateSize);
        return () => {
            observer.disconnect();
            window.removeEventListener("resize", updateSize);
        };
    }, []);

    useEffect(() => {
        const canvas = canvasQuery.data;
        if (!canvas) return;
        const loadKey = `${runId}:${canvas.productKey}`;
        if (loadedRef.current !== loadKey) {
            loadedRef.current = loadKey;
            serverMergeRef.current = `${canvas.updatedAt || ""}:${canvas.nodes.length}:${canvas.edges.length}`;
            suppressNextSaveRef.current = true;
            setNodes(hydrateCreativeNodes(canvas, token));
            setConnections(canvas.edges || []);
            setBackgroundMode((canvas.backgroundMode as CanvasBackgroundMode) || "lines");
            setShowImageInfo(canvas.showImageInfo !== false);
            setSelectedNodeIds(new Set());
            setSelectedConnectionId("");
            historyRef.current = { undo: [], redo: [] };
            setHistoryVersion((value) => value + 1);
            if (canvas.viewport) onViewportChange(canvas.viewport);
            return;
        }
        const mergeKey = `${canvas.updatedAt || ""}:${canvas.nodes.length}:${canvas.edges.length}`;
        if (serverMergeRef.current === mergeKey) return;
        serverMergeRef.current = mergeKey;
        const incomingNodes = hydrateCreativeNodes(canvas, token);
        setNodes((current) => mergeIncomingCreativeNodes(current, incomingNodes));
        setConnections((current) => mergeIncomingCreativeConnections(current, canvas.edges || []));
    }, [canvasQuery.data, onViewportChange, runId, token]);

    useEffect(() => {
        nodes.forEach((node) => {
            if (!shouldProbeCreativeMediaNode(node)) return;
            const content = creativeNodeContent(node);
            if (!content) return;
            const key = `${node.id}:${content}`;
            if (mediaSizeProbeRef.current.has(key)) return;
            mediaSizeProbeRef.current.add(key);
            void probeCreativeMediaSize(node.type, content)
                .then((size) => {
                    if (!size) return;
                    setNodes((current) =>
                        current.map((item) => {
                            if (item.id !== node.id || creativeNodeContent(item) !== content || item.metadata?.naturalWidth || item.metadata?.naturalHeight) return item;
                            return {
                                ...item,
                                ...creativeMediaNodeSizePatch(item, item.type, size.width, size.height),
                                metadata: {
                                    ...item.metadata,
                                    naturalWidth: size.width,
                                    naturalHeight: size.height,
                                },
                            };
                        }),
                    );
                })
                .catch(() => undefined);
        });
    }, [nodes]);

    const saveLatestCreativeCanvas = useCallback(() => {
        if (savingRef.current) {
            saveAgainRef.current = true;
            return;
        }
        savingRef.current = true;
        void savePDDCreativeCanvas(
            runId,
            productKey,
            {
                nodes: dehydrateCreativeNodes(nodesRef.current),
                edges: connectionsRef.current,
                viewport: viewportRef.current,
                backgroundMode: backgroundModeRef.current,
                showImageInfo: showImageInfoRef.current,
            },
            token,
        )
            .catch((error) => message.warning(error instanceof Error ? error.message : "创作画布保存失败"))
            .finally(() => {
                savingRef.current = false;
                if (saveAgainRef.current) {
                    saveAgainRef.current = false;
                    window.setTimeout(() => saveLatestCreativeCanvas(), 150);
                }
            });
    }, [message, productKey, runId, token]);

    useEffect(() => {
        if (!loadedRef.current) return;
        if (suppressNextSaveRef.current) {
            suppressNextSaveRef.current = false;
            return;
        }
        const timer = window.setTimeout(saveLatestCreativeCanvas, 900);
        return () => window.clearTimeout(timer);
    }, [backgroundMode, connections, nodes, saveLatestCreativeCanvas, showImageInfo, viewport]);

    useEffect(() => {
        const handleMove = (event: MouseEvent) => {
            if (dragRef.current) {
                const dx = (event.clientX - dragRef.current.startX) / viewport.k;
                const dy = (event.clientY - dragRef.current.startY) / viewport.k;
                setNodes((current) =>
                    current.map((node) =>
                        node.id === dragRef.current?.nodeId
                            ? {
                                  ...node,
                                  position: { x: dragRef.current.originX + dx, y: dragRef.current.originY + dy },
                              }
                            : node,
                    ),
                );
            }
            if (connecting) setMouseWorld(screenToWorld(event.clientX, event.clientY, containerRef.current, viewport));
        };
        const handleUp = (event: MouseEvent) => {
            if (connecting) {
                const world = screenToWorld(event.clientX, event.clientY, containerRef.current, viewport);
                const target = nodeAtWorld(nodes, world);
                if (target && target.id !== connecting.nodeId) {
                    const fromNodeId = connecting.handleType === "source" ? connecting.nodeId : target.id;
                    const toNodeId = connecting.handleType === "source" ? target.id : connecting.nodeId;
                    const id = `creative-${fromNodeId}-${toNodeId}`;
                    if (!connections.some((item) => item.fromNodeId === fromNodeId && item.toNodeId === toNodeId)) {
                        recordHistory();
                        setConnections((current) => [...current, { id, fromNodeId, toNodeId }]);
                    }
                }
                setConnecting(null);
            }
            dragRef.current = null;
        };
        window.addEventListener("mousemove", handleMove);
        window.addEventListener("mouseup", handleUp);
        return () => {
            window.removeEventListener("mousemove", handleMove);
            window.removeEventListener("mouseup", handleUp);
        };
    }, [connecting, connections, nodes, recordHistory, viewport]);

    useEffect(() => {
        const handleKeyDown = (event: KeyboardEvent) => {
            if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return;
            if (event.key === "Delete" || event.key === "Backspace") {
                event.preventDefault();
                deleteSelected();
                return;
            }
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "z") {
                event.preventDefault();
                if (event.shiftKey) redo();
                else undo();
            }
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "y") {
                event.preventDefault();
                redo();
            }
        };
        window.addEventListener("keydown", handleKeyDown);
        return () => window.removeEventListener("keydown", handleKeyDown);
    }, [deleteSelected, redo, undo]);

    const setZoomScale = (scale: number) => {
        const nextScale = Math.min(Math.max(scale, 0.05), 5);
        onViewportChange({
            x: size.width / 2 - ((size.width / 2 - viewport.x) / viewport.k) * nextScale,
            y: size.height / 2 - ((size.height / 2 - viewport.y) / viewport.k) * nextScale,
            k: nextScale,
        });
    };

    if (canvasQuery.isLoading) return <div className="flex h-full items-center justify-center text-sm text-stone-500">正在加载创作画布...</div>;
    if (canvasQuery.isError) return <Empty description={canvasQuery.error instanceof Error ? canvasQuery.error.message : "创作画布加载失败"} className="mt-16" />;

    return (
        <div className="relative h-full min-h-0">
            <InfiniteCanvas
                containerRef={containerRef}
                viewport={viewport}
                backgroundMode={backgroundMode}
                onViewportChange={onViewportChange}
                onCanvasMouseDown={() => {
                    setSelectedNodeIds(new Set());
                    setSelectedConnectionId("");
                }}
                onCanvasDeselect={() => {
                    setSelectedNodeIds(new Set());
                    setSelectedConnectionId("");
                }}
            >
                <svg className="absolute left-0 top-0 overflow-visible" width={5000} height={2600}>
                    {connections.map((connection) => {
                        const from = nodeById[connection.fromNodeId];
                        const to = nodeById[connection.toNodeId];
                        if (!from || !to) return null;
                        return <ConnectionPath key={connection.id} connection={connection} from={from} to={to} active={selectedConnectionId === connection.id} onSelect={() => setSelectedConnectionId(connection.id)} />;
                    })}
                    {connecting ? <ActiveConnectionPath node={nodeById[connecting.nodeId]} handle={connecting} mouseWorld={mouseWorld} /> : null}
                </svg>
                {nodes.map((node) => (
                    <CanvasNode
                        key={node.id}
                        data={node}
                        scale={viewport.k}
                        isSelected={selectedNodeIds.has(node.id)}
                        isRelated={Boolean(toolbarNode && (connections.some((connection) => connection.fromNodeId === toolbarNode.id && connection.toNodeId === node.id) || connections.some((connection) => connection.toNodeId === toolbarNode.id && connection.fromNodeId === node.id)))}
                        isFocusRelated={toolbarNode?.id === node.id}
                        isConnectionTarget={Boolean(connecting && connecting.nodeId !== node.id)}
                        isConnecting={Boolean(connecting)}
                        editRequestNonce={editingNodeId === node.id ? editRequestNonce : 0}
                        showPanel={dialogNodeId === node.id}
                        showImageInfo={showImageInfo}
                        mediaFrame
                        renderPanel={(panelNode) => (
                            <CanvasNodePromptPanel
                                node={panelNode}
                                isRunning={runningNodeId === panelNode.id}
                                onPromptChange={handleNodePromptChange}
                                onConfigChange={handleConfigNodeChange}
                                onGenerate={(nodeId, generationMode, prompt) => void handleGenerateNode(nodeId, generationMode, prompt)}
                                onImageSettingsOpenChange={(open) => {
                                    setNodeImageSettingsOpen(open);
                                    if (open) setToolbarNodeId(null);
                                }}
                            />
                        )}
                        renderNodeContent={(contentNode) => {
                            const inputs = buildNodeGenerationInputs(contentNode.id, nodes, connections);
                            return (
                                <CanvasConfigNodePanel
                                    node={contentNode}
                                    isRunning={runningNodeId === contentNode.id}
                                    inputSummary={getInputSummary(inputs)}
                                    inputs={inputs}
                                    onConfigChange={handleConfigNodeChange}
                                    onTextInputChange={(nodeId, content) => setNodes((current) => current.map((item) => (item.id === nodeId ? { ...item, metadata: { ...item.metadata, content, source: item.type === CanvasNodeType.Text ? "creative_text_edit" : item.metadata?.source } } : item)))}
                                    onGenerate={(nodeId) => {
                                        const target = nodesRef.current.find((item) => item.id === nodeId);
                                        void handleGenerateNode(nodeId, target?.metadata?.generationMode || "image", target?.metadata?.prompt || "");
                                    }}
                                />
                            );
                        }}
                        onMouseDown={(event, nodeId) => {
                            event.stopPropagation();
                            const target = nodes.find((item) => item.id === nodeId);
                            if (!target) return;
                            keepNodeToolbar(nodeId);
                            if (event.shiftKey) {
                                setSelectedNodeIds((current) => {
                                    const next = new Set(current);
                                    if (next.has(nodeId)) next.delete(nodeId);
                                    else next.add(nodeId);
                                    return next;
                                });
                            } else {
                                setSelectedNodeIds(new Set([nodeId]));
                            }
                            setSelectedConnectionId("");
                            recordHistory();
                            dragRef.current = { nodeId, startX: event.clientX, startY: event.clientY, originX: target.position.x, originY: target.position.y };
                        }}
                        onHoverStart={(nodeId) => {
                            if (dragRef.current) return;
                            setHoveredNodeId(nodeId);
                            keepNodeToolbar(nodeId);
                        }}
                        onHoverEnd={(nodeId) => {
                            setHoveredNodeId((current) => (current === nodeId ? null : current));
                            hideNodeToolbar();
                        }}
                        onConnectStart={(event, nodeId, handleType) => {
                            event.preventDefault();
                            event.stopPropagation();
                            setConnecting({ nodeId, handleType });
                            setMouseWorld(screenToWorld(event.clientX, event.clientY, containerRef.current, viewport));
                        }}
                        onResize={(nodeId, width, height, position) => {
                            setNodes((current) => current.map((item) => (item.id === nodeId ? { ...item, width, height, position: position || item.position } : item)));
                        }}
                        onContentChange={(nodeId, content) => {
                            setNodes((current) => current.map((item) => (item.id === nodeId ? { ...item, metadata: { ...item.metadata, content, source: item.type === CanvasNodeType.Text ? "creative_text_edit" : item.metadata?.source } } : item)));
                        }}
                        onRetry={(node) => void handleRetryNode(node)}
                        onGenerateImage={generateImageFromTextNode}
                        onContextMenu={(event, nodeId) => {
                            event.preventDefault();
                            const node = nodeById[nodeId];
                            if (node) openNodePreview(node);
                        }}
                    />
                ))}
            </InfiniteCanvas>
            <CanvasNodeHoverToolbar
                node={applyingNodeId ? null : toolbarNode}
                viewport={viewport}
                onKeep={keepNodeToolbar}
                onLeave={hideNodeToolbar}
                onInfo={(node) => setInfoNodeId(node.id)}
                onEditText={(node) => {
                    setEditingNodeId(node.id);
                    setEditRequestNonce((value) => value + 1);
                }}
                onDecreaseFont={(node) => {
                    recordHistory();
                    setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, fontSize: Math.max(10, (item.metadata?.fontSize || 14) - 2) } } : item)));
                }}
                onIncreaseFont={(node) => {
                    recordHistory();
                    setNodes((current) => current.map((item) => (item.id === node.id ? { ...item, metadata: { ...item.metadata, fontSize: Math.min(72, (item.metadata?.fontSize || 14) + 2) } } : item)));
                }}
                onToggleDialog={(node) => {
                    setSelectedNodeIds(new Set([node.id]));
                    setSelectedConnectionId("");
                    setDialogNodeId((current) => (current === node.id ? "" : node.id));
                }}
                onGenerateImage={generateImageFromTextNode}
                onUpload={(node) => startUpload(node.type === CanvasNodeType.Video ? CanvasNodeType.Video : CanvasNodeType.Image, node.id)}
                onDownload={downloadNode}
                onSaveAsset={(node) => void saveNodeAsset(node)}
                onCrop={(node) => setCropNodeId(node.id)}
                onAngle={(node) => setAngleNodeId(node.id)}
                onViewImage={openNodePreview}
                onRetry={(node) => void handleRetryNode(node)}
                onApplyToWorkflow={applyCreativeNodeToWorkflow}
                onToggleFreeResize={(node) => toggleCreativeNodeFreeResize(node.id)}
                onDelete={(node) => {
                    setSelectedNodeIds(new Set([node.id]));
                    recordHistory();
                    setNodes((current) => current.filter((item) => item.id !== node.id));
                    setConnections((current) => current.filter((connection) => connection.fromNodeId !== node.id && connection.toNodeId !== node.id));
                }}
            />
            {isMiniMapOpen ? <Minimap nodes={nodes} viewport={viewport} viewportSize={size} onViewportChange={onViewportChange} compactNodeGlyphs /> : null}
            <CanvasZoomControls scale={viewport.k} onScaleChange={setZoomScale} onReset={() => onViewportChange({ x: size.width / 2, y: size.height / 2, k: 1 })} isMiniMapOpen={isMiniMapOpen} onToggleMiniMap={() => setIsMiniMapOpen((value) => !value)} />
            <CanvasToolbar
                selectedCount={selectedNodeIds.size + (selectedConnectionId ? 1 : 0)}
                canUndo={canUndo}
                canRedo={canRedo}
                backgroundMode={backgroundMode}
                showImageInfo={showImageInfo}
                onAddImage={() => addNode(CanvasNodeType.Image)}
                onAddVideo={() => addNode(CanvasNodeType.Video)}
                onAddText={() => addNode(CanvasNodeType.Text)}
                onAddConfig={() => addNode(CanvasNodeType.Config)}
                onUndo={undo}
                onRedo={redo}
                onUpload={() => startUpload(CanvasNodeType.Image)}
                onDelete={deleteSelected}
                onClear={clearCanvas}
                onDeselect={() => {
                    setSelectedNodeIds(new Set());
                    setSelectedConnectionId("");
                }}
                onBackgroundModeChange={setBackgroundMode}
                onShowImageInfoChange={setShowImageInfo}
                onOpenAssets={() => message.info("请在顶部素材中心管理长期素材；当前画布支持直接上传图片/视频。")}
            />
            <CanvasNodeInfoModal node={infoNode} open={Boolean(infoNode)} onClose={() => setInfoNodeId("")} />
            {cropNode?.metadata?.content ? <CanvasNodeCropDialog dataUrl={cropNode.metadata.content} open={Boolean(cropNode)} confirming={cropSubmitting} onClose={() => setCropNodeId(null)} onConfirm={(crop) => void cropImageNode(cropNode, crop)} /> : null}
            {angleNode?.metadata?.content ? <CanvasNodeAngleDialog dataUrl={angleNode.metadata.content} open={Boolean(angleNode)} confirming={angleSubmitting} onClose={() => setAngleNodeId(null)} onConfirm={(params) => void generateAngleNode(angleNode, params)} /> : null}
            <input ref={fileInputRef} type="file" accept={fileInputAccept} className="hidden" onChange={handleFileInput} />
        </div>
    );
}

function createCreativeCanvasNode(type: CanvasNodeType, position: Position, metadata?: CanvasNodeData["metadata"]): CanvasNodeData {
    const spec = getNodeSpec(type);
    return {
        id: `creative-${type}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`,
        type,
        title: spec.title,
        position,
        width: spec.width,
        height: spec.height,
        metadata: { ...(spec.metadata || {}), ...(metadata || {}) },
    };
}

function canvasCenterPosition(size: { width: number; height: number }, viewport: ViewportTransform, type: CanvasNodeType): Position {
    const spec = getNodeSpec(type);
    return {
        x: (size.width / 2 - viewport.x) / viewport.k - spec.width / 2,
        y: (size.height / 2 - viewport.y) / viewport.k - spec.height / 2,
    };
}

function cloneCreativeCanvasNodes(nodes: CanvasNodeData[]) {
    return nodes.map((node) => ({ ...node, position: { ...node.position }, metadata: { ...(node.metadata || {}) } }));
}

function cloneCreativeConnections(connections: CanvasConnection[]) {
    return connections.map((connection) => ({ ...connection }));
}

function readCreativeFileAsDataURL(file: File) {
    return new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result || ""));
        reader.onerror = () => reject(reader.error || new Error("读取文件失败"));
        reader.readAsDataURL(file);
    });
}

function safeDownloadName(value: string) {
    return value.replace(/[\/:*?"<>|]+/g, "_").trim() || "canvas-node";
}

function creativeNodeContent(node: CanvasNodeData) {
    return typeof node.metadata?.content === "string" ? node.metadata.content : "";
}

function shouldProbeCreativeMediaNode(node: CanvasNodeData) {
    if (node.type !== CanvasNodeType.Image && node.type !== CanvasNodeType.Video) return false;
    if (!creativeNodeContent(node)) return false;
    return !node.metadata?.naturalWidth || !node.metadata?.naturalHeight;
}

function fitCreativeMediaNodeSize(type: CanvasNodeType, width: number, height: number) {
    return type === CanvasNodeType.Video ? fitNodeSize(width, height, creativeVideoNodeMaxWidth, creativeVideoNodeMaxHeight) : fitNodeSize(width, height);
}

function blobToCreativeDataUrl(blob: Blob) {
    return new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result || ""));
        reader.onerror = () => reject(new Error("读取素材失败"));
        reader.readAsDataURL(blob);
    });
}

async function ensureCreativeDataUrl(input: string | Blob, mimeType = "application/octet-stream") {
    if (input instanceof Blob) return blobToCreativeDataUrl(input);
    const value = input.trim();
    if (value.startsWith("data:")) return value;
    if (/^(https?:|blob:|\/)/i.test(value)) return blobToCreativeDataUrl(await (await fetch(value)).blob());
    if (mimeType.startsWith("text/")) return `data:${mimeType};charset=utf-8,${encodeURIComponent(value)}`;
    return value;
}

function dataUrlMimeType(value: string, fallback = "application/octet-stream") {
    return value.match(/^data:([^;,]+)/)?.[1] || fallback;
}

function findFreeCreativePosition(nodes: CanvasNodeData[], sourceNode: CanvasNodeData, width: number, height: number): Position {
    const gap = 112;
    const rowGap = 72;
    const baseX = sourceNode.position.x + sourceNode.width + gap;
    const baseY = sourceNode.position.y + sourceNode.height / 2 - height / 2;
    const rowOffsets = [0, 1, -1, 2, -2, 3, -3, 4, -4, 5, -5, 6, -6];
    for (let column = 0; column < 8; column += 1) {
        for (const row of rowOffsets) {
            const position = {
                x: baseX + column * (width + gap),
                y: baseY + row * (height + rowGap),
            };
            if (!creativeRectOverlaps(nodes, position, width, height)) return position;
        }
    }
    const right = nodes.reduce((max, node) => Math.max(max, node.position.x + node.width), sourceNode.position.x + sourceNode.width);
    return { x: right + gap, y: sourceNode.position.y };
}

function creativeRectOverlaps(nodes: CanvasNodeData[], position: Position, width: number, height: number) {
    const padding = 42;
    return nodes.some((node) => {
        const left = position.x - padding;
        const right = position.x + width + padding;
        const top = position.y - padding;
        const bottom = position.y + height + padding;
        return left < node.position.x + node.width && right > node.position.x && top < node.position.y + node.height && bottom > node.position.y;
    });
}

function creativeMediaNodeSizePatch(node: CanvasNodeData, type: CanvasNodeType, width: number, height: number): Pick<CanvasNodeData, "position" | "width" | "height"> {
    const next = fitCreativeMediaNodeSize(type, width, height);
    return {
        width: next.width,
        height: next.height,
        position: {
            x: node.position.x + node.width / 2 - next.width / 2,
            y: node.position.y + node.height / 2 - next.height / 2,
        },
    };
}

function probeCreativeMediaSize(type: CanvasNodeType, src: string) {
    return new Promise<{ width: number; height: number } | null>((resolve) => {
        if (type === CanvasNodeType.Image) {
            const image = new Image();
            image.onload = () => resolve(image.naturalWidth && image.naturalHeight ? { width: image.naturalWidth, height: image.naturalHeight } : null);
            image.onerror = () => resolve(null);
            image.src = src;
            return;
        }
        if (type === CanvasNodeType.Video) {
            const video = document.createElement("video");
            video.preload = "metadata";
            video.onloadedmetadata = () => {
                resolve(video.videoWidth && video.videoHeight ? { width: video.videoWidth, height: video.videoHeight } : null);
            };
            video.onerror = () => resolve(null);
            video.src = src;
            return;
        }
        resolve(null);
    });
}

function metadataString(node: CanvasNodeData | null | undefined, key: keyof CanvasNodeMetadata) {
    const value = node?.metadata?.[key];
    return typeof value === "string" ? value : "";
}

function creativeOriginWorkflowNodeId(node: CanvasNodeData | null | undefined) {
    return metadataString(node, "originWorkflowNodeId") || metadataString(node, "workflowNodeId");
}

function applyCreativeNodeConfigPatch(node: CanvasNodeData, patch: Partial<CanvasNodeData["metadata"]>) {
    const metadataPatch = (patch || {}) as Partial<CanvasNodeMetadata>;
    const next = { ...node, metadata: { ...node.metadata, ...metadataPatch } };
    const spec = node.type === CanvasNodeType.Video ? NODE_DEFAULT_SIZE[CanvasNodeType.Video] : NODE_DEFAULT_SIZE[CanvasNodeType.Image];
    const size = typeof metadataPatch.size === "string" && !node.metadata?.content ? nodeSizeFromRatio(metadataPatch.size, spec.width, spec.height) : null;
    return size && (node.type === CanvasNodeType.Image || node.type === CanvasNodeType.Video) ? { ...next, ...size, position: { x: node.position.x + node.width / 2 - size.width / 2, y: node.position.y + node.height / 2 - size.height / 2 } } : next;
}

function buildCreativeGenerationConfig(config: AiConfig, node: CanvasNodeData | undefined, mode: CanvasNodeGenerationMode): AiConfig {
    const defaultModel = mode === "image" ? config.imageModel : mode === "video" ? config.videoModel : config.textModel;
    return {
        ...config,
        model: node?.metadata?.model || defaultModel || config.model || defaultConfig.model,
        quality: node?.metadata?.quality || config.quality || defaultConfig.quality,
        size: node?.metadata?.size || config.size || defaultConfig.size,
        videoSeconds: node?.metadata?.seconds || config.videoSeconds || defaultConfig.videoSeconds,
        videoReferenceMode: node?.metadata?.videoReferenceMode || config.videoReferenceMode || defaultConfig.videoReferenceMode,
        vquality: node?.metadata?.vquality || config.vquality || defaultConfig.vquality,
        count: String(node?.metadata?.count || (mode === "image" ? 3 : config.count) || defaultConfig.count),
    };
}

function isCreativeAiConfigReady(config: AiConfig, model: string) {
    return Boolean(model.trim()) && (config.channelMode === "remote" || Boolean(config.baseUrl.trim() && config.apiKey.trim()));
}

function getGenerationCount(count: string) {
    return Math.max(1, Math.min(15, Math.floor(Math.abs(Number(count)) || 1)));
}

function sourceNodeReferenceImages(node: CanvasNodeData | null) {
    if (!node || node.type !== CanvasNodeType.Image || !node.metadata?.content) return [];
    return [
        {
            id: node.id,
            name: `${node.title || node.id}.png`,
            type: node.metadata.mimeType || "image/png",
            dataUrl: node.metadata.content,
            storageKey: node.metadata.storageKey,
        },
    ];
}

function findCreativeRetrySourceNode(nodeId: string, nodes: CanvasNodeData[], connections: CanvasConnection[]) {
    const queue = connections.filter((connection) => connection.toNodeId === nodeId).map((connection) => connection.fromNodeId);
    const visited = new Set<string>();
    while (queue.length) {
        const id = queue.shift()!;
        if (visited.has(id)) continue;
        visited.add(id);
        const node = nodes.find((item) => item.id === id);
        if (node?.type === CanvasNodeType.Config) return node;
        connections.filter((connection) => connection.toNodeId === id).forEach((connection) => queue.push(connection.fromNodeId));
    }
    return null;
}

function buildCreativeImageGenerationMetadata(type: CanvasImageGenerationType, config: AiConfig, count: number, references: ReferenceImage[]): CanvasNodeMetadata {
    return {
        generationType: type,
        model: config.model,
        size: config.size,
        quality: config.quality,
        count,
        references: references.map(referenceUrl).filter((url): url is string => Boolean(url)),
    };
}

function referenceUrl(image: ReferenceImage) {
    return image.storageKey || image.url || (!image.dataUrl.startsWith("data:") ? image.dataUrl : undefined);
}

async function resolveCreativeMetadataReferences(runId: string, token: string, metadata: CanvasNodeMetadata) {
    if (metadata.generationType !== "edit") return [];
    if (!metadata.references?.length) return null;
    const references = await Promise.all(
        metadata.references.map(async (value, index) => {
            const source = creativeReferenceURL(runId, token, value);
            const dataUrl = await ensureCreativeDataUrl(source, "image/png").catch(() => "");
            if (!dataUrl) return null;
            return { id: `reference-${index}`, name: `reference-${index}.png`, type: dataUrlMimeType(dataUrl, "image/png"), dataUrl, storageKey: value } satisfies ReferenceImage;
        }),
    );
    return references.every(Boolean) ? (references as ReferenceImage[]) : null;
}

function creativeReferenceURL(runId: string, token: string, value: string) {
    if (value.startsWith("data:") || value.startsWith("blob:") || /^https?:/i.test(value)) return value;
    if (value.startsWith("/api/workflows/pdd/")) return withPDDFileToken(value, token);
    if (value.includes("/") || value.includes("\\")) return withPDDFileToken(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/file?path=${encodeURIComponent(value)}`, token);
    return value;
}

function getInputSummary(inputs: NodeGenerationInput[]) {
    return {
        textCount: inputs.filter((input) => input.type === "text").length,
        imageCount: inputs.filter((input) => input.type === "image").length,
    };
}

function buildAngleLabel(params: CanvasImageAngleParams) {
    const horizontal = params.horizontalAngle === 0 ? "正面视角" : params.horizontalAngle > 0 ? `向右旋转 ${params.horizontalAngle} 度` : `向左旋转 ${Math.abs(params.horizontalAngle)} 度`;
    const pitch = params.pitchAngle === 0 ? "水平视角" : params.pitchAngle > 0 ? `俯视 ${params.pitchAngle} 度` : `仰视 ${Math.abs(params.pitchAngle)} 度`;
    return `AI 多角度：${horizontal}，${pitch}，镜头距离 ${params.cameraDistance.toFixed(1)}，${params.wideAngle ? "广角" : "标准"}镜头`;
}

function buildAnglePrompt(params: CanvasImageAngleParams) {
    return `基于参考图重新生成同一主体的新视角，保持主体、颜色、材质和画面风格一致，不要只做透视变形。${buildAngleLabel(params)}。`;
}

function hydrateCreativeNodes(canvas: PDDCreativeCanvas, token: string): CanvasNodeData[] {
    return (canvas.nodes || []).map((node) => {
        const metadata = { ...(node.metadata || {}) } as NonNullable<CanvasNodeData["metadata"]>;
        if (typeof metadata.content === "string" && metadata.content.startsWith("/api/workflows/pdd/")) {
            metadata.content = withPDDFileToken(metadata.content, token);
        }
        if ((metadata as Record<string, unknown>).status === "running") metadata.status = "loading";
        return {
            id: node.id,
            type: node.type === "video" ? CanvasNodeType.Video : node.type === "text" ? CanvasNodeType.Text : node.type === "config" ? CanvasNodeType.Config : CanvasNodeType.Image,
            title: node.title,
            position: node.position,
            width: node.width,
            height: node.height,
            metadata,
        };
    });
}

function dehydrateCreativeNodes(nodes: CanvasNodeData[]) {
    return nodes.map((node) => {
        const metadata = { ...(node.metadata || {}) };
        if (typeof metadata.content === "string" && metadata.content.includes("/api/workflows/pdd/")) {
            metadata.content = stripTokenFromURL(metadata.content);
        }
        return {
            id: node.id,
            type: node.type,
            title: node.title,
            position: node.position,
            width: node.width,
            height: node.height,
            metadata,
        };
    });
}

function mergeIncomingCreativeNodes(current: CanvasNodeData[], incoming: CanvasNodeData[]) {
    const incomingById = new Map(incoming.map((node) => [node.id, node]));
    let changed = false;
    const nodes = current.map((node) => {
        const next = incomingById.get(node.id);
        if (!next) return node;
        incomingById.delete(node.id);
        const merged = mergeIncomingCreativeNode(node, next);
        if (merged !== node) changed = true;
        return merged;
    });
    if (incomingById.size) {
        changed = true;
        nodes.push(...incomingById.values());
    }
    return changed ? nodes : current;
}

function mergeIncomingCreativeConnections(current: CanvasConnection[], incoming: CanvasConnection[]) {
    const seen = new Set(current.map((connection) => `${connection.fromNodeId}->${connection.toNodeId}`));
    const additions = incoming.filter((connection) => {
        const key = `${connection.fromNodeId}->${connection.toNodeId}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
    });
    return additions.length ? [...current, ...additions] : current;
}

function mergeIncomingCreativeNode(current: CanvasNodeData, incoming: CanvasNodeData) {
    const metadata = mergeIncomingCreativeMetadata(current.metadata, incoming.metadata) || {};
    const contentChanged = current.metadata?.content !== metadata.content;
    let width = current.width;
    let height = current.height;
    let position = current.position;
    if (contentChanged && !metadata.freeResize && (current.type === CanvasNodeType.Image || current.type === CanvasNodeType.Video) && metadata.naturalWidth && metadata.naturalHeight) {
        const size = fitCreativeMediaNodeSize(current.type, metadata.naturalWidth, metadata.naturalHeight);
        width = size.width;
        height = size.height;
        position = {
            x: current.position.x + current.width / 2 - size.width / 2,
            y: current.position.y + current.height / 2 - size.height / 2,
        };
    }
    const title = current.title || incoming.title;
    const type = current.type || incoming.type;
    if (title === current.title && type === current.type && width === current.width && height === current.height && position === current.position && metadata === current.metadata) return current;
    return { ...current, title, type, width, height, position, metadata };
}

function mergeIncomingCreativeMetadata(current?: CanvasNodeMetadata, incoming?: CanvasNodeMetadata) {
    if (!incoming) return current;
    const local = current || {};
    const result: CanvasNodeMetadata = { ...local };
    let changed = false;
    const assign = <Key extends keyof CanvasNodeMetadata>(key: Key, value: CanvasNodeMetadata[Key], overwrite = true) => {
        if (value === undefined || (!overwrite && result[key] !== undefined)) return;
        if (result[key] !== value) {
            result[key] = value;
            changed = true;
        }
    };
    assign("workflowNodeId", incoming.workflowNodeId, false);
    assign("originWorkflowNodeId", incoming.originWorkflowNodeId, false);
    assign("artifactKind", incoming.artifactKind, false);
    assign("source", incoming.source, false);

    const localOverride = creativeNodeHasLocalContentOverride(result);
    if (!localOverride) {
        assign("status", incoming.status);
        assign("errorDetails", incoming.errorDetails);
        if (incoming.status && incoming.status !== "error" && result.errorDetails !== undefined && incoming.errorDetails === undefined) {
            result.errorDetails = undefined;
            changed = true;
        }
        assign("operation", incoming.operation);
    } else {
        assign("status", incoming.status, false);
    }

    if (!localOverride) {
        assign("content", incoming.content);
        assign("artifactPath", incoming.artifactPath);
        assign("storageKey", incoming.storageKey);
        assign("mimeType", incoming.mimeType);
        assign("bytes", incoming.bytes);
        assign("naturalWidth", incoming.naturalWidth);
        assign("naturalHeight", incoming.naturalHeight);
        assign("artifactKind", incoming.artifactKind);
    }

    assign("prompt", incoming.prompt, false);
    assign("model", incoming.model, false);
    assign("size", incoming.size, false);
    assign("quality", incoming.quality, false);
    assign("count", incoming.count, false);
    assign("seconds", incoming.seconds, false);
    assign("vquality", incoming.vquality, false);
    assign("videoReferenceMode", incoming.videoReferenceMode, false);
    return changed ? result : current;
}

function creativeNodeHasLocalContentOverride(metadata?: CanvasNodeMetadata) {
    const source = metadata?.source || "";
    return source === "user_upload" || source.startsWith("creative_");
}

function stripTokenFromURL(value: string) {
    try {
        const url = new URL(value, window.location.origin);
        url.searchParams.delete("token");
        return `${url.pathname}${url.search}`;
    } catch {
        return value.replace(/([?&])token=[^&]+&?/, "$1").replace(/[?&]$/, "");
    }
}

function screenToWorld(clientX: number, clientY: number, container: HTMLDivElement | null, viewport: ViewportTransform): Position {
    const rect = container?.getBoundingClientRect();
    return {
        x: ((clientX - (rect?.left || 0)) - viewport.x) / viewport.k,
        y: ((clientY - (rect?.top || 0)) - viewport.y) / viewport.k,
    };
}

function nodeAtWorld(nodes: CanvasNodeData[], point: Position) {
    for (let index = nodes.length - 1; index >= 0; index -= 1) {
        const node = nodes[index];
        if (point.x >= node.position.x && point.x <= node.position.x + node.width && point.y >= node.position.y && point.y <= node.position.y + node.height) return node;
    }
    return null;
}

function toMinimapNode(node: { id: string; type?: string; title: string; x: number; y: number; width: number; height: number }): CanvasNodeData {
    const type = node.type === "image" ? CanvasNodeType.Image : node.type === "video" ? CanvasNodeType.Video : node.type === "text" ? CanvasNodeType.Text : CanvasNodeType.Config;
    return {
        id: node.id,
        type,
        title: node.title,
        position: { x: node.x, y: node.y },
        width: node.width,
        height: node.height,
    };
}

function OverviewGraphNode({ node, selected, onSelectStage }: { node: PDDStageNode & { x: number; y: number; width: number; height: number }; selected: boolean; onSelectStage: (stage: PDDStageNode) => void }) {
    return (
        <button
            data-node-id={node.id}
            type="button"
            className={`absolute rounded-xl border p-3 text-left shadow-sm transition hover:scale-[1.01] ${statusMeta[node.status].className} ${selected ? "ring-2 ring-blue-500" : ""}`}
            style={{ left: node.x, top: node.y, width: node.width, height: node.height }}
            onClick={() => onSelectStage(node)}
        >
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="truncate text-base font-semibold">{node.title}</div>
                    <div className="mt-1 font-mono text-xs text-stone-500">{node.id}</div>
                </div>
                <StatusTag status={node.status} />
            </div>
            <div className="mt-4 grid grid-cols-4 gap-2 text-center text-xs">
                <Metric label="总" value={node.total} />
                <Metric label="成功" value={node.success} />
                <Metric label="失败" value={node.failed} />
                <Metric label="运行" value={node.running} />
            </div>
            <div className="mt-3 flex items-center justify-between text-xs text-stone-500">
                <span>skipped {node.skipped}</span>
                <span>{formatDuration(node.durationSeconds)}</span>
            </div>
            {node.recentError ? <div className="mt-2 truncate text-xs text-red-500">{node.recentError}</div> : null}
        </button>
    );
}

function ProductTable({
    rows,
    loading,
    isCustomWorkflow,
    selectedKey,
    keyword,
    statusFilter,
    onKeywordChange,
    onStatusFilterChange,
    onSelect,
}: {
    rows: PDDProductSummary[];
    loading: boolean;
    isCustomWorkflow: boolean;
    selectedKey: string;
    keyword: string;
    statusFilter: string;
    onKeywordChange: (value: string) => void;
    onStatusFilterChange: (value: string) => void;
    onSelect: (product: PDDProductSummary) => void;
}) {
    const columns: ColumnsType<PDDProductSummary> = [
        {
            title: "商品",
            dataIndex: "sourceProduct",
            ellipsis: true,
            render: (_, item) => (
                <button type="button" className="w-full text-left" onClick={() => onSelect(item)}>
                    <div className="truncate text-sm font-medium">{item.sourceProduct}</div>
                    <div className="truncate text-xs text-stone-500">{item.product || item.generatedProduct}</div>
                </button>
            ),
        },
        { title: "状态", dataIndex: "status", width: 92, render: (status: PDDRunStatus) => <StatusTag status={status} /> },
        {
            title: isCustomWorkflow ? "产物" : "图",
            width: 88,
            render: (_, item) => <span className="font-mono text-xs">{isCustomWorkflow ? item.artifactCount || 0 : `${item.generatedImages}/${item.specImages}/${item.mainImages}`}</span>,
        },
    ];
    return (
        <section className="flex min-h-0 flex-col gap-3 p-4">
            <div className="flex items-center gap-2">
                <Input.Search allowClear size="small" placeholder="搜索商品/错误" value={keyword} onChange={(event) => onKeywordChange(event.target.value)} />
                <Select
                    size="small"
                    className="w-[110px]"
                    value={statusFilter}
                    onChange={onStatusFilterChange}
                    options={[
                        { value: "", label: "全部" },
                        { value: "running", label: "running" },
                        { value: "success", label: "success" },
                        { value: "error", label: "error" },
                        { value: "idle", label: "idle" },
                    ]}
                />
            </div>
            <Table
                rowKey="key"
                size="small"
                columns={columns}
                dataSource={rows}
                loading={loading}
                pagination={{ pageSize: 12, size: "small" }}
                rowClassName={(record) => (record.key === selectedKey ? "bg-blue-50 dark:bg-blue-950/30" : "")}
                scroll={{ y: 360 }}
            />
        </section>
    );
}

function ProductGraphNode({
    node,
    product,
    token,
    selected,
    onSelectNode,
    onOpenArtifact,
}: {
    node: PDDGraphNode;
    product: PDDProductDetail;
    token: string;
    selected: boolean;
    onSelectNode: (node: PDDGraphNode, product: PDDProductDetail) => void;
    onOpenArtifact: (artifact: PDDArtifact, node: PDDGraphNode, product: PDDProductDetail) => void;
}) {
    const preview = node.artifacts?.[0];
    return (
        <div
            data-node-id={node.id}
            role="button"
            tabIndex={0}
            className={`absolute overflow-hidden rounded-xl border p-3 text-left shadow-sm transition hover:scale-[1.01] ${statusMeta[node.status].className} ${selected ? "ring-2 ring-blue-500" : ""}`}
            style={{ left: node.x, top: node.y, width: node.width, height: node.height }}
            onClick={() => onSelectNode(node, product)}
            onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") onSelectNode(node, product);
            }}
        >
            <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                    <div className="truncate text-sm font-semibold">{node.title}</div>
                    <div className="mt-1 truncate text-xs text-stone-500">{node.summary || node.type}</div>
                </div>
                <StatusTag status={node.status} />
            </div>
            {preview ? (
                <div className="mt-3 flex gap-2 overflow-hidden">
                    {(node.artifacts || []).slice(0, 4).map((artifact) => (
                        <Tooltip key={artifact.id} title={artifact.title}>
                            <button
                                type="button"
                                className="block size-16 overflow-hidden rounded-md border border-stone-200 bg-white dark:border-stone-700 dark:bg-stone-900"
                                onClick={(event) => {
                                    event.stopPropagation();
                                    onOpenArtifact(artifact, node, product);
                                }}
                            >
                                <img src={withPDDFileToken(artifact.url, token)} alt={artifact.title} width={64} height={64} className="size-16 object-cover" draggable={false} />
                            </button>
                        </Tooltip>
                    ))}
                </div>
            ) : shouldShowNodeFileBadge(node) ? (
                <div className="mt-5 flex items-center gap-2 text-xs text-stone-500">
                    <FileJson className="size-4" />
                    {node.files?.length || 0} 个详情文件
                </div>
            ) : null}
        </div>
    );
}

function shouldShowNodeFileBadge(node: PDDGraphNode) {
    return node.type !== "text" && node.type !== "material" && Boolean(node.files?.length);
}

function DetailPanel({
    target,
    runId,
    token,
    actionOutput,
    recentErrors,
    activeTab,
    onTabChange,
    onOpenImage,
}: {
    target: DetailTarget | null;
    runId: string;
    token: string;
    actionOutput: string;
    recentErrors: string[];
    activeTab: DetailTabKey;
    onTabChange: (tab: DetailTabKey) => void;
    onOpenImage: (image: { title: string; src: string; path: string }) => void;
}) {
    const [activeFile, setActiveFile] = useState<PDDDetailFile | null>(null);
    const [fileText, setFileText] = useState("");
    const [logText, setLogText] = useState("");

    useEffect(() => {
        setActiveFile(null);
        setFileText("");
    }, [target]);

    useEffect(() => {
        if (!activeFile || activeFile.kind === "image") return;
        let cancelled = false;
        void fetch(withPDDFileToken(activeFile.url, token))
            .then((response) => response.text())
            .then((text) => {
                if (!cancelled) setFileText(text);
            })
            .catch(() => {
                if (!cancelled) setFileText("读取文件失败");
            });
        return () => {
            cancelled = true;
        };
    }, [activeFile, token]);

    useEffect(() => {
        if (!token) return;
        const source = new EventSource(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/log-stream?token=${encodeURIComponent(token)}`);
        source.onmessage = (event) => {
            setLogText((current) => `${current}\n${event.data}`.slice(-12000));
        };
        source.onerror = () => {
            setLogText((current) => current || "日志文件不存在或暂时不可读");
            source.close();
        };
        return () => source.close();
    }, [runId, token]);

    const files = detailFilesFromTarget(target);
    const artifact = target?.kind === "artifact" ? target.artifact : null;
    const stage = target?.kind === "stage" ? target.stage : null;

    return (
        <div className="flex h-full min-h-0 flex-col">
            <Tabs
                activeKey={activeTab}
                onChange={(key) => onTabChange(key as DetailTabKey)}
                className="min-h-0 flex-1"
                items={[
                    {
                        key: "summary",
                        label: "摘要",
                        children: (
                            <div className="space-y-4 overflow-auto pb-5">
                                {stage ? <StageSummary stage={stage} /> : null}
                                {artifact ? <ArtifactPreview artifact={artifact} token={token} onOpenImage={onOpenImage} /> : null}
                                {target?.kind === "product-node" ? <NodeSummary node={target.node} /> : null}
                                <ErrorList recentErrors={recentErrors} />
                                {actionOutput ? <PreBlock title="最近动作输出" value={actionOutput} /> : null}
                            </div>
                        ),
                    },
                    {
                        key: "files",
                        label: "文件",
                        children: (
                            <div className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-3 pb-5">
                                <Space wrap>
                                    {files.map((file) => (
                                        <Button key={file.path} size="small" icon={file.kind === "image" ? <ImageIcon className="size-3.5" /> : <FileJson className="size-3.5" />} onClick={() => setActiveFile(file)}>
                                            {file.title}
                                        </Button>
                                    ))}
                                    {!files.length ? <Typography.Text type="secondary">当前节点没有关联文件</Typography.Text> : null}
                                </Space>
                                <FileViewer file={activeFile} token={token} text={fileText} onOpenImage={onOpenImage} />
                            </div>
                        ),
                    },
                    {
                        key: "log",
                        label: "实时日志",
                        children: <PreBlock title="remote_workflow.log" value={logText || "等待日志更新..."} />,
                    },
                ]}
            />
        </div>
    );
}

function StageSummary({ stage }: { stage: PDDStageNode }) {
    return (
        <Card size="small">
            <div className="mb-3 flex items-center justify-between">
                <StatusTag status={stage.status} />
                <span className="font-mono text-xs text-stone-500">{formatDuration(stage.durationSeconds)}</span>
            </div>
            <div className="grid grid-cols-5 gap-2 text-center text-xs">
                <Metric label="总数" value={stage.total} />
                <Metric label="成功" value={stage.success} />
                <Metric label="失败" value={stage.failed} />
                <Metric label="运行" value={stage.running} />
                <Metric label="跳过" value={stage.skipped} />
            </div>
            {stage.recentError ? <Typography.Paragraph className="!mb-0 !mt-3 text-xs text-red-500">{stage.recentError}</Typography.Paragraph> : null}
        </Card>
    );
}

function NodeSummary({ node }: { node: PDDGraphNode }) {
    const config = node.config || {};
    const rows = [
        ["操作", configString(config.operation)],
        ["模型", configString(config.model)],
        ["数量", configString(config.count)],
        ["尺寸", configString(config.size)],
        ["质量", configString(config.quality)],
        ["秒数", configString(config.seconds)],
        ["视频质量", configString(config.videoQuality)],
        ["耗时", node.durationSeconds ? formatDuration(node.durationSeconds) : configDuration(config.durationSeconds)],
        ["上游输入", configList(config.upstream)],
    ].filter(([, value]) => value);
    const prompt = configString(config.prompt);
    const mappings = config.outputMappings ? stringifyConfig(config.outputMappings) : "";
    const extra = config.extra ? stringifyConfig(config.extra) : "";
    return (
        <Card size="small">
            <div className="mb-2 flex items-center gap-2">
                <Boxes className="size-4" />
                <span className="font-semibold">{node.type}</span>
                <StatusTag status={node.status} />
            </div>
            <Typography.Paragraph className="!mb-0 text-sm">{node.summary || "无摘要"}</Typography.Paragraph>
            {rows.length ? (
                <div className="mt-3 grid grid-cols-[72px_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs">
                    {rows.map(([label, value]) => (
                        <div key={label} className="contents">
                            <span className="text-stone-500">{label}</span>
                            <span className="min-w-0 break-words font-mono">{value}</span>
                        </div>
                    ))}
                </div>
            ) : null}
            {prompt ? <CompactPreBlock title="Prompt" value={prompt} /> : null}
            {mappings ? <CompactPreBlock title="输出路径模板" value={mappings} /> : null}
            {extra ? <CompactPreBlock title="扩展配置" value={extra} /> : null}
        </Card>
    );
}

function configString(value: unknown) {
    if (value === null || value === undefined || value === "") return "";
    if (typeof value === "string") return value;
    if (typeof value === "number" || typeof value === "boolean") return String(value);
    return "";
}

function configList(value: unknown) {
    if (!Array.isArray(value)) return "";
    return value
        .map((item) => configString(item))
        .filter(Boolean)
        .join(", ");
}

function configDuration(value: unknown) {
    if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) return "";
    return formatDuration(value);
}

function stringifyConfig(value: unknown) {
    try {
        return JSON.stringify(value, null, 2);
    } catch {
        return "";
    }
}

function CompactPreBlock({ title, value }: { title: string; value: string }) {
    return (
        <div className="mt-3 rounded-lg border border-stone-200 dark:border-stone-800">
            <div className="border-b border-stone-200 px-3 py-2 text-xs font-medium text-stone-500 dark:border-stone-800">{title}</div>
            <pre className="thin-scrollbar m-0 max-h-52 overflow-auto whitespace-pre-wrap break-words p-3 text-xs leading-5">{value}</pre>
        </div>
    );
}

function ArtifactPreview({ artifact, token, onOpenImage }: { artifact: PDDArtifact; token: string; onOpenImage: (image: { title: string; src: string; path: string }) => void }) {
    const src = withPDDFileToken(artifact.url, token);
    return (
        <Card
            size="small"
            title={artifact.path}
            extra={
                <Link href={src} target="_blank">
                    <ExternalLink className="size-4" />
                </Link>
            }
        >
            <button type="button" className="block w-full overflow-hidden rounded-lg bg-white dark:bg-stone-950" onClick={() => onOpenImage({ title: artifact.title, src, path: artifact.path })}>
                <img src={src} alt={artifact.title} className="mx-auto max-h-[420px] w-full object-contain" draggable={false} />
            </button>
        </Card>
    );
}

function FileViewer({ file, token, text, onOpenImage }: { file: PDDDetailFile | null; token: string; text: string; onOpenImage: (image: { title: string; src: string; path: string }) => void }) {
    if (!file) return <div className="flex items-center justify-center text-sm text-stone-500">选择一个文件查看内容</div>;
    if (file.kind === "image") {
        const src = withPDDFileToken(file.url, token);
        return (
            <button type="button" className="flex h-full min-h-[220px] w-full items-center justify-center overflow-hidden rounded-lg bg-white dark:bg-stone-950" onClick={() => onOpenImage({ title: file.title, src, path: file.path })}>
                <img src={src} alt={file.title} className="max-h-[640px] w-full object-contain" draggable={false} />
            </button>
        );
    }
    return <PreBlock title={file.path} value={text || "读取中..."} />;
}

function ErrorList({ recentErrors }: { recentErrors: string[] }) {
    if (!recentErrors.length) return <Card size="small">暂无最近错误</Card>;
    return (
        <Card size="small" title="最近错误">
            <div className="space-y-2">
                {recentErrors.map((error, index) => (
                    <div key={`${index}-${error}`} className="rounded-md bg-red-50 p-2 text-xs text-red-600 dark:bg-red-950/30 dark:text-red-300">
                        {error}
                    </div>
                ))}
            </div>
        </Card>
    );
}

function PreBlock({ title, value }: { title: string; value: string }) {
    return (
        <div className="flex h-full min-h-[220px] flex-col rounded-lg border border-stone-200 dark:border-stone-800">
            <div className="border-b border-stone-200 px-3 py-2 text-xs font-medium text-stone-500 dark:border-stone-800">{title}</div>
            <pre className="thin-scrollbar m-0 min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-words p-3 text-xs leading-5">{value}</pre>
        </div>
    );
}

function detailFilesFromTarget(target: DetailTarget | null) {
    if (!target) return [];
    if (target.kind === "file") return [target.file];
    if (target.kind === "product-node") return target.node.files || [];
    if (target.kind === "artifact") return [{ title: target.artifact.title, path: target.artifact.path, url: target.artifact.url, kind: "image" as const }];
    return [];
}

function GraphEdges({ edges, byId }: { edges: PDDGraphEdge[]; byId: Record<string, { x: number; y: number; width: number; height: number }> }) {
    return (
        <svg className="pointer-events-none absolute left-0 top-0 overflow-visible" width={4000} height={900}>
            {edges.map((edge) => {
                const from = byId[edge.from];
                const to = byId[edge.to];
                if (!from || !to) return null;
                const x1 = from.x + from.width;
                const y1 = from.y + from.height / 2;
                const x2 = to.x;
                const y2 = to.y + to.height / 2;
                const mid = (x1 + x2) / 2;
                return <path key={edge.id} d={`M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`} fill="none" stroke="currentColor" strokeWidth={2} className="text-stone-300 dark:text-stone-700" />;
            })}
        </svg>
    );
}

function StatusTag({ status }: { status: PDDRunStatus }) {
    const meta = statusMeta[status] || statusMeta.idle;
    return (
        <Tag color={meta.color} className="m-0">
            {status === "running" ? <LoaderCircle className="mr-1 inline size-3 animate-spin align-[-2px]" /> : null}
            {meta.label}
        </Tag>
    );
}

function Metric({ label, value }: { label: string; value: number }) {
    return (
        <div className="rounded-md bg-white/70 px-2 py-1 dark:bg-black/20">
            <div className="font-mono text-sm font-semibold">{value}</div>
            <div className="text-[11px] text-stone-500">{label}</div>
        </div>
    );
}

function formatDuration(value?: number) {
    if (!value) return "-";
    if (value < 60) return `${value.toFixed(1)}s`;
    if (value < 3600) return `${Math.round(value / 60)}m`;
    return `${(value / 3600).toFixed(1)}h`;
}

function formatElapsed(startedAt?: string) {
    if (!startedAt) return "-";
    const started = Date.parse(startedAt);
    if (!Number.isFinite(started)) return "-";
    const seconds = Math.max(0, Math.floor((Date.now() - started) / 1000));
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
    return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}

function ImageLightbox({ image, onClose }: { image: LightboxImage | null; onClose: () => void }) {
    const [scale, setScale] = useState(1);
    const [offset, setOffset] = useState({ x: 0, y: 0 });
    const dragRef = useRef<{ startX: number; startY: number; originX: number; originY: number; moved: boolean } | null>(null);

    useEffect(() => {
        setScale(1);
        setOffset({ x: 0, y: 0 });
    }, [image?.src]);

    useEffect(() => {
        if (!image) return;
        const close = (event: KeyboardEvent) => {
            if (event.key === "Escape") onClose();
        };
        window.addEventListener("keydown", close);
        return () => window.removeEventListener("keydown", close);
    }, [image, onClose]);

    if (!image) return null;

    const zoom = (delta: number) => {
        setScale((value) => {
            const next = Math.max(0.25, Math.min(5, Number((value + delta).toFixed(2))));
            if (next <= 1) setOffset({ x: 0, y: 0 });
            return next;
        });
    };

    return (
        <div
            className="fixed inset-0 z-[120] flex items-center justify-center bg-black/82 p-8"
            data-canvas-no-zoom
            onClick={(event) => {
                if (event.target === event.currentTarget) onClose();
            }}
            onWheel={(event) => {
                event.preventDefault();
                event.stopPropagation();
                zoom(event.deltaY > 0 ? -0.12 : 0.12);
            }}
        >
            <div className="absolute left-6 top-5 max-w-[calc(100vw-9rem)] truncate text-sm text-white/80">{image.path || image.title}</div>
            <button type="button" className="absolute right-5 top-5 rounded-full bg-white/10 p-2 text-white transition hover:bg-white/20" onClick={onClose} aria-label="关闭预览">
                <X className="size-5" />
            </button>
            <div
                className="max-h-full max-w-full overflow-hidden"
                onClick={(event) => event.stopPropagation()}
                onPointerDown={(event) => {
                    event.stopPropagation();
                    if (event.button !== 0 || scale <= 1) return;
                    event.preventDefault();
                    event.currentTarget.setPointerCapture(event.pointerId);
                    dragRef.current = { startX: event.clientX, startY: event.clientY, originX: offset.x, originY: offset.y, moved: false };
                }}
                onPointerMove={(event) => {
                    if (!dragRef.current) return;
                    event.stopPropagation();
                    const dx = event.clientX - dragRef.current.startX;
                    const dy = event.clientY - dragRef.current.startY;
                    if (Math.abs(dx) > 2 || Math.abs(dy) > 2) dragRef.current.moved = true;
                    setOffset({ x: dragRef.current.originX + dx, y: dragRef.current.originY + dy });
                }}
                onPointerUp={(event) => {
                    event.stopPropagation();
                    dragRef.current = null;
                }}
                onPointerCancel={() => {
                    dragRef.current = null;
                }}
            >
                <img
                    src={image.src}
                    alt={image.title}
                    className={`max-h-[calc(100vh-6rem)] max-w-[calc(100vw-6rem)] select-none object-contain transition-transform ${scale > 1 ? "cursor-grab active:cursor-grabbing" : ""}`}
                    draggable={false}
                    style={{ transform: `translate(${offset.x}px, ${offset.y}px) scale(${scale})` }}
                />
            </div>
            <div className="absolute bottom-6 left-1/2 flex -translate-x-1/2 items-center gap-2 rounded-xl border border-white/15 bg-black/45 px-2 py-1 text-white shadow-xl backdrop-blur">
                <Button type="text" className="!text-white" icon={<ZoomOut className="size-4" />} onClick={() => zoom(-0.2)} />
                <Button type="text" className="!text-white" onClick={() => setScale(1)}>
                    {Math.round(scale * 100)}%
                </Button>
                <Button type="text" className="!text-white" icon={<ZoomIn className="size-4" />} onClick={() => zoom(0.2)} />
                <Button type="link" className="!text-white" href={image.src} target="_blank">
                    打开原图
                </Button>
            </div>
        </div>
    );
}

function ManualMaskCanvas({ imageUrl, onChange, onClear }: { imageUrl: string; onChange: (dataUrl: string) => void; onClear: () => void }) {
    const imageRef = useRef<HTMLImageElement | null>(null);
    const overlayRef = useRef<HTMLCanvasElement | null>(null);
    const maskRef = useRef<HTMLCanvasElement | null>(null);
    const drawingRef = useRef(false);
    const [brushSize, setBrushSize] = useState(48);

    const resetMask = () => {
        const overlay = overlayRef.current;
        const mask = maskRef.current;
        const image = imageRef.current;
        if (!overlay || !mask || !image?.naturalWidth || !image?.naturalHeight) return;
        overlay.width = image.naturalWidth;
        overlay.height = image.naturalHeight;
        mask.width = image.naturalWidth;
        mask.height = image.naturalHeight;
        overlay.getContext("2d")?.clearRect(0, 0, overlay.width, overlay.height);
        const maskContext = mask.getContext("2d");
        if (maskContext) {
            maskContext.globalCompositeOperation = "source-over";
            maskContext.fillStyle = "black";
            maskContext.fillRect(0, 0, mask.width, mask.height);
        }
        onClear();
    };

    const canvasPoint = (event: ReactPointerEvent<HTMLCanvasElement>) => {
        const canvas = overlayRef.current;
        if (!canvas) return null;
        const rect = canvas.getBoundingClientRect();
        if (!rect.width || !rect.height) return null;
        return {
            x: ((event.clientX - rect.left) / rect.width) * canvas.width,
            y: ((event.clientY - rect.top) / rect.height) * canvas.height,
        };
    };

    const paint = (event: ReactPointerEvent<HTMLCanvasElement>) => {
        const point = canvasPoint(event);
        const overlay = overlayRef.current;
        const mask = maskRef.current;
        if (!point || !overlay || !mask) return;
        const radius = Math.max(8, brushSize) / 2;
        const overlayContext = overlay.getContext("2d");
        if (overlayContext) {
            overlayContext.globalCompositeOperation = "source-over";
            overlayContext.fillStyle = "rgba(248,113,113,.42)";
            overlayContext.beginPath();
            overlayContext.arc(point.x, point.y, radius, 0, Math.PI * 2);
            overlayContext.fill();
        }
        const maskContext = mask.getContext("2d");
        if (maskContext) {
            maskContext.globalCompositeOperation = "destination-out";
            maskContext.beginPath();
            maskContext.arc(point.x, point.y, radius, 0, Math.PI * 2);
            maskContext.fill();
            onChange(mask.toDataURL("image/png"));
        }
    };

    return (
        <div className="mb-4 space-y-3 rounded-xl border border-stone-200 p-3 dark:border-stone-800" data-canvas-no-zoom>
            <div className="flex flex-wrap items-center justify-between gap-2">
                <Typography.Text className="text-sm font-medium">局部蒙版</Typography.Text>
                <Space>
                    <InputNumber min={12} max={160} value={brushSize} addonBefore="笔刷" addonAfter="px" onChange={(value) => setBrushSize(Number(value || 48))} />
                    <Button onClick={resetMask}>清空</Button>
                </Space>
            </div>
            <Typography.Text type="secondary" className="block text-xs">
                在图片上涂抹需要修改的区域；未涂抹区域会尽量保持不变。
            </Typography.Text>
            <div className="max-h-[420px] overflow-auto rounded-lg bg-stone-950/5 p-2 dark:bg-black/30">
                <div className="relative inline-block max-w-full">
                    <img
                        ref={imageRef}
                        src={imageUrl}
                        alt="蒙版参考图"
                        className="block max-h-[390px] max-w-full select-none object-contain"
                        draggable={false}
                        onLoad={resetMask}
                    />
                    <canvas
                        ref={overlayRef}
                        className="absolute inset-0 h-full w-full cursor-crosshair touch-none"
                        onPointerDown={(event) => {
                            event.preventDefault();
                            event.stopPropagation();
                            event.currentTarget.setPointerCapture(event.pointerId);
                            drawingRef.current = true;
                            paint(event);
                        }}
                        onPointerMove={(event) => {
                            if (!drawingRef.current) return;
                            event.preventDefault();
                            event.stopPropagation();
                            paint(event);
                        }}
                        onPointerUp={(event) => {
                            event.preventDefault();
                            event.stopPropagation();
                            drawingRef.current = false;
                        }}
                        onPointerCancel={() => {
                            drawingRef.current = false;
                        }}
                    />
                    <canvas ref={maskRef} className="hidden" />
                </div>
            </div>
        </div>
    );
}
