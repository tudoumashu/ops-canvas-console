"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ComponentType, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent, ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQuery } from "@tanstack/react-query";
import { App, Button, Card, Input, InputNumber, Modal, Select, Space, Switch, Tag, Typography, Upload } from "antd";
import {
    ArrowLeft,
    BookOpen,
    Box,
    ChevronDown,
    ChevronRight,
    CircleDot,
    Eraser,
    GitBranch,
    Hand,
    Image as ImageIcon,
    Link2,
    PackageOpen,
    Palette,
    Play,
    Plus,
    Redo2,
    Save,
    Settings2,
    Square,
    Trash2,
    Type,
    Undo2,
    Upload as UploadIcon,
    Video,
} from "lucide-react";

import { ActiveConnectionPath, ConnectionPath, type CanvasConnectionRoute, type CanvasConnectionVariant } from "@/app/(user)/canvas/components/canvas-connections";
import { CanvasImageSettingsPopover } from "@/app/(user)/canvas/components/canvas-image-settings-popover";
import { Minimap } from "@/app/(user)/canvas/components/canvas-mini-map";
import { CanvasZoomControls } from "@/app/(user)/canvas/components/canvas-zoom-controls";
import { InfiniteCanvas } from "@/app/(user)/canvas/components/infinite-canvas";
import { CanvasVideoSettingsPopover } from "@/app/(user)/canvas/components/canvas-video-settings-popover";
import { ModelPicker } from "@/components/model-picker";
import { PromptSelectDialog } from "@/components/prompts/prompt-select-dialog";
import { CanvasNodeType, type CanvasConnection, type CanvasNodeData, type ConnectionHandle, type Position, type SelectionBox, type ViewportTransform } from "@/app/(user)/canvas/types";
import { canvasThemes, type CanvasBackgroundMode, type CanvasTheme } from "@/lib/canvas-theme";
import { defaultConfig, useConfigStore, useEffectiveConfig, type AiConfig } from "@/stores/use-config-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { fetchAdminAssets } from "@/services/api/admin";
import { fetchPDDWorkflowTemplate, fetchPDDWorkflowThemes, savePDDWorkflowTemplate, startPDDWorkflowTemplateRun, type WorkflowNodeRetry, type WorkflowTemplate, type WorkflowTemplateEdge, type WorkflowTemplateNode, type WorkflowTemplateSpec } from "@/services/api/pdd";
import { fetchLocalPDDWorkflowTemplate, isLocalWorkflowTemplateId, saveLocalPDDWorkflowTemplate, startLocalPDDWorkflowTemplateRun } from "@/services/local-workflow-templates";
import { useAssetStore } from "@/stores/use-asset-store";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";
import { useUserStore } from "@/stores/use-user-store";

type DragState = {
    isDragging: boolean;
    hasMoved: boolean;
    startX: number;
    startY: number;
    initialNodes: { id: string; x: number; y: number }[];
};

type ResizeCorner = "top-left" | "top-right" | "bottom-left" | "bottom-right";
type ResizeState = {
    isResizing: boolean;
    nodeId: string;
    corner: ResizeCorner;
    startX: number;
    startY: number;
    startLeft: number;
    startTop: number;
    startWidth: number;
    startHeight: number;
};

type ClipboardState = {
    nodes: WorkflowTemplateNode[];
    edges: WorkflowTemplateEdge[];
};

type TemplateContextMenu = {
    x: number;
    y: number;
    nodeId: string;
};

type PendingConnectionCreate = {
    connection: ConnectionHandle;
    position: Position;
};

type NodeSummaryLine = {
    label: string;
    value: string;
    multiline?: boolean;
};

type TemplateViewMode = "canvas" | "flow";

type FlowGroup = {
    id: string;
    title: string;
    summary: string;
    childIds: string[];
};

type DisplayWorkflowNode = WorkflowTemplateNode & {
    __virtual?: boolean;
    __group?: FlowGroup;
};

type DisplayWorkflowEdge = WorkflowTemplateEdge & {
    __visualKind?: CanvasConnectionVariant;
    __route?: CanvasConnectionRoute;
};

type WorkflowNodePreset = {
    label: string;
    type: WorkflowTemplateNode["type"];
    operation: WorkflowTemplateNode["operation"];
    title: string;
    icon: ComponentType<{ className?: string }>;
    model?: string;
    prompt?: string;
    count?: number;
    size?: string;
    quality?: string;
    seconds?: string;
    videoQuality?: string;
    width?: number;
    height?: number;
    extra?: Record<string, unknown>;
};

const nodeTypeMeta = {
    material: { label: "素材", icon: PackageOpen, canvasType: CanvasNodeType.Config },
    text: { label: "文字", icon: Type, canvasType: CanvasNodeType.Text },
    image: { label: "图片", icon: ImageIcon, canvasType: CanvasNodeType.Image },
    video: { label: "视频", icon: Video, canvasType: CanvasNodeType.Video },
} as const;

const operationOptions = [
    { label: "输入对象", value: "input" },
    { label: "素材库查找", value: "material_lookup" },
    { label: "静态文字", value: "text_static" },
    { label: "文字生成", value: "text_generation" },
    { label: "条件分支", value: "condition" },
    { label: "脚本执行", value: "script" },
    { label: "图片选择", value: "image_select" },
    { label: "图片生成", value: "image_generation" },
    { label: "图片编辑", value: "image_edit" },
    { label: "视频生成", value: "video_generation" },
];

const operationOptionsByCategory = {
    text: operationOptions.filter((option) => ["text_static", "text_generation"].includes(option.value)),
    image: operationOptions.filter((option) => ["material_lookup", "image_select", "image_generation", "image_edit"].includes(option.value)),
    video: operationOptions.filter((option) => option.value === "video_generation"),
    function: operationOptions.filter((option) => ["input", "condition", "script"].includes(option.value)),
} as const;

const workflowNodePresets: WorkflowNodePreset[] = [
    { label: "文本", title: "文本节点", type: "text", operation: "text_generation", icon: Type, model: "gpt-5.5", extra: { outputFormat: "text" } },
    { label: "图片", title: "图片节点", type: "image", operation: "image_generation", icon: ImageIcon, model: "gpt-image-2", size: "1:1", quality: "auto" },
    { label: "视频", title: "视频节点", type: "video", operation: "video_generation", icon: Video, model: "sora-2", size: "1280x720", seconds: "6", videoQuality: "auto", width: 340 },
    { label: "功能", title: "功能节点", type: "text", operation: "condition", icon: Settings2, extra: { nodeCategory: "function", conditions: '[{"jsonPath":"$.decision","operator":"eq","value":"pass","output":"pass"}]', defaultOutput: "default" } },
];

const minNodeWidth = 220;
const minNodeHeight = 150;

const flowGroups: FlowGroup[] = [
    {
        id: "flow_group_source_quality",
        title: "源图质量关卡",
        summary: "当前源图、质检、判定与修复循环",
        childIds: ["current_source", "source_review", "source_decision", "source_repair"],
    },
    {
        id: "flow_group_main_quality",
        title: "主图质量关卡",
        summary: "当前主图、复检、判定与主图修复循环",
        childIds: ["current_main", "main_review", "main_decision", "main_repair"],
    },
];

