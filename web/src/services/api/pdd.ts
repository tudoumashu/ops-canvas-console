import { apiDelete, apiGet, apiPost, compactApiParams, type ApiParams } from "@/services/api/request";

export type PDDRunStatus = "idle" | "running" | "success" | "error";

export type PDDRunItem = {
    runId: string;
    status: PDDRunStatus;
    runDir: string;
    updatedAt: string;
    customWorkflow?: boolean;
    startedAt?: string;
    finishedAt?: string;
    completed: boolean;
    hasLogs: boolean;
    productTotal: number;
    completedProducts: number;
    failedProducts: number;
    runningProducts: number;
    recentError?: string;
};

export type PDDRunList = {
    root: string;
    items: PDDRunItem[];
};

export type PDDStageNode = {
    id: string;
    title: string;
    type?: string;
    status: PDDRunStatus;
    total: number;
    success: number;
    failed: number;
    running: number;
    idle: number;
    skipped: number;
    x?: number;
    y?: number;
    width?: number;
    height?: number;
    durationSeconds?: number;
    recentError?: string;
};

export type PDDGraphEdge = {
    id: string;
    from: string;
    to: string;
};

export type PDDProductSummary = {
    key: string;
    sourceProduct: string;
    generatedProduct?: string;
    product: string;
    themeName: string;
    status: PDDRunStatus;
    rawStatus: string;
    startedAt?: string;
    finishedAt?: string;
    error?: string;
    generatedImages: number;
    specImages: number;
    mainImages: number;
    artifactCount?: number;
};

export type PDDRunOverview = {
    run: PDDRunItem;
    stages: PDDStageNode[];
    edges: PDDGraphEdge[];
    products: PDDProductSummary[];
    recentErrors: string[];
};

export type PDDArtifact = {
    id: string;
    title: string;
    path: string;
    url: string;
    kind: string;
    mimeType?: string;
};

export type PDDDetailFile = {
    title: string;
    path: string;
    url: string;
    kind: "image" | "json" | "text" | "file";
};

export type PDDGraphNode = {
    id: string;
    type: string;
    title: string;
    status: PDDRunStatus;
    x: number;
    y: number;
    width: number;
    height: number;
    summary?: string;
    config?: Record<string, unknown>;
    durationSeconds?: number;
    artifacts?: PDDArtifact[];
    files?: PDDDetailFile[];
};

export type PDDProductDetail = {
    runId: string;
    product: PDDProductSummary;
    nodes: PDDGraphNode[];
    edges: PDDGraphEdge[];
    files: PDDDetailFile[];
};

export type PDDCreativeCanvasNode = {
    id: string;
    type: "image" | "text" | "video" | "config";
    title: string;
    position: { x: number; y: number };
    width: number;
    height: number;
    metadata?: Record<string, unknown>;
};

export type PDDCreativeCanvasEdge = {
    id: string;
    fromNodeId: string;
    toNodeId: string;
};

export type PDDCreativeCanvas = {
    runId: string;
    productKey: string;
    product: PDDProductSummary;
    nodes: PDDCreativeCanvasNode[];
    edges: PDDCreativeCanvasEdge[];
    viewport?: { x: number; y: number; k: number };
    backgroundMode?: string;
    showImageInfo?: boolean;
    saved: boolean;
    updatedAt?: string;
};

export type PDDCreativeCanvasSaveRequest = {
    nodes: PDDCreativeCanvasNode[];
    edges: PDDCreativeCanvasEdge[];
    viewport?: { x: number; y: number; k: number };
    backgroundMode?: string;
    showImageInfo?: boolean;
};

export type PDDCreativeCanvasAssetRequest = {
    productKey?: string;
    nodeId: string;
    fileName?: string;
    mimeType?: string;
    content: string;
};

export type PDDCreativeCanvasAsset = {
    url: string;
    path: string;
    fileName: string;
    mimeType: string;
    bytes: number;
    width?: number;
    height?: number;
};

