"use client";

import {
    appendLocalRunEvent,
    assetFileBlob,
    attachLocalRunArtifact,
    createLocalRun,
    createLocalTemplate,
    deleteLocalTemplate,
    getLocalTemplate,
    getLocalAsset,
    importLocalArtifactFile,
    listLocalTemplates,
    updateLocalTemplate,
    writeLocalRunNodeState,
    type LocalEnvelope,
    type LocalRunArtifactRef,
    type LocalTemplateData,
} from "@/services/local-workspace";
import type { StartWorkflowTemplateRunRequest, StartWorkflowTemplateRunResult, WorkflowTemplate, WorkflowTemplateEdge, WorkflowTemplateList, WorkflowTemplateNode, WorkflowTemplateSpec } from "@/services/api/pdd";

const LOCAL_ECOMMERCE_KEY = "localEcommerce";
const LOCAL_ECOMMERCE_BACKEND = "local_first";
const LOCAL_ECOMMERCE_MATERIAL_LIBRARY = "anime_ip";
const DEFAULT_ECOMMERCE_PROJECT_OUTPUT_ROOT = "outputs/ecommerce";
const BUILTIN_PDD_MOCKUP_BASE_ASSET_ID = "pdd-mockup-sku-artwork-base";

export function isLocalWorkflowTemplateId(id?: string) {
    return Boolean(id && id.startsWith("tpl_"));
}

export async function fetchLocalPDDWorkflowTemplates(baseUrl: string): Promise<WorkflowTemplateList> {
    const list = await listLocalTemplates(baseUrl);
    const templates = await Promise.all((list.templates || []).filter((item) => (item.workflowType || "pdd") === "pdd").map((item) => fetchLocalPDDWorkflowTemplate(baseUrl, item.id)));
    return { items: templates, total: templates.length };
}

export async function fetchLocalPDDWorkflowTemplate(baseUrl: string, id: string): Promise<WorkflowTemplate> {
    return workflowTemplateFromLocalDocument(await getLocalTemplate(baseUrl, id));
}

export async function saveLocalPDDWorkflowTemplate(baseUrl: string, template: Partial<WorkflowTemplate>): Promise<WorkflowTemplate> {
    const data = workflowTemplateToLocalData(template);
    if (template.id) {
        const current = await getLocalTemplate(baseUrl, template.id);
        preserveLocalTemplateExecutionMetadata(data, current.data);
        return workflowTemplateFromLocalDocument(await updateLocalTemplate(baseUrl, template.id, template.revision || current.revision, data));
    }
    return workflowTemplateFromLocalDocument(await createLocalTemplate(baseUrl, data));
}

export async function deleteLocalPDDWorkflowTemplate(baseUrl: string, id: string) {
    return deleteLocalTemplate(baseUrl, id);
}