export default function PDDWorkflowTemplateEditor({ templateId }: { templateId: string }) {
    const { message } = App.useApp();
    const router = useRouter();
    const token = useUserStore((state) => state.token);
    const localWorkspaceStatus = useLocalWorkspaceStore((state) => state.status);
    const localWorkspace = useLocalWorkspaceStore((state) => state.workspace);
    const localWorkspaceBaseUrl = useLocalWorkspaceStore((state) => state.baseUrl);
    const useLocalTemplates = localWorkspaceStatus === "connected" && Boolean(localWorkspace);
    const colorTheme = useThemeStore((state) => state.theme);
    const theme = canvasThemes[colorTheme];
    const normalizedTemplateId = templateId || "";
    const isNew = normalizedTemplateId === "new";

    const [template, setTemplate] = useState<WorkflowTemplate | null>(null);
    const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set());
    const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null);
    const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
    const [connectionTargetNodeId, setConnectionTargetNodeId] = useState<string | null>(null);
    const [connectingParams, setConnectingParams] = useState<ConnectionHandle | null>(null);
    const [pendingConnectionCreate, setPendingConnectionCreate] = useState<PendingConnectionCreate | null>(null);
    const [mouseWorld, setMouseWorld] = useState<Position>({ x: 0, y: 0 });
    const [selectionBox, setSelectionBox] = useState<SelectionBox | null>(null);
    const [contextMenu, setContextMenu] = useState<TemplateContextMenu | null>(null);
    const [viewport, setViewport] = useState<ViewportTransform>({ x: 160, y: 120, k: 1 });
    const [size, setSize] = useState({ width: 0, height: 0 });
    const [backgroundMode, setBackgroundMode] = useState<CanvasBackgroundMode>("lines");
    const [isMiniMapOpen, setIsMiniMapOpen] = useState(false);
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [clearOpen, setClearOpen] = useState(false);
    const [startOpen, setStartOpen] = useState(false);
    const [inputsText, setInputsText] = useState("");
    const [historyState, setHistoryState] = useState({ canUndo: false, canRedo: false });
    const [viewMode, setViewMode] = useState<TemplateViewMode>("canvas");
    const [advancedMode, setAdvancedMode] = useState(false);
    const [expandedFlowGroups, setExpandedFlowGroups] = useState<Set<string>>(new Set());

    const containerRef = useRef<HTMLDivElement | null>(null);
    const specRef = useRef<WorkflowTemplateSpec>(emptySpec());
    const viewportRef = useRef(viewport);
    const nodesRef = useRef<WorkflowTemplateNode[]>([]);
    const edgesRef = useRef<WorkflowTemplateEdge[]>([]);
    const selectedNodeIdsRef = useRef(selectedNodeIds);
    const selectedEdgeIdRef = useRef(selectedEdgeId);
    const connectingParamsRef = useRef<ConnectionHandle | null>(null);
    const pendingConnectionCreateRef = useRef<PendingConnectionCreate | null>(null);
    const connectionTargetNodeIdRef = useRef<string | null>(null);
    const selectionBoxRef = useRef<SelectionBox | null>(null);
    const dragRef = useRef<DragState>({ isDragging: false, hasMoved: false, startX: 0, startY: 0, initialNodes: [] });
    const resizeRef = useRef<ResizeState | null>(null);
    const interactionHistoryRef = useRef<WorkflowTemplateSpec | null>(null);
    const clipboardRef = useRef<ClipboardState | null>(null);
    const historyRef = useRef<{ past: WorkflowTemplateSpec[]; future: WorkflowTemplateSpec[] }>({ past: [], future: [] });
    const rafRef = useRef<number | null>(null);
    const viewModeInitializedRef = useRef(false);

    const query = useQuery({
        queryKey: ["pdd-workflow-template", useLocalTemplates ? "local" : "server", normalizedTemplateId, useLocalTemplates ? localWorkspaceBaseUrl : token],
        queryFn: () => (useLocalTemplates ? fetchLocalPDDWorkflowTemplate(localWorkspaceBaseUrl, normalizedTemplateId) : fetchPDDWorkflowTemplate(normalizedTemplateId, token || "")),
        enabled: Boolean((useLocalTemplates || token) && normalizedTemplateId && !isNew),
    });

    useEffect(() => {
        viewportRef.current = viewport;
    }, [viewport]);

    useEffect(() => {
        selectedNodeIdsRef.current = selectedNodeIds;
    }, [selectedNodeIds]);

    useEffect(() => {
        selectedEdgeIdRef.current = selectedEdgeId;
    }, [selectedEdgeId]);

    useEffect(() => {
        connectingParamsRef.current = connectingParams;
    }, [connectingParams]);

    useEffect(() => {
        pendingConnectionCreateRef.current = pendingConnectionCreate;
    }, [pendingConnectionCreate]);

    useEffect(() => {
        connectionTargetNodeIdRef.current = connectionTargetNodeId;
    }, [connectionTargetNodeId]);

    useEffect(() => {
        selectionBoxRef.current = selectionBox;
    }, [selectionBox]);

    useEffect(() => {
        if (!query.data) return;
        loadTemplate(query.data);
    }, [query.data]);

    useEffect(() => {
        if (!isNew) return;
        loadTemplate(createEmptyTemplate());
    }, [isNew]);

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
    }, [template]);

    const spec = template?.spec || emptySpec();
    const flowView = useMemo(() => buildFlowView(spec, expandedFlowGroups), [expandedFlowGroups, spec]);
    const displayNodes: DisplayWorkflowNode[] = viewMode === "flow" ? flowView.nodes : spec.nodes;
    const displayEdges: DisplayWorkflowEdge[] = viewMode === "flow" ? flowView.edges : spec.edges;
    const canvasNodeById = useMemo(() => new Map(displayNodes.map((node) => [node.id, toCanvasNode(node)])), [displayNodes]);
    const canvasNodes = useMemo(() => displayNodes.map(toCanvasNode), [displayNodes]);
    const selectedNode = selectedNodeIds.size === 1 ? spec.nodes.find((node) => selectedNodeIds.has(node.id)) || null : null;
    const selectedEdge = selectedEdgeId ? spec.edges.find((edge) => edge.id === selectedEdgeId) || null : null;
    const related = useMemo(() => getRelatedIds(displayEdges, selectedNodeIds, hoveredNodeId), [displayEdges, hoveredNodeId, selectedNodeIds]);

    useEffect(() => {
        specRef.current = spec;
        nodesRef.current = spec.nodes;
        edgesRef.current = spec.edges;
    }, [spec]);

    const saveMutation = useMutation({
        mutationFn: () => {
            if (!template) throw new Error("模板为空");
            return useLocalTemplates ? saveLocalPDDWorkflowTemplate(localWorkspaceBaseUrl, template) : savePDDWorkflowTemplate(template, token || "");
        },
        onSuccess: (saved) => {
            loadTemplate(saved);
            message.success("模板已保存");
            if (isNew) router.replace(`/workflows/ecommerce/templates/${encodeURIComponent(saved.id)}`);
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "保存失败"),
    });

    const startMutation = useMutation({
        mutationFn: () => {
            if (!template) throw new Error("模板为空");
            const payload = {
                inputs: parseInputs(inputsText),
                productConcurrency: template.spec.settings.productConcurrency,
                maxRetries: template.spec.settings.maxRetries,
            };
            if (useLocalTemplates || isLocalWorkflowTemplateId(template.id)) {
                if (!useLocalTemplates) throw new Error("请先连接本地工作区");
                return startLocalPDDWorkflowTemplateRun(localWorkspaceBaseUrl, template.id, payload);
            }
            if (!token) throw new Error("启动 PDD/VPS run 需要管理员登录");
            return startPDDWorkflowTemplateRun(template.id, payload, token || "");
        },
        onSuccess: (result) => {
            message.success(result.runId.startsWith("run_") ? `已创建本地 run ${result.runId}` : `已启动 ${result.runId}`);
            setStartOpen(false);
            router.push(`/workflows/ecommerce/${encodeURIComponent(result.runId)}`);
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "启动失败"),
    });

    const loadTemplate = (next: WorkflowTemplate) => {
        const normalized = { ...next, spec: normalizeSpec(next.spec) };
        specRef.current = normalized.spec;
        nodesRef.current = normalized.spec.nodes;
        edgesRef.current = normalized.spec.edges;
        historyRef.current = { past: [], future: [] };
        setHistoryState({ canUndo: false, canRedo: false });
        setSelectedNodeIds(new Set());
        setSelectedEdgeId(null);
        setConnectingParams(null);
        setConnectionTargetNodeId(null);
        setSelectionBox(null);
        setContextMenu(null);
        if (!viewModeInitializedRef.current) {
            setViewMode(normalized.spec.nodes.length >= 10 ? "flow" : "canvas");
            viewModeInitializedRef.current = true;
        }
        setTemplate(normalized);
    };

    const setTemplatePatch = (patch: Partial<WorkflowTemplate>) => {
        setTemplate((current) => (current ? { ...current, ...patch } : current));
    };

    const setSpec = (nextSpec: WorkflowTemplateSpec) => {
        const normalized = normalizeSpec(nextSpec);
        specRef.current = normalized;
        nodesRef.current = normalized.nodes;
        edgesRef.current = normalized.edges;
        setTemplate((current) => (current ? { ...current, spec: normalized } : current));
    };

    const setTemplateViewMode = (mode: TemplateViewMode) => {
        viewModeInitializedRef.current = true;
        setViewMode(mode);
        setSelectedEdgeId(null);
        setContextMenu(null);
        setConnectingParams(null);
        setConnectionTargetNodeId(null);
        setSelectionBox(null);
    };

    const toggleFlowGroup = (groupId: string) => {
        setExpandedFlowGroups((current) => {
            const next = new Set(current);
            if (next.has(groupId)) next.delete(groupId);
            else next.add(groupId);
            return next;
        });
        setSelectedNodeIds(new Set());
        setSelectedEdgeId(null);
    };

    const autoArrangeFlowLayout = () => {
        const layout = buildFlowView(specRef.current, new Set(flowGroups.map((group) => group.id)));
        const positionById = new Map(layout.nodes.filter((node) => !node.__virtual).map((node) => [node.id, node]));
        mutateSpec((current) => ({
            ...current,
            nodes: current.nodes.map((node) => {
                const positioned = positionById.get(node.id);
                return positioned ? { ...node, x: positioned.x, y: positioned.y, width: positioned.width, height: positioned.height } : node;
            }),
        }));
        message.success("已按流程图整理布局");
    };

    const refreshHistoryState = () => setHistoryState({ canUndo: historyRef.current.past.length > 0, canRedo: historyRef.current.future.length > 0 });

    const pushHistory = (previous: WorkflowTemplateSpec) => {
        historyRef.current.past.push(cloneSpec(previous));
        if (historyRef.current.past.length > 80) historyRef.current.past.shift();
        historyRef.current.future = [];
        refreshHistoryState();
    };

    const mutateSpec = (updater: (current: WorkflowTemplateSpec) => WorkflowTemplateSpec, recordHistory = true) => {
        const current = specRef.current;
        const next = normalizeSpec(updater(current));
        if (recordHistory) pushHistory(current);
        setSpec(next);
    };

    const beginCanvasInteraction = () => {
        interactionHistoryRef.current = cloneSpec(specRef.current);
    };

    const finishCanvasInteraction = (changed: boolean) => {
        const previous = interactionHistoryRef.current;
        interactionHistoryRef.current = null;
        if (changed && previous) pushHistory(previous);
    };

    const updateNode = (id: string, patch: Partial<WorkflowTemplateNode>, recordHistory = true) => {
        mutateSpec(
            (current) => ({
                ...current,
                nodes: current.nodes.map((node) => (node.id === id ? { ...node, ...patch } : node)),
            }),
            recordHistory,
        );
    };

    const createNodeFromPreset = (preset: WorkflowNodePreset, position?: Position) => {
        const center = getCanvasCenter();
        const existingNodes = specRef.current.nodes;
        const selectedAnchor = existingNodes.find((node) => selectedNodeIdsRef.current.has(node.id)) || existingNodes[existingNodes.length - 1];
        const type = preset.type;
        const id = `${type}_${Date.now().toString(36)}`;
        const x = position ? position.x : selectedAnchor ? selectedAnchor.x + selectedAnchor.width + 120 : center.x - 150;
        const y = position ? position.y : selectedAnchor ? selectedAnchor.y : center.y - 95;
        return {
            id,
            type,
            title: preset.title,
            x,
            y,
            width: preset.width || (type === "video" ? 340 : 300),
            height: preset.height || 190,
            operation: preset.operation,
            model: preset.model || "",
            prompt: preset.prompt || "",
            count: preset.count || 1,
            size: preset.size || (type === "image" ? "1:1" : type === "video" ? "1280x720" : ""),
            quality: preset.quality || (type === "image" ? "auto" : ""),
            seconds: preset.seconds || (type === "video" ? "6" : ""),
            videoQuality: preset.videoQuality || (type === "video" ? "auto" : ""),
            retry: defaultWorkflowNodeRetry(preset.operation),
            outputMappings: [],
            extra: preset.extra ? { ...preset.extra } : undefined,
        };
    };

    const addNode = (preset: WorkflowNodePreset) => {
        const node = createNodeFromPreset(preset);
        mutateSpec((current) => ({ ...current, nodes: [...current.nodes, node] }));
        setSelectedNodeIds(new Set([node.id]));
        setSelectedEdgeId(null);
    };

    const addConnectedNode = (preset: WorkflowNodePreset, pending: PendingConnectionCreate) => {
        const node = createNodeFromPreset(preset, pending.position);
        const from = pending.connection.handleType === "source" ? pending.connection.nodeId : node.id;
        const to = pending.connection.handleType === "source" ? node.id : pending.connection.nodeId;
        mutateSpec((current) => ({
            ...current,
            nodes: [...current.nodes, node],
            edges: from && to && from !== to && !current.edges.some((edge) => edge.from === from && edge.to === to) ? [...current.edges, { id: `${from}-${to}-${Date.now().toString(36)}`, from, to }] : current.edges,
        }));
        setSelectedNodeIds(new Set([node.id]));
        setSelectedEdgeId(null);
        setPendingConnectionCreate(null);
        setConnectingParams(null);
        setConnectionTargetNodeId(null);
    };

    const addEdge = (from: string, to: string, recordHistory = true) => {
        if (!from || !to || from === to || specRef.current.edges.some((edge) => edge.from === from && edge.to === to)) return;
        mutateSpec((current) => ({ ...current, edges: [...current.edges, { id: `${from}-${to}-${Date.now().toString(36)}`, from, to }] }), recordHistory);
    };

    const updateEdge = (id: string, patch: Partial<WorkflowTemplateEdge>, recordHistory = true) => {
        mutateSpec(
            (current) => ({
                ...current,
                edges: current.edges.map((edge) => (edge.id === id ? { ...edge, ...patch } : edge)),
            }),
            recordHistory,
        );
    };

    const deleteEdges = (ids: Set<string>) => {
        if (!ids.size) return;
        mutateSpec((current) => ({ ...current, edges: current.edges.filter((edge) => !ids.has(edge.id)) }));
        setSelectedEdgeId(null);
    };

    const deleteNodes = (ids: Set<string>) => {
        if (!ids.size) return;
        mutateSpec((current) => ({ ...current, nodes: current.nodes.filter((node) => !ids.has(node.id)), edges: current.edges.filter((edge) => !ids.has(edge.from) && !ids.has(edge.to)) }));
        setSelectedNodeIds(new Set());
        setSelectedEdgeId(null);
        setContextMenu(null);
        setPendingConnectionCreate(null);
    };

    const duplicateNode = (nodeId: string) => {
        const source = nodesRef.current.find((node) => node.id === nodeId);
        if (!source) return;
        const id = `${source.type}_${Date.now().toString(36)}`;
        const next = { ...cloneNode(source), id, title: source.title.endsWith("副本") ? source.title : `${source.title} 副本`, x: source.x + 36, y: source.y + 36 };
        mutateSpec((current) => ({ ...current, nodes: [...current.nodes, next] }));
        setSelectedNodeIds(new Set([id]));
        setSelectedEdgeId(null);
    };

    const copySelectedNodes = () => {
        const selectedIds = selectedNodeIdsRef.current;
        if (!selectedIds.size) return;
        const copiedNodes = nodesRef.current.filter((node) => selectedIds.has(node.id)).map(cloneNode);
        clipboardRef.current = {
            nodes: copiedNodes,
            edges: edgesRef.current.filter((edge) => selectedIds.has(edge.from) && selectedIds.has(edge.to)).map(cloneEdge),
        };
        message.success("已复制节点");
    };

    const pasteCopiedNodes = () => {
        const clipboard = clipboardRef.current;
        if (!clipboard?.nodes.length) return false;
        const center = getCanvasCenter();
        const bounds = getNodeBounds(clipboard.nodes);
        const dx = center.x - (bounds.left + bounds.right) / 2;
        const dy = center.y - (bounds.top + bounds.bottom) / 2;
        const idMap = new Map<string, string>();
        const nextNodes = clipboard.nodes.map((node, index) => {
            const id = `${node.type}_${Date.now().toString(36)}_${index}`;
            idMap.set(node.id, id);
            return { ...cloneNode(node), id, title: node.title.endsWith("副本") ? node.title : `${node.title} 副本`, x: node.x + dx, y: node.y + dy };
        });
        const nextEdges = clipboard.edges.flatMap((edge, index) => {
            const from = idMap.get(edge.from);
            const to = idMap.get(edge.to);
            if (!from || !to) return [];
            return [{ ...cloneEdge(edge), id: `${from}-${to}-${Date.now().toString(36)}-${index}`, from, to }];
        });
        mutateSpec((current) => ({ ...current, nodes: [...current.nodes, ...nextNodes], edges: [...current.edges, ...nextEdges] }));
        setSelectedNodeIds(new Set(nextNodes.map((node) => node.id)));
        setSelectedEdgeId(null);
        message.success("已粘贴节点");
        return true;
    };

    const undoCanvas = () => {
        const previous = historyRef.current.past.pop();
        if (!previous) return;
        historyRef.current.future.push(cloneSpec(specRef.current));
        setSpec(previous);
        setSelectedNodeIds(new Set());
        setSelectedEdgeId(null);
        setContextMenu(null);
        setPendingConnectionCreate(null);
        refreshHistoryState();
    };

    const redoCanvas = () => {
        const next = historyRef.current.future.pop();
        if (!next) return;
        historyRef.current.past.push(cloneSpec(specRef.current));
        setSpec(next);
        setSelectedNodeIds(new Set());
        setSelectedEdgeId(null);
        setContextMenu(null);
        setPendingConnectionCreate(null);
        refreshHistoryState();
    };

    const clearCanvas = () => {
        mutateSpec((current) => ({ ...current, nodes: [], edges: [] }));
        setSelectedNodeIds(new Set());
        setSelectedEdgeId(null);
        setPendingConnectionCreate(null);
        setClearOpen(false);
    };

    const resetViewport = () => {
        setViewport({ x: Math.max(80, size.width / 2 - 520), y: Math.max(80, size.height / 2 - 260), k: 1 });
        setContextMenu(null);
        setPendingConnectionCreate(null);
    };

    const setZoomScale = (scale: number) => {
        const nextScale = Math.min(Math.max(scale, 0.05), 5);
        setViewport((current) => ({
            x: size.width / 2 - ((size.width / 2 - current.x) / current.k) * nextScale,
            y: size.height / 2 - ((size.height / 2 - current.y) / current.k) * nextScale,
            k: nextScale,
        }));
        setContextMenu(null);
        setPendingConnectionCreate(null);
    };

    const getCanvasCenter = () => {
        const current = viewportRef.current;
        return {
            x: (Math.max(size.width, 1) / 2 - current.x) / current.k,
            y: (Math.max(size.height, 1) / 2 - current.y) / current.k,
        };
    };

    const screenToCanvas = useCallback((clientX: number, clientY: number): Position => {
        const rect = containerRef.current?.getBoundingClientRect();
        const current = viewportRef.current;
        if (!rect) return { x: 0, y: 0 };
        return {
            x: (clientX - rect.left - current.x) / current.k,
            y: (clientY - rect.top - current.y) / current.k,
        };
    }, []);

    const getConnectableNodeAtPoint = useCallback(
        (clientX: number, clientY: number, currentConnection: ConnectionHandle) => {
            const world = screenToCanvas(clientX, clientY);
            const nodes = nodesRef.current;
            const handleHitMargin = 32;
            for (let index = nodes.length - 1; index >= 0; index -= 1) {
                const node = nodes[index];
                if (node.id === currentConnection.nodeId) continue;
                if (world.x >= node.x - handleHitMargin && world.x <= node.x + node.width + handleHitMargin && world.y >= node.y - handleHitMargin && world.y <= node.y + node.height + handleHitMargin) return node.id;
            }
            return null;
        },
        [screenToCanvas],
    );

    const connectNodes = useCallback((connection: ConnectionHandle, targetNodeId: string) => {
        const from = connection.handleType === "source" ? connection.nodeId : targetNodeId;
        const to = connection.handleType === "source" ? targetNodeId : connection.nodeId;
        addEdge(from, to);
    }, []);

    const handleCanvasMouseDown = (event: ReactPointerEvent<HTMLDivElement>) => {
        setContextMenu(null);
        setPendingConnectionCreate(null);
        if (event.button !== 0) return;
        const world = screenToCanvas(event.clientX, event.clientY);
        const nextSelection: SelectionBox = {
            startWorldX: world.x,
            startWorldY: world.y,
            currentWorldX: world.x,
            currentWorldY: world.y,
            additive: event.shiftKey,
            initialSelectedNodeIds: event.shiftKey ? Array.from(selectedNodeIdsRef.current) : [],
        };
        selectionBoxRef.current = nextSelection;
        setSelectionBox(nextSelection);
        if (!event.shiftKey) setSelectedNodeIds(new Set());
        setSelectedEdgeId(null);
    };

    const deselectCanvas = () => {
        setSelectedNodeIds(new Set());
        setSelectedEdgeId(null);
        setContextMenu(null);
        setSelectionBox(null);
        setHoveredNodeId(null);
        setConnectingParams(null);
        setConnectionTargetNodeId(null);
        setPendingConnectionCreate(null);
    };

    const handleNodeMouseDown = (event: ReactMouseEvent, nodeId: string) => {
        event.stopPropagation();
        setContextMenu(null);
        setPendingConnectionCreate(null);
        setSelectedEdgeId(null);
        const currentSelected = selectedNodeIdsRef.current;
        const nextSelected = new Set(currentSelected);
        if (event.shiftKey || event.metaKey || event.ctrlKey) {
            if (nextSelected.has(nodeId)) nextSelected.delete(nodeId);
            else nextSelected.add(nodeId);
        } else if (!nextSelected.has(nodeId)) {
            nextSelected.clear();
            nextSelected.add(nodeId);
        }
        setSelectedNodeIds(nextSelected);
        const dragIds = new Set(nextSelected.size ? nextSelected : [nodeId]);
        dragRef.current = {
            isDragging: true,
            hasMoved: false,
            startX: event.clientX,
            startY: event.clientY,
            initialNodes: nodesRef.current.filter((node) => dragIds.has(node.id)).map((node) => ({ id: node.id, x: node.x, y: node.y })),
        };
        beginCanvasInteraction();
    };

    const handleFlowNodeMouseDown = (event: ReactMouseEvent, nodeId: string) => {
        event.stopPropagation();
        setContextMenu(null);
        setPendingConnectionCreate(null);
        setSelectedEdgeId(null);
        setSelectedNodeIds(new Set([nodeId]));
    };

    const finishNodeDrag = (clientX?: number, clientY?: number) => {
        if (rafRef.current) {
            cancelAnimationFrame(rafRef.current);
            rafRef.current = null;
        }
        if (!dragRef.current.isDragging) return;
        const changed = dragRef.current.hasMoved;
        if (changed && clientX != null && clientY != null) {
            const current = viewportRef.current;
            const dx = (clientX - dragRef.current.startX) / current.k;
            const dy = (clientY - dragRef.current.startY) / current.k;
            const initialNodes = dragRef.current.initialNodes;
            mutateSpec(
                (currentSpec) => ({
                    ...currentSpec,
                    nodes: currentSpec.nodes.map((node) => {
                        const initial = initialNodes.find((item) => item.id === node.id);
                        return initial ? { ...node, x: initial.x + dx, y: initial.y + dy } : node;
                    }),
                }),
                false,
            );
        }
        dragRef.current = { isDragging: false, hasMoved: false, startX: 0, startY: 0, initialNodes: [] };
        finishCanvasInteraction(changed);
    };

    const handleConnectStart = (event: ReactMouseEvent, nodeId: string, handleType: "source" | "target") => {
        event.stopPropagation();
        event.preventDefault();
        setMouseWorld(screenToCanvas(event.clientX, event.clientY));
        setConnectingParams({ nodeId, handleType });
        setConnectionTargetNodeId(null);
        setSelectedEdgeId(null);
        setSelectedNodeIds(new Set([nodeId]));
    };

    const handleResizeStart = (event: ReactMouseEvent, node: WorkflowTemplateNode, corner: ResizeCorner) => {
        event.stopPropagation();
        event.preventDefault();
        resizeRef.current = {
            isResizing: true,
            nodeId: node.id,
            corner,
            startX: event.clientX,
            startY: event.clientY,
            startLeft: node.x,
            startTop: node.y,
            startWidth: node.width,
            startHeight: node.height,
        };
        setSelectedNodeIds(new Set([node.id]));
        setSelectedEdgeId(null);
        beginCanvasInteraction();
    };

    useEffect(() => {
        const handleGlobalMouseMove = (event: MouseEvent) => {
            const resize = resizeRef.current;
            if (resize?.isResizing) {
                const current = viewportRef.current;
                const dx = (event.clientX - resize.startX) / current.k;
                const dy = (event.clientY - resize.startY) / current.k;
                const fromLeft = resize.corner.includes("left");
                const fromTop = resize.corner.includes("top");
                const width = Math.max(minNodeWidth, resize.startWidth + (fromLeft ? -dx : dx));
                const height = Math.max(minNodeHeight, resize.startHeight + (fromTop ? -dy : dy));
                const x = fromLeft ? resize.startLeft + resize.startWidth - width : resize.startLeft;
                const y = fromTop ? resize.startTop + resize.startHeight - height : resize.startTop;
                mutateSpec((currentSpec) => ({ ...currentSpec, nodes: currentSpec.nodes.map((node) => (node.id === resize.nodeId ? { ...node, x, y, width, height } : node)) }), false);
                return;
            }

            if (dragRef.current.isDragging) {
                const current = viewportRef.current;
                const dx = (event.clientX - dragRef.current.startX) / current.k;
                const dy = (event.clientY - dragRef.current.startY) / current.k;
                if (Math.abs(event.clientX - dragRef.current.startX) > 3 || Math.abs(event.clientY - dragRef.current.startY) > 3) dragRef.current.hasMoved = true;
                const initialNodes = dragRef.current.initialNodes;
                if (rafRef.current) cancelAnimationFrame(rafRef.current);
                rafRef.current = requestAnimationFrame(() => {
                    mutateSpec(
                        (currentSpec) => ({
                            ...currentSpec,
                            nodes: currentSpec.nodes.map((node) => {
                                const initial = initialNodes.find((item) => item.id === node.id);
                                return initial ? { ...node, x: initial.x + dx, y: initial.y + dy } : node;
                            }),
                        }),
                        false,
                    );
                    rafRef.current = null;
                });
                return;
            }

            if (connectingParamsRef.current) {
                const targetNodeId = getConnectableNodeAtPoint(event.clientX, event.clientY, connectingParamsRef.current);
                connectionTargetNodeIdRef.current = targetNodeId;
                setConnectionTargetNodeId(targetNodeId);
                setMouseWorld(screenToCanvas(event.clientX, event.clientY));
            }
        };

        const handleGlobalPointerMove = (event: PointerEvent) => {
            const currentSelection = selectionBoxRef.current;
            if (!currentSelection) return;
            if (event.buttons === 0) {
                selectionBoxRef.current = null;
                setSelectionBox(null);
                return;
            }
            const world = screenToCanvas(event.clientX, event.clientY);
            const rectX = Math.min(currentSelection.startWorldX, world.x);
            const rectY = Math.min(currentSelection.startWorldY, world.y);
            const rectW = Math.abs(world.x - currentSelection.startWorldX);
            const rectH = Math.abs(world.y - currentSelection.startWorldY);
            const nextSelected = new Set<string>(currentSelection.additive ? currentSelection.initialSelectedNodeIds : []);
            nodesRef.current.forEach((node) => {
                const intersects = rectX < node.x + node.width && rectX + rectW > node.x && rectY < node.y + node.height && rectY + rectH > node.y;
                if (intersects) nextSelected.add(node.id);
            });
            const nextSelection = { ...currentSelection, currentWorldX: world.x, currentWorldY: world.y };
            selectionBoxRef.current = nextSelection;
            setSelectionBox(nextSelection);
            setSelectedNodeIds(nextSelected);
            setSelectedEdgeId(null);
        };

        const handleGlobalMouseUp = (event: MouseEvent) => {
            const resize = resizeRef.current;
            if (resize?.isResizing) {
                resizeRef.current = null;
                finishCanvasInteraction(true);
            }
            finishNodeDrag(event.clientX, event.clientY);
            selectionBoxRef.current = null;
            setSelectionBox(null);
            if (pendingConnectionCreateRef.current) return;
            const currentConnection = connectingParamsRef.current;
            if (currentConnection) {
                const targetNodeId = getConnectableNodeAtPoint(event.clientX, event.clientY, currentConnection) || connectionTargetNodeIdRef.current;
                if (targetNodeId) {
                    connectNodes(currentConnection, targetNodeId);
                } else {
                    setMouseWorld(screenToCanvas(event.clientX, event.clientY));
                    setPendingConnectionCreate({ connection: currentConnection, position: screenToCanvas(event.clientX, event.clientY) });
                }
                setConnectingParams(null);
                setConnectionTargetNodeId(null);
            }
        };

        const cancelInteractions = () => {
            finishNodeDrag();
            resizeRef.current = null;
            setConnectingParams(null);
            setConnectionTargetNodeId(null);
            setPendingConnectionCreate(null);
            selectionBoxRef.current = null;
            setSelectionBox(null);
        };

        window.addEventListener("mousemove", handleGlobalMouseMove);
        window.addEventListener("mouseup", handleGlobalMouseUp);
        window.addEventListener("pointermove", handleGlobalPointerMove);
        window.addEventListener("blur", cancelInteractions);
        return () => {
            window.removeEventListener("mousemove", handleGlobalMouseMove);
            window.removeEventListener("mouseup", handleGlobalMouseUp);
            window.removeEventListener("pointermove", handleGlobalPointerMove);
            window.removeEventListener("blur", cancelInteractions);
        };
    }, [connectNodes, getConnectableNodeAtPoint, screenToCanvas]);

    useEffect(() => {
        const handleKeyDown = (event: KeyboardEvent) => {
            if (isEditableKeyboardTarget(event.target)) return;
            const key = event.key.toLowerCase();
            const isModifierShortcut = event.metaKey || event.ctrlKey;
            if (isModifierShortcut && !event.altKey && key === "z") {
                event.preventDefault();
                if (event.shiftKey) redoCanvas();
                else undoCanvas();
                return;
            }
            if (isModifierShortcut && !event.altKey && key === "y") {
                event.preventDefault();
                redoCanvas();
                return;
            }
            if (isModifierShortcut && !event.altKey && key === "a") {
                event.preventDefault();
                setSelectedNodeIds(new Set(nodesRef.current.map((node) => node.id)));
                setSelectedEdgeId(null);
                setContextMenu(null);
                setSelectionBox(null);
                return;
            }
            if (isModifierShortcut && !event.altKey && key === "c") {
                event.preventDefault();
                copySelectedNodes();
                return;
            }
            if (isModifierShortcut && !event.altKey && key === "v") {
                event.preventDefault();
                pasteCopiedNodes();
                return;
            }
            if (event.key === "Delete" || event.key === "Backspace") {
                if (selectedNodeIdsRef.current.size) {
                    event.preventDefault();
                    deleteNodes(new Set(selectedNodeIdsRef.current));
                } else if (selectedEdgeIdRef.current) {
                    event.preventDefault();
                    deleteEdges(new Set([selectedEdgeIdRef.current]));
                }
                return;
            }
            if (event.key === "Escape") {
                deselectCanvas();
                setSettingsOpen(false);
                setClearOpen(false);
            }
        };
        window.addEventListener("keydown", handleKeyDown);
        return () => window.removeEventListener("keydown", handleKeyDown);
    });

    if (!useLocalTemplates && !token) {
        return (
            <main className="flex h-full items-center justify-center bg-background px-6 text-foreground">
                <Card className="w-full max-w-md">
                    <Typography.Title level={3}>需要登录</Typography.Title>
                    <Typography.Paragraph type="secondary">连接本地工作区后可编辑私有模板；未连接时需要管理员登录访问服务器模板。</Typography.Paragraph>
                    <Button type="primary" href="/login">
                        去登录
                    </Button>
                </Card>
            </main>
        );
    }

    if (!template) {
        const errorText = query.error instanceof Error ? query.error.message : "";
        return (
            <main className="grid h-full place-items-center bg-background px-6 text-foreground">
                <Card className="w-full max-w-md">
                    <Typography.Title level={3}>{query.isError ? "模板加载失败" : "加载中..."}</Typography.Title>
                    {query.isError ? (
                        <Space direction="vertical">
                            <Typography.Paragraph type="secondary">{errorText || "请刷新后重试"}</Typography.Paragraph>
                            <Space>
                                <Button onClick={() => void query.refetch()}>重试</Button>
                                <Button href="/workflows/ecommerce/templates">返回模板列表</Button>
                            </Space>
                        </Space>
                    ) : null}
                </Card>
            </main>
        );
    }

    return (
        <main className="flex h-full min-h-0" style={{ background: theme.canvas.background, color: theme.node.text }}>
            <section className="relative flex min-w-0 flex-1 flex-col">
                <TemplateTopBar
                    template={template}
                    spec={spec}
                    isNew={isNew}
                    saving={saveMutation.isPending}
                    onBack={() => router.push("/workflows/ecommerce/templates")}
                    onTitleChange={(title) => setTemplatePatch({ title })}
                    onOpenSettings={() => setSettingsOpen(true)}
                    viewMode={viewMode}
                    onViewModeChange={setTemplateViewMode}
                    advancedMode={advancedMode}
                    onAdvancedModeChange={setAdvancedMode}
                    onAutoArrange={autoArrangeFlowLayout}
                    onSave={() => saveMutation.mutate()}
                    runDisabled={isNew || saveMutation.isPending || (!useLocalTemplates && isLocalWorkflowTemplateId(template.id))}
                    onRun={() => setStartOpen(true)}
                />
                <div className="relative min-h-0 flex-1">
                    <InfiniteCanvas
                        containerRef={containerRef}
                        viewport={viewport}
                        backgroundMode={backgroundMode}
                        onViewportChange={(next) => {
                            setViewport(next);
                            setContextMenu(null);
                            setPendingConnectionCreate(null);
                        }}
                        onCanvasMouseDown={viewMode === "canvas" ? handleCanvasMouseDown : undefined}
                        onCanvasDeselect={deselectCanvas}
                        onContextMenu={(event) => event.preventDefault()}
                    >
                        <svg className="absolute left-0 top-0 h-[10000px] w-[10000px] overflow-visible" style={{ pointerEvents: "none", transform: "translateZ(0)", zIndex: 6 }}>
                            {displayEdges.map((edge) => {
                                const from = canvasNodeById.get(edge.from);
                                const to = canvasNodeById.get(edge.to);
                                if (!from || !to) return null;
                                const connection: CanvasConnection = { id: edge.id, fromNodeId: edge.from, toNodeId: edge.to };
                                return (
                                    <ConnectionPath
                                        key={edge.id}
                                        connection={connection}
                                        from={from}
                                        to={to}
                                        active={selectedEdgeId === edge.id || related.edgeIds.has(edge.id)}
                                        variant={viewMode === "flow" ? edge.__visualKind : "default"}
                                        shape={viewMode === "flow" ? "orthogonal" : "curve"}
                                        route={viewMode === "flow" ? edge.__route : undefined}
                                        onSelect={() => {
                                            setSelectedEdgeId(edge.id);
                                            setSelectedNodeIds(new Set());
                                            setContextMenu(null);
                                        }}
                                    />
                                );
                            })}
                            {viewMode === "canvas" && connectingParams ? <ActiveConnectionPath node={canvasNodeById.get(connectingParams.nodeId)} handle={connectingParams} mouseWorld={mouseWorld} /> : null}
                        </svg>

                        {displayNodes.map((node) =>
                            node.__virtual && node.__group ? (
                                <WorkflowGroupCard key={node.id} node={node} expanded={expandedFlowGroups.has(node.__group.id)} onToggle={() => toggleFlowGroup(node.__group!.id)} />
                            ) : (
                                <WorkflowNodeCard
                                    key={node.id}
                                    node={node}
                                    selected={selectedNodeIds.has(node.id)}
                                    related={related.nodeIds.has(node.id)}
                                    readOnlyLayout={viewMode === "flow"}
                                    isConnectionTarget={viewMode === "canvas" && connectionTargetNodeId === node.id}
                                    isConnecting={viewMode === "canvas" && Boolean(connectingParams)}
                                    hovered={hoveredNodeId === node.id}
                                    onMouseDown={viewMode === "flow" ? handleFlowNodeMouseDown : handleNodeMouseDown}
                                    onHoverStart={setHoveredNodeId}
                                    onHoverEnd={(id) => setHoveredNodeId((current) => (current === id ? null : current))}
                                    onConnectStart={handleConnectStart}
                                    onResizeStart={handleResizeStart}
                                    onOpenConfig={(id) => {
                                        setSelectedNodeIds(new Set([id]));
                                        setSelectedEdgeId(null);
                                    }}
                                    onDelete={(id) => deleteNodes(new Set([id]))}
                                    onContextMenu={(event, id) => {
                                        if (viewMode !== "canvas") return;
                                        event.preventDefault();
                                        event.stopPropagation();
                                        setContextMenu({ x: event.clientX, y: event.clientY, nodeId: id });
                                        setSelectedNodeIds(new Set([id]));
                                        setSelectedEdgeId(null);
                                    }}
                                />
                            ),
                        )}

                        {selectionBox ? (
                            <div
                                className="pointer-events-none absolute z-[100] border"
                                style={{
                                    left: Math.min(selectionBox.startWorldX, selectionBox.currentWorldX),
                                    top: Math.min(selectionBox.startWorldY, selectionBox.currentWorldY),
                                    width: Math.abs(selectionBox.currentWorldX - selectionBox.startWorldX),
                                    height: Math.abs(selectionBox.currentWorldY - selectionBox.startWorldY),
                                    borderColor: theme.canvas.selectionStroke,
                                    background: theme.canvas.selectionFill,
                                }}
                            />
                        ) : null}
                        {pendingConnectionCreate ? <TemplateConnectionCreateMenu pending={pendingConnectionCreate} onCreate={(preset) => addConnectedNode(preset, pendingConnectionCreate)} onClose={() => setPendingConnectionCreate(null)} /> : null}
                    </InfiniteCanvas>

                    <TemplateCanvasToolbar
                        selectedCount={selectedNodeIds.size + (selectedEdgeId ? 1 : 0)}
                        canUndo={historyState.canUndo}
                        canRedo={historyState.canRedo}
                        backgroundMode={backgroundMode}
                        onDeselect={deselectCanvas}
                        onUndo={undoCanvas}
                        onRedo={redoCanvas}
                        onAddNode={addNode}
                        onDelete={() => {
                            if (selectedNodeIds.size) deleteNodes(new Set(selectedNodeIds));
                            else if (selectedEdgeId) deleteEdges(new Set([selectedEdgeId]));
                        }}
                        onClear={() => setClearOpen(true)}
                        onOpenSettings={() => setSettingsOpen(true)}
                        onBackgroundModeChange={setBackgroundMode}
                    />

                    {isMiniMapOpen ? <Minimap nodes={canvasNodes} viewport={viewport} viewportSize={size} onViewportChange={setViewport} /> : null}
                    <CanvasZoomControls scale={viewport.k} onScaleChange={setZoomScale} onReset={resetViewport} isMiniMapOpen={isMiniMapOpen} onToggleMiniMap={() => setIsMiniMapOpen((value) => !value)} />
                    {contextMenu ? <TemplateNodeContextMenu menu={contextMenu} onClose={() => setContextMenu(null)} onDuplicate={() => duplicateNode(contextMenu.nodeId)} onDelete={() => deleteNodes(new Set([contextMenu.nodeId]))} /> : null}
                </div>
            </section>

            {selectedNode ? (
                <aside className="w-[420px] shrink-0 overflow-y-auto border-l p-4" style={{ background: theme.node.panel, borderColor: theme.toolbar.border }}>
                    <NodeEditor
                        node={selectedNode}
                        nodes={spec.nodes}
                        edges={spec.edges}
                        advancedMode={advancedMode}
                        useLocalAssets={useLocalTemplates}
                        onChange={(patch) => updateNode(selectedNode.id, patch)}
                        onAddEdge={(from) => addEdge(from, selectedNode.id)}
                        onUpdateEdge={(id, patch) => updateEdge(id, patch)}
                        onDeleteEdge={(id) => deleteEdges(new Set([id]))}
                        onDelete={() => deleteNodes(new Set([selectedNode.id]))}
                    />
                </aside>
            ) : selectedNodeIds.size > 1 ? (
                <aside className="w-[320px] shrink-0 overflow-y-auto border-l p-4" style={{ background: theme.node.panel, borderColor: theme.toolbar.border }}>
                    <Card size="small" title="已选择多个节点">
                        <Space direction="vertical" className="w-full">
                            <div className="text-sm opacity-70">共选择 {selectedNodeIds.size} 个节点。</div>
                            <Button block icon={<Trash2 className="size-4" />} danger onClick={() => deleteNodes(new Set(selectedNodeIds))}>
                                删除选中节点
                            </Button>
                        </Space>
                    </Card>
                </aside>
            ) : selectedEdge ? (
                <aside className="w-[420px] shrink-0 overflow-y-auto border-l p-4" style={{ background: theme.node.panel, borderColor: theme.toolbar.border }}>
                    <EdgeEditor edge={selectedEdge} nodes={spec.nodes} onChange={(patch) => updateEdge(selectedEdge.id, patch)} onDelete={() => deleteEdges(new Set([selectedEdge.id]))} />
                </aside>
            ) : null}

            <TemplateSettingsModal open={settingsOpen} template={template} spec={spec} onCancel={() => setSettingsOpen(false)} onTemplateChange={setTemplatePatch} onSpecChange={(patch) => mutateSpec((current) => ({ ...current, ...patch }))} />
            <Modal
                title="清空模板画布？"
                open={clearOpen}
                centered
                onCancel={() => setClearOpen(false)}
                footer={
                    <Space>
                        <Button onClick={() => setClearOpen(false)}>取消</Button>
                        <Button danger type="primary" onClick={clearCanvas}>
                            清空
                        </Button>
                    </Space>
                }
            >
                <p className="text-sm opacity-70">这会删除当前模板中的所有节点和连线。</p>
            </Modal>
            <StartTemplateRunModal open={startOpen} loading={startMutation.isPending} value={inputsText} onChange={setInputsText} onImport={setInputsText} onCancel={() => setStartOpen(false)} onStart={() => startMutation.mutate()} />
        </main>
    );
}