export type PDDCreativeCanvasApplyRequest = {
    productKey?: string;
    sourceNodeId: string;
    targetNodeId: string;
    artifactPath?: string;
    content?: string;
    mimeType?: string;
    rerunDownstream?: boolean;
};

export type PDDActionRequest = {
    action: "run" | "stop" | "health_check" | "docker_status" | "restart_chatgpt2api" | "restart_sub2api" | "restart_cli_proxy" | "warp_reconnect";
    runId?: string;
    countPerTheme?: number;
    extraArgs?: string[];
    consoleSpec?: Record<string, unknown>;
};

export type PDDActionResult = {
    action: string;
    runId?: string;
    output: string;
};

export type PDDManualEditRequest = {
    productKey: string;
    nodeId: string;
    artifactPath: string;
    maskPath?: string;
    maskDataUrl?: string;
    prompt: string;
    model?: string;
    count?: number;
    size?: string;
    quality?: string;
    apply?: boolean;
    rerunDownstream?: boolean;
};

export type PDDManualEditApplyRequest = {
    productKey: string;
    nodeId: string;
    rerunDownstream?: boolean;
};

export type PDDManualEditResult = {
    editId: string;
    productKey: string;
    nodeId: string;
    artifacts: PDDArtifact[];
    applied: boolean;
    rerunDownstream: boolean;
    output?: string;
};

export type WorkflowRunStatus = "idle" | "running" | "success" | "error";

export type WorkflowOutputMapping = {
    path: string;
    kind: string;
};

export type WorkflowNodeRetry = {
    enabled?: boolean;
    retryCount?: number;
    intervalSeconds?: number;
};

export type WorkflowTemplateNode = {
    id: string;
    type: "material" | "text" | "image" | "video";
    title: string;
    x: number;
    y: number;
    width: number;
    height: number;
    operation: "input" | "material_lookup" | "text_static" | "text_generation" | "condition" | "script" | "image_select" | "image_generation" | "image_edit" | "video_generation";
    model?: string;
    prompt?: string;
    count?: number;
    size?: string;
    quality?: string;
    seconds?: string;
    videoQuality?: string;
    retry?: WorkflowNodeRetry;
    outputMappings?: WorkflowOutputMapping[];
    extra?: Record<string, unknown>;
};

export type WorkflowTemplateEdge = {
    id: string;
    from: string;
    to: string;
    fromHandle?: string;
    inputOrder?: number;
    inputAlias?: string;
    fileSelector?: string;
    condition?: Record<string, unknown>;
    loop?: Record<string, unknown>;
};

export type WorkflowTemplateSpec = {
    version: number;
    nodes: WorkflowTemplateNode[];
    edges: WorkflowTemplateEdge[];
    settings: {
        productConcurrency: number;
        maxRetries: number;
    };
};

export type WorkflowTemplate = {
    id: string;
    workflowType: string;
    title: string;
    description: string;
    spec: WorkflowTemplateSpec;
    revision?: number;
    createdAt: string;
    updatedAt: string;
};

export type WorkflowTemplateList = {
    items: WorkflowTemplate[];
    total: number;
};

export type WorkflowRun = {
    id: string;
    workflowType: string;
    templateId: string;
    templateTitle: string;
    status: WorkflowRunStatus;
    runDir: string;
    inputCount: number;
    completedCount: number;
    failedCount: number;
    error?: string;
    specSnapshot: WorkflowTemplateSpec;
    createdAt: string;
    updatedAt: string;
};

export type StartWorkflowTemplateRunRequest = {
    runId?: string;
    inputs: Array<Record<string, unknown>>;
    productConcurrency?: number;
    maxRetries?: number;
    profileId?: string;
    projectId?: string;
};

export type StartWorkflowTemplateRunResult = {
    runId: string;
    runDir: string;
};

export function fetchPDDRuns(token: string) {
    return apiGet<PDDRunList>("/api/workflows/pdd/runs", undefined, token);
}