export async function startLocalPDDWorkflowTemplateRun(baseUrl: string, templateId: string, payload: StartWorkflowTemplateRunRequest): Promise<StartWorkflowTemplateRunResult> {
    const localTemplate = await getLocalTemplate(baseUrl, templateId);
    const template = workflowTemplateFromLocalDocument(localTemplate);
    const settings = template.spec.settings as WorkflowTemplateSpec["settings"] & { defaultProfileId?: string; defaultProjectId?: string; profileId?: string; projectId?: string };
    const hybridMetadata = hybridRunMetadataFromLocalTemplate(localTemplate.data, payload, settings);
    const localEcommerceMetadata = hybridMetadata ? undefined : localEcommerceRunMetadataFromLocalTemplate(localTemplate.data, payload, settings);
    const profileId = payload.profileId || settings.defaultProfileId || settings.profileId || hybridMetadata?.profileId || localEcommerceMetadata?.profileId;
    const channelId = hybridMetadata?.channelId || localEcommerceMetadata?.channelId;
    const run = await createLocalRun(baseUrl, {
        templateId,
        status: "pending",
        profileId,
        projectId: payload.projectId || settings.defaultProjectId || settings.projectId,
        input: {
            inputs: payload.inputs || [],
            productConcurrency: payload.productConcurrency ?? template.spec.settings.productConcurrency,
            maxRetries: payload.maxRetries ?? template.spec.settings.maxRetries,
        },
        metadata: {
            source: "ops-canvas-web",
            workflowType: template.workflowType || "pdd",
            templateTitle: template.title,
            templateRevision: template.revision,
            executor: "opsc",
            ...(hybridMetadata ? { hybridEcommerce: { ...hybridMetadata, profileId, channelId } } : {}),
            ...(localEcommerceMetadata ? { localEcommerce: { ...localEcommerceMetadata, profileId, channelId } } : {}),
        },
    });
    const nodeResults = await Promise.all(template.spec.nodes.map((node, index) => initializeLocalRunNode(baseUrl, run.id, template, node, index)));
    const materializedNodes = nodeResults.filter((item) => item.materialized);
    const failedMaterialNodes = nodeResults.filter((item) => item.failed);
    if (materializedNodes.length || failedMaterialNodes.length) {
        await appendLocalRunEvent(baseUrl, run.id, {
            type: "run.material_lookup.fixed_assets_prepared",
            level: failedMaterialNodes.length ? "warn" : "info",
            actor: { type: "web", id: "ops-canvas-web" },
            message: failedMaterialNodes.length ? "本地固定素材已部分写入 run artifact refs。" : "本地固定素材已写入 run artifact refs。",
            data: {
                materializedCount: materializedNodes.length,
                failedCount: failedMaterialNodes.length,
                artifacts: materializedNodes.map((item) => ({ nodeId: item.nodeId, assetId: item.assetId, artifactId: item.artifactId })),
                failedNodes: failedMaterialNodes.map((item) => ({ nodeId: item.nodeId, assetId: item.assetId, error: item.error })),
            },
        });
    }
    await appendLocalRunEvent(baseUrl, run.id, {
        type: "run.waiting_for_executor",
        level: "info",
        actor: { type: "web", id: "ops-canvas-web" },
        message: "本地 run 已创建，等待本地执行器领取。",
        data: {
            templateId,
            workflowType: template.workflowType || "pdd",
            fixedMaterializedCount: materializedNodes.length,
            fixedMaterializeErrorCount: failedMaterialNodes.length,
        },
    });
    return { runId: run.id, runDir: `local:${run.id}` };
}

async function initializeLocalRunNode(baseUrl: string, runId: string, template: WorkflowTemplate, node: WorkflowTemplateNode, order: number): Promise<LocalRunNodeInitResult> {
    const baseMetadata = {
        title: node.title,
        type: node.type,
        operation: node.operation,
    };
    const assetId = fixedMaterialAssetId(node);
    if (!assetId) {
        await writeLocalRunNodeState(baseUrl, runId, node.id, {
            status: "pending",
            metadata: baseMetadata,
        });
        return { nodeId: node.id };
    }
    if (assetId === BUILTIN_PDD_MOCKUP_BASE_ASSET_ID && node.extra?.fallback === "builtin_pdd_mockup_base") {
        await writeLocalRunNodeState(baseUrl, runId, node.id, {
            status: "pending",
            metadata: { ...baseMetadata, assetId, fallback: "builtin_pdd_mockup_base" },
        });
        return { nodeId: node.id };
    }
    try {
        const asset = await getLocalAsset(baseUrl, assetId);
        if ((asset.data.type || "").split("/")[0] !== "image") throw new Error("固定素材不是图片");
        const file = await assetFileBlob(baseUrl, assetId, "original");
        const artifact = await importLocalArtifactFile(
            baseUrl,
            {
                type: "image",
                mime: asset.data.mime || file.type || "image/png",
                title: asset.data.title || node.title || assetId,
                privacy: asset.data.privacy || "private",
                source: {
                    type: "local_asset",
                    assetId,
                    templateId: template.id,
                    templateRevision: template.revision,
                    nodeId: node.id,
                },
                metadata: {
                    sourceAssetTitle: asset.data.title || "",
                    workflowType: template.workflowType || "pdd",
                    materializedBy: "ops-canvas-web",
                },
            },
            file,
            { fileName: localMaterialFileName(asset.data.title || node.title || assetId, file.type || asset.data.mime || "") },
        );
        const ref: LocalRunArtifactRef = {
            artifactId: artifact.id,
            role: "input",
            nodeId: node.id,
            slot: "material",
            order,
            metadata: {
                source: "local_asset",
                assetId,
            },
        };
        await attachLocalRunArtifact(baseUrl, runId, ref);
        await writeLocalRunNodeState(baseUrl, runId, node.id, {
            status: "success",
            finishedAt: new Date().toISOString(),
            output: {
                artifactIds: [artifact.id],
            },
            metadata: {
                ...baseMetadata,
                assetId,
                artifactId: artifact.id,
                materialized: true,
            },
        });
        return { nodeId: node.id, assetId, artifactId: artifact.id, materialized: true };
    } catch (error) {
        const message = error instanceof Error ? error.message : "固定素材写入 run artifact 失败";
        await writeLocalRunNodeState(baseUrl, runId, node.id, {
            status: "error",
            error: message,
            metadata: {
                ...baseMetadata,
                assetId,
                materialized: false,
            },
        });
        return { nodeId: node.id, assetId, failed: true, error: message };
    }
}