function TemplateTopBar({
    template,
    spec,
    isNew,
    saving,
    viewMode,
    advancedMode,
    onBack,
    onTitleChange,
    onOpenSettings,
    onViewModeChange,
    onAdvancedModeChange,
    onAutoArrange,
    onSave,
    runDisabled,
    onRun,
}: {
    template: WorkflowTemplate;
    spec: WorkflowTemplateSpec;
    isNew: boolean;
    saving: boolean;
    viewMode: TemplateViewMode;
    advancedMode: boolean;
    onBack: () => void;
    onTitleChange: (title: string) => void;
    onOpenSettings: () => void;
    onViewModeChange: (mode: TemplateViewMode) => void;
    onAdvancedModeChange: (value: boolean) => void;
    onAutoArrange: () => void;
    onSave: () => void;
    runDisabled: boolean;
    onRun: () => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <header className="flex h-16 shrink-0 items-center justify-between border-b px-4" style={{ background: theme.toolbar.panel, borderColor: theme.toolbar.border }}>
            <Space>
                <Button icon={<ArrowLeft className="size-4" />} onClick={onBack}>
                    模板列表
                </Button>
                <Input value={template.title} onChange={(event) => onTitleChange(event.target.value)} className="w-72" />
                <Tag color="blue">{spec.nodes.length} 节点</Tag>
                <Tag color="geekblue">{spec.edges.length} 连线</Tag>
            </Space>
            <Space>
                <Button icon={<GitBranch className="size-4" />} onClick={() => onViewModeChange(viewMode === "flow" ? "canvas" : "flow")}>
                    {viewMode === "flow" ? "流程图视图" : "自由画布"}
                </Button>
                <Button disabled={viewMode !== "flow"} onClick={onAutoArrange}>
                    整理布局
                </Button>
                <Button type={advancedMode ? "primary" : "default"} onClick={() => onAdvancedModeChange(!advancedMode)}>
                    {advancedMode ? "高级模式" : "简单模式"}
                </Button>
                <Button icon={<Settings2 className="size-4" />} onClick={onOpenSettings}>
                    模板设置
                </Button>
                <Button icon={<Save className="size-4" />} loading={saving} onClick={onSave}>
                    保存
                </Button>
                <Button type="primary" icon={<Play className="size-4" />} disabled={runDisabled} onClick={onRun}>
                    运行模板
                </Button>
            </Space>
        </header>
    );
}