export function fetchPDDRunOverview(runId: string, token: string) {
    return apiGet<PDDRunOverview>(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/overview`, undefined, token);
}

export function fetchPDDRunProducts(runId: string, token: string, params?: ApiParams) {
    return apiGet<PDDProductSummary[]>(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/products`, compactApiParams(params || {}), token);
}

export function fetchPDDProductDetail(runId: string, productKey: string, token: string) {
    return apiGet<PDDProductDetail>(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/product-detail`, { key: productKey }, token);
}

export function fetchPDDCreativeCanvas(runId: string, productKey: string, token: string) {
    return apiGet<PDDCreativeCanvas>(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/creative-canvas`, { key: productKey }, token);
}

export function savePDDCreativeCanvas(runId: string, productKey: string, payload: PDDCreativeCanvasSaveRequest, token: string) {
    return apiPost<PDDCreativeCanvas>(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/creative-canvas?key=${encodeURIComponent(productKey)}`, payload, token);
}

export function uploadPDDCreativeCanvasAsset(runId: string, productKey: string, payload: PDDCreativeCanvasAssetRequest, token: string) {
    return apiPost<PDDCreativeCanvasAsset>(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/creative-canvas/assets?key=${encodeURIComponent(productKey)}`, payload, token);
}

export function applyPDDCreativeCanvasOutput(runId: string, productKey: string, payload: PDDCreativeCanvasApplyRequest, token: string) {
    return apiPost<PDDManualEditResult>(`/api/workflows/pdd/runs/${encodeURIComponent(runId)}/creative-canvas/apply?key=${encodeURIComponent(productKey)}`, payload, token);
}

export function runPDDAction(payload: PDDActionRequest, token: string) {
    return apiPost<PDDActionResult>("/api/admin/workflows/pdd/actions", payload, token);
}

export function createPDDManualEdit(runId: string, payload: PDDManualEditRequest, token: string) {
    return apiPost<PDDManualEditResult>(`/api/admin/workflows/pdd/runs/${encodeURIComponent(runId)}/manual-edits`, payload, token);
}

export function applyPDDManualEdit(runId: string, editId: string, payload: PDDManualEditApplyRequest, token: string) {
    return apiPost<PDDManualEditResult>(`/api/admin/workflows/pdd/runs/${encodeURIComponent(runId)}/manual-edits/${encodeURIComponent(editId)}/apply`, payload, token);
}

export function fetchPDDWorkflowTemplates(token: string) {
    return apiGet<WorkflowTemplateList>("/api/admin/workflows/pdd/templates", undefined, token);
}

export function fetchPDDWorkflowTemplate(id: string, token: string) {
    return apiGet<WorkflowTemplate>(`/api/admin/workflows/pdd/templates/${encodeURIComponent(id)}`, undefined, token);
}

export function savePDDWorkflowTemplate(template: Partial<WorkflowTemplate>, token: string) {
    const path = template.id ? `/api/admin/workflows/pdd/templates/${encodeURIComponent(template.id)}` : "/api/admin/workflows/pdd/templates";
    return apiPost<WorkflowTemplate>(path, template, token);
}

export function deletePDDWorkflowTemplate(id: string, token: string) {
    return apiDelete<boolean>(`/api/admin/workflows/pdd/templates/${encodeURIComponent(id)}`, token);
}

export function startPDDWorkflowTemplateRun(templateId: string, payload: StartWorkflowTemplateRunRequest, token: string) {
    return apiPost<StartWorkflowTemplateRunResult>(`/api/admin/workflows/pdd/templates/${encodeURIComponent(templateId)}/runs`, payload, token);
}

export function fetchPDDWorkflowThemes(token: string) {
    return apiGet<Array<Record<string, unknown>>>("/api/admin/workflows/pdd/themes", undefined, token);
}

export function withPDDFileToken(url: string, token: string) {
    if (!url || !token) return url;
    const separator = url.includes("?") ? "&" : "?";
    return `${url}${separator}token=${encodeURIComponent(token)}`;
}
