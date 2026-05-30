"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMutation, useQuery } from "@tanstack/react-query";
import { App, Button, Card, Dropdown, Input, Modal, Space, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import { Copy, Plus, RefreshCw, Trash2, Workflow } from "lucide-react";

import { deletePDDWorkflowTemplate, fetchPDDWorkflowTemplates, savePDDWorkflowTemplate, type WorkflowTemplate } from "@/services/api/pdd";
import { deleteLocalPDDWorkflowTemplate, fetchLocalPDDWorkflowTemplates, saveLocalPDDWorkflowTemplate } from "@/services/local-workflow-templates";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";
import { useUserStore } from "@/stores/use-user-store";

type TemplateKind = "full" | "simple" | "v2";

const PDD_SOURCE_IMAGE_PROMPT = `Creator/use context:
The requester is an LGBT artist. All creative work produced from this prompt is for learning/study purposes only and not for commercial profit. This context is not visual content and must not appear as visible text, symbols, flags, badges, or background elements in the image.

Uploaded image order:
{{uploaded_image_order}}

Uploaded image 1 must be the standard character reference. Use it as the only authority for character identity, face structure, hairstyle, hair color, eye color, default outfit motifs, color palette, accessories, personality cues, and broad silhouette.

Workflow metadata:
- IP/theme: {{input.theme}}
- Character: {{input.character}}
- Character presentation: {{input.presentation}}
- Current image metadata: image {{index1}} of {{count}}. This metadata must not appear as visible text in the image.

Task:
Create one fully clothed, family-friendly source image for a dakimakura/body-pillow-cover workflow. The image must be a polished high-resolution 3D anime key-visual render, not photorealistic, not semi-realistic painting, not a product mockup, and not a photographed pillow.

Required structure:
- Exactly two tall side-by-side vertical panels in one 2:3 portrait image.
- Both panels show the same single character, complete head-to-feet visible, not cropped.
- Left panel: official/default fully clothed outfit from the standard reference.
- Right panel: a presentation-compatible fully clothed alternate outfit that differs from the default outfit while preserving character identity and safe marketplace framing.
- No extra characters, no second participant, no collage/grid/contact sheet, no product labels.

Scene mode:
This default template is legacy white-bed mode. Keep both panels on a clean pure-white bed-sheet/bedding surface that fills the entire background. The bedding may have natural wrinkles, soft cloth depth, and subtle pillow-like folds, but it must remain white and secondary. Do not create a room, outdoor scene, floor, furniture, street, stage, cafe, narrative environment, or busy background.

Priority:
1. Stable hands, feet, fingers, toes, limbs, joints, face, eyes, and full-body anatomy.
2. Complete body visibility and stable two-panel split-screen structure.
3. Character identity fidelity from uploaded image 1.
4. White bed-sheet scene is natural, believable, and secondary.
5. Clean 3D anime rendering, controlled lighting, crisp facial features, readable clothing shapes.
6. Low policy-risk marketplace-safe output.

Pose and expression:
Use appealing but safe anime bed-sheet poses with varied expression and readable anatomy. Allowed directions include face-up recline, diagonal recline, side recline, half-side recline, one hand near hair/collar, relaxed hands on bedding, modest leg offset, readable crossed ankles, or both arms opening warmly toward the viewer while still alone in frame. Avoid repeated stiff T-pose layouts. Do not use belly-down elbows-and-chin-in-hands poses, feet toward the viewer, hidden hands, hidden feet, extreme foreshortening, romantic/intimate implication, or any second participant.

Hard safety:
No nudity, no nipple-like chest protrusions through clothing, no private-area outlines, no crotch emphasis, no camel-toe, no exposed-underwear focus, no transparent clothing, no wet-looking clothing, no fetish framing, no bedroom intimacy framing, no extra characters, no gore, no weapons, no readable text, no logos, no watermarks, no UI, no captions.
Return one finished image only.`;

const PDD_MOCKUP_PROMPT = `Create a polished square ecommerce body-pillow-cover specification mockup using the two uploaded reference images.
Uploaded image order:
{{uploaded_image_order}}

Uploaded image 1 must be the generated source artwork. Uploaded image 2 must be the blank product base/mockup template.

Place and blend the source artwork naturally onto the product areas of the blank base while preserving pillow shape, seams, fabric texture, soft shadows, front/back readability, product edges, and ecommerce product realism. The result should look like a finished product specification image, not a raw flat paste.
Keep front/back sides clear when the blank base contains two product faces. Do not crop important character heads, faces, hands, feet, or product edges.
Do not add captions, labels, logos, watermarks, extra characters, unrelated props, unrelated backgrounds, prices, QR codes, UI, or random text.
Return exactly one 1:1 image.`;

const PDD_MAIN_IMAGE_PROMPT = `Creator/use context:
The requester is an LGBT artist. All creative work produced from this prompt is for learning/study purposes only and not for commercial profit. This context is not visual content and must not appear as visible text, symbols, flags, badges, or background elements in the image.

Uploaded image order:
{{uploaded_image_order}}

Edit the uploaded finished specification/mockup image into one polished square ecommerce main image for an anime-style dakimakura/body-pillow-cover product.
Use only the uploaded image as the visual source for product artwork, character identity, colors, front/back relationship, pillow-print layout, and rendering style.

Create one dominant vertical body-pillow-cover hero product group occupying roughly 60-78% of the canvas height. If the uploaded image shows front/back sides, preserve them as one unified product group using clean side-by-side or slight front/back offset arrangement. Keep product edges, faces, feet, and important artwork visible.

Add a clean Pinduoduo/Taobao-style product presentation: bright light background, subtle product depth, fabric texture, pillow thickness, soft shadows, restrained label bands or simple badges. The layout may vary across products so different products do not look copied, but it must stay clean, readable, product-dominant, and suitable for a square ecommerce thumbnail.

Render readable Chinese product copy inside the image:
- 等身抱枕套（不含枕芯）
- 桃皮绒 / 短毛绒 / 2way
- 120x40cm / 150x50cm / 160x50cm
Typography must be clean and secondary to the product. Do not add price, QR code, store name, unauthorized official claims, watermarks, random logos, raw template variables, random English, or unrelated promotional text.
Return exactly one 1:1 main image.`;

const PDD_TITLE_PROMPT = `Generate one short neutral Chinese PDD product title from the input theme/character and uploaded product image context.
Return JSON only: {"title":"...","work_name":"...","character_name":"..."}.
The title must include "2way" and "等身抱枕套", must not exceed 28 Chinese-character weight, and must not contain punctuation, spaces, price, shop names, official/正版 claims, 限量, 现货, 包邮, 福利, offensive crowd labels, or excessive marketing words.
Use evidence priority: uploaded product image, input theme/character JSON, then common knowledge only when needed.
Prefer: work/common abbreviation + common Chinese character name + 2way等身抱枕套 + optional neutral suffix such as 二次元、动漫周边、同人周边 if length allows.
If length is tight, preserve character + 2way等身抱枕套 first and shorten the work name.`;

const PDD_SOURCE_REVIEW_PROMPT = `You are a strict practical image QA reviewer for PDD anime body-pillow source artwork.
Uploaded image order:
{{uploaded_image_order}}

Image 1 should be the candidate generated source image. Image 2, when present, should be the standard character reference.
Review only clear major/critical production blockers that should trigger automated repair: severe hands/feet/fingers/toes/face/eye/anatomy defects, extra or missing limbs, broken two-panel layout, missing full-body visibility, unsafe/suggestive clothing details, obvious character identity drift against the standard reference, or white bed-sheet mode failure.
Do not report minor style differences, slight finger ambiguity, harmless cloth wrinkles, subjective taste, normal anime simplification, or small safe visual deviations.
Return JSON only with fields: {"decision":"pass|repair","severity":"major|critical|none","issues":[],"repair_prompt":""}.`;

const PDD_MAIN_REVIEW_PROMPT = `Review the square ecommerce final main image for upload readiness.
Uploaded image order:
{{uploaded_image_order}}

Image 1 should be the generated final main image. Extra images, when present, are SKU/specification references.
Pass unless there is a clear major/critical blocker: wrong product or character, missing/damaged hero product, unreadable required Chinese copy, raw template variables, chaotic layout, severe image corruption, or unsafe/sexualized presentation.
Accept layout variation, commercial retouching, subtle decorative accents, front/back product display, mild perspective, and normal mockup lighting.
Return JSON only with fields: {"decision":"pass|repair","severity":"major|critical|none","issues":[],"repair_prompt":""}.`;

const PDD_SOURCE_REPAIR_PROMPT = `Repair the generated source artwork using the failed source image, the standard character reference, and the QA JSON.
Uploaded image order:
{{uploaded_image_order}}

Image 1 must be the failed source image and is the primary edit target. Image 2, when present, must be the standard character reference. Text input may include QA JSON.
Keep the same character, two-panel body-pillow source layout, pure-white bed-sheet/bedding background, fully clothed safe presentation, and polished 3D anime style.
Fix only clear blockers: hands, feet, fingers, toes, face, eyes, limbs, full-body crop, panel split, unsafe clothing contours, extra characters, text/logos, or major identity drift. Preserve approved visual content and avoid broad redesign unless anatomy or structure is clearly broken.`;

const PDD_MAIN_REPAIR_PROMPT = `Repair the final ecommerce main image using the approved mockup/specification image, failed main image, and QA JSON.
Uploaded image order:
{{uploaded_image_order}}

Image 1 must be the failed final main image and is the primary edit target. Image 2, when present, must be the approved SKU/specification reference. Text input may include QA JSON.
Preserve the product identity, front/back relationship, hero product dominance, required Chinese copy, clean ecommerce layout, and safe presentation.
Fix only upload-blocking issues such as missing text, unreadable text, wrong product, severe crop, layout overlap, corruption, unsafe content, or raw template variables. Do not redesign a good layout just for stylistic preference.`;

export default function PDDWorkflowTemplatesPage() {
    const { message, modal } = App.useApp();
    const router = useRouter();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const localWorkspaceStatus = useLocalWorkspaceStore((state) => state.status);
    const localWorkspace = useLocalWorkspaceStore((state) => state.workspace);
    const localWorkspaceBaseUrl = useLocalWorkspaceStore((state) => state.baseUrl);
    const useLocalTemplates = localWorkspaceStatus === "connected" && Boolean(localWorkspace);
    const [keyword, setKeyword] = useState("");
    const query = useQuery({
        queryKey: ["pdd-workflow-templates", useLocalTemplates ? "local" : "server", useLocalTemplates ? localWorkspaceBaseUrl : token],
        queryFn: () => (useLocalTemplates ? fetchLocalPDDWorkflowTemplates(localWorkspaceBaseUrl) : fetchPDDWorkflowTemplates(token || "")),
        enabled: useLocalTemplates || Boolean(token),
    });
    const createMutation = useMutation({
        mutationFn: (kind: TemplateKind) => (useLocalTemplates ? saveLocalPDDWorkflowTemplate(localWorkspaceBaseUrl, defaultTemplate(kind)) : savePDDWorkflowTemplate(defaultTemplate(kind), token || "")),
        onSuccess: (template) => {
            message.success("模板已创建");
            router.push(`/workflows/ecommerce/templates/${encodeURIComponent(template.id)}`);
        },
        onError: (error) => message.error(error instanceof Error ? error.message : "创建失败"),
    });
    const rows = (query.data?.items || []).filter((item) => !keyword.trim() || item.title.toLowerCase().includes(keyword.trim().toLowerCase()) || item.description.toLowerCase().includes(keyword.trim().toLowerCase()));
    const columns: ColumnsType<WorkflowTemplate> = [
        {
            title: "模板",
            dataIndex: "title",
            render: (_, item) => (
                <div>
                    <Link className="font-medium text-blue-600 dark:text-blue-300" href={`/workflows/ecommerce/templates/${encodeURIComponent(item.id)}`}>
                        {item.title}
                    </Link>
                    <div className="mt-1 text-xs text-stone-500">{item.description || "无描述"}</div>
                </div>
            ),
        },
        {
            title: "节点",
            width: 100,
            render: (_, item) => <Tag>{item.spec?.nodes?.length || 0}</Tag>,
        },
        {
            title: "连线",
            width: 100,
            render: (_, item) => <Tag>{item.spec?.edges?.length || 0}</Tag>,
        },
        {
            title: "更新时间",
            dataIndex: "updatedAt",
            width: 210,
            render: (value: string) => <span className="font-mono text-xs">{value || "-"}</span>,
        },
        {
            title: "操作",
            width: 150,
            render: (_, item) => (
                <Space>
                    <Button
                        size="small"
                        icon={<Copy className="size-3.5" />}
                        onClick={() =>
                            void (useLocalTemplates ? saveLocalPDDWorkflowTemplate(localWorkspaceBaseUrl, { ...item, id: "", title: `${item.title} 副本` }) : savePDDWorkflowTemplate({ ...item, id: "", title: `${item.title} 副本` }, token || ""))
                                .then(() => {
                                    message.success("已复制");
                                    void query.refetch();
                                })
                                .catch((error) => message.error(error instanceof Error ? error.message : "复制失败"))
                        }
                    />
                    <Button
                        danger
                        size="small"
                        icon={<Trash2 className="size-3.5" />}
                        onClick={() =>
                            modal.confirm({
                                title: "删除模板",
                                content: `确认删除「${item.title}」？`,
                                okButtonProps: { danger: true },
                                onOk: async () => {
                                    if (useLocalTemplates) await deleteLocalPDDWorkflowTemplate(localWorkspaceBaseUrl, item.id);
                                    else await deletePDDWorkflowTemplate(item.id, token || "");
                                    message.success("已删除");
                                    await query.refetch();
                                },
                            })
                        }
                    />
                </Space>
            ),
        },
    ];

    if (!useLocalTemplates && (!token || !user)) {
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

    return (
        <main className="h-full overflow-auto bg-background text-foreground">
            <div className="mx-auto flex w-full max-w-7xl flex-col gap-5 px-6 py-8">
                <header className="flex flex-wrap items-end justify-between gap-4 border-b border-stone-200 pb-5 dark:border-stone-800">
                    <div>
                        <Typography.Text type="secondary" className="text-xs">
                            {useLocalTemplates ? `Local Workspace / ${localWorkspace?.name || "Templates"}` : "Ecommerce / Templates"}
                        </Typography.Text>
                        <Typography.Title level={2} className="!mb-0 !mt-2">
                            电商工作流模板
                        </Typography.Title>
                    </div>
                    <Space wrap>
                        <Input.Search allowClear placeholder="搜索模板" value={keyword} onChange={(event) => setKeyword(event.target.value)} className="w-[260px]" />
                        <Button icon={<RefreshCw className="size-4" />} loading={query.isFetching} onClick={() => void query.refetch()}>
                            刷新
                        </Button>
                        <Dropdown
                            menu={{
                                items: [
                                    { key: "v2", label: "新建 v2 无代码模板" },
                                    { key: "full", label: "新建复杂模板" },
                                    { key: "simple", label: "新建简化模板" },
                                ],
                                onClick: ({ key }) => createMutation.mutate(key as TemplateKind),
                            }}
                        >
                            <Button type="primary" icon={<Plus className="size-4" />} loading={createMutation.isPending}>
                                新建模板
                            </Button>
                        </Dropdown>
                        <Button icon={<Workflow className="size-4" />} href="/workflows/ecommerce">
                            返回运行列表
                        </Button>
                    </Space>
                </header>
                <Table rowKey="id" columns={columns} dataSource={rows} loading={query.isLoading} pagination={{ pageSize: 20, showSizeChanger: true }} />
            </div>
        </main>
    );
}

function defaultTemplate(kind: TemplateKind): Partial<WorkflowTemplate> {
    if (kind === "v2") return defaultV2Template();
    return kind === "simple" ? defaultSimpleTemplate() : defaultFullTemplate();
}

function defaultV2Template(): Partial<WorkflowTemplate> {
    return {
        title: "电商商品主图模板 v2",
        description: "无代码版主链路：输入商品、匹配素材、源图生成内置质检修复、Mockup、最终主图内置复检修复、产物打包、同步本地/可选上传。",
	        spec: {
	            version: 1,
	            settings: { productConcurrency: 2, maxRetries: 3 },
	            nodes: [inputNode(), referenceNode(), sourceNode(440, 200, sourceGuardrail()), mockupBaseNode(800, 500), mockupNode(1160, 250, mockupGuardrail()), mainNode(1520, 250, mainGuardrail()), packageNode(1880, 250, "source", "main"), syncLocalNode(2240, 250)],
	            edges: [
	                { id: "input-source", from: "input", to: "source" },
	                { id: "reference-source", from: "reference", to: "source", inputOrder: 1, inputAlias: "standard_reference", fileSelector: "first" },
	                { id: "source-mockup", from: "source", to: "mockup", inputOrder: 1, inputAlias: "source_artwork", fileSelector: "first" },
	                { id: "mockup-base-mockup", from: "mockup_base", to: "mockup", inputOrder: 2, inputAlias: "blank_mockup_base", fileSelector: "first" },
	                { id: "mockup-main", from: "mockup", to: "main", inputOrder: 1, inputAlias: "sku_spec", fileSelector: "first" },
                { id: "source-package", from: "source", to: "package" },
                { id: "mockup-package", from: "mockup", to: "package" },
                { id: "main-package", from: "main", to: "package" },
                { id: "package-sync-local", from: "package", to: "sync_local" },
            ],
        },
    };
}

function defaultSimpleTemplate(): Partial<WorkflowTemplate> {
    return {
        title: "电商商品主图简化模板",
        description: "输入 theme/character 后，跳过质检、修复和标题生成，直接生成源图、Mockup、主图、打包并同步本地。",
	        spec: {
	            version: 1,
	            settings: { productConcurrency: 2, maxRetries: 3 },
	            nodes: [inputNode(), referenceNode(), sourceNode(440, 200), mockupBaseNode(800, 500), mockupNode(1160, 250), mainNode(1520, 250), packageNode(1880, 250, "source", "main"), syncLocalNode(2240, 250)],
	            edges: [
	                { id: "input-source", from: "input", to: "source" },
	                { id: "reference-source", from: "reference", to: "source", inputOrder: 1, inputAlias: "standard_reference", fileSelector: "first" },
	                { id: "source-mockup", from: "source", to: "mockup", inputOrder: 1, inputAlias: "source_artwork", fileSelector: "first" },
	                { id: "mockup-base-mockup", from: "mockup_base", to: "mockup", inputOrder: 2, inputAlias: "blank_mockup_base", fileSelector: "first" },
	                { id: "mockup-main", from: "mockup", to: "main", inputOrder: 1, inputAlias: "sku_spec", fileSelector: "first" },
                { id: "source-package", from: "source", to: "package" },
                { id: "mockup-package", from: "mockup", to: "package" },
                { id: "main-package", from: "main", to: "package" },
                { id: "package-sync-local", from: "package", to: "sync_local" },
            ],
        },
    };
}

function defaultFullTemplate(): Partial<WorkflowTemplate> {
    return {
        title: "电商商品主图模板",
        description: "输入 theme/character 后，按素材、源图、质检修复、Mockup、标题、主图、复检修复、打包和同步节点批量生成 PDD 商品产物。",
        spec: {
            version: 1,
            settings: { productConcurrency: 1, maxRetries: 3 },
            nodes: [
                inputNode(),
                referenceNode(),
                sourceNode(440, 200),
                { id: "current_source", type: "image", title: "当前源图", x: 760, y: 210, width: 280, height: 160, operation: "image_select", prompt: "", count: 1, outputMappings: [], extra: { selectMode: "last" } },
                textGenerationNode("source_review", "源图质检", 1120, 80, "JSON", PDD_SOURCE_REVIEW_PROMPT, { outputFormat: "json" }),
                {
                    id: "source_decision",
                    type: "text",
                    title: "源图判定",
                    x: 1480,
                    y: 80,
                    width: 280,
                    height: 160,
                    operation: "condition",
                    prompt: "",
                    count: 1,
                    outputMappings: [],
                    extra: { defaultDecision: "pass", conditions: [{ path: "decision", operator: "eq", value: "repair", output: "repair" }] },
                },
                imageEditNode(
                    "source_repair",
                    "源图修复",
                    1120,
                    390,
                    "2:3",
                    "",
                    PDD_SOURCE_REPAIR_PROMPT,
                    [],
                ),
                mockupBaseNode(1840, 500),
                mockupNode(2200, 250),
                textGenerationNode("title", "标题生成", 1840, 60, "JSON", PDD_TITLE_PROMPT, { outputFormat: "json", titleProvider: true }),
                mainNode(2560, 250),
                { id: "current_main", type: "image", title: "当前主图", x: 2920, y: 260, width: 280, height: 160, operation: "image_select", prompt: "", count: 1, outputMappings: [], extra: { selectMode: "last" } },
                textGenerationNode("main_review", "主图质检", 3280, 120, "JSON", PDD_MAIN_REVIEW_PROMPT, { outputFormat: "json" }),
                {
                    id: "main_decision",
                    type: "text",
                    title: "主图判定",
                    x: 3640,
                    y: 120,
                    width: 280,
                    height: 160,
                    operation: "condition",
                    prompt: "",
                    count: 1,
                    outputMappings: [],
                    extra: { defaultDecision: "pass", conditions: [{ path: "decision", operator: "eq", value: "repair", output: "repair" }] },
                },
                imageEditNode(
                    "main_repair",
                    "主图修复",
                    3280,
                    430,
                    "1:1",
                    "high",
                    PDD_MAIN_REPAIR_PROMPT,
                    [],
                ),
                packageNode(4000, 250, "current_source", "current_main"),
                syncLocalNode(4360, 250),
            ],
	            edges: [
	                { id: "input-source", from: "input", to: "source" },
	                { id: "reference-source", from: "reference", to: "source", inputOrder: 1, inputAlias: "standard_reference", fileSelector: "first" },
	                { id: "source-current", from: "source", to: "current_source", inputOrder: 1, inputAlias: "generated_source", fileSelector: "last" },
	                { id: "source-repair-current", from: "source_repair", to: "current_source", inputOrder: 2, inputAlias: "repaired_source", fileSelector: "last", loop: { enabled: true, maxIterations: 5 } },
	                { id: "reference-source-review", from: "reference", to: "source_review", inputOrder: 2, inputAlias: "standard_reference", fileSelector: "first" },
	                { id: "current-source-review", from: "current_source", to: "source_review", inputOrder: 1, inputAlias: "candidate_source", fileSelector: "first" },
	                { id: "source-review-decision", from: "source_review", to: "source_decision" },
	                { id: "reference-source-repair", from: "reference", to: "source_repair", inputOrder: 2, inputAlias: "standard_reference", fileSelector: "first" },
	                { id: "current-source-repair", from: "current_source", to: "source_repair", inputOrder: 1, inputAlias: "failed_source", fileSelector: "first" },
	                { id: "source-review-repair", from: "source_review", to: "source_repair" },
	                { id: "source-decision-repair", from: "source_decision", to: "source_repair", fromHandle: "repair" },
	                { id: "source-decision-mockup", from: "source_decision", to: "mockup", fromHandle: "pass" },
	                { id: "current-source-mockup", from: "current_source", to: "mockup", inputOrder: 1, inputAlias: "source_artwork", fileSelector: "first" },
	                { id: "mockup-base-mockup", from: "mockup_base", to: "mockup", inputOrder: 2, inputAlias: "blank_mockup_base", fileSelector: "first" },
	                { id: "input-title", from: "input", to: "title" },
	                { id: "mockup-title", from: "mockup", to: "title", inputOrder: 1, inputAlias: "product_spec", fileSelector: "first" },
	                { id: "mockup-main", from: "mockup", to: "main", inputOrder: 1, inputAlias: "sku_spec", fileSelector: "first" },
	                { id: "title-main", from: "title", to: "main" },
	                { id: "main-current", from: "main", to: "current_main", inputOrder: 1, inputAlias: "generated_main", fileSelector: "last" },
	                { id: "main-repair-current", from: "main_repair", to: "current_main", inputOrder: 2, inputAlias: "repaired_main", fileSelector: "last", loop: { enabled: true, maxIterations: 5 } },
	                { id: "mockup-main-review", from: "mockup", to: "main_review", inputOrder: 2, inputAlias: "sku_reference", fileSelector: "first" },
	                { id: "current-main-review", from: "current_main", to: "main_review", inputOrder: 1, inputAlias: "candidate_main", fileSelector: "first" },
	                { id: "main-review-decision", from: "main_review", to: "main_decision" },
	                { id: "mockup-main-repair", from: "mockup", to: "main_repair", inputOrder: 2, inputAlias: "sku_reference", fileSelector: "first" },
	                { id: "current-main-repair", from: "current_main", to: "main_repair", inputOrder: 1, inputAlias: "failed_main", fileSelector: "first" },
                { id: "main-review-repair", from: "main_review", to: "main_repair" },
                { id: "main-decision-repair", from: "main_decision", to: "main_repair", fromHandle: "repair" },
                { id: "main-decision-package", from: "main_decision", to: "package", fromHandle: "pass" },
                { id: "title-package", from: "title", to: "package" },
                { id: "current-source-package", from: "current_source", to: "package" },
                { id: "mockup-package", from: "mockup", to: "package" },
                { id: "current-main-package", from: "current_main", to: "package" },
                { id: "package-sync-local", from: "package", to: "sync_local" },
            ],
        },
    };
}

function inputNode() {
    return { id: "input", type: "text" as const, title: "输入主题", x: 80, y: 330, width: 280, height: 150, operation: "input" as const, prompt: "", count: 1, outputMappings: [] };
}

function referenceNode() {
    return { id: "reference", type: "material" as const, title: "素材库参考图", x: 80, y: 80, width: 280, height: 150, operation: "material_lookup" as const, prompt: "", count: 1, outputMappings: [] };
}

function mockupBaseNode(x: number, y: number) {
    return {
        id: "mockup_base",
        type: "material" as const,
        title: "规格图模板素材",
        x,
        y,
        width: 300,
        height: 160,
        operation: "material_lookup" as const,
        prompt: "",
        count: 1,
        outputMappings: [],
        extra: { assetMode: "fixed", assetId: "pdd-mockup-sku-artwork-base" },
    };
}

function sourceNode(x: number, y: number, extra?: Record<string, unknown>) {
    return imageEditNode(
        "source",
        "源图生成",
        x,
        y,
        "2:3",
        "high",
        PDD_SOURCE_IMAGE_PROMPT,
        [],
        extra,
    );
}

function mockupNode(x: number, y: number, extra?: Record<string, unknown>) {
    return imageEditNode(
        "mockup",
        "Mockup生成",
        x,
        y,
        "1:1",
        "high",
        PDD_MOCKUP_PROMPT,
        [],
        extra,
    );
}

function mainNode(x: number, y: number, extra?: Record<string, unknown>) {
    return imageEditNode(
        "main",
        "最终主图",
        x,
        y,
        "1:1",
        "high",
        PDD_MAIN_IMAGE_PROMPT,
        [],
        extra,
    );
}

function imageEditNode(id: string, title: string, x: number, y: number, size: string, quality: string, prompt: string, outputMappings: NonNullable<WorkflowTemplate["spec"]>["nodes"][number]["outputMappings"], extra?: Record<string, unknown>) {
    return { id, type: "image" as const, title, x, y, width: 320, height: 190, operation: "image_edit" as const, model: "gpt-image-2", prompt, count: 1, size, quality, outputMappings, extra };
}

function sourceGuardrail() {
    return {
        guardrail: {
            enabled: true,
            preset: "pdd_source",
            failureStrategy: "regenerate",
            review: { enabled: true, model: "gpt-5.5", strictness: "strict", minorPolicy: "record_only" },
            repair: { enabled: true, model: "gpt-image-2", maxRounds: 5, includeReferenceImages: true },
            regenerate: { maxRounds: 2 },
            transientRetry: { maxAttempts: 100 },
        },
    };
}

function mockupGuardrail() {
    return {
        guardrail: {
            enabled: true,
            preset: "pdd_mockup",
            failureStrategy: "manual_review",
            review: { enabled: true, model: "gpt-5.5", strictness: "basic", minorPolicy: "record_only" },
            repair: { enabled: false, model: "gpt-image-2", maxRounds: 0, includeReferenceImages: true },
            transientRetry: { maxAttempts: 100 },
        },
    };
}

function mainGuardrail() {
    return {
        guardrail: {
            enabled: true,
            preset: "pdd_main",
            failureStrategy: "manual_review",
            review: { enabled: true, model: "gpt-5.5", strictness: "strict", minorPolicy: "record_only" },
            repair: { enabled: true, model: "gpt-image-2", maxRounds: 5, includeReferenceImages: true },
            transientRetry: { maxAttempts: 100 },
        },
    };
}

function textGenerationNode(id: string, title: string, x: number, y: number, format: string, prompt: string, extra: Record<string, unknown>) {
    return { id, type: "text" as const, title, x, y, width: 320, height: 180, operation: "text_generation" as const, model: "gpt-5.5", prompt, count: 1, outputMappings: [], extra: { ...extra, outputFormat: format.toLowerCase() } };
}

function packageNode(x: number, y: number, sourceNodeId: string, mainNodeId: string) {
    return {
        id: "package",
        type: "text" as const,
        title: "产物打包",
        x,
        y,
        width: 320,
        height: 190,
        operation: "script" as const,
        prompt: "",
        count: 1,
        outputMappings: [],
        extra: {
            executor: "vps",
            scriptPath: "scripts/package_custom_workflow_product.py",
            timeoutSeconds: 600,
            args: JSON.stringify([
                "--run-dir",
                "{{runDir}}",
                "--source-file",
                `{{node.${sourceNodeId}.first_file}}`,
                "--mockup-file",
                "{{node.mockup.first_file}}",
                "--main-file",
                `{{node.${mainNodeId}.first_file}}`,
                "--source-dir-name",
                "{{sourceTitle}}",
                "--product-title",
                "{{productTitle}}",
            ]),
        },
    };
}

function syncLocalNode(x: number, y: number) {
    return {
        id: "sync_local",
        type: "text" as const,
        title: "同步本地/可选上传",
        x,
        y,
        width: 340,
        height: 190,
        operation: "script" as const,
        prompt: "",
        count: 1,
        outputMappings: [],
        extra: {
            executor: "vps",
            scriptPath: "scripts/trigger_local_receive_and_upload.sh",
            timeoutSeconds: 1800,
            args: JSON.stringify(["--run-id", "{{runId}}"]),
        },
    };
}