function TemplateConnectionCreateMenu({ pending, onCreate, onClose }: { pending: PendingConnectionCreate; onCreate: (preset: WorkflowNodePreset) => void; onClose: () => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div
            className="absolute z-[120] w-[300px] rounded-[18px] border p-3 shadow-2xl backdrop-blur"
            data-connection-create-menu
            style={{ left: pending.position.x, top: pending.position.y, background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}
            onMouseDown={(event) => event.stopPropagation()}
            onPointerDown={(event) => event.stopPropagation()}
        >
            <div className="mb-2 flex items-center justify-between px-1">
                <span className="text-sm font-medium" style={{ color: theme.node.muted }}>
                    引用该节点生成
                </span>
                <button type="button" className="grid size-7 place-items-center rounded-lg text-base opacity-55 transition hover:bg-white/10 hover:opacity-100" onClick={onClose} aria-label="关闭">
                    ×
                </button>
            </div>
            <div className="grid gap-1">
                {workflowNodePresets.map((preset) => {
                    const Icon = preset.icon;
                    return <ConnectionCreateOption key={preset.label} theme={theme} icon={<Icon className="size-5" />} title={preset.label} description={connectionCreateDescription(preset)} onClick={() => onCreate(preset)} />;
                })}
            </div>
        </div>
    );
}

function ConnectionCreateOption({ theme, icon, title, description, onClick }: { theme: CanvasTheme; icon: ReactNode; title: string; description?: string; onClick?: () => void }) {
    return (
        <button type="button" className="flex h-16 w-full cursor-pointer items-center gap-3 rounded-2xl px-3 text-left transition" style={{ color: theme.node.text }} onClick={onClick} onMouseEnter={(event) => (event.currentTarget.style.background = theme.node.fill)} onMouseLeave={(event) => (event.currentTarget.style.background = "transparent")}>
            <span className="grid size-11 shrink-0 place-items-center rounded-xl" style={{ background: theme.node.fill, color: theme.node.muted }}>
                {icon}
            </span>
            <span className="min-w-0 flex-1">
                <span className="flex items-center gap-2 text-base font-semibold leading-5">{title}</span>
                {description ? (
                    <span className="mt-1 block truncate text-sm" style={{ color: theme.node.muted }}>
                        {description}
                    </span>
                ) : null}
            </span>
        </button>
    );
}

function TemplateCanvasToolbar({
    selectedCount,
    canUndo,
    canRedo,
    backgroundMode,
    onDeselect,
    onUndo,
    onRedo,
    onAddNode,
    onDelete,
    onClear,
    onOpenSettings,
    onBackgroundModeChange,
}: {
    selectedCount: number;
    canUndo: boolean;
    canRedo: boolean;
    backgroundMode: CanvasBackgroundMode;
    onDeselect: () => void;
    onUndo: () => void;
    onRedo: () => void;
    onAddNode: (preset: WorkflowNodePreset) => void;
    onDelete: () => void;
    onClear: () => void;
    onOpenSettings: () => void;
    onBackgroundModeChange: (mode: CanvasBackgroundMode) => void;
}) {
    const [appearanceOpen, setAppearanceOpen] = useState(false);
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const dockStyle = { background: theme.toolbar.panel, borderColor: theme.toolbar.border, color: theme.toolbar.item, boxShadow: "0 18px 45px rgba(0,0,0,.26)" };
    return (
        <div className="pointer-events-none fixed bottom-5 left-1/2 z-50 flex max-w-[calc(100vw-2rem)] -translate-x-1/2 justify-center">
            <div className="thin-scrollbar pointer-events-auto flex h-14 max-w-full items-center gap-1 overflow-x-auto rounded-xl border px-2 shadow-lg backdrop-blur [&>*]:shrink-0" style={dockStyle} data-canvas-no-zoom>
                <ToolbarButton label="移动/选择" active={!selectedCount} onClick={onDeselect}>
                    <Hand className="size-4.5" />
                </ToolbarButton>
                <ToolbarButton label="撤销" disabled={!canUndo} onClick={onUndo}>
                    <Undo2 className="size-4.5" />
                </ToolbarButton>
                <ToolbarButton label="重做" disabled={!canRedo} onClick={onRedo}>
                    <Redo2 className="size-4.5" />
                </ToolbarButton>
                <ToolbarDivider theme={theme} />
                {workflowNodePresets.map((preset) => {
                    const Icon = preset.icon;
                    return (
                        <ToolbarButton key={preset.operation} label={preset.label} onClick={() => onAddNode(preset)}>
                            <Icon className="size-4.5" />
                        </ToolbarButton>
                    );
                })}
                <ToolbarDivider theme={theme} />
                <ToolbarButton label="模板设置" onClick={onOpenSettings}>
                    <Settings2 className="size-4.5" />
                </ToolbarButton>
                <ToolbarButton label="画布外观" active={appearanceOpen} onClick={() => setAppearanceOpen((value) => !value)}>
                    <Palette className="size-4.5" />
                </ToolbarButton>
                {selectedCount ? (
                    <>
                        <ToolbarDivider theme={theme} />
                        <ToolbarButton label="删除选中" danger onClick={onDelete}>
                            <Trash2 className="size-4.5" />
                        </ToolbarButton>
                    </>
                ) : null}
                <ToolbarDivider theme={theme} />
                <ToolbarButton label="清空画布" danger onClick={onClear}>
                    <Eraser className="size-4.5" />
                </ToolbarButton>
            </div>
            {appearanceOpen ? (
                <div
                    className="pointer-events-auto absolute bottom-[72px] z-30 w-[220px] rounded-xl border p-2.5 shadow-xl backdrop-blur"
                    style={{ background: theme.toolbar.panel, borderColor: theme.toolbar.border, color: theme.toolbar.item }}
                    data-canvas-no-zoom
                >
                    <div className="px-1 pb-2 text-sm font-medium opacity-65">画布外观</div>
                    <div className="grid grid-cols-3 gap-1">
                        <AppearanceButton active={backgroundMode === "dots"} label="点" onClick={() => onBackgroundModeChange("dots")}>
                            <CircleDot className="size-4" />
                        </AppearanceButton>
                        <AppearanceButton active={backgroundMode === "lines"} label="线" onClick={() => onBackgroundModeChange("lines")}>
                            <Box className="size-4" />
                        </AppearanceButton>
                        <AppearanceButton active={backgroundMode === "blank"} label="空白" onClick={() => onBackgroundModeChange("blank")}>
                            <Square className="size-4" />
                        </AppearanceButton>
                    </div>
                </div>
            ) : null}
        </div>
    );
}

function ToolbarButton({ label, active, disabled, danger, onClick, children }: { label: string; active?: boolean; disabled?: boolean; danger?: boolean; onClick?: () => void; children: ReactNode }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <Button
            type="text"
            aria-label={label}
            title={label}
            className="!h-8 !w-8 !min-w-8 !p-0"
            disabled={disabled}
            style={{
                background: active ? theme.toolbar.activeBg : undefined,
                color: danger ? "#f87171" : active ? theme.toolbar.activeText : theme.toolbar.item,
                opacity: disabled ? 0.35 : 1,
            }}
            icon={children}
            onClick={onClick}
        />
    );
}

function ToolbarDivider({ theme }: { theme: CanvasTheme }) {
    return <div className="mx-1 h-6 w-px" style={{ background: theme.toolbar.border }} />;
}

function AppearanceButton({ active, label, onClick, children }: { active: boolean; label: string; onClick: () => void; children: ReactNode }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <button
            type="button"
            className="inline-flex h-8 items-center justify-center gap-1 rounded-md px-2 text-sm transition"
            style={{ background: active ? theme.toolbar.activeBg : theme.toolbar.itemHover, color: active ? theme.toolbar.activeText : theme.toolbar.item }}
            onClick={onClick}
        >
            {children}
            {label}
        </button>
    );
}

