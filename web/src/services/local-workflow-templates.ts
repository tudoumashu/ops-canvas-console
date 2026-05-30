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
        return workflowTemplateFromLocalDocument(await updateLocalTemplate(baseUrl, template.id, template.revision || current.revision, data));
    }
    return workflowTemplateFromLocalDocument(await createLocalTemplate(baseUrl, data));
}

export async function deleteLocalPDDWorkflowTemplate(baseUrl: string, id: string) {
    return deleteLocalTemplate(baseUrl, id);
}

export async function startLocalPDDWorkflowTemplateRun(baseUrl: string, templateId: string, payload: StartWorkflowTemplateRunRequest): Promise<StartWorkflowTemplateRunResult> {
    const template = await fetchLocalPDDWorkflowTemplate(baseUrl, templateId);
    const settings = template.spec.settings as WorkflowTemplateSpec["settings"] & { defaultProfileId?: string; defaultProjectId?: string; profileId?: string; projectId?: string };
    const run = await createLocalRun(baseUrl, {
        templateId,
        status: "pending",
        profileId: payload.profileId || settings.defaultProfileId || settings.profileId,
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
            executor: "not_connected",
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
        nodes: (spec.nodes || []) as unknown as Array<Record<string, unknown>>,
        edges: (spec.edges || []) as unknown as Array<Record<string, unknown>>,
        settings: { ...(spec.settings || {}) },
        metadata: { source: "web-ui" },
    };
}

function localSpecFromData(data: LocalTemplateData): WorkflowTemplateSpec {
    const settings = data.settings || {};
    return {
        version: Number(data.version || 1),
        nodes: (data.nodes || []) as unknown as WorkflowTemplateNode[],
        edges: (data.edges || []) as unknown as WorkflowTemplateEdge[],
        settings: {
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
