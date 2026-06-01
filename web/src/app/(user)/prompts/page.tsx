"use client";

import { Edit3, FolderPlus, Search, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { type UIEvent, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Alert, App, Button, Empty, Form, Input, Modal, Select, Spin, Tabs } from "antd";

import { PromptCard } from "@/components/prompts/prompt-card";
import { PromptDetailDialog } from "@/components/prompts/prompt-detail-dialog";
import { usePromptList } from "@/components/prompts/use-prompt-list";
import { LocalWorkspaceStatusAlert } from "@/components/local-workspace/local-workspace-status-alert";
import { useCopyText } from "@/hooks/use-copy-text";
import { cn } from "@/lib/utils";
import { deleteAdminPrompt, saveAdminPrompt } from "@/services/api/admin";
import { ALL_PROMPTS_OPTION, type Prompt } from "@/services/api/prompts";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";
import { usePromptStore, type MyPrompt } from "@/stores/use-prompt-store";
import { useUserStore } from "@/stores/use-user-store";

type PromptCenterTab = "library" | "mine";
type MediaFilter = "" | "image" | "text" | "video";

type PromptFormValues = {
    title: string;
    prompt: string;
    coverUrl?: string;
    tags?: string[];
    domain?: string;
    stage?: string;
    source?: string;
    note?: string;
};

const mediaFilterOptions = [
    { label: "全部", value: "" },
    { label: "图片", value: "image" },
    { label: "文本", value: "text" },
    { label: "视频", value: "video" },
];

const promptPurposeOptions = [
    { label: "全部用途", value: "", media: "" },
    { label: "通用", value: "general", media: "" },
    { label: "图片修复", value: "repair", media: "image" },
    { label: "电商主图", value: "main_image", media: "image" },
    { label: "电商规格图", value: "spec_image", media: "image" },
    { label: "图片质检", value: "quality_review", media: "text" },
];

export default function PromptsPage() {
    const { message } = App.useApp();
    const copyText = useCopyText();
    const queryClient = useQueryClient();
    const [form] = Form.useForm<PromptFormValues>();
    const [publicForm] = Form.useForm<PromptFormValues>();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const isAdmin = user?.role === "admin" && Boolean(token);
    const prompts = usePromptStore((state) => state.prompts);
    const promptsWorkspaceLoaded = usePromptStore((state) => state.workspaceLoaded);
    const promptsLoadedWorkspaceId = usePromptStore((state) => state.loadedWorkspaceId);
    const promptsLoading = usePromptStore((state) => state.loading);
    const promptsError = usePromptStore((state) => state.lastError);
    const loadPromptsFromWorkspace = usePromptStore((state) => state.loadFromWorkspace);
    const addPrompt = usePromptStore((state) => state.addPrompt);
    const updatePrompt = usePromptStore((state) => state.updatePrompt);
    const removePrompt = usePromptStore((state) => state.removePrompt);
    const localWorkspaceStatus = useLocalWorkspaceStore((state) => state.status);
    const localWorkspace = useLocalWorkspaceStore((state) => state.workspace);
    const localWorkspaceId = localWorkspace?.id || "";

    const [activeTab, setActiveTab] = useState<PromptCenterTab>("mine");
    const [libraryKeyword, setLibraryKeyword] = useState("");
    const [libraryTags, setLibraryTags] = useState<string[]>([]);
    const [libraryMedia, setLibraryMedia] = useState<MediaFilter>("");
    const [libraryPurpose, setLibraryPurpose] = useState("");
    const [librarySource, setLibrarySource] = useState(ALL_PROMPTS_OPTION);
    const [mineKeyword, setMineKeyword] = useState("");
    const [mineTags, setMineTags] = useState<string[]>([]);
    const [mineMedia, setMineMedia] = useState<MediaFilter>("");
    const [minePurpose, setMinePurpose] = useState("");
    const [mineSource, setMineSource] = useState("");
    const [selectedPrompt, setSelectedPrompt] = useState<Prompt | null>(null);
    const [editingPrompt, setEditingPrompt] = useState<MyPrompt | null>(null);
    const [promptFormOpen, setPromptFormOpen] = useState(false);
    const [editingPublicPrompt, setEditingPublicPrompt] = useState<Prompt | null>(null);
    const [publicPromptFormOpen, setPublicPromptFormOpen] = useState(false);

    const {
        query,
        items: promptItems,
        tags: promptTags,
        facets,
        total: totalPrompts,
    } = usePromptList({
        keyword: libraryKeyword,
        tags: libraryTags,
        category: librarySource,
        domain: libraryMedia,
        stage: libraryPurpose,
    });

    const minePromptsReady = localWorkspaceStatus === "connected" && promptsWorkspaceLoaded && promptsLoadedWorkspaceId === localWorkspaceId;
    const minePrompts = minePromptsReady ? prompts : [];
    const filteredMinePrompts = useMemo(() => {
        const queryText = mineKeyword.trim().toLowerCase();
        return minePrompts.filter((prompt) => {
            if (mineMedia && prompt.domain !== mineMedia) return false;
            if (minePurpose && prompt.stage !== minePurpose) return false;
            if (mineSource && prompt.source !== mineSource) return false;
            if (mineTags.length && !mineTags.every((tag) => (prompt.tags || []).includes(tag))) return false;
            if (!queryText) return true;
            return [prompt.title, prompt.prompt, prompt.source, prompt.note || "", (prompt.tags || []).join(" ")].join(" ").toLowerCase().includes(queryText);
        });
    }, [mineKeyword, mineMedia, minePrompts, minePurpose, mineSource, mineTags]);
    const mineAvailableTags = useMemo(() => sortedUnique(minePrompts.filter((prompt) => !mineMedia || prompt.domain === mineMedia).flatMap((prompt) => prompt.tags || [])), [mineMedia, minePrompts]);
    const mineAvailableStages = useMemo(
        () =>
            sortedUnique(
                minePrompts
                    .filter((prompt) => !mineMedia || prompt.domain === mineMedia)
                    .map((prompt) => prompt.stage)
                    .filter(Boolean),
            ),
        [mineMedia, minePrompts],
    );
    const mineSourceOptions = useMemo(() => [{ label: "全部来源", value: "" }, ...sortedUnique(minePrompts.map((prompt) => prompt.source)).map((source) => ({ label: promptSourceLabel(source), value: source }))], [minePrompts]);
    const librarySourceOptions = useMemo(() => [{ label: "全部来源", value: ALL_PROMPTS_OPTION }, ...(facets?.categories || []).map((category) => ({ label: promptSourceLabel(category), value: category }))], [facets?.categories]);

    useEffect(() => {
        if (query.isError) message.error(query.error instanceof Error ? query.error.message : "获取提示词失败");
    }, [message, query.error, query.isError]);

    useEffect(() => {
        if (localWorkspaceStatus === "connected" && localWorkspaceId) void loadPromptsFromWorkspace();
    }, [loadPromptsFromWorkspace, localWorkspaceId, localWorkspaceStatus]);

    const savePromptToMine = async (item: Prompt) => {
        const id = await addPrompt({
            title: item.title,
            prompt: item.prompt,
            coverUrl: item.coverUrl,
            tags: item.tags || [],
            domain: item.domain || "image",
            stage: item.stage || "general",
            source: item.category || "prompt-library",
            metadata: { source: "prompt-library", promptId: item.id, githubUrl: item.githubUrl, model: item.model, mode: item.mode },
        });
        if (!id) {
            message.error(usePromptStore.getState().lastError || "请先连接本地工作区");
            return;
        }
        message.success("已加入我的提示词");
    };

    const openCreateMinePrompt = () => {
        setEditingPrompt(null);
        form.setFieldsValue({ title: "", prompt: "", coverUrl: "", tags: [], domain: "image", stage: "general", source: "手动添加", note: "" });
        setPromptFormOpen(true);
    };

    const openEditMinePrompt = (prompt: MyPrompt) => {
        setEditingPrompt(prompt);
        form.setFieldsValue({ title: prompt.title, prompt: prompt.prompt, coverUrl: prompt.coverUrl, tags: prompt.tags, domain: prompt.domain, stage: prompt.stage, source: prompt.source, note: prompt.note });
        setPromptFormOpen(true);
    };

    const saveMinePrompt = async () => {
        const values = await form.validateFields();
        const payload = {
            title: values.title.trim(),
            prompt: values.prompt.trim(),
            coverUrl: values.coverUrl?.trim(),
            tags: values.tags || [],
            domain: normalizePromptDomain(values.domain),
            stage: values.stage || "general",
            source: values.source?.trim() || "手动添加",
            note: values.note?.trim(),
            metadata: editingPrompt?.metadata || { source: "manual" },
        };
        if (editingPrompt) {
            await updatePrompt(editingPrompt.id, payload);
        } else {
            const id = await addPrompt(payload);
            if (!id) {
                message.error(usePromptStore.getState().lastError || "请先连接本地工作区");
                return;
            }
        }
        message.success(editingPrompt ? "提示词已更新" : "提示词已保存");
        setPromptFormOpen(false);
    };

    const deleteMinePrompt = async (id: string) => {
        await removePrompt(id);
        const error = usePromptStore.getState().lastError;
        if (error) message.error(error);
        else message.success("提示词已删除");
    };

    const openCreatePublicPrompt = () => {
        setEditingPublicPrompt(null);
        publicForm.setFieldsValue({ title: "", prompt: "", coverUrl: "", tags: [], domain: "image", stage: "general", source: "manual-prompts", note: "" });
        setPublicPromptFormOpen(true);
    };

    const openEditPublicPrompt = (prompt: Prompt) => {
        setEditingPublicPrompt(prompt);
        publicForm.setFieldsValue({
            title: prompt.title,
            prompt: prompt.prompt,
            coverUrl: prompt.coverUrl,
            tags: prompt.tags || [],
            domain: prompt.domain || "image",
            stage: prompt.stage || "general",
            source: prompt.category || "manual-prompts",
            note: "",
        });
        setPublicPromptFormOpen(true);
    };

    const savePublicPrompt = async () => {
        if (!token) return message.error("请先登录管理员账号");
        const values = await publicForm.validateFields();
        const domain = normalizePromptDomain(values.domain);
        const stage = values.stage || "general";
        await saveAdminPrompt(token, {
            ...(editingPublicPrompt || {}),
            title: values.title.trim(),
            prompt: values.prompt.trim(),
            coverUrl: values.coverUrl?.trim() || "/logo.svg",
            tags: values.tags || [],
            category: values.source?.trim() || editingPublicPrompt?.category || "manual-prompts",
            domain,
            stage,
            provider: editingPublicPrompt?.provider || defaultProviderForDomain(domain),
            model: editingPublicPrompt?.model || defaultModelForDomain(domain),
            mode: editingPublicPrompt?.mode || "general",
            inputType: inputTypeForPrompt(domain, stage),
            outputType: outputTypeForPrompt(domain, stage),
            status: editingPublicPrompt?.status || "production",
        });
        await queryClient.invalidateQueries({ queryKey: ["prompts"] });
        setPublicPromptFormOpen(false);
        message.success(editingPublicPrompt ? "公共提示词已更新" : "公共提示词已新增");
    };

    const deletePublicPrompt = async (prompt: Prompt) => {
        if (!token) return message.error("请先登录管理员账号");
        Modal.confirm({
            title: "删除公共提示词",
            content: `确定删除「${prompt.title}」吗？`,
            okText: "删除",
            okButtonProps: { danger: true },
            cancelText: "取消",
            onOk: async () => {
                await deleteAdminPrompt(token, prompt.id);
                await queryClient.invalidateQueries({ queryKey: ["prompts"] });
                message.success("公共提示词已删除");
            },
        });
    };

    const resetLibraryFilters = () => {
        setLibraryKeyword("");
        setLibraryTags([]);
        setLibraryMedia("");
        setLibraryPurpose("");
        setLibrarySource(ALL_PROMPTS_OPTION);
    };

    const resetMineFilters = () => {
        setMineKeyword("");
        setMineTags([]);
        setMineMedia("");
        setMinePurpose("");
        setMineSource("");
    };

    const handleListScroll = (event: UIEvent<HTMLDivElement>) => {
        if (activeTab !== "library") return;
        const target = event.currentTarget;
        if (query.hasNextPage && !query.isFetchingNextPage && target.scrollTop + target.clientHeight >= target.scrollHeight - 160) void query.fetchNextPage();
    };

    return (
        <div className="flex h-full flex-col overflow-hidden bg-background text-stone-800 dark:text-stone-100">
            <main
                className="min-h-0 flex-1 overflow-y-auto bg-background bg-[radial-gradient(#e5e7eb_1px,transparent_1px)] px-6 py-8 [background-size:16px_16px] dark:bg-[radial-gradient(rgba(245,245,244,.16)_1px,transparent_1px)]"
                onScroll={handleListScroll}
            >
                <div className="pb-8">
                    <div className="mx-auto max-w-5xl text-center">
                        <h1 className="text-4xl font-semibold tracking-tight text-stone-950 dark:text-stone-100">提示词中心</h1>
                        <p className="mt-3 text-sm text-stone-500 dark:text-stone-400">统一管理公共提示词库和本地工作区提示词，按用途、来源和自由标签快速查找。</p>
                    </div>
                </div>
                <div className="mx-auto max-w-7xl">
                    <Tabs
                        activeKey={activeTab}
                        onChange={(key) => setActiveTab(key as PromptCenterTab)}
                        items={[
                            {
                                key: "mine",
                                label: "我的提示词",
                                children: (
                                    <div className="space-y-4">
                                        <LocalWorkspaceStatusAlert message="我的提示词现在以本地工作区为事实源" />
                                        {promptsError && localWorkspaceStatus === "connected" ? <Alert type="error" showIcon title={promptsError} /> : null}
                                        {localWorkspace ? <Alert type="info" showIcon title={`当前工作区：${localWorkspace.name}`} /> : null}
                                        <PromptLibrarySection
                                            keyword={mineKeyword}
                                            setKeyword={setMineKeyword}
                                            media={mineMedia}
                                            setMedia={(value) => {
                                                setMineMedia(value);
                                                setMinePurpose("");
                                            }}
                                            purpose={minePurpose}
                                            setPurpose={setMinePurpose}
                                            source={mineSource}
                                            setSource={setMineSource}
                                            sourceOptions={mineSourceOptions}
                                            tags={mineTags}
                                            setTags={setMineTags}
                                            tagOptions={mineAvailableTags}
                                            availableStages={mineAvailableStages}
                                            countText={`当前可选：${promptPurposeOptionsForMedia(mineMedia, mineAvailableStages).length - 1} 个用途、${mineAvailableTags.length} 个自由标签`}
                                            onReset={resetMineFilters}
                                            loading={promptsLoading}
                                            items={filteredMinePrompts.map(myPromptToPrompt)}
                                            total={filteredMinePrompts.length}
                                            onOpen={setSelectedPrompt}
                                            onCopy={(prompt) => copyText(prompt.prompt, "提示词已复制")}
                                            extraAction={(prompt) => (
                                                <>
                                                    <Button size="small" onClick={() => openEditMinePrompt(prompt.metadata?.localPrompt as MyPrompt)}>
                                                        编辑
                                                    </Button>
                                                    <Button size="small" danger icon={<Trash2 className="size-3.5" />} onClick={() => void deleteMinePrompt(prompt.id)}>
                                                        删除
                                                    </Button>
                                                </>
                                            )}
                                            headerAction={
                                                <button
                                                    type="button"
                                                    className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300"
                                                    onClick={openCreateMinePrompt}
                                                >
                                                    新增提示词
                                                </button>
                                            }
                                        />
                                    </div>
                                ),
                            },
                            {
                                key: "library",
                                label: "提示词库",
                                children: (
                                    <PromptLibrarySection
                                        keyword={libraryKeyword}
                                        setKeyword={setLibraryKeyword}
                                        media={libraryMedia}
                                        setMedia={(value) => {
                                            setLibraryMedia(value);
                                            setLibraryPurpose("");
                                        }}
                                        purpose={libraryPurpose}
                                        setPurpose={setLibraryPurpose}
                                        source={librarySource}
                                        setSource={setLibrarySource}
                                        sourceOptions={librarySourceOptions}
                                        tags={libraryTags}
                                        setTags={setLibraryTags}
                                        tagOptions={promptTags.filter((tag) => tag !== ALL_PROMPTS_OPTION)}
                                        availableStages={facets?.stages || []}
                                        countText={`当前可选：${promptPurposeOptionsForMedia(libraryMedia, facets?.stages || []).length - 1} 个用途、${promptTags.length > 0 ? promptTags.length - 1 : 0} 个自由标签`}
                                        onReset={resetLibraryFilters}
                                        loading={query.isLoading}
                                        items={promptItems}
                                        total={totalPrompts}
                                        onOpen={setSelectedPrompt}
                                        onCopy={(prompt) => copyText(prompt.prompt, "提示词已复制")}
                                        extraAction={(prompt) => (
                                            <>
                                                <Button size="small" icon={<FolderPlus className="size-3.5" />} onClick={() => void savePromptToMine(prompt)}>
                                                    加入我的提示词
                                                </Button>
                                                {isAdmin ? (
                                                    <Button size="small" icon={<Edit3 className="size-3.5" />} onClick={() => openEditPublicPrompt(prompt)}>
                                                        编辑
                                                    </Button>
                                                ) : null}
                                                {isAdmin ? (
                                                    <Button size="small" danger icon={<Trash2 className="size-3.5" />} onClick={() => void deletePublicPrompt(prompt)}>
                                                        删除
                                                    </Button>
                                                ) : null}
                                            </>
                                        )}
                                        headerAction={
                                            isAdmin ? (
                                                <button
                                                    type="button"
                                                    className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300"
                                                    onClick={openCreatePublicPrompt}
                                                >
                                                    新增提示词
                                                </button>
                                            ) : null
                                        }
                                        footer={query.isFetchingNextPage ? "加载中..." : query.hasNextPage ? "继续向下滚动加载更多" : promptItems.length > 0 ? "已经到底了" : ""}
                                    />
                                ),
                            },
                        ]}
                    />
                </div>
            </main>

            <PromptDetailDialog prompt={selectedPrompt} onClose={() => setSelectedPrompt(null)} onCopy={(prompt) => copyText(prompt, "提示词已复制")} onSaveAsset={activeTab === "library" ? savePromptToMine : undefined} />

            <Modal title={editingPrompt ? "编辑我的提示词" : "新增我的提示词"} open={promptFormOpen} width={820} onCancel={() => setPromptFormOpen(false)} onOk={() => void saveMinePrompt()} okText="保存" cancelText="取消" destroyOnHidden>
                <Form form={form} layout="vertical" requiredMark={false} initialValues={{ domain: "image", stage: "general", tags: [] }}>
                    <Form.Item name="title" label="标题" rules={[{ required: true, message: "请输入标题" }]}>
                        <Input />
                    </Form.Item>
                    <div className="grid gap-4 sm:grid-cols-3">
                        <Form.Item name="domain" label="分类">
                            <Select options={mediaFilterOptions.filter((item) => item.value).map(({ label, value }) => ({ label, value }))} />
                        </Form.Item>
                        <Form.Item name="stage" label="用途">
                            <Select options={promptPurposeOptions.filter((item) => item.value).map(({ label, value }) => ({ label, value }))} />
                        </Form.Item>
                        <Form.Item name="source" label="来源">
                            <Input placeholder="手动添加 / 项目沉淀 / 其他来源" />
                        </Form.Item>
                    </div>
                    <Form.Item name="tags" label="自由标签">
                        <Select mode="tags" tokenSeparators={[",", "，"]} placeholder="输入标签后回车" />
                    </Form.Item>
                    <Form.Item name="coverUrl" label="封面 URL">
                        <Input />
                    </Form.Item>
                    <Form.Item name="prompt" label="提示词内容" rules={[{ required: true, message: "请输入提示词内容" }]}>
                        <Input.TextArea rows={10} />
                    </Form.Item>
                    <Form.Item name="note" label="备注">
                        <Input />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={editingPublicPrompt ? "编辑提示词库提示词" : "新增提示词库提示词"}
                open={publicPromptFormOpen}
                width={820}
                onCancel={() => setPublicPromptFormOpen(false)}
                onOk={() => void savePublicPrompt()}
                okText="保存"
                cancelText="取消"
                destroyOnHidden
            >
                <Form form={publicForm} layout="vertical" requiredMark={false} initialValues={{ domain: "image", stage: "general", tags: [], source: "manual-prompts" }}>
                    <Form.Item name="title" label="标题" rules={[{ required: true, message: "请输入标题" }]}>
                        <Input />
                    </Form.Item>
                    <div className="grid gap-4 sm:grid-cols-3">
                        <Form.Item name="domain" label="分类">
                            <Select options={mediaFilterOptions.filter((item) => item.value).map(({ label, value }) => ({ label, value }))} />
                        </Form.Item>
                        <Form.Item name="stage" label="用途">
                            <Select options={promptPurposeOptions.filter((item) => item.value).map(({ label, value }) => ({ label, value }))} />
                        </Form.Item>
                        <Form.Item name="source" label="来源">
                            <Select options={[{ label: "手动提示词", value: "manual-prompts" }, ...librarySourceOptions.filter((item) => item.value !== ALL_PROMPTS_OPTION)]} />
                        </Form.Item>
                    </div>
                    <Form.Item name="tags" label="自由标签">
                        <Select mode="tags" tokenSeparators={[",", "，"]} placeholder="输入标签后回车" />
                    </Form.Item>
                    <Form.Item name="coverUrl" label="封面 URL">
                        <Input />
                    </Form.Item>
                    <Form.Item name="prompt" label="提示词内容" rules={[{ required: true, message: "请输入提示词内容" }]}>
                        <Input.TextArea rows={10} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}

function PromptLibrarySection({
    keyword,
    setKeyword,
    media,
    setMedia,
    purpose,
    setPurpose,
    source,
    setSource,
    sourceOptions,
    tags,
    setTags,
    tagOptions,
    availableStages,
    countText,
    onReset,
    loading,
    items,
    total,
    onOpen,
    onCopy,
    extraAction,
    headerAction,
    footer,
}: {
    keyword: string;
    setKeyword: (value: string) => void;
    media: MediaFilter;
    setMedia: (value: MediaFilter) => void;
    purpose: string;
    setPurpose: (value: string) => void;
    source: string;
    setSource: (value: string) => void;
    sourceOptions: { label: string; value: string }[];
    tags: string[];
    setTags: (value: string[]) => void;
    tagOptions: string[];
    availableStages?: string[];
    countText: string;
    onReset: () => void;
    loading: boolean;
    items: Prompt[];
    total: number;
    onOpen: (prompt: Prompt) => void;
    onCopy: (prompt: Prompt) => void;
    extraAction?: (prompt: Prompt) => ReactNode;
    headerAction?: ReactNode;
    footer?: string;
}) {
    return (
        <div className="grid gap-6 xl:grid-cols-[220px_minmax(0,1fr)]">
            <MediaFilterSidebar value={media} onChange={setMedia} />
            <div className="space-y-6">
                <div className="rounded-2xl border border-stone-200 bg-background/80 p-4 dark:border-stone-800 dark:bg-stone-950/60">
                    <div className="space-y-4">
                        <Input.Search allowClear prefix={<Search className="size-4 text-stone-400" />} value={keyword} placeholder="搜索标题、内容、自由标签或来源" onChange={(event) => setKeyword(event.target.value)} onSearch={setKeyword} />
                        <div className="grid gap-3 md:grid-cols-3">
                            <Select value={purpose} options={promptPurposeOptionsForMedia(media, availableStages)} onChange={setPurpose} />
                            <Select value={source} options={sourceOptions} onChange={setSource} />
                            <Select mode="tags" tokenSeparators={[",", "，"]} allowClear maxTagCount="responsive" value={tags} options={tagOptions.map((tag) => ({ label: tag, value: tag }))} placeholder="自由标签" onChange={setTags} />
                        </div>
                        <div className="flex flex-wrap items-center justify-between gap-3">
                            <div className="text-xs text-stone-500 dark:text-stone-400">
                                {countText} · 共 {total} 条
                            </div>
                            <div className="flex flex-wrap gap-4">
                                <button type="button" className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300" onClick={onReset}>
                                    重置筛选
                                </button>
                                {headerAction}
                            </div>
                        </div>
                    </div>
                </div>
                {loading ? (
                    <div className="flex h-60 items-center justify-center">
                        <Spin />
                    </div>
                ) : (
                    <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
                        {items.map((item) => (
                            <PromptCard key={item.id} item={item} onOpen={() => onOpen(item)} onCopy={() => onCopy(item)} extraAction={extraAction?.(item)} />
                        ))}
                    </div>
                )}
                {!loading && items.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有找到匹配的提示词" className="py-16" /> : null}
                {footer ? <div className="text-center text-xs text-stone-500 dark:text-stone-400">{footer}</div> : null}
            </div>
        </div>
    );
}

function MediaFilterSidebar({ value, onChange }: { value: MediaFilter; onChange: (value: MediaFilter) => void }) {
    return (
        <aside className="rounded-2xl border border-stone-200 bg-background/80 p-4 dark:border-stone-800 dark:bg-stone-950/60">
            <div className="mb-3 text-sm font-semibold">分类</div>
            <div className="space-y-1">
                {mediaFilterOptions.map((option) => (
                    <button
                        key={option.value || "all"}
                        type="button"
                        className={cn(
                            "block w-full rounded-md px-3 py-1.5 text-left text-sm transition hover:bg-stone-100 dark:hover:bg-stone-800",
                            value === option.value && "bg-stone-200 font-medium text-stone-950 dark:bg-stone-800 dark:text-stone-100",
                        )}
                        onClick={() => onChange(option.value as MediaFilter)}
                    >
                        {option.label}
                    </button>
                ))}
            </div>
        </aside>
    );
}

function promptPurposeOptionsForMedia(media: MediaFilter, availableStages: string[] = []) {
    const stages = new Set(availableStages.filter(Boolean));
    return promptPurposeOptions.filter((item) => {
        if (!item.value) return true;
        if (stages.size > 0 && !stages.has(item.value)) return false;
        return !item.media || !media || item.media === media;
    });
}

function myPromptToPrompt(item: MyPrompt): Prompt {
    return {
        id: item.id,
        title: item.title,
        coverUrl: item.coverUrl || "/logo.svg",
        prompt: item.prompt,
        tags: item.tags,
        category: item.source,
        domain: item.domain,
        stage: item.stage,
        provider: "local",
        model: "",
        mode: "",
        inputType: "text",
        outputType: item.domain,
        status: "local",
        metadata: { ...(item.metadata || {}), localPrompt: item },
        githubUrl: "",
        preview: "",
        createdAt: item.createdAt,
        updatedAt: item.updatedAt,
    };
}

function promptSourceLabel(value?: string) {
    if (!value || value === ALL_PROMPTS_OPTION) return "全部来源";
    const labels: Record<string, string> = {
        "gpt-image-2-prompts": "gpt-image-2-prompts",
        "awesome-gpt-image": "awesome-gpt-image",
        "awesome-gpt4o-image-prompts": "awesome-gpt4o-image-prompts",
        "youmind-gpt-image-2": "youmind-gpt-image-2",
        "youmind-nano-banana-pro": "youmind-nano-banana-pro",
        "davidwu-gpt-image2-prompts": "davidwu-gptimage2-prompts",
        "manual-prompts": "手动提示词",
        system: "系统提示词",
    };
    return labels[value] || value;
}

function normalizePromptDomain(value?: string) {
    return value === "text" || value === "video" ? value : "image";
}

function defaultProviderForDomain(domain: string) {
    return domain === "video" ? "openai" : "chatgpt2api";
}

function defaultModelForDomain(domain: string) {
    if (domain === "video") return "sora-2";
    if (domain === "text") return "gpt-5.5";
    return "gpt-image-2";
}

function inputTypeForPrompt(domain: string, stage: string) {
    if (stage === "quality_review" || stage === "repair" || stage === "main_image" || stage === "spec_image") return "image";
    return domain === "video" ? "text" : "text";
}

function outputTypeForPrompt(domain: string, stage: string) {
    if (stage === "quality_review") return "json";
    if (stage === "repair" || stage === "main_image" || stage === "spec_image") return "image";
    if (domain === "video") return "video";
    if (domain === "text") return "text";
    return "image";
}

function sortedUnique(values: string[]) {
    return Array.from(new Set(values.filter(Boolean))).sort((a, b) => a.localeCompare(b, "zh-CN"));
}