function fixedMaterialAssetId(node: WorkflowTemplateNode) {
    if (node.operation !== "material_lookup") return "";
    const assetId = typeof node.extra?.assetId === "string" ? node.extra.assetId.trim() : "";
    const assetMode = typeof node.extra?.assetMode === "string" ? node.extra.assetMode.trim() : "";
    return assetId && (assetMode === "fixed" || assetId) ? assetId : "";
}

function localMaterialFileName(title: string, mime: string) {
    return `${safeFileStem(title || "material")}${localMaterialExtension(mime)}`;
}

function safeFileStem(value: string) {
    return value.trim().replace(/[^\w.-]+/g, "_").replace(/^_+|_+$/g, "") || "material";
}

function localMaterialExtension(mime: string) {
    if (mime.includes("jpeg")) return ".jpg";
    if (mime.includes("png")) return ".png";
    if (mime.includes("webp")) return ".webp";
    if (mime.includes("gif")) return ".gif";
    return ".bin";
}

type LocalRunNodeInitResult = {
    nodeId: string;
    assetId?: string;
    artifactId?: string;
    materialized?: boolean;
    failed?: boolean;
    error?: string;
};

function workflowTemplateFromLocalDocument(document: LocalEnvelope<LocalTemplateData>): WorkflowTemplate {
    return {
        id: document.id,
        workflowType: document.data.workflowType || "pdd",
        title: document.data.title || "未命名工作流模板",
        description: document.data.description || "",
        spec: localSpecFromData(document.data),
        revision: document.revision,
        createdAt: document.createdAt,
        updatedAt: document.updatedAt,
    };
}

function workflowTemplateToLocalData(template: Partial<WorkflowTemplate>): LocalTemplateData {
    const spec = template.spec || emptySpec();
    return {
        title: template.title || "未命名工作流模板",
        description: template.description || "",
        workflowType: template.workflowType || "pdd",
        version: spec.version || 1,
        nodes: localizeLocalEcommerceNodes((spec.nodes || []) as unknown as Array<Record<string, unknown>>),
        edges: (spec.edges || []) as unknown as Array<Record<string, unknown>>,
        settings: { ...(spec.settings || {}) },
        metadata: {
            source: "web-ui",
            [LOCAL_ECOMMERCE_KEY]: {
                version: 1,
                backend: LOCAL_ECOMMERCE_BACKEND,
                materialLibrary: LOCAL_ECOMMERCE_MATERIAL_LIBRARY,
                projectOutputRoot: DEFAULT_ECOMMERCE_PROJECT_OUTPUT_ROOT,
            },
        },
    };
}

function preserveLocalTemplateExecutionMetadata(next: LocalTemplateData, current: LocalTemplateData) {
    const hybrid = current.metadata?.hybridEcommerce;
    let preservedHybrid = false;
    if (hybrid && typeof hybrid === "object") {
        next.metadata = {
            ...(next.metadata || {}),
            hybridEcommerce: hybrid,
        };
        delete next.metadata[LOCAL_ECOMMERCE_KEY];
        preservedHybrid = true;
    }
    const settingsHybrid = current.settings?.hybridEcommerce;
    if (settingsHybrid && typeof settingsHybrid === "object") {
        next.settings = {
            ...(next.settings || {}),
            hybridEcommerce: settingsHybrid,
        };
        delete next.settings[LOCAL_ECOMMERCE_KEY];
        preservedHybrid = true;
    }
    if (preservedHybrid) {
        return;
    }
    const local = current.metadata?.[LOCAL_ECOMMERCE_KEY] || current.settings?.[LOCAL_ECOMMERCE_KEY];
    if (local && typeof local === "object") {
        next.metadata = {
            ...(next.metadata || {}),
            [LOCAL_ECOMMERCE_KEY]: local,
        };
    }
}