function WorkflowNodeCard({
    node,
    selected,
    related,
    hovered,
    readOnlyLayout = false,
    isConnectionTarget,
    isConnecting,
    onMouseDown,
    onHoverStart,
    onHoverEnd,
    onConnectStart,
    onResizeStart,
    onOpenConfig,
    onDelete,
    onContextMenu,
}: {
    node: WorkflowTemplateNode;
    selected: boolean;
    related: boolean;
    hovered: boolean;
    readOnlyLayout?: boolean;
    isConnectionTarget: boolean;
    isConnecting: boolean;
    onMouseDown: (event: ReactMouseEvent, nodeId: string) => void;
    onHoverStart: (nodeId: string) => void;
    onHoverEnd: (nodeId: string) => void;
    onConnectStart: (event: ReactMouseEvent, nodeId: string, handleType: "source" | "target") => void;
    onResizeStart: (event: ReactMouseEvent, node: WorkflowTemplateNode, corner: ResizeCorner) => void;
    onOpenConfig: (nodeId: string) => void;
    onDelete: (nodeId: string) => void;
    onContextMenu: (event: ReactMouseEvent, nodeId: string) => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const meta = nodeTypeMeta[node.type];
    const Icon = meta.icon;
    const active = selected || isConnectionTarget;
    const summaryLines = nodeSummaryLines(node);
    return (
        <div
            data-node-id={node.id}
            className="absolute select-none overflow-visible"
            style={{ transform: `translate(${node.x}px, ${node.y}px)`, width: node.width, height: node.height, zIndex: active ? 20 : 10 }}
            onMouseEnter={() => onHoverStart(node.id)}
            onMouseLeave={() => onHoverEnd(node.id)}
            onContextMenu={(event) => onContextMenu(event, node.id)}
        >
            <div
                className="relative h-full w-full overflow-hidden rounded-2xl border-2 shadow-2xl"
                style={{
                    background: theme.node.fill,
                    borderColor: active ? "#2f80ff" : related ? theme.node.muted : theme.node.stroke,
                    boxShadow: active ? "0 0 0 1px rgba(47,128,255,.45)" : related ? `0 0 0 1px ${theme.node.muted}55` : undefined,
                }}
                onMouseDown={(event) => onMouseDown(event, node.id)}
            >
                <div className={`flex items-center justify-between border-b px-3 py-2 ${readOnlyLayout ? "cursor-default" : "cursor-move"}`} style={{ borderColor: theme.toolbar.border, color: theme.node.text }}>
                    <div className="flex min-w-0 items-center gap-2">
                        <Icon className="size-4 shrink-0 text-blue-400" />
                        <span className="truncate text-sm font-semibold">{node.title || node.id}</span>
                    </div>
                    <Tag className="!mr-0">{meta.label}</Tag>
                </div>
                <div className="flex h-[calc(100%-42px)] flex-col justify-center gap-2 px-4 text-xs" style={{ color: theme.node.muted }}>
                    {summaryLines.map((line, index) => (
                        <div key={`${line.label}-${index}`} className={line.multiline ? "line-clamp-3 whitespace-pre-wrap break-words opacity-75" : "truncate"}>
                            {line.label ? `${line.label}：` : ""}
                            {line.value}
                        </div>
                    ))}
                </div>
                {!readOnlyLayout ? (
                    <>
                        <ResizeHandle corner="top-left" onMouseDown={(event) => onResizeStart(event, node, "top-left")} />
                        <ResizeHandle corner="top-right" onMouseDown={(event) => onResizeStart(event, node, "top-right")} />
                        <ResizeHandle corner="bottom-left" onMouseDown={(event) => onResizeStart(event, node, "bottom-left")} />
                        <ResizeHandle corner="bottom-right" onMouseDown={(event) => onResizeStart(event, node, "bottom-right")} />
                    </>
                ) : null}
            </div>
            <ConnectionHandleDot side="left" visible={!readOnlyLayout && (hovered || selected || isConnecting)} onMouseDown={(event) => onConnectStart(event, node.id, "target")} />
            <ConnectionHandleDot side="right" visible={!readOnlyLayout && (hovered || selected || isConnecting)} onMouseDown={(event) => onConnectStart(event, node.id, "source")} />
        </div>
    );
}

function WorkflowGroupCard({ node, expanded, onToggle }: { node: DisplayWorkflowNode; expanded: boolean; onToggle: () => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const group = node.__group;
    if (!group) return null;
    const Icon = expanded ? ChevronDown : ChevronRight;
    if (expanded) {
        return (
            <div
                data-node-id={node.id}
                className="pointer-events-none absolute select-none rounded-3xl border-2 shadow-2xl"
                style={{
                    transform: `translate(${node.x}px, ${node.y}px)`,
                    width: node.width,
                    height: node.height,
                    zIndex: 4,
                    background: `${theme.toolbar.itemHover}8a`,
                    borderColor: `${theme.node.activeStroke}99`,
                    color: theme.node.text,
                    boxShadow: `inset 0 0 0 1px ${theme.node.activeStroke}22, 0 22px 60px rgba(0,0,0,.18)`,
                }}
            >
                <button
                    type="button"
                    className="pointer-events-auto absolute left-4 top-4 flex items-center gap-2 rounded-xl border px-3 py-2 text-left text-sm font-semibold transition hover:scale-[1.01]"
                    style={{ background: `${theme.node.fill}f2`, borderColor: theme.node.activeStroke, color: theme.node.text }}
                    onClick={(event) => {
                        event.stopPropagation();
                        onToggle();
                    }}
                >
                    <GitBranch className="size-4 text-blue-400" />
                    <span>{group.title}</span>
                    <span className="inline-flex items-center gap-1 rounded-lg px-2 py-0.5 text-xs font-normal" style={{ background: theme.toolbar.itemHover, color: theme.node.muted }}>
                        <Icon className="size-3.5" />
                        收起
                    </span>
                </button>
                <div className="absolute left-5 top-[62px] max-w-[360px] text-xs opacity-65">{group.summary}</div>
            </div>
        );
    }
    return (
        <button
            type="button"
            data-node-id={node.id}
            className="absolute select-none rounded-2xl border-2 px-4 py-3 text-left shadow-2xl transition hover:scale-[1.01]"
            style={{
                transform: `translate(${node.x}px, ${node.y}px)`,
                width: node.width,
                height: node.height,
                zIndex: 12,
                background: `${theme.node.fill}e6`,
                borderColor: theme.node.activeStroke,
                color: theme.node.text,
                boxShadow: `0 0 0 1px ${theme.node.activeStroke}44, 0 18px 42px rgba(0,0,0,.22)`,
            }}
            onClick={(event) => {
                event.stopPropagation();
                onToggle();
            }}
        >
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm font-semibold">
                        <GitBranch className="size-4 text-blue-400" />
                        <span>{group.title}</span>
                    </div>
                    <div className="mt-2 text-xs opacity-70">{group.summary}</div>
                </div>
                <span className="inline-flex shrink-0 items-center gap-1 rounded-lg border px-2 py-1 text-xs" style={{ borderColor: theme.toolbar.border, color: theme.node.muted }}>
                    <Icon className="size-3.5" />
                    {group.childIds.length} 节点
                </span>
            </div>
            <div className="mt-4 flex flex-wrap gap-1.5 text-[11px] opacity-75">
                {group.childIds.map((id) => (
                    <span key={id} className="rounded-md px-1.5 py-0.5" style={{ background: theme.toolbar.itemHover }}>
                        {id}
                    </span>
                ))}
            </div>
        </button>
    );
}

function ResizeHandle({ corner, onMouseDown }: { corner: ResizeCorner; onMouseDown: (event: ReactMouseEvent) => void }) {
    const positionClass = {
        "top-left": "-left-[14px] -top-[14px] cursor-nwse-resize",
        "top-right": "-right-[14px] -top-[14px] cursor-nesw-resize",
        "bottom-left": "-bottom-[14px] -left-[14px] cursor-nesw-resize",
        "bottom-right": "-bottom-[14px] -right-[14px] cursor-nwse-resize",
    }[corner];
    return <div className={`absolute z-50 size-7 ${positionClass}`} onMouseDown={onMouseDown} />;
}

function ConnectionHandleDot({ side, visible, onMouseDown }: { side: "left" | "right"; visible: boolean; onMouseDown: (event: ReactMouseEvent) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <div
            className={`absolute top-1/2 z-30 flex size-12 -translate-y-1/2 cursor-crosshair items-center justify-center transition-opacity duration-150 ${
                side === "left" ? "-left-6" : "-right-6"
            } ${visible ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0"}`}
            onMouseDown={onMouseDown}
        >
            <div className="size-3 rounded-full border-2 transition-all hover:scale-125" style={{ background: theme.node.panel, borderColor: theme.node.muted }} />
        </div>
    );
}

function TemplateNodeContextMenu({ menu, onClose, onDuplicate, onDelete }: { menu: TemplateContextMenu; onClose: () => void; onDuplicate: () => void; onDelete: () => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    useEffect(() => {
        const close = () => onClose();
        window.addEventListener("pointerdown", close);
        return () => window.removeEventListener("pointerdown", close);
    }, [onClose]);
    return (
        <div
            className="fixed z-[80] min-w-40 overflow-hidden rounded-xl border py-1 shadow-2xl"
            style={{ left: menu.x, top: menu.y, background: theme.toolbar.panel, borderColor: theme.toolbar.border, color: theme.node.text }}
            onPointerDown={(event) => event.stopPropagation()}
            data-canvas-no-zoom
        >
            <MenuButton icon={<Plus className="size-4" />} label="复制节点" onClick={onDuplicate} />
            <MenuButton icon={<Trash2 className="size-4" />} label="删除节点" onClick={onDelete} danger />
        </div>
    );
}

function MenuButton({ icon, label, onClick, danger = false }: { icon: ReactNode; label: string; onClick: () => void; danger?: boolean }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    return (
        <button type="button" className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs transition-colors hover:opacity-80" style={{ color: danger ? "#f87171" : theme.node.text }} onClick={onClick}>
            {icon}
            <span>{label}</span>
        </button>
    );
}

function EdgeEditor({ edge, nodes, onChange, onDelete }: { edge: WorkflowTemplateEdge; nodes: WorkflowTemplateNode[]; onChange: (patch: Partial<WorkflowTemplateEdge>) => void; onDelete: () => void }) {
    const from = nodes.find((node) => node.id === edge.from);
    const to = nodes.find((node) => node.id === edge.to);
    const condition = edge.condition || {};
    const loopEnabled = Boolean(edge.loop?.enabled);
    return (
        <Card size="small" title="连线配置" extra={<Button danger size="small" icon={<Trash2 className="size-3.5" />} onClick={onDelete} />}>
            <Space direction="vertical" className="w-full" size="middle">
	                <div className="rounded-lg border border-white/10 p-2 text-xs opacity-75">
	                    {from?.title || edge.from} → {to?.title || edge.to}
	                </div>
	                <Input value={edge.fromHandle || ""} addonBefore="出口名" placeholder="pass / repair / regenerate" onChange={(event) => onChange({ fromHandle: event.target.value || undefined })} />
	                <Card size="small" title="输入引用">
	                    <Space direction="vertical" className="w-full">
	                        <InputNumber className="!w-full" min={1} max={99} value={edge.inputOrder || undefined} addonBefore="输入顺序" placeholder="按连线顺序" onChange={(value) => onChange({ inputOrder: Number(value || 0) || undefined })} />
	                        <Input value={edge.inputAlias || ""} addonBefore="输入别名" placeholder="standard_reference / source_artwork / mockup_base" onChange={(event) => onChange({ inputAlias: event.target.value.trim() || undefined })} />
	                        <Select
	                            className="w-full"
	                            value={edge.fileSelector || "all"}
	                            options={[
	                                { label: "全部文件", value: "all" },
	                                { label: "第一张/第一个文件", value: "first" },
	                                { label: "最后一张/最后一个文件", value: "last" },
	                                { label: "第 1 张", value: "index:1" },
	                                { label: "第 2 张", value: "index:2" },
	                                { label: "第 3 张", value: "index:3" },
	                            ]}
	                            onChange={(fileSelector) => onChange({ fileSelector: fileSelector === "all" ? undefined : fileSelector })}
	                        />
	                    </Space>
	                </Card>
	                <Card size="small" title="条件 JSONPath">
                    <Space direction="vertical" className="w-full">
                        <Input value={edgeConditionString(edge, "jsonPath")} addonBefore="JSONPath" placeholder="$.decision" onChange={(event) => onChange({ condition: { ...condition, jsonPath: event.target.value } })} />
                        <Select
                            className="w-full"
                            value={edgeConditionString(edge, "operator") || "eq"}
                            options={[
                                { label: "等于", value: "eq" },
                                { label: "不等于", value: "neq" },
                                { label: "包含于", value: "in" },
                                { label: "包含文本", value: "contains" },
                                { label: "存在", value: "exists" },
                                { label: "真值", value: "truthy" },
                            ]}
                            onChange={(operator) => onChange({ condition: { ...condition, operator } })}
                        />
                        <Input value={edgeConditionString(edge, "value")} addonBefore="值" placeholder="pass" onChange={(event) => onChange({ condition: { ...condition, value: event.target.value } })} />
                    </Space>
                </Card>
                <Card size="small" title="受控循环">
                    <Space direction="vertical" className="w-full">
                        <Select
                            className="w-full"
                            value={loopEnabled ? "on" : "off"}
                            options={[
                                { label: "普通连线", value: "off" },
                                { label: "循环连线", value: "on" },
                            ]}
                            onChange={(value) => onChange({ loop: value === "on" ? { ...(edge.loop || {}), enabled: true } : undefined })}
                        />
                        {loopEnabled ? (
                            <InputNumber
                                className="!w-full"
                                min={1}
                                max={100}
                                addonBefore="最大轮次"
                                value={edgeLoopNumber(edge, "maxIterations", 5)}
                                onChange={(value) => onChange({ loop: { ...(edge.loop || {}), enabled: true, maxIterations: Number(value || 5) } })}
                            />
                        ) : null}
                    </Space>
                </Card>
            </Space>
        </Card>
    );
}

function NodeEditor({
    node,
    nodes,
    edges,
    advancedMode,
    useLocalAssets,
    onChange,
    onAddEdge,
    onUpdateEdge,
    onDeleteEdge,
    onDelete,
}: {
    node: WorkflowTemplateNode;
    nodes: WorkflowTemplateNode[];
    edges: WorkflowTemplateEdge[];
    advancedMode: boolean;
    useLocalAssets: boolean;
    onChange: (patch: Partial<WorkflowTemplateNode>) => void;
    onAddEdge: (from: string) => void;
    onUpdateEdge: (id: string, patch: Partial<WorkflowTemplateEdge>) => void;
    onDeleteEdge: (id: string) => void;
    onDelete: () => void;
}) {
    const upstreamEdges = edges.filter((edge) => edge.to === node.id);
    const token = useUserStore((state) => state.token);
    const localWorkspaceId = useLocalWorkspaceStore((state) => state.workspace?.id || "");
    const localAssets = useAssetStore((state) => state.assets);
    const localAssetsLoaded = useAssetStore((state) => state.workspaceLoaded);
    const localAssetsLoadedWorkspaceId = useAssetStore((state) => state.loadedWorkspaceId);
    const localAssetsLoading = useAssetStore((state) => state.loading);
    const loadLocalAssets = useAssetStore((state) => state.loadFromWorkspace);
    const effectiveConfig = useEffectiveConfig();
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const nodeConfig = buildTemplateNodeConfig(effectiveConfig, node);
    const needsModel = shouldShowModelPicker(node);
    const nodeCategory = workflowNodeCategory(node);
    const modelModality = workflowModelModality(node);
    const isMaterialLookup = node.operation === "material_lookup";
    const isTextGeneration = node.operation === "text_generation";
    const isCondition = node.operation === "condition";
    const isScript = node.operation === "script";
    const isImageSelect = node.operation === "image_select";
    const isImageConfig = node.operation === "image_generation" || node.operation === "image_edit";
    const isVideoConfig = node.operation === "video_generation";
    const showPromptEditor = shouldShowPromptEditor(node);
    const showPromptLibrary = shouldShowPromptLibrary(node);
    const showUpstreamEditor = shouldShowUpstreamEditor(node);
    const showOutputMappings = advancedMode && shouldShowOutputMappings(node);
    const [promptLibraryOpen, setPromptLibraryOpen] = useState(false);
    const fixedAssetId = nodeExtraString(node, "assetId");
    const fixedMaterialMode = nodeExtraString(node, "assetMode") === "fixed" || fixedAssetId !== "";
    const assetQuery = useQuery({
        queryKey: ["workflow-template-image-assets", token],
        queryFn: () => fetchAdminAssets(token || "", { type: "image", pageSize: 200 }),
        enabled: Boolean(!useLocalAssets && token && isMaterialLookup),
    });
    useEffect(() => {
        if (!useLocalAssets || !isMaterialLookup || !localWorkspaceId) return;
        if (localAssetsLoaded && localAssetsLoadedWorkspaceId === localWorkspaceId) return;
        void loadLocalAssets();
    }, [isMaterialLookup, loadLocalAssets, localAssetsLoaded, localAssetsLoadedWorkspaceId, localWorkspaceId, useLocalAssets]);
    const materialAssetOptions = useMemo(() => {
        if (useLocalAssets) return localAssets.filter((asset) => asset.kind === "image").map((asset) => ({ label: asset.title, value: asset.id }));
        return (assetQuery.data?.items || []).map((item) => ({ label: item.title, value: item.id }));
    }, [assetQuery.data?.items, localAssets, useLocalAssets]);
    const materialAssetsLoading = useLocalAssets ? localAssetsLoading || (Boolean(localWorkspaceId) && (!localAssetsLoaded || localAssetsLoadedWorkspaceId !== localWorkspaceId)) : assetQuery.isLoading;
    const updateImageSetting = (key: keyof AiConfig, value: string) => {
        if (key === "count") onChange({ count: Number(value) || 1 });
        else if (key === "size") onChange({ size: value });
        else if (key === "quality") onChange({ quality: value });
    };
    const updateVideoSetting = (key: keyof AiConfig, value: string) => {
        if (key === "videoSeconds") onChange({ seconds: value });
        else if (key === "vquality") onChange({ videoQuality: value });
        else if (key === "size") onChange({ size: value });
        else if (key === "videoReferenceMode") onChange({ extra: { ...(node.extra || {}), videoReferenceMode: value } });
    };
    const updateGuardrail = (patch: Record<string, unknown>) => {
        onChange({ extra: { ...(node.extra || {}), guardrail: { ...nodeGuardrail(node), ...patch } } });
    };
    const updateGuardrailSection = (section: string, patch: Record<string, unknown>) => {
        const current = nodeGuardrail(node);
        const value = current[section];
        const sectionValue = value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
        updateGuardrail({ [section]: { ...sectionValue, ...patch } });
    };
    const updateRetry = (patch: Partial<WorkflowNodeRetry>) => {
        onChange({ retry: { ...normalizeWorkflowNodeRetry(node.retry), ...patch } });
    };
    const insertPromptVariable = (value: string) => onChange({ prompt: `${node.prompt || ""}${value}` });
    const insertOutputVariable = (value: string) => {
        const lines = (node.outputMappings || []).map((item) => item.path);
        if (lines.length === 0) lines.push(value);
        else lines[lines.length - 1] = `${lines[lines.length - 1]}${value}`;
        onChange({ outputMappings: lines.map((path) => ({ path, kind: inferOutputKind(path) })) });
    };
    const changeOperation = (operation: WorkflowTemplateNode["operation"]) => {
        onChange(operationDefaultsPatch(node, operation, effectiveConfig));
    };
    return (
        <Card size="small" title="节点配置" extra={<Button danger size="small" icon={<Trash2 className="size-3.5" />} onClick={onDelete} />}>
            <Space direction="vertical" className="w-full" size="middle">
                <Input value={node.title} onChange={(event) => onChange({ title: event.target.value })} addonBefore="标题" />
                <LabeledControl label="节点功能">
                    <Select className="w-full" value={node.operation} options={operationOptionsByCategory[nodeCategory]} onChange={(value) => changeOperation(value as WorkflowTemplateNode["operation"])} />
                </LabeledControl>
                {isMaterialLookup ? (
                    <LabeledControl label="素材来源">
                        <Space direction="vertical" className="w-full">
                            <Select
                                className="w-full"
                                value={fixedMaterialMode ? "fixed" : "auto"}
                                options={[
                                    { label: "按输入主题/角色自动匹配", value: "auto" },
                                    { label: "固定选择一个素材", value: "fixed" },
                                ]}
                                onChange={(value) => onChange({ extra: value === "fixed" ? { ...(node.extra || {}), assetMode: "fixed" } : omitNodeExtra(node.extra, "assetId", "assetMode") })}
                            />
                            {fixedMaterialMode ? (
                                <Select
                                    showSearch
                                    className="w-full"
                                    placeholder="选择固定素材"
                                    loading={materialAssetsLoading}
                                    value={fixedAssetId || undefined}
                                    optionFilterProp="label"
                                    options={materialAssetOptions}
                                    onChange={(assetId) => onChange({ extra: { ...(node.extra || {}), assetId } })}
                                />
                            ) : null}
                        </Space>
                    </LabeledControl>
                ) : null}
                {isCondition ? (
                    <LabeledControl label="条件规则 JSON">
                        <Input.TextArea
                            rows={8}
                            value={nodeExtraJSONText(node, "conditions", '[{"jsonPath":"$.decision","operator":"eq","value":"pass","output":"pass"}]')}
                            onChange={(event) => onChange({ extra: { ...(node.extra || {}), conditions: event.target.value } })}
                            placeholder={'[{"jsonPath":"$.severity","operator":"in","values":["major","critical"],"output":"repair"}]'}
                        />
                    </LabeledControl>
                ) : null}
                {isCondition ? (
                    <Input value={nodeExtraString(node, "defaultOutput")} addonBefore="默认出口" placeholder="default" onChange={(event) => onChange({ extra: { ...(node.extra || {}), defaultOutput: event.target.value } })} />
                ) : null}
                {isScript ? (
                    <LabeledControl label="脚本设置">
                        <Space direction="vertical" className="w-full">
                            <Select
                                className="w-full"
                                value={nodeExtraString(node, "executor") || "vps"}
                                options={[
                                    { label: "VPS 执行", value: "vps" },
                                    { label: "本地 Agent 执行", value: "local_agent" },
                                ]}
                                onChange={(executor) => onChange({ extra: { ...(node.extra || {}), executor } })}
                            />
                            <Input value={nodeExtraString(node, "scriptPath")} addonBefore="脚本路径" placeholder="scripts/upload_products.sh" onChange={(event) => onChange({ extra: { ...(node.extra || {}), scriptPath: event.target.value } })} />
                            <InputNumber
                                className="!w-full"
                                min={1}
                                max={86400}
                                value={nodeExtraNumber(node, "timeoutSeconds", 600)}
                                addonBefore="超时秒数"
                                onChange={(value) => onChange({ extra: { ...(node.extra || {}), timeoutSeconds: Number(value || 600) } })}
                            />
                            <Input.TextArea
                                rows={4}
                                value={nodeExtraString(node, "args")}
                                onChange={(event) => onChange({ extra: { ...(node.extra || {}), args: event.target.value } })}
                                placeholder={"脚本参数，每行一个；支持变量，例如：\n--run-id\n{{runId}}"}
                            />
                        </Space>
                    </LabeledControl>
                ) : null}
                {isImageSelect ? (
                    <LabeledControl label="图片选择">
                        <Select
                            className="w-full"
                            value={nodeExtraString(node, "selectMode") || "last"}
                            options={[
                                { label: "最后一个有图的上游", value: "last" },
                                { label: "第一个有图的上游", value: "first" },
                                { label: "合并所有上游图片", value: "all" },
                            ]}
                            onChange={(selectMode) => onChange({ extra: { ...(node.extra || {}), selectMode } })}
                        />
                    </LabeledControl>
                ) : null}
                {needsModel ? (
                    <LabeledControl label="模型">
                        <ModelPicker config={nodeConfig} value={nodeConfig.model} modality={modelModality} onChange={(model) => onChange({ model })} onMissingConfig={() => openConfigDialog(true)} fullWidth />
                    </LabeledControl>
                ) : null}
                {needsModel ? <RetryConfigEditor retry={node.retry} onChange={updateRetry} /> : null}
                {isTextGeneration ? (
                    <LabeledControl label="输出格式">
                        <Select
                            className="w-full"
                            value={nodeTextOutputFormat(node)}
                            options={[
                                { label: "普通文本", value: "text" },
                                { label: "JSON", value: "json" },
                            ]}
                            onChange={(outputFormat) => onChange({ extra: { ...(node.extra || {}), outputFormat } })}
                        />
                    </LabeledControl>
                ) : null}
                {isImageConfig ? (
                    <LabeledControl label="图像设置">
                        <CanvasImageSettingsPopover
                            config={nodeConfig}
                            placement="bottomLeft"
                            buttonClassName="!h-10 !w-full !max-w-none !justify-start !rounded-lg !px-3"
                            onConfigChange={updateImageSetting}
                            onMissingConfig={() => openConfigDialog(true)}
                        />
                    </LabeledControl>
                ) : null}
                {isImageConfig ? <ImageGuardrailEditor config={effectiveConfig} guardrail={nodeGuardrail(node)} onChange={updateGuardrail} onSectionChange={updateGuardrailSection} /> : null}
                {isVideoConfig ? (
                    <LabeledControl label="视频设置">
                        <CanvasVideoSettingsPopover config={nodeConfig} placement="bottomLeft" buttonClassName="!h-10 !w-full !max-w-none !justify-start !rounded-lg !px-3" onConfigChange={updateVideoSetting} />
                    </LabeledControl>
                ) : null}
                {showPromptEditor ? (
                    <LabeledControl
                        label={
                            <div className="flex items-center justify-between gap-2">
                                <span>{nodePromptLabel(node)}</span>
                                {showPromptLibrary ? (
                                    <Button size="small" icon={<BookOpen className="size-3.5" />} onClick={() => setPromptLibraryOpen(true)}>
                                        查看提示词中心
                                    </Button>
                                ) : null}
                            </div>
                        }
                    >
                        <VariableInsertSelect onInsert={insertPromptVariable} />
                        <Input.TextArea value={node.prompt} rows={node.operation === "text_static" ? 5 : 8} onChange={(event) => onChange({ prompt: event.target.value })} placeholder={nodePromptPlaceholder(node)} />
                    </LabeledControl>
                ) : null}
                {showUpstreamEditor ? (
                    <Card size="small" title="上游输入">
                        <Space direction="vertical" className="w-full">
                            <Select
                                className="w-full"
                                placeholder="添加上游节点"
                                value={undefined}
                                options={nodes.filter((item) => item.id !== node.id && !upstreamEdges.some((edge) => edge.from === item.id)).map((item) => ({ label: `${item.title} (${item.id})`, value: item.id }))}
                                onChange={onAddEdge}
                            />
	                            {upstreamEdges.length ? (
	                                upstreamEdges.map((edge) => {
	                                    const from = nodes.find((item) => item.id === edge.from);
	                                    return (
	                                        <div key={edge.id} className="space-y-2 rounded-lg border border-white/10 p-2 text-xs">
	                                            <div className="flex items-center justify-between gap-2">
	                                                <span className="inline-flex min-w-0 items-center gap-1">
	                                                    <Link2 className="size-3 shrink-0" />
	                                                    <span className="truncate">{from?.title || edge.from} → {node.title}</span>
	                                                </span>
	                                                <Button size="small" type="text" danger icon={<Trash2 className="size-3" />} onClick={() => onDeleteEdge(edge.id)} />
	                                            </div>
	                                            <div className="grid grid-cols-[88px_1fr] gap-2">
	                                                <InputNumber className="!w-full" min={1} max={99} size="small" value={edge.inputOrder || undefined} placeholder="顺序" onChange={(value) => onUpdateEdge(edge.id, { inputOrder: Number(value || 0) || undefined })} />
	                                                <Input size="small" value={edge.inputAlias || ""} placeholder="别名：standard_reference" onChange={(event) => onUpdateEdge(edge.id, { inputAlias: event.target.value.trim() || undefined })} />
	                                            </div>
	                                            <Select
	                                                size="small"
	                                                className="w-full"
	                                                value={edge.fileSelector || "all"}
	                                                options={[
	                                                    { label: "全部文件", value: "all" },
	                                                    { label: "第一张/第一个文件", value: "first" },
	                                                    { label: "最后一张/最后一个文件", value: "last" },
	                                                    { label: "第 1 张", value: "index:1" },
	                                                    { label: "第 2 张", value: "index:2" },
	                                                    { label: "第 3 张", value: "index:3" },
	                                                ]}
	                                                onChange={(fileSelector) => onUpdateEdge(edge.id, { fileSelector: fileSelector === "all" ? undefined : fileSelector })}
	                                            />
	                                        </div>
	                                    );
	                                })
                            ) : (
                                <div className="text-xs text-stone-500">暂无上游输入</div>
                            )}
                        </Space>
                    </Card>
                ) : null}
                {showOutputMappings ? (
                    <LabeledControl label="输出路径模板">
                        <VariableInsertSelect onInsert={insertOutputVariable} />
                        <Input.TextArea
                            value={(node.outputMappings || []).map((item) => item.path).join("\n")}
                            rows={4}
                            onChange={(event) =>
                                onChange({
                                    outputMappings: event.target.value
                                        .split(/\r?\n/)
                                        .map((line) => line.trim())
                                        .filter(Boolean)
                                        .map((path) => ({ path, kind: inferOutputKind(path) })),
                                })
                            }
                            placeholder={"每行一个，例如：\ngenerated/{{productTitle}}/{{index4}}.png\n待上架/{{productTitle}}/规格图/{{index1}}_规格图.png\n待上架/{{productTitle}}/主图/{{index1}}_主图.png"}
                        />
                    </LabeledControl>
                ) : null}
            </Space>
            {showPromptLibrary ? <PromptSelectDialog open={promptLibraryOpen} onOpenChange={setPromptLibraryOpen} onSelect={(prompt) => onChange({ prompt })} /> : null}
        </Card>
    );
}

function LabeledControl({ label, children }: { label: ReactNode; children: ReactNode }) {
    return (
        <div className="space-y-1.5">
            <div className="text-xs font-medium opacity-65">{label}</div>
            {children}
        </div>
    );
}

function RetryConfigEditor({ retry, onChange }: { retry?: WorkflowNodeRetry; onChange: (patch: Partial<WorkflowNodeRetry>) => void }) {
    const value = normalizeWorkflowNodeRetry(retry);
    const disabled = value.enabled === false;
    return (
        <Card size="small" title="失败重试">
            <Space direction="vertical" className="w-full">
                <div className="flex items-center justify-between gap-3">
                    <Typography.Text className="text-sm">启用失败重试</Typography.Text>
                    <Switch checked={!disabled} onChange={(enabled) => onChange({ enabled })} />
                </div>
                <div className="grid grid-cols-2 gap-2">
                    <InputNumber className="!w-full" min={0} max={9999} disabled={disabled} value={value.retryCount} addonBefore="重试次数" onChange={(retryCount) => onChange({ retryCount: Number(retryCount ?? 0) })} />
                    <InputNumber className="!w-full" min={0} max={86400} disabled={disabled} value={value.intervalSeconds} addonBefore="间隔秒" onChange={(intervalSeconds) => onChange({ intervalSeconds: Number(intervalSeconds ?? 0) })} />
                </div>
                <Typography.Text type="secondary" className="text-xs">
                    次数 0 表示一直重试，间隔 0 使用系统退避。
                </Typography.Text>
            </Space>
        </Card>
    );
}

function ImageGuardrailEditor({
    config,
    guardrail,
    onChange,
    onSectionChange,
}: {
    config: AiConfig;
    guardrail: Record<string, unknown>;
    onChange: (patch: Record<string, unknown>) => void;
    onSectionChange: (section: string, patch: Record<string, unknown>) => void;
}) {
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const enabled = Boolean(guardrail.enabled);
    const review = objectRecord(guardrail.review);
    const repair = objectRecord(guardrail.repair);
    const transientRetry = objectRecord(guardrail.transientRetry);
    const transientRetryConfig = normalizeWorkflowNodeRetry({ retryCount: numberValue(transientRetry.retryCount, numberValue(transientRetry.maxAttempts, 0)), intervalSeconds: numberValue(transientRetry.intervalSeconds, 0) });
    const regenerate = objectRecord(guardrail.regenerate);
    const reviewConfig = { ...config, model: stringValue(review.model, config.textModel || config.model || defaultConfig.textModel) };
    const repairConfig = { ...config, model: stringValue(repair.model, config.imageModel || config.model || defaultConfig.imageModel) };
    return (
        <Card size="small" title="内置质检 / 修复">
            <Space direction="vertical" className="w-full">
                <div className="flex items-center justify-between gap-3">
                    <Typography.Text className="text-sm">启用图片质检与自动修复</Typography.Text>
                    <Switch checked={enabled} onChange={(checked) => onChange({ enabled: checked })} />
                </div>
                {enabled ? (
                    <>
                        <Select
                            className="w-full"
                            value={stringValue(guardrail.preset, "generic_image")}
                            options={[
                                { label: "通用图片", value: "generic_image" },
                                { label: "PDD 源图", value: "pdd_source" },
                                { label: "PDD Mockup", value: "pdd_mockup" },
                                { label: "PDD 最终主图", value: "pdd_main" },
                            ]}
                            onChange={(preset) => onChange({ preset })}
                        />
                        <Select
                            className="w-full"
                            value={stringValue(guardrail.failureStrategy, "manual_review")}
                            options={[
                                { label: "失败后人工复查", value: "manual_review" },
                                { label: "失败后重新生成", value: "regenerate" },
                                { label: "失败后停止商品", value: "fail" },
                            ]}
                            onChange={(failureStrategy) => onChange({ failureStrategy })}
                        />
                        <div className="grid grid-cols-3 gap-2">
                            <InputNumber className="!w-full" min={0} max={20} value={numberValue(repair.maxRounds, 5)} addonBefore="修复轮次" onChange={(value) => onSectionChange("repair", { maxRounds: Number(value ?? 5), enabled: true })} />
                            <InputNumber className="!w-full" min={0} max={9999} value={transientRetryConfig.retryCount} addonBefore="瞬时重试" onChange={(value) => onSectionChange("transientRetry", { retryCount: Number(value ?? 0) })} />
                            <InputNumber className="!w-full" min={0} max={86400} value={transientRetryConfig.intervalSeconds} addonBefore="间隔秒" onChange={(value) => onSectionChange("transientRetry", { intervalSeconds: Number(value ?? 0) })} />
                        </div>
                        <Typography.Text type="secondary" className="text-xs">
                            瞬时重试 0 表示一直重试，间隔 0 使用系统退避。
                        </Typography.Text>
                        <LabeledControl label="质检模型">
                            <ModelPicker config={reviewConfig} value={reviewConfig.model} modality="text" onChange={(model) => onSectionChange("review", { model })} onMissingConfig={() => openConfigDialog(true)} fullWidth />
                        </LabeledControl>
                        <LabeledControl label="修复模型">
                            <ModelPicker config={repairConfig} value={repairConfig.model} modality="image" onChange={(model) => onSectionChange("repair", { model, enabled: true })} onMissingConfig={() => openConfigDialog(true)} fullWidth />
                        </LabeledControl>
                        <InputNumber className="!w-full" min={1} max={20} value={numberValue(regenerate.maxRounds, 1)} addonBefore="重生轮次" onChange={(value) => onSectionChange("regenerate", { maxRounds: Number(value ?? 1) })} />
                    </>
                ) : null}
            </Space>
        </Card>
    );
}

function VariableInsertSelect({ onInsert }: { onInsert: (value: string) => void }) {
    return (
        <Select
            className="mb-2 w-full"
            placeholder="插入变量"
            value={undefined}
            options={[
                { label: "作品名 / 主题", value: "{{input.theme}}" },
                { label: "角色名", value: "{{input.character}}" },
                { label: "角色 presentation", value: "{{input.presentation}}" },
                { label: "商品标题", value: "{{productTitle}}" },
                { label: "商品标题原文", value: "{{productTitleRaw}}" },
                { label: "商品序号", value: "{{input.index}}" },
	                { label: "当前输出序号", value: "{{index}}" },
	                { label: "当前输出序号 1", value: "{{index1}}" },
	                { label: "当前输出序号 0001", value: "{{index4}}" },
	                { label: "节点输出数量", value: "{{count}}" },
	                { label: "上传参考图顺序说明", value: "{{uploaded_image_order}}" },
	                { label: "上游引用 JSON", value: "{{refs_json}}" },
	                { label: "别名引用第一张图", value: "{{ref.<alias>.first_file}}" },
	                { label: "别名引用第 2 张图", value: "{{ref.<alias>.image_2}}" },
	                { label: "别名引用文件列表 JSON", value: "{{ref.<alias>.files_json}}" },
	                { label: "别名引用文本", value: "{{ref.<alias>.text}}" },
	                { label: "上游节点文本", value: "{{node.<id>.text}}" },
	                { label: "上游节点第一张图片", value: "{{node.<id>.first_file}}" },
                { label: "上游节点图片列表 JSON", value: "{{node.<id>.files_json}}" },
            ]}
            onChange={(value) => onInsert(String(value || ""))}
        />
    );
}

function TemplateSettingsModal({
    open,
    template,
    spec,
    onCancel,
    onTemplateChange,
    onSpecChange,
}: {
    open: boolean;
    template: WorkflowTemplate;
    spec: WorkflowTemplateSpec;
    onCancel: () => void;
    onTemplateChange: (patch: Partial<WorkflowTemplate>) => void;
    onSpecChange: (patch: Partial<WorkflowTemplateSpec>) => void;
}) {
    return (
        <Modal
            title="模板设置"
            open={open}
            onCancel={onCancel}
            footer={
                <Button type="primary" onClick={onCancel}>
                    完成
                </Button>
            }
            width={620}
        >
            <Space direction="vertical" className="w-full" size="middle">
                <Input.TextArea value={template.description} onChange={(event) => onTemplateChange({ description: event.target.value })} rows={3} placeholder="模板说明" />
                <div className="grid grid-cols-2 gap-2">
                    <InputNumber className="!w-full" min={1} max={20} value={spec.settings.productConcurrency} addonBefore="商品并发" onChange={(value) => onSpecChange({ settings: { ...spec.settings, productConcurrency: Number(value || 1) } })} />
                    <InputNumber className="!w-full" min={1} max={100} value={spec.settings.maxRetries} addonBefore="重试" onChange={(value) => onSpecChange({ settings: { ...spec.settings, maxRetries: Number(value || 1) } })} />
                </div>
            </Space>
        </Modal>
    );
}

function StartTemplateRunModal({
    open,
    loading,
    value,
    onChange,
    onImport,
    onCancel,
    onStart,
}: {
    open: boolean;
    loading: boolean;
    value: string;
    onChange: (value: string) => void;
    onImport: (value: string) => void;
    onCancel: () => void;
    onStart: () => void;
}) {
    const { message } = App.useApp();
    const token = useUserStore((state) => state.token);
    const [loadingThemes, setLoadingThemes] = useState(false);
    const importJson = async (file: File) => {
        try {
            const text = await file.text();
            JSON.parse(text);
            onImport(text);
            message.success("JSON 已导入");
        } catch {
            message.error("JSON 导入失败");
        }
        return false;
    };
    const importThemes = async () => {
        if (!token) return;
        setLoadingThemes(true);
        try {
            const items = await fetchPDDWorkflowThemes(token);
            onImport(items.map((item) => JSON.stringify(item)).join("\n"));
            message.success(`已载入 ${items.length} 条 themes.json 输入`);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "载入 themes.json 失败");
        } finally {
            setLoadingThemes(false);
        }
    };
    return (
        <Modal
            title="运行工作流模板"
            open={open}
            onCancel={onCancel}
            width={760}
            footer={
                <Space>
                    <Button onClick={onCancel}>取消</Button>
                    <Button type="primary" icon={<Play className="size-4" />} loading={loading} onClick={onStart}>
                        启动
                    </Button>
                </Space>
            }
        >
            <Space direction="vertical" className="w-full">
                <div className="text-sm text-stone-500">
                    每一行是一个商品输入；也可以导入 JSON 数组。对象字段可在 prompt 和输出路径中用 {"{{input.theme}}"}、{"{{input.character}}"} 读取。
                </div>
                <Space wrap>
                    <Upload accept=".json,application/json" showUploadList={false} beforeUpload={importJson}>
                        <Button icon={<UploadIcon className="size-4" />}>导入 JSON</Button>
                    </Upload>
                    <Button icon={<UploadIcon className="size-4" />} loading={loadingThemes} onClick={importThemes}>
                        从 themes.json 载入
                    </Button>
                </Space>
                <Input.TextArea
                    rows={10}
                    value={value}
                    onChange={(event) => onChange(event.target.value)}
                    placeholder={'{"theme":"《原神》","character":"七七","presentation":"feminine"}\n{"theme":"《原神》","character":"刻晴","presentation":"feminine"}'}
                />
            </Space>
        </Modal>
    );
}

function parseInputs(text: string) {
    const value = text.trim();
    if (!value) throw new Error("请输入至少 1 条输入");
    if (value.startsWith("[") || value.startsWith("{")) {
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

function emptySpec(): WorkflowTemplateSpec {
    return { version: 1, nodes: [], edges: [], settings: { productConcurrency: 2, maxRetries: 3 } };
}

function createEmptyTemplate(): WorkflowTemplate {
    return {
        id: "",
        workflowType: "pdd",
        title: "未命名电商工作流模板",
        description: "",
        spec: emptySpec(),
        createdAt: "",
        updatedAt: "",
    };
}

function normalizeSpec(spec?: WorkflowTemplateSpec): WorkflowTemplateSpec {
    const fallback = emptySpec();
    return {
        version: spec?.version || fallback.version,
        settings: {
            productConcurrency: spec?.settings?.productConcurrency || fallback.settings.productConcurrency,
            maxRetries: spec?.settings?.maxRetries || fallback.settings.maxRetries,
        },
        nodes: (spec?.nodes || []).map((node) => {
            const rawOperation = String((node as WorkflowTemplateNode & { operation?: string }).operation || defaultOperation(node.type));
            const operation = rawOperation === "json_generation" ? "text_generation" : rawOperation;
            const typedOperation = operation as WorkflowTemplateNode["operation"];
            return {
                ...node,
                operation: typedOperation,
                extra: rawOperation === "json_generation" ? { ...(node.extra || {}), outputFormat: "json" } : node.extra,
                retry: workflowOperationUsesModel(typedOperation) ? normalizeWorkflowNodeRetry(node.retry) : undefined,
                width: Math.max(minNodeWidth, node.width || 300),
                height: Math.max(minNodeHeight, node.height || 190),
                x: Number.isFinite(node.x) ? node.x : 120,
                y: Number.isFinite(node.y) ? node.y : 120,
                outputMappings: node.outputMappings || [],
            };
        }),
        edges: spec?.edges || [],
    };
}

function cloneSpec(spec: WorkflowTemplateSpec): WorkflowTemplateSpec {
    return {
        version: spec.version,
        settings: { ...spec.settings },
        nodes: spec.nodes.map(cloneNode),
        edges: spec.edges.map(cloneEdge),
    };
}

function cloneNode(node: WorkflowTemplateNode): WorkflowTemplateNode {
    return {
        ...node,
        retry: node.retry ? { ...node.retry } : undefined,
        outputMappings: node.outputMappings?.map((item) => ({ ...item })) || [],
        extra: node.extra ? { ...node.extra } : undefined,
    };
}

function cloneEdge(edge: WorkflowTemplateEdge): WorkflowTemplateEdge {
    return {
        ...edge,
        condition: edge.condition ? { ...edge.condition } : undefined,
        loop: edge.loop ? { ...edge.loop } : undefined,
    };
}

function shouldShowModelPicker(node: WorkflowTemplateNode) {
    return workflowOperationUsesModel(node.operation);
}

function workflowOperationUsesModel(operation: WorkflowTemplateNode["operation"] | string) {
    return operation === "text_generation" || operation === "image_generation" || operation === "image_edit" || operation === "video_generation";
}

function normalizeWorkflowNodeRetry(retry?: WorkflowNodeRetry): WorkflowNodeRetry {
    return {
        enabled: retry?.enabled !== false,
        retryCount: Math.max(0, Number(retry?.retryCount ?? 0) || 0),
        intervalSeconds: Math.max(0, Number(retry?.intervalSeconds ?? 0) || 0),
    };
}

function defaultWorkflowNodeRetry(operation: WorkflowTemplateNode["operation"]) {
    return workflowOperationUsesModel(operation) ? normalizeWorkflowNodeRetry() : undefined;
}

function workflowNodeCategory(node: WorkflowTemplateNode): keyof typeof operationOptionsByCategory {
    if (nodeExtraString(node, "nodeCategory") === "function" || ["input", "condition", "script"].includes(node.operation)) return "function";
    if (node.type === "video" || node.operation === "video_generation") return "video";
    if (node.type === "image" || node.type === "material" || ["material_lookup", "image_select", "image_generation", "image_edit"].includes(node.operation)) return "image";
    return "text";
}

function workflowModelModality(node: WorkflowTemplateNode): "any" | "image" | "text" | "video" {
    if (node.operation === "video_generation") return "video";
    if (node.operation === "image_generation" || node.operation === "image_edit") return "image";
    if (node.operation === "text_generation") return "text";
    return "any";
}

function operationDefaultsPatch(node: WorkflowTemplateNode, operation: WorkflowTemplateNode["operation"], config: AiConfig): Partial<WorkflowTemplateNode> {
    const currentExtra = { ...(node.extra || {}) };
    const nextExtra = { ...currentExtra };
    let type: WorkflowTemplateNode["type"] = node.type;
    let model = node.model || "";
    let size = node.size || "";
    let quality = node.quality || "";
    let seconds = node.seconds || "";
    let videoQuality = node.videoQuality || "";

    if (["input", "condition", "script"].includes(operation)) {
        type = "text";
        nextExtra.nodeCategory = "function";
    } else {
        delete nextExtra.nodeCategory;
    }
    if (operation === "text_static" || operation === "text_generation") {
        type = "text";
    }
    if (["material_lookup", "image_select", "image_generation", "image_edit"].includes(operation)) {
        type = "image";
    }
    if (operation === "video_generation") {
        type = "video";
    }
    if (operation === "text_generation") {
        model = config.textModel || config.model || defaultConfig.textModel;
        nextExtra.outputFormat = nodeExtraString(node, "outputFormat") || "text";
    }
    if (operation === "condition") {
        nextExtra.conditions = nextExtra.conditions || '[{"jsonPath":"$.decision","operator":"eq","value":"pass","output":"pass"}]';
        nextExtra.defaultOutput = nextExtra.defaultOutput || "default";
    }
    if (operation === "script") {
        nextExtra.executor = nextExtra.executor || "vps";
        nextExtra.timeoutSeconds = nextExtra.timeoutSeconds || 600;
    }
    if (operation === "image_select") {
        nextExtra.selectMode = nextExtra.selectMode || "last";
    }
    if (operation === "image_generation" || operation === "image_edit") {
        model = config.imageModel || config.model || defaultConfig.imageModel;
        size = size || config.size || defaultConfig.size;
        quality = quality || config.quality || defaultConfig.quality;
    }
    if (operation === "video_generation") {
        model = config.videoModel || config.model || defaultConfig.videoModel;
        size = size || config.size || "1280x720";
        seconds = seconds || config.videoSeconds || defaultConfig.videoSeconds;
        videoQuality = videoQuality || config.vquality || defaultConfig.vquality;
        nextExtra.videoReferenceMode = nextExtra.videoReferenceMode || config.videoReferenceMode || defaultConfig.videoReferenceMode;
    }
    return {
        type,
        operation,
        model,
        size,
        quality,
        seconds,
        videoQuality,
        retry: workflowOperationUsesModel(operation) ? normalizeWorkflowNodeRetry(node.retry) : undefined,
        extra: Object.keys(nextExtra).length ? nextExtra : undefined,
    };
}

function connectionCreateDescription(preset: WorkflowNodePreset) {
    if (preset.operation === "text_generation") return "生成文案、JSON、标题或质检文本";
    if (preset.operation === "image_generation") return "生成、编辑、选择图片或查找素材";
    if (preset.operation === "video_generation") return "生成视频内容";
    return "输入、条件分支或脚本执行";
}

function shouldShowPromptEditor(node: WorkflowTemplateNode) {
    return node.operation === "text_static" || node.operation === "text_generation" || node.operation === "image_generation" || node.operation === "image_edit" || node.operation === "video_generation";
}

function shouldShowPromptLibrary(node: WorkflowTemplateNode) {
    return node.operation === "text_generation" || node.operation === "image_generation" || node.operation === "image_edit" || node.operation === "video_generation";
}

function shouldShowUpstreamEditor(node: WorkflowTemplateNode) {
    return node.operation !== "input";
}

function shouldShowOutputMappings(node: WorkflowTemplateNode) {
    return (
        node.operation === "text_generation" ||
        node.operation === "condition" ||
        node.operation === "script" ||
        node.operation === "image_select" ||
        node.operation === "image_generation" ||
        node.operation === "image_edit" ||
        node.operation === "video_generation"
    );
}

function nodePromptLabel(node: WorkflowTemplateNode) {
    if (node.operation === "text_static") return "文本内容";
    if (node.operation === "text_generation") return nodeTextOutputFormat(node) === "json" ? "JSON 生成要求" : "文字生成要求";
    if (node.operation === "video_generation") return "视频生成要求";
    return "图像生成要求";
}

function nodePromptPlaceholder(node: WorkflowTemplateNode) {
    if (node.operation === "text_static") return "输入这段静态文字。可用变量：{{input.theme}}、{{input.character}}、{{productTitle}}、{{node.<id>.text}}、{{index}}";
    if (node.operation === "text_generation" && nodeTextOutputFormat(node) === "json") return "要求模型只输出可解析 JSON，不要输出 Markdown 代码块或解释文字。可用变量：{{input.theme}}、{{input.character}}、{{productTitle}}、{{node.<id>.text}}、{{index}}";
    return "节点生成要求。可用变量：{{input.theme}}、{{input.character}}、{{productTitle}}、{{node.<id>.text}}、{{index}}";
}

function nodeExtraString(node: WorkflowTemplateNode, key: string) {
    const value = node.extra?.[key];
    return typeof value === "string" ? value : "";
}

function nodeGuardrail(node: WorkflowTemplateNode) {
    return objectRecord(node.extra?.guardrail);
}

function objectRecord(value: unknown): Record<string, unknown> {
    return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function stringValue(value: unknown, fallback = "") {
    return typeof value === "string" && value.trim() ? value : fallback;
}

function numberValue(value: unknown, fallback: number) {
    return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function nodeExtraNumber(node: WorkflowTemplateNode, key: string, fallback: number) {
    const value = node.extra?.[key];
    return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function nodeExtraJSONText(node: WorkflowTemplateNode, key: string, fallback: string) {
    const value = node.extra?.[key];
    if (typeof value === "string") return value;
    if (value) return JSON.stringify(value, null, 2);
    return fallback;
}

function nodeTextOutputFormat(node: WorkflowTemplateNode) {
    return nodeExtraString(node, "outputFormat") === "json" || nodeExtraString(node, "output_format") === "json" ? "json" : "text";
}

function edgeConditionString(edge: WorkflowTemplateEdge, key: string) {
    const value = edge.condition?.[key];
    return typeof value === "string" ? value : "";
}

function edgeLoopNumber(edge: WorkflowTemplateEdge, key: string, fallback: number) {
    const value = edge.loop?.[key];
    return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function omitNodeExtra(extra: WorkflowTemplateNode["extra"], ...keys: string[]) {
    const next = { ...(extra || {}) };
    keys.forEach((key) => delete next[key]);
    return Object.keys(next).length ? next : undefined;
}

function nodeSummaryLines(node: WorkflowTemplateNode): NodeSummaryLine[] {
    const lines: NodeSummaryLine[] = [{ label: "操作", value: node.operation }];
    const prompt = (node.prompt || "").trim();
    if (node.operation === "input") {
        return [...lines, { label: "", value: "作为每个商品的输入数据" }];
    }
    if (node.operation === "material_lookup") {
        const assetID = nodeExtraString(node, "assetId");
        return [...lines, { label: "素材来源", value: assetID ? `固定素材 ${assetID}` : "按输入主题/角色匹配素材库图片" }];
    }
    if (node.operation === "text_static") {
        return [...lines, { label: "文本内容", value: prompt || "空文本", multiline: true }];
    }
    if (node.operation === "text_generation") {
        lines.push({ label: "输出格式", value: nodeTextOutputFormat(node) === "json" ? "JSON" : "普通文本" });
    }
    if (node.operation === "condition") {
        return [...lines, { label: "条件规则", value: nodeExtraJSONText(node, "conditions", "未设置条件"), multiline: true }];
    }
    if (node.operation === "script") {
        return [...lines, { label: "执行位置", value: nodeExtraString(node, "executor") || "vps" }, { label: "脚本", value: nodeExtraString(node, "scriptPath") || "未设置脚本" }];
    }
    if (node.operation === "image_select") {
        return [...lines, { label: "选择模式", value: nodeExtraString(node, "selectMode") || "last" }];
    }
    if (shouldShowModelPicker(node)) {
        lines.push({ label: "模型", value: node.model || defaultTemplateNodeModel(defaultConfig, node) });
        const retry = normalizeWorkflowNodeRetry(node.retry);
        lines.push({ label: "失败重试", value: retry.enabled === false ? "关闭" : `${retry.retryCount === 0 ? "无限" : `${retry.retryCount} 次`} · ${retry.intervalSeconds === 0 ? "系统退避" : `${retry.intervalSeconds}s`}` });
    }
    if ((node.operation === "image_generation" || node.operation === "image_edit") && nodeGuardrail(node).enabled) {
        const guardrail = nodeGuardrail(node);
        lines.push({ label: "内置质检/修复", value: `${stringValue(guardrail.preset, "generic_image")} · ${stringValue(guardrail.failureStrategy, "manual_review")}` });
    }
    if (shouldShowPromptEditor(node)) {
        lines.push({ label: nodePromptLabel(node), value: prompt || "未设置生成要求", multiline: true });
    }
    return lines;
}

function buildTemplateNodeConfig(config: AiConfig, node: WorkflowTemplateNode): AiConfig {
    return {
        ...config,
        model: node.model || defaultTemplateNodeModel(config, node),
        quality: node.quality || config.quality || defaultConfig.quality,
        size: node.size || config.size || defaultConfig.size,
        count: String(node.count || config.count || defaultConfig.count),
        videoSeconds: node.seconds || config.videoSeconds || defaultConfig.videoSeconds,
        videoReferenceMode: nodeExtraString(node, "videoReferenceMode") || config.videoReferenceMode || defaultConfig.videoReferenceMode,
        vquality: node.videoQuality || config.vquality || defaultConfig.vquality,
    };
}

function defaultTemplateNodeModel(config: AiConfig, node: WorkflowTemplateNode) {
    if (node.operation === "video_generation" || node.type === "video") return config.videoModel || config.model || defaultConfig.videoModel;
    if (node.operation === "text_generation" || node.type === "text") return config.textModel || config.model || defaultConfig.textModel;
    if (node.operation === "image_generation" || node.operation === "image_edit" || node.type === "image") return config.imageModel || config.model || defaultConfig.imageModel;
    return config.model || defaultConfig.model;
}

function toCanvasNode(node: WorkflowTemplateNode): CanvasNodeData {
    return {
        id: node.id,
        type: nodeTypeMeta[node.type].canvasType,
        title: node.title || node.id,
        position: { x: node.x, y: node.y },
        width: node.width,
        height: node.height,
        metadata: {
            prompt: node.prompt,
            model: node.model,
            count: node.count,
            size: node.size,
            quality: node.quality,
        },
    };
}

function buildFlowView(spec: WorkflowTemplateSpec, expandedGroups: Set<string>): { nodes: DisplayWorkflowNode[]; edges: DisplayWorkflowEdge[] } {
    const nodeIDs = new Set(spec.nodes.map((node) => node.id));
    const activeGroups = flowGroups.filter((group) => group.childIds.every((id) => nodeIDs.has(id)));
    const hiddenToGroup = new Map<string, FlowGroup>();
    activeGroups.forEach((group) => {
        if (expandedGroups.has(group.id)) return;
        group.childIds.forEach((id) => hiddenToGroup.set(id, group));
    });

    const layout = flowLayoutPositions(spec, activeGroups, expandedGroups);
    const nodes: DisplayWorkflowNode[] = [];
    spec.nodes.forEach((node) => {
        if (hiddenToGroup.has(node.id)) return;
        const positioned = layout.get(node.id);
        nodes.push({ ...node, ...(positioned || {}) });
    });
    activeGroups.forEach((group) => {
        const positioned = layout.get(group.id) || { x: 780, y: 200, width: 320, height: 170 };
        nodes.push({
            id: group.id,
            type: "text",
            title: group.title,
            operation: "text_static",
            model: "",
            prompt: "",
            count: 1,
            size: "",
            quality: "",
            seconds: "",
            videoQuality: "",
            outputMappings: [],
            ...positioned,
            __virtual: true,
            __group: group,
        });
    });

    const edgeByKey = new Map<string, DisplayWorkflowEdge>();
    spec.edges.forEach((edge) => {
        const from = hiddenToGroup.get(edge.from)?.id || edge.from;
        const to = hiddenToGroup.get(edge.to)?.id || edge.to;
        if (from === to) return;
        if (!nodes.some((node) => node.id === from) || !nodes.some((node) => node.id === to)) return;
        const visualKind = workflowEdgeVisualKind(edge);
        const key = `${from}->${to}:${edge.fromHandle || ""}:${visualKind}`;
        if (edgeByKey.has(key)) return;
        edgeByKey.set(key, { ...edge, from, to, __visualKind: visualKind });
    });
    return { nodes, edges: withFlowEdgeRoutes(Array.from(edgeByKey.values()), nodes) };
}

function flowLayoutPositions(spec: WorkflowTemplateSpec, activeGroups: FlowGroup[], expandedGroups: Set<string>) {
    const positions = new Map<string, Pick<WorkflowTemplateNode, "x" | "y" | "width" | "height">>();
    const has = (id: string) => spec.nodes.some((node) => node.id === id);
    const set = (id: string, x: number, y: number, width = 300, height = 160) => {
        if (id.startsWith("flow_group_") || has(id)) positions.set(id, { x, y, width, height });
    };
    const sourceExpanded = expandedGroups.has("flow_group_source_quality") && activeGroups.some((group) => group.id === "flow_group_source_quality");
    const mainExpanded = expandedGroups.has("flow_group_main_quality") && activeGroups.some((group) => group.id === "flow_group_main_quality");
    const baseX = 80;
    const gap = 360;
    const groupPaddingX = 34;
    const groupPaddingTop = 104;
    const groupWidth = 1120;
    const groupHeight = 620;

    set("reference", baseX, 80, 280, 150);
    set("input", baseX, 330, 280, 150);
    set("source", baseX + gap, 200, 320, 190);

    let nextX = baseX + gap * 2;
    if (sourceExpanded) {
        const groupX = nextX - 26;
        const groupY = 40;
        set("flow_group_source_quality", groupX, groupY, groupWidth, groupHeight);
        set("current_source", groupX + groupPaddingX, groupY + groupPaddingTop + 155, 280, 160);
        set("source_review", groupX + groupPaddingX + 360, groupY + groupPaddingTop, 320, 180);
        set("source_decision", groupX + groupPaddingX + 720, groupY + groupPaddingTop, 280, 160);
        set("source_repair", groupX + groupPaddingX + 360, groupY + groupPaddingTop + 310, 320, 180);
        nextX += groupWidth + 80;
    } else {
        set("flow_group_source_quality", nextX, 205, 330, 180);
        nextX += gap;
    }

    set("title", nextX, 60, 300, 160);
    set("mockup_base", nextX, 500, 300, 160);
    set("mockup", nextX + gap, 250, 320, 190);
    set("main", nextX + gap * 2, 250, 320, 190);
    nextX += gap * 3;

    if (mainExpanded) {
        const groupX = nextX - 26;
        const groupY = 80;
        set("flow_group_main_quality", groupX, groupY, groupWidth, groupHeight);
        set("current_main", groupX + groupPaddingX, groupY + groupPaddingTop + 155, 280, 160);
        set("main_review", groupX + groupPaddingX + 360, groupY + groupPaddingTop, 320, 180);
        set("main_decision", groupX + groupPaddingX + 720, groupY + groupPaddingTop, 280, 160);
        set("main_repair", groupX + groupPaddingX + 360, groupY + groupPaddingTop + 310, 320, 180);
        nextX += groupWidth + 80;
    } else {
        set("flow_group_main_quality", nextX, 255, 330, 180);
        nextX += gap;
    }
    set("package", nextX, 250, 320, 190);

    spec.nodes.forEach((node) => {
        if (positions.has(node.id)) return;
        const index = positions.size;
        positions.set(node.id, { x: baseX + (index % 5) * gap, y: 720 + Math.floor(index / 5) * 210, width: node.width, height: node.height });
    });
    return positions;
}

function withFlowEdgeRoutes(edges: DisplayWorkflowEdge[], nodes: DisplayWorkflowNode[]): DisplayWorkflowEdge[] {
    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    const outgoing = countBy(edges, (edge) => edge.from);
    const incoming = countBy(edges, (edge) => edge.to);
    const outgoingIndex = new Map<string, number>();
    const incomingIndex = new Map<string, number>();
    const railIndex = new Map<string, number>();

    const nextSlot = (bucket: Map<string, number>, key: string) => {
        const current = bucket.get(key) || 0;
        bucket.set(key, current + 1);
        return current;
    };
    const slotOffset = (index: number, total: number) => {
        if (total <= 1) return 0;
        return (index - (total - 1) / 2) * 24;
    };
    const nextRail = (key: string) => {
        const current = railIndex.get(key) || 0;
        railIndex.set(key, current + 1);
        return current;
    };

    return edges.map((edge) => {
        const fromNode = nodeById.get(edge.from);
        const toNode = nodeById.get(edge.to);
        const visualKind = edge.__visualKind || workflowEdgeVisualKind(edge);
        if (!fromNode || !toNode) return edge;

        const startOffsetY = slotOffset(nextSlot(outgoingIndex, edge.from), outgoing.get(edge.from) || 1);
        const endOffsetY = slotOffset(nextSlot(incomingIndex, edge.to), incoming.get(edge.to) || 1);
        const route: CanvasConnectionRoute = { startOffsetY, endOffsetY, elbowPadding: visualKind === "repair" || visualKind === "loop" ? 72 : 56 };
        const startY = fromNode.y + fromNode.height / 2 + startOffsetY;
        const endY = toNode.y + toNode.height / 2 + endOffsetY;
        const minY = Math.min(startY, endY);
        const maxY = Math.max(startY, endY);
        const spanX = Math.abs(toNode.x - (fromNode.x + fromNode.width));

        if (visualKind === "repair" || visualKind === "loop") {
            route.railY = maxY + 94 + nextRail("repair") * 34;
        } else if (visualKind === "support") {
            const lowerSupport = edge.from === "mockup_base" || fromNode.y > toNode.y + 80;
            route.railY = lowerSupport ? maxY + 76 + nextRail("support-bottom") * 30 : minY - 76 - nextRail("support-top") * 30;
        } else if (visualKind === "pass" && spanX > 320) {
            route.railY = minY - 58 - nextRail("pass") * 28;
        } else if (spanX > 760 && Math.abs(startY - endY) > 34) {
            route.railY = minY - 48 - nextRail("main-long") * 24;
        }

        return { ...edge, __visualKind: visualKind, __route: route };
    });
}

function countBy<T>(items: T[], keyOf: (item: T) => string) {
    const counts = new Map<string, number>();
    items.forEach((item) => {
        const key = keyOf(item);
        counts.set(key, (counts.get(key) || 0) + 1);
    });
    return counts;
}

function workflowEdgeVisualKind(edge: WorkflowTemplateEdge): CanvasConnectionVariant {
    if (edge.loop?.enabled) return "loop";
    if (edge.fromHandle === "repair") return "repair";
    if (edge.fromHandle === "pass") return "pass";
    if (edge.from === "reference" || edge.from === "mockup_base" || edge.to === "package" || edge.from === "title") return "support";
    return "main";
}

function getRelatedIds(edges: WorkflowTemplateEdge[], selectedNodeIds: Set<string>, hoveredNodeId: string | null) {
    const focusIds = new Set(selectedNodeIds);
    if (hoveredNodeId) focusIds.add(hoveredNodeId);
    const nodeIds = new Set<string>();
    const edgeIds = new Set<string>();
    edges.forEach((edge) => {
        if (!focusIds.has(edge.from) && !focusIds.has(edge.to)) return;
        edgeIds.add(edge.id);
        nodeIds.add(edge.from);
        nodeIds.add(edge.to);
    });
    return { nodeIds, edgeIds };
}

function getNodeBounds(nodes: WorkflowTemplateNode[]) {
    return nodes.reduce(
        (acc, node) => ({
            left: Math.min(acc.left, node.x),
            top: Math.min(acc.top, node.y),
            right: Math.max(acc.right, node.x + node.width),
            bottom: Math.max(acc.bottom, node.y + node.height),
        }),
        { left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity },
    );
}

function defaultOperation(type: WorkflowTemplateNode["type"]): WorkflowTemplateNode["operation"] {
    if (type === "material") return "material_lookup";
    if (type === "text") return "text_static";
    if (type === "video") return "video_generation";
    return "image_generation";
}

function inferOutputKind(path: string) {
    if (path.includes("主图")) return "main";
    if (path.includes("规格图")) return "spec";
    if (path.includes("generated")) return "generated";
    return "artifact";
}

function isEditableKeyboardTarget(target: EventTarget | null) {
    if (!(target instanceof HTMLElement)) return false;
    return Boolean(target.closest("input,textarea,select,[contenteditable='true'],.ant-input,.ant-select,.ant-modal,.ant-drawer,.ant-popover,.ant-dropdown"));
}