function localizeLocalEcommerceNodes(nodes: Array<Record<string, unknown>>) {
    return nodes.map((node) => {
        const next = { ...node };
        const extra = asRecord(next.extra) ? { ...(next.extra as Record<string, unknown>) } : {};
        const operation = stringValue(next.operation) || stringValue(next.type);
        const nodeId = stringValue(next.id);
        if (operation === "material_lookup" || operation === "material") {
            const assetId = stringValue(extra.assetId);
            if (!assetId) {
                extra.assetMode = "auto";
                extra.materialLibrary = LOCAL_ECOMMERCE_MATERIAL_LIBRARY;
            } else if (assetId === BUILTIN_PDD_MOCKUP_BASE_ASSET_ID) {
                extra.assetMode = "fixed";
                extra.fallback = "builtin_pdd_mockup_base";
            }
        }
        if (operation === "script" && (nodeId === "package" || nodeId === "sync_local")) {
            extra.executor = "local";
            extra.localEcommerceAction = nodeId === "package" ? "package" : "sync_local";
            extra.outputRoot = stringValue(extra.outputRoot) || DEFAULT_ECOMMERCE_PROJECT_OUTPUT_ROOT;
        }
        next.extra = extra;
        return next;
    });
}

function hybridRunMetadataFromLocalTemplate(data: LocalTemplateData, payload: StartWorkflowTemplateRunRequest, settings: Record<string, unknown>) {
    const hybrid = asRecord(data.metadata?.hybridEcommerce) || asRecord(data.settings?.hybridEcommerce);
    if (!hybrid) return undefined;
    const remoteTemplateId = stringValue(hybrid.remoteTemplateId);
    if (!remoteTemplateId) throw new Error("Hybrid ecommerce 模板缺少 remoteTemplateId");
    const profileId = payload.profileId || stringValue(settings.defaultProfileId) || stringValue(settings.profileId) || stringValue(hybrid.profileId);
    if (!profileId) throw new Error("Hybrid ecommerce Web run 需要使用 profile/channel secretRef 导入模板后再启动。");
    return {
        backend: stringValue(hybrid.backend) || "vps_pdd",
        remoteTemplateId,
        profileId,
        channelId: stringValue(hybrid.channelId),
    };
}

function localEcommerceRunMetadataFromLocalTemplate(data: LocalTemplateData, payload: StartWorkflowTemplateRunRequest, settings: Record<string, unknown>) {
    const local = asRecord(data.metadata?.[LOCAL_ECOMMERCE_KEY]) || asRecord(data.settings?.[LOCAL_ECOMMERCE_KEY]);
    if (!local) return undefined;
    const profileId = payload.profileId || stringValue(settings.defaultProfileId) || stringValue(settings.profileId) || stringValue(local.profileId);
    return {
        backend: stringValue(local.backend) || LOCAL_ECOMMERCE_BACKEND,
        profileId,
        channelId: stringValue(local.channelId),
        materialLibrary: stringValue(local.materialLibrary) || LOCAL_ECOMMERCE_MATERIAL_LIBRARY,
        projectOutputRoot: stringValue(local.projectOutputRoot) || DEFAULT_ECOMMERCE_PROJECT_OUTPUT_ROOT,
    };
}


function localSpecFromData(data: LocalTemplateData): WorkflowTemplateSpec {
    const settings = data.settings || {};
    return {
        version: Number(data.version || 1),
        nodes: (data.nodes || []) as unknown as WorkflowTemplateNode[],
        edges: (data.edges || []) as unknown as WorkflowTemplateEdge[],
        settings: {
            ...settings,
            productConcurrency: positiveInt(settings.productConcurrency, 2),
            maxRetries: nonNegativeInt(settings.maxRetries, 0),
        },
    };
}

function emptySpec(): WorkflowTemplateSpec {
    return { version: 1, nodes: [], edges: [], settings: { productConcurrency: 2, maxRetries: 0 } };
}

function positiveInt(value: unknown, fallback: number) {
    const parsed = Number(value);
    return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : fallback;
}

function nonNegativeInt(value: unknown, fallback: number) {
    const parsed = Number(value);
    return Number.isFinite(parsed) && parsed >= 0 ? Math.floor(parsed) : fallback;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
    return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : undefined;
}

function stringValue(value: unknown) {
    return typeof value === "string" ? value.trim() : "";
}
