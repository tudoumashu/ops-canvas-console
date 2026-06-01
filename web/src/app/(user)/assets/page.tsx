"use client";

import { Copy, Download, PencilLine, Plus, Search, Trash2, Upload } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, App, Button, Card, Drawer, Empty, Form, Image, Input, Modal, Pagination, Select, Space, Spin, Tabs, Tag, Typography } from "antd";
import { saveAs } from "file-saver";

import { useCopyText } from "@/hooks/use-copy-text";
import { LocalWorkspaceStatusAlert } from "@/components/local-workspace/local-workspace-status-alert";
import { formatBytes, readFileAsDataUrl, readImageMeta } from "@/lib/image-utils";
import { cn } from "@/lib/utils";
import { useAssetStore, type Asset, type AssetKind, type ImageAsset, type VideoAsset } from "@/stores/use-asset-store";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";
import { fetchAssetLibrary, type AssetLibraryItem } from "@/services/api/assets";
import { deleteAdminAsset, saveAdminAsset, type AdminAsset } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";
import { exportAssets, importAssetPackage } from "./asset-transfer";

type AssetCenterTab = "mine" | "library";
type MediaFilter = "" | AssetKind;

type AssetFormValues = {
    kind: AssetKind;
    title: string;
    coverUrl: string;
    tags: string[];
    source?: string;
    purpose?: string;
    categoryPath?: string;
    note?: string;
    content?: string;
    url?: string;
};

type LibraryAssetFormValues = {
    type: AssetKind;
    title: string;
    coverUrl: string;
    tags: string[];
    category: string;
    categoryPath: string;
    purpose: string;
    source: string;
    description: string;
    content: string;
    url: string;
};

type ImageDraft = ImageAsset["data"] | null;
type VideoDraft = VideoAsset["data"] | null;

const mediaFilterOptions = [
    { label: "全部", value: "" },
    { label: "图片", value: "image" },
    { label: "文本", value: "text" },
    { label: "视频", value: "video" },
];

const editLibraryTypeOptions = mediaFilterOptions.filter((item) => item.value);

const assetCategoryOptions = [
    { label: "通用素材", value: "通用素材", media: "" },
    { label: "通用图片", value: "通用图片", media: "image" },
    { label: "文本素材", value: "文本素材", media: "text" },
    { label: "视频素材", value: "视频素材", media: "video" },
    { label: "角色参考图/标准参考图", value: "角色参考图/标准参考图", media: "image" },
    { label: "角色参考图/官方参考图", value: "角色参考图/官方参考图", media: "image" },
    { label: "规格图模板", value: "规格图模板", media: "image" },
];

const assetPurposeOptions = [
    { label: "全部用途", value: "" },
    { label: "通用素材", value: "generic", media: "" },
    { label: "标准参考图", value: "standard_reference", media: "image" },
    { label: "官方参考图", value: "official_reference", media: "image" },
    { label: "规格图模板", value: "spec_template", media: "image" },
];

const assetSourceOptions = [
    { label: "全部来源", value: "" },
    { label: "本地上传", value: "local_upload" },
    { label: "ai生成", value: "ai_generated" },
    { label: "云端素材", value: "cloud_asset" },
];

export default function AssetsPage() {
    const { message } = App.useApp();
    const copyText = useCopyText();
    const queryClient = useQueryClient();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const isAdmin = user?.role === "admin" && Boolean(token);

    const [localForm] = Form.useForm<AssetFormValues>();
    const [libraryForm] = Form.useForm<LibraryAssetFormValues>();
    const coverInputRef = useRef<HTMLInputElement>(null);
    const imageInputRef = useRef<HTMLInputElement>(null);
    const videoInputRef = useRef<HTMLInputElement>(null);
    const packageInputRef = useRef<HTMLInputElement>(null);

    const assets = useAssetStore((state) => state.assets);
    const localAssetsWorkspaceLoaded = useAssetStore((state) => state.workspaceLoaded);
    const localAssetsLoadedWorkspaceId = useAssetStore((state) => state.loadedWorkspaceId);
    const localAssetsLoading = useAssetStore((state) => state.loading);
    const localAssetsError = useAssetStore((state) => state.lastError);
    const loadAssetsFromWorkspace = useAssetStore((state) => state.loadFromWorkspace);
    const addAsset = useAssetStore((state) => state.addAsset);
    const updateAsset = useAssetStore((state) => state.updateAsset);
    const removeAsset = useAssetStore((state) => state.removeAsset);
    const localWorkspaceStatus = useLocalWorkspaceStore((state) => state.status);
    const localWorkspace = useLocalWorkspaceStore((state) => state.workspace);
    const localWorkspaceId = localWorkspace?.id || "";

    const [activeTab, setActiveTab] = useState<AssetCenterTab>("mine");
    const [localKeyword, setLocalKeyword] = useState("");
    const [localKindFilter, setLocalKindFilter] = useState<MediaFilter>("");
    const [localPurpose, setLocalPurpose] = useState("");
    const [localSource, setLocalSource] = useState("");
    const [localTags, setLocalTags] = useState<string[]>([]);
    const [localPage, setLocalPage] = useState(1);
    const [localPageSize, setLocalPageSize] = useState(10);
    const [libraryKeyword, setLibraryKeyword] = useState("");
    const [libraryKindFilter, setLibraryKindFilter] = useState<MediaFilter>("");
    const [libraryPurpose, setLibraryPurpose] = useState("");
    const [librarySource, setLibrarySource] = useState("");
    const [libraryTags, setLibraryTags] = useState<string[]>([]);
    const [libraryPage, setLibraryPage] = useState(1);
    const [libraryPageSize, setLibraryPageSize] = useState(12);

    const [editingLocalAsset, setEditingLocalAsset] = useState<Asset | null>(null);
    const [localAssetOpen, setLocalAssetOpen] = useState(false);
    const [previewLocalAsset, setPreviewLocalAsset] = useState<Asset | null>(null);
    const [deletingLocalAsset, setDeletingLocalAsset] = useState<Asset | null>(null);
    const [localFormKind, setLocalFormKind] = useState<AssetKind>("text");
    const [imageDraft, setImageDraft] = useState<ImageDraft>(null);
    const [videoDraft, setVideoDraft] = useState<VideoDraft>(null);

    const [previewLibraryAsset, setPreviewLibraryAsset] = useState<AssetLibraryItem | null>(null);
    const [editingLibraryAsset, setEditingLibraryAsset] = useState<Partial<AdminAsset> | null>(null);
    const [libraryAssetOpen, setLibraryAssetOpen] = useState(false);
    const [deletingLibraryAsset, setDeletingLibraryAsset] = useState<AssetLibraryItem | null>(null);

    const localCoverUrl = Form.useWatch("coverUrl", localForm) || "";
    const localTitle = Form.useWatch("title", localForm) || "";
    const localFormTags = Form.useWatch("tags", localForm) || [];
    const localContent = Form.useWatch("content", localForm) || "";
    const localURL = Form.useWatch("url", localForm) || "";
    const libraryCoverUrl = Form.useWatch("coverUrl", libraryForm) || "";
    const libraryTitle = Form.useWatch("title", libraryForm) || "";
    const libraryFormTags = Form.useWatch("tags", libraryForm) || [];
    const libraryContent = Form.useWatch("content", libraryForm) || "";
    const libraryURL = Form.useWatch("url", libraryForm) || "";
    const libraryFormType = Form.useWatch("type", libraryForm) || editingLibraryAsset?.type || "text";

    const localAssetsReady = localWorkspaceStatus === "connected" && localAssetsWorkspaceLoaded && localAssetsLoadedWorkspaceId === localWorkspaceId;
    const workspaceAssets = localAssetsReady ? assets : [];
    const validLocalAssets = useMemo(() => workspaceAssets.filter((asset) => asset.kind === "text" || asset.kind === "image" || asset.kind === "video"), [workspaceAssets]);
    const filteredLocalAssets = useMemo(() => {
        const query = localKeyword.trim().toLowerCase();
        return validLocalAssets.filter((asset) => {
            if (localKindFilter && asset.kind !== localKindFilter) return false;
            if (localPurpose && localAssetPurposeValue(asset) !== localPurpose) return false;
            if (localSource && localAssetSourceValue(asset) !== localSource) return false;
            if (localTags.length && !localTags.every((tag) => (asset.tags || []).includes(tag))) return false;
            if (!query) return true;
            return assetSearchText(asset).includes(query);
        });
    }, [validLocalAssets, localKeyword, localKindFilter, localPurpose, localSource, localTags]);
    const localAvailableTags = useMemo(() => sortedUnique(validLocalAssets.filter((asset) => !localKindFilter || asset.kind === localKindFilter).flatMap((asset) => asset.tags || [])), [validLocalAssets, localKindFilter]);
    const visibleLocalAssets = useMemo(() => {
        const start = (localPage - 1) * localPageSize;
        return filteredLocalAssets.slice(start, start + localPageSize);
    }, [filteredLocalAssets, localPage, localPageSize]);

    const libraryQuery = useQuery({
        queryKey: ["asset-library", libraryKeyword, libraryKindFilter, libraryPurpose, librarySource, libraryTags, libraryPage, libraryPageSize],
        queryFn: () => fetchAssetLibrary({ keyword: libraryKeyword, mediaType: libraryKindFilter, purpose: libraryPurpose, source: librarySource, tag: libraryTags, page: libraryPage, pageSize: libraryPageSize }),
        retry: false,
    });
    const libraryAssets = libraryQuery.data?.items || [];
    const libraryAvailableTags = libraryQuery.data?.freeTags || libraryQuery.data?.tags || [];
    const libraryFacets = libraryQuery.data?.facets;
    const libraryTotal = libraryQuery.data?.total || 0;

    useEffect(() => {
        const tab = new URLSearchParams(window.location.search).get("tab");
        if (tab === "library") setActiveTab("library");
    }, []);

    useEffect(() => {
        const maxPage = Math.max(1, Math.ceil(filteredLocalAssets.length / localPageSize));
        setLocalPage((value) => Math.min(value, maxPage));
    }, [filteredLocalAssets.length, localPageSize]);

    useEffect(() => {
        if (libraryQuery.isError) {
            message.error(libraryQuery.error instanceof Error ? libraryQuery.error.message : "获取素材库失败");
        }
    }, [message, libraryQuery.error, libraryQuery.isError]);

    useEffect(() => {
        if (localWorkspaceStatus === "connected" && localWorkspaceId) void loadAssetsFromWorkspace();
    }, [loadAssetsFromWorkspace, localWorkspaceId, localWorkspaceStatus]);

    const openCreateLocalAsset = () => {
        setEditingLocalAsset(null);
        setImageDraft(null);
        setVideoDraft(null);
        setLocalFormKind("text");
        localForm.setFieldsValue({ kind: "text", title: "", coverUrl: "", tags: [], source: "local_upload", purpose: "generic", categoryPath: "文本素材", note: "", content: "", url: "" });
        setLocalAssetOpen(true);
    };

    const openEditLocalAsset = (asset: Asset) => {
        setEditingLocalAsset(asset);
        setLocalFormKind(asset.kind);
        setImageDraft(asset.kind === "image" ? asset.data : null);
        setVideoDraft(asset.kind === "video" ? asset.data : null);
        localForm.setFieldsValue({
            kind: asset.kind,
            title: asset.title,
            coverUrl: asset.coverUrl,
            tags: asset.tags || [],
            source: localAssetSourceValue(asset),
            purpose: localAssetPurposeValue(asset),
            categoryPath: localAssetCategoryPath(asset),
            note: asset.note,
            content: asset.kind === "text" ? asset.data.content : "",
            url: asset.kind === "image" ? asset.data.dataUrl : asset.kind === "video" ? asset.data.url : "",
        });
        setLocalAssetOpen(true);
    };

    const saveLocalAsset = async () => {
        const values = await localForm.validateFields();
        const base = {
            title: values.title.trim(),
            coverUrl: values.coverUrl?.trim() || (values.kind === "image" && imageDraft ? imageDraft.dataUrl : ""),
            tags: values.tags || [],
            source: assetSourceLabel(values.source || "local_upload"),
            note: values.note?.trim(),
            metadata: { ...(editingLocalAsset?.metadata || {}), source: values.source || "local_upload", purpose: values.purpose || "generic", categoryPath: values.categoryPath || categoryPathForMedia(values.kind) },
        };

        if (values.kind === "text") {
            const asset = { ...base, kind: "text" as const, data: { content: (values.content || "").trim() } };
            if (!(await saveLocalAssetPayload(asset))) return;
        } else if (values.kind === "image") {
            const image = imageDraft || (values.url?.trim() ? await imageDataFromUrl(values.url.trim()) : null);
            if (!image) {
                message.error("请选择图片文件或填写图片 URL");
                return;
            }
            const asset = { ...base, coverUrl: values.coverUrl?.trim() || image.dataUrl, kind: "image" as const, data: image };
            if (!(await saveLocalAssetPayload(asset))) return;
        } else {
            const video = videoDraft || (values.url?.trim() ? { url: values.url.trim(), width: 0, height: 0, bytes: 0, mimeType: mimeTypeFromURL(values.url.trim()) } : null);
            if (!video) {
                message.error("请选择视频文件或填写视频 URL");
                return;
            }
            const asset = { ...base, coverUrl: values.coverUrl?.trim() || video.url, kind: "video" as const, data: video };
            if (!(await saveLocalAssetPayload(asset))) return;
        }

        message.success(editingLocalAsset ? "素材已更新" : "素材已保存");
        setLocalAssetOpen(false);
    };

    const saveLocalAssetPayload = async (asset: Omit<Asset, "id" | "createdAt" | "updatedAt">) => {
        if (editingLocalAsset) {
            await updateAsset(editingLocalAsset.id, asset);
        } else {
            const id = await addAsset(asset);
            if (!id) {
                message.error(useAssetStore.getState().lastError || "请先连接本地工作区");
                return false;
            }
        }
        const error = useAssetStore.getState().lastError;
        if (error) {
            message.error(error);
            return false;
        }
        return true;
    };

    const readCoverFile = async (file?: File) => {
        if (!file) return;
        const dataUrl = await readFileAsDataUrl(file);
        localForm.setFieldValue("coverUrl", dataUrl);
    };

    const readImageFile = async (file?: File) => {
        if (!file || !file.type.startsWith("image/")) return;
        const url = URL.createObjectURL(file);
        const meta = await readImageMeta(url);
        const draft = { dataUrl: url, width: meta.width, height: meta.height, bytes: file.size, mimeType: file.type || meta.mimeType };
        setImageDraft(draft);
        localForm.setFieldValue("url", draft.dataUrl);
        if (!localForm.getFieldValue("coverUrl")) localForm.setFieldValue("coverUrl", draft.dataUrl);
        if (!localForm.getFieldValue("title")) localForm.setFieldValue("title", file.name);
    };

    const readVideoFile = async (file?: File) => {
        if (!file || !file.type.startsWith("video/")) return;
        const url = URL.createObjectURL(file);
        const meta = await readVideoMeta(url);
        const draft = { url, width: meta.width, height: meta.height, bytes: file.size, mimeType: file.type || "video/mp4" };
        setVideoDraft(draft);
        localForm.setFieldValue("url", draft.url);
        if (!localForm.getFieldValue("coverUrl")) localForm.setFieldValue("coverUrl", draft.url);
        if (!localForm.getFieldValue("title")) localForm.setFieldValue("title", file.name);
    };

    const copyLocalAssetText = async (asset: Asset) => {
        if (asset.kind !== "text") return;
        copyText(asset.data.content, "文本已复制");
    };

    const downloadLocalAsset = (asset: Asset) => {
        if (asset.kind !== "image" && asset.kind !== "video") return;
        saveAs(asset.kind === "video" ? asset.data.url : asset.data.dataUrl, `${asset.title || "asset"}.${asset.data.mimeType.split("/")[1] || "png"}`);
    };

    const exportAllLocalAssets = async () => {
        if (!validLocalAssets.length) {
            message.warning("暂无素材可导出");
            return;
        }
        await exportAssets(validLocalAssets);
    };

    const importLocalAssetPackage = async (file?: File) => {
        if (!file) return;
        try {
            if (localWorkspaceStatus !== "connected") {
                message.warning("请先连接本地工作区");
                return;
            }
            const result = await importAssetPackage(file, addAsset);
            if (!result.imported) {
                message.error("导入失败，请选择有效的素材压缩包");
                return;
            }
            message.success(result.failed ? `已导入 ${result.imported} 个素材，${result.failed} 个失败` : `已导入 ${result.imported} 个素材`);
        } catch {
            message.error("导入失败，请选择有效的素材压缩包");
        } finally {
            if (packageInputRef.current) packageInputRef.current.value = "";
        }
    };

    const confirmDeleteLocalAsset = async () => {
        if (!deletingLocalAsset) return;
        await removeAsset(deletingLocalAsset.id);
        const error = useAssetStore.getState().lastError;
        if (error) {
            message.error(error);
            return;
        }
        message.success("素材已删除");
        setDeletingLocalAsset(null);
    };

    const copyLibraryAssetToMine = async (asset: AssetLibraryItem) => {
        try {
            if (asset.type === "image") {
                const image = await imageDataFromUrl(asset.url || asset.coverUrl);
                const id = await addAsset({
                    kind: "image",
                    title: asset.title,
                    coverUrl: image.dataUrl,
                    tags: asset.tags || [],
                    source: assetSourceLabel("cloud_asset"),
                    note: asset.description,
                    data: { dataUrl: image.dataUrl, width: image.width, height: image.height, bytes: image.bytes, mimeType: image.mimeType },
                    metadata: { source: "cloud_asset", assetId: asset.id, purpose: normalizeAssetPurposeValue(asset.purpose), categoryPath: normalizeAssetCategoryPath(asset.categoryPath || asset.category, asset.purpose) },
                });
                if (!id) throw new Error(useAssetStore.getState().lastError || "请先连接本地工作区");
            } else if (asset.type === "video") {
                const id = await addAsset({
                    kind: "video",
                    title: asset.title,
                    coverUrl: asset.coverUrl,
                    tags: asset.tags || [],
                    source: assetSourceLabel("cloud_asset"),
                    note: asset.description,
                    data: { url: asset.url, width: 0, height: 0, bytes: 0, mimeType: mimeTypeFromURL(asset.url) },
                    metadata: { source: "cloud_asset", assetId: asset.id, purpose: normalizeAssetPurposeValue(asset.purpose), categoryPath: normalizeAssetCategoryPath(asset.categoryPath || asset.category, asset.purpose) },
                });
                if (!id) throw new Error(useAssetStore.getState().lastError || "请先连接本地工作区");
            } else {
                const id = await addAsset({
                    kind: "text",
                    title: asset.title,
                    coverUrl: asset.coverUrl,
                    tags: asset.tags || [],
                    source: assetSourceLabel("cloud_asset"),
                    note: asset.description,
                    data: { content: asset.content },
                    metadata: { source: "cloud_asset", assetId: asset.id, purpose: normalizeAssetPurposeValue(asset.purpose), categoryPath: normalizeAssetCategoryPath(asset.categoryPath || asset.category, asset.purpose) },
                });
                if (!id) throw new Error(useAssetStore.getState().lastError || "请先连接本地工作区");
            }
            message.success("已复制到我的素材");
        } catch {
            message.error("复制失败");
        }
    };

    const copyLibraryAssetContent = (asset: AssetLibraryItem) => {
        copyText(asset.type === "text" ? asset.content : asset.url || asset.coverUrl, "内容已复制");
    };

    const downloadLibraryAsset = (asset: AssetLibraryItem) => {
        const url = asset.url || asset.coverUrl;
        if (!url) return;
        saveAs(url, `${asset.title || "asset"}.${mimeExtension(mimeTypeFromURL(url))}`);
    };

    const openCreateLibraryAsset = () => {
        if (!isAdmin) return;
        setEditingLibraryAsset({ type: "text", tags: [] });
        libraryForm.setFieldsValue({ type: "text", title: "", coverUrl: "", tags: [], category: "文本素材", categoryPath: "文本素材", purpose: "generic", source: "cloud_asset", description: "", content: "", url: "" });
        setLibraryAssetOpen(true);
    };

    const openEditLibraryAsset = (asset: AssetLibraryItem) => {
        if (!isAdmin) return;
        const item = libraryAssetToAdminAsset(asset);
        setEditingLibraryAsset(item);
        libraryForm.setFieldsValue({
            type: item.type || "text",
            title: item.title,
            coverUrl: item.coverUrl,
            tags: item.tags || [],
            category: item.category,
            categoryPath: item.categoryPath || item.category,
            purpose: normalizeAssetPurposeValue(item.purpose),
            source: normalizeAssetSourceValue(item.source),
            description: item.description,
            content: item.content,
            url: item.url,
        });
        setLibraryAssetOpen(true);
    };

    const saveLibraryAsset = async () => {
        if (!isAdmin || !token) return;
        const values = await libraryForm.validateFields();
        const nextType = values.type || "text";
        await saveAdminAsset(token, {
            ...editingLibraryAsset,
            ...values,
            type: nextType,
            mediaType: nextType,
            category: values.categoryPath || values.category || categoryPathForMedia(nextType),
            categoryPath: values.categoryPath || values.category || categoryPathForMedia(nextType),
            purpose: normalizeAssetPurposeValue(values.purpose || "generic"),
            source: normalizeAssetSourceValue(values.source || "cloud_asset"),
            coverUrl: values.coverUrl || (nextType !== "text" ? values.url : ""),
            tags: values.tags || [],
        });
        await queryClient.invalidateQueries({ queryKey: ["asset-library"] });
        await queryClient.invalidateQueries({ queryKey: ["workflow-template-image-assets"] });
        message.success(editingLibraryAsset?.id ? "素材库素材已保存" : "素材库素材已新增");
        setLibraryAssetOpen(false);
    };

    const confirmDeleteLibraryAsset = async () => {
        if (!isAdmin || !token || !deletingLibraryAsset) return;
        await deleteAdminAsset(token, deletingLibraryAsset.id);
        await queryClient.invalidateQueries({ queryKey: ["asset-library"] });
        await queryClient.invalidateQueries({ queryKey: ["workflow-template-image-assets"] });
        message.success("素材库素材已删除");
        setDeletingLibraryAsset(null);
    };

    const resetLibraryFilters = () => {
        setLibraryKeyword("");
        setLibraryKindFilter("");
        setLibraryPurpose("");
        setLibrarySource("");
        setLibraryTags([]);
        setLibraryPage(1);
    };

    const resetLocalFilters = () => {
        setLocalKeyword("");
        setLocalKindFilter("");
        setLocalPurpose("");
        setLocalSource("");
        setLocalTags([]);
        setLocalPage(1);
    };

    return (
        <div className="flex h-full flex-col overflow-hidden bg-background text-stone-900 dark:text-stone-100">
            <main className="min-h-0 flex-1 overflow-y-auto bg-[radial-gradient(#e5e7eb_1px,transparent_1px)] px-6 py-8 [background-size:16px_16px] dark:bg-[radial-gradient(rgba(245,245,244,.14)_1px,transparent_1px)]">
                <div className="pb-8">
                    <div className="mx-auto max-w-5xl text-center">
                        <h1 className="text-4xl font-semibold tracking-tight text-stone-950 dark:text-stone-100">素材中心</h1>
                        <p className="mt-3 text-sm text-stone-500 dark:text-stone-400">统一管理本地工作区素材和服务器素材库，按来源安全使用。</p>
                    </div>
                </div>

                <div className="mx-auto max-w-7xl">
                    <Tabs
                        activeKey={activeTab}
                        onChange={(key) => setActiveTab(key as AssetCenterTab)}
                        items={[
                            {
                                key: "mine",
                                label: "我的素材",
                                children: (
                                    <div className="grid gap-6 xl:grid-cols-[220px_minmax(0,1fr)]">
                                        <MediaFilterSidebar
                                            value={localKindFilter}
                                            onChange={(value) => {
                                                setLocalPage(1);
                                                setLocalKindFilter(value);
                                                setLocalPurpose("");
                                            }}
                                        />
                                        <div className="space-y-6">
                                            <LocalWorkspaceStatusAlert message="我的素材现在以本地工作区为事实源" />
                                            {localAssetsError && localWorkspaceStatus === "connected" ? <Alert type="error" showIcon title={localAssetsError} /> : null}
                                            {localWorkspace ? <Alert type="info" showIcon title={`当前工作区：${localWorkspace.name}`} /> : null}
                                            <div className="rounded-2xl border border-stone-200 bg-background/80 p-4 dark:border-stone-800 dark:bg-stone-950/60">
                                                <div className="space-y-4">
                                                    <Input.Search
                                                        allowClear
                                                        prefix={<Search className="size-4 text-stone-400" />}
                                                        value={localKeyword}
                                                        placeholder="搜索标题、内容、自由标签或来源"
                                                        onChange={(event) => {
                                                            setLocalPage(1);
                                                            setLocalKeyword(event.target.value);
                                                        }}
                                                        onSearch={(value) => {
                                                            setLocalPage(1);
                                                            setLocalKeyword(value);
                                                        }}
                                                    />
                                                    <div className="grid gap-3 md:grid-cols-3">
                                                        <Select
                                                            value={localPurpose}
                                                            options={assetPurposeOptionsForMedia(localKindFilter)}
                                                            onChange={(value) => {
                                                                setLocalPage(1);
                                                                setLocalPurpose(value);
                                                            }}
                                                        />
                                                        <Select
                                                            value={localSource}
                                                            options={assetSourceOptions}
                                                            onChange={(value) => {
                                                                setLocalPage(1);
                                                                setLocalSource(value);
                                                            }}
                                                        />
                                                        <Select
                                                            mode="tags"
                                                            tokenSeparators={[",", "，"]}
                                                            allowClear
                                                            maxTagCount="responsive"
                                                            value={localTags}
                                                            options={localAvailableTags.map((tag) => ({ label: tag, value: tag }))}
                                                            placeholder="自由标签"
                                                            onChange={(value) => {
                                                                setLocalPage(1);
                                                                setLocalTags(value);
                                                            }}
                                                        />
                                                    </div>
                                                    <div className="flex flex-wrap items-center justify-between gap-3">
                                                        <div className="text-xs text-stone-500 dark:text-stone-400">
                                                            当前可选：{assetPurposeOptionsForMedia(localKindFilter).length - 1} 个用途、{localAvailableTags.length} 个自由标签
                                                        </div>
                                                        <div className="flex flex-wrap gap-4">
                                                            <button
                                                                type="button"
                                                                className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300"
                                                                onClick={resetLocalFilters}
                                                            >
                                                                重置筛选
                                                            </button>
                                                            <button
                                                                type="button"
                                                                className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300"
                                                                onClick={() => void exportAllLocalAssets()}
                                                            >
                                                                导出素材
                                                            </button>
                                                            <button
                                                                type="button"
                                                                className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300"
                                                                onClick={() => packageInputRef.current?.click()}
                                                            >
                                                                导入素材包
                                                            </button>
                                                            <button
                                                                type="button"
                                                                className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300"
                                                                onClick={openCreateLocalAsset}
                                                            >
                                                                新增素材
                                                            </button>
                                                        </div>
                                                    </div>
                                                </div>
                                            </div>

                                            {localAssetsLoading ? (
                                                <div className="flex h-60 items-center justify-center">
                                                    <Spin />
                                                </div>
                                            ) : (
                                                <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                                                    {visibleLocalAssets.map((asset) => (
                                                        <AssetCard
                                                            key={asset.id}
                                                            asset={asset}
                                                            onOpen={() => setPreviewLocalAsset(asset)}
                                                            onEdit={() => openEditLocalAsset(asset)}
                                                            onCopy={copyLocalAssetText}
                                                            onDownload={downloadLocalAsset}
                                                            onDelete={() => setDeletingLocalAsset(asset)}
                                                        />
                                                    ))}
                                                </div>
                                            )}

                                            {!localAssetsLoading && !visibleLocalAssets.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有找到素材" className="py-20" /> : null}

                                            <div className="flex justify-center">
                                                <Pagination
                                                    current={localPage}
                                                    pageSize={localPageSize}
                                                    total={filteredLocalAssets.length}
                                                    showSizeChanger
                                                    pageSizeOptions={[10, 20, 50, 100]}
                                                    onChange={(nextPage, nextPageSize) => {
                                                        setLocalPage(nextPage);
                                                        setLocalPageSize(nextPageSize);
                                                    }}
                                                />
                                            </div>
                                        </div>
                                    </div>
                                ),
                            },
                            {
                                key: "library",
                                label: "素材库",
                                children: (
                                    <div className="grid gap-6 xl:grid-cols-[220px_minmax(0,1fr)]">
                                        <MediaFilterSidebar
                                            value={libraryKindFilter}
                                            onChange={(value) => {
                                                setLibraryPage(1);
                                                setLibraryKindFilter(value);
                                                setLibraryPurpose("");
                                            }}
                                        />
                                        <div className="space-y-6">
                                            <div className="rounded-2xl border border-stone-200 bg-background/80 p-4 dark:border-stone-800 dark:bg-stone-950/60">
                                                <div className="space-y-4">
                                                    <Input.Search
                                                        allowClear
                                                        prefix={<Search className="size-4 text-stone-400" />}
                                                        value={libraryKeyword}
                                                        placeholder="搜索标题、描述、自由标签或 URL"
                                                        onChange={(event) => {
                                                            setLibraryPage(1);
                                                            setLibraryKeyword(event.target.value);
                                                        }}
                                                        onSearch={(value) => {
                                                            setLibraryPage(1);
                                                            setLibraryKeyword(value);
                                                        }}
                                                    />
                                                    <div className="grid gap-3 md:grid-cols-3">
                                                        <Select
                                                            value={libraryPurpose}
                                                            options={assetPurposeOptionsForMedia(libraryKindFilter).filter((option) => !option.value || !libraryFacets?.purposes?.length || libraryFacets.purposes.includes(option.value))}
                                                            onChange={(value) => {
                                                                setLibraryPage(1);
                                                                setLibraryPurpose(value);
                                                            }}
                                                        />
                                                        <Select
                                                            value={librarySource}
                                                            options={assetSourceOptions}
                                                            onChange={(value) => {
                                                                setLibraryPage(1);
                                                                setLibrarySource(value);
                                                            }}
                                                        />
                                                        <Select
                                                            mode="tags"
                                                            tokenSeparators={[",", "，"]}
                                                            allowClear
                                                            maxTagCount="responsive"
                                                            value={libraryTags}
                                                            options={libraryAvailableTags.map((tag) => ({ label: tag, value: tag }))}
                                                            placeholder="自由标签"
                                                            onChange={(value) => {
                                                                setLibraryPage(1);
                                                                setLibraryTags(value);
                                                            }}
                                                        />
                                                    </div>
                                                    <div className="flex flex-wrap items-center justify-between gap-3">
                                                        <div className="text-xs text-stone-500 dark:text-stone-400">
                                                            当前可选：{libraryFacets?.purposes?.length || 0} 个用途、{libraryAvailableTags.length} 个自由标签
                                                        </div>
                                                        <div className="flex flex-wrap gap-4">
                                                            <button
                                                                type="button"
                                                                className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300"
                                                                onClick={resetLibraryFilters}
                                                            >
                                                                重置筛选
                                                            </button>
                                                            {isAdmin ? (
                                                                <button
                                                                    type="button"
                                                                    className="cursor-pointer text-sm font-medium text-stone-700 underline-offset-4 hover:underline focus-visible:outline-none focus-visible:underline dark:text-stone-300"
                                                                    onClick={openCreateLibraryAsset}
                                                                >
                                                                    新增素材库素材
                                                                </button>
                                                            ) : null}
                                                        </div>
                                                    </div>
                                                </div>
                                            </div>

                                            {libraryQuery.isLoading ? (
                                                <div className="flex h-60 items-center justify-center">
                                                    <Spin />
                                                </div>
                                            ) : (
                                                <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
                                                    {libraryAssets.map((asset) => (
                                                        <LibraryAssetCard
                                                            key={asset.id}
                                                            asset={asset}
                                                            isAdmin={isAdmin}
                                                            onOpen={() => setPreviewLibraryAsset(asset)}
                                                            onCopy={() => copyLibraryAssetContent(asset)}
                                                            onDownload={() => downloadLibraryAsset(asset)}
                                                            onCopyToMine={() => void copyLibraryAssetToMine(asset)}
                                                            onEdit={() => openEditLibraryAsset(asset)}
                                                            onDelete={() => setDeletingLibraryAsset(asset)}
                                                        />
                                                    ))}
                                                </div>
                                            )}

                                            {!libraryAssets.length && !libraryQuery.isLoading ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有找到素材库素材" className="py-20" /> : null}

                                            <div className="flex justify-center">
                                                <Pagination
                                                    current={libraryPage}
                                                    pageSize={libraryPageSize}
                                                    total={libraryTotal}
                                                    showSizeChanger
                                                    pageSizeOptions={[12, 24, 48, 96]}
                                                    onChange={(nextPage, nextPageSize) => {
                                                        setLibraryPage(nextPage);
                                                        setLibraryPageSize(nextPageSize);
                                                    }}
                                                />
                                            </div>
                                        </div>
                                    </div>
                                ),
                            },
                        ]}
                    />
                </div>
            </main>
            <input
                ref={packageInputRef}
                type="file"
                accept="application/zip,.zip"
                className="hidden"
                onChange={(event) => {
                    void importLocalAssetPackage(event.target.files?.[0]);
                    event.target.value = "";
                }}
            />

            <Modal title={editingLocalAsset ? "编辑我的素材" : "新增我的素材"} open={localAssetOpen} width={980} onCancel={() => setLocalAssetOpen(false)} onOk={() => void saveLocalAsset()} okText="保存" cancelText="取消" destroyOnHidden>
                <div className="grid gap-6 pt-1 lg:grid-cols-[minmax(0,1fr)_320px]">
                    <Form form={localForm} layout="vertical" requiredMark={false} initialValues={{ kind: "text", tags: [] }}>
                        <Form.Item name="kind" label="类型" rules={[{ required: true, message: "请选择类型" }]}>
                            <Select
                                options={editLibraryTypeOptions}
                                onChange={(value) => {
                                    setLocalFormKind(value);
                                    setImageDraft(null);
                                    setVideoDraft(null);
                                    localForm.setFieldsValue({ categoryPath: categoryPathForMedia(value), purpose: "generic", url: "", content: "" });
                                }}
                            />
                        </Form.Item>
                        <Form.Item name="title" label="标题" rules={[{ required: true, message: "请输入标题" }]}>
                            <Input placeholder="给素材起一个容易检索的名字" />
                        </Form.Item>
                        <Form.Item name="coverUrl" label="封面 URL">
                            <Space.Compact className="w-full">
                                <Input placeholder="可粘贴图片 URL，也可以上传本地封面" />
                                <Button icon={<Upload className="size-3.5" />} onClick={() => coverInputRef.current?.click()}>
                                    上传
                                </Button>
                            </Space.Compact>
                        </Form.Item>
                        <Form.Item name="tags" label="标签">
                            <Select mode="tags" tokenSeparators={[",", "，"]} placeholder="输入标签后回车" />
                        </Form.Item>
                        <div className="grid gap-4 sm:grid-cols-2">
                            <Form.Item name="categoryPath" label="分类">
                                <Select options={assetCategoryOptions.filter((item) => !item.media || item.media === localFormKind).map(({ label, value }) => ({ label, value }))} />
                            </Form.Item>
                            <Form.Item name="purpose" label="用途">
                                <Select options={assetPurposeOptionsForMedia(localFormKind).filter((item) => item.value)} />
                            </Form.Item>
                            <Form.Item name="source" label="来源">
                                <Select options={assetSourceOptions.filter((item) => item.value)} />
                            </Form.Item>
                            <Form.Item name="note" label="备注">
                                <Input placeholder="可选" />
                            </Form.Item>
                        </div>
                        {localFormKind === "text" ? (
                            <Form.Item name="content" label="文本内容" rules={[{ required: true, message: "请输入文本内容" }]}>
                                <Input.TextArea rows={8} placeholder="保存提示词、说明文案、参考描述等文本素材" />
                            </Form.Item>
                        ) : (
                            <>
                                <Form.Item
                                    name="url"
                                    label={localFormKind === "video" ? "视频 URL" : "图片 URL"}
                                    rules={[{ required: !imageDraft && !videoDraft, message: localFormKind === "video" ? "请输入视频 URL 或上传视频文件" : "请输入图片 URL 或上传图片文件" }]}
                                >
                                    <Input placeholder={localFormKind === "video" ? "可粘贴视频 URL，也可以上传本地视频" : "可粘贴图片 URL，也可以上传本地图片"} />
                                </Form.Item>
                                <div className="rounded-lg border border-dashed border-stone-300 p-4 dark:border-stone-700">
                                    <Button icon={<Upload className="size-4" />} onClick={() => (localFormKind === "video" ? videoInputRef.current?.click() : imageInputRef.current?.click())}>
                                        {localFormKind === "video" ? "选择视频文件" : "选择图片文件"}
                                    </Button>
                                    {localFormKind === "video" && videoDraft ? (
                                        <Typography.Text type="secondary" className="ml-3 text-xs">
                                            {videoDraft.width || "-"}x{videoDraft.height || "-"} · {formatBytes(videoDraft.bytes)}
                                        </Typography.Text>
                                    ) : imageDraft ? (
                                        <Typography.Text type="secondary" className="ml-3 text-xs">
                                            {imageDraft.width}x{imageDraft.height} · {formatBytes(imageDraft.bytes)}
                                        </Typography.Text>
                                    ) : (
                                        <Typography.Text type="secondary" className="ml-3 text-xs">
                                            {localFormKind === "video" ? "未选择视频" : "未选择图片"}
                                        </Typography.Text>
                                    )}
                                </div>
                            </>
                        )}
                    </Form>
                    <AssetFormPreview title={localTitle} tags={localFormTags} coverUrl={localCoverUrl || imageDraft?.dataUrl || videoDraft?.url || localURL} fallback={localContent} />
                </div>
                <input
                    ref={coverInputRef}
                    type="file"
                    accept="image/*"
                    className="hidden"
                    onChange={(event) => {
                        void readCoverFile(event.target.files?.[0]);
                        event.target.value = "";
                    }}
                />
                <input
                    ref={imageInputRef}
                    type="file"
                    accept="image/*"
                    className="hidden"
                    onChange={(event) => {
                        void readImageFile(event.target.files?.[0]);
                        event.target.value = "";
                    }}
                />
                <input
                    ref={videoInputRef}
                    type="file"
                    accept="video/*"
                    className="hidden"
                    onChange={(event) => {
                        void readVideoFile(event.target.files?.[0]);
                        event.target.value = "";
                    }}
                />
            </Modal>

            <Modal title={editingLibraryAsset?.id ? "编辑素材库素材" : "新增素材库素材"} open={libraryAssetOpen} width={820} onCancel={() => setLibraryAssetOpen(false)} onOk={() => void saveLibraryAsset()} okText="保存" cancelText="取消" destroyOnHidden>
                <div className="grid gap-6 pt-1 lg:grid-cols-[minmax(0,1fr)_300px]">
                    <Form form={libraryForm} layout="vertical" requiredMark={false} initialValues={{ type: "text", tags: [] }}>
                        <Form.Item name="type" label="类型" rules={[{ required: true, message: "请选择类型" }]}>
                            <Select options={editLibraryTypeOptions} />
                        </Form.Item>
                        <Form.Item name="title" label="标题" rules={[{ required: true, message: "请输入标题" }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="coverUrl" label="封面 URL">
                            <Input placeholder="留空时图片素材会使用图片 URL" />
                        </Form.Item>
                        <Form.Item name="tags" label="标签">
                            <Select mode="tags" tokenSeparators={[",", "，"]} placeholder="输入标签后回车" />
                        </Form.Item>
                        <div className="grid gap-4 sm:grid-cols-2">
                            <Form.Item name="categoryPath" label="分类">
                                <Select options={assetCategoryOptions.filter((item) => !item.media || item.media === libraryFormType).map(({ label, value }) => ({ label, value }))} />
                            </Form.Item>
                            <Form.Item name="purpose" label="用途">
                                <Select options={assetPurposeOptionsForMedia(libraryFormType).filter((item) => item.value)} />
                            </Form.Item>
                            <Form.Item name="source" label="来源">
                                <Select options={assetSourceOptions.filter((item) => item.value)} />
                            </Form.Item>
                            <Form.Item name="description" label="描述">
                                <Input />
                            </Form.Item>
                        </div>
                        {libraryFormType !== "text" ? (
                            <Form.Item name="url" label={libraryFormType === "video" ? "视频 URL" : "图片 URL"} rules={[{ required: true, message: libraryFormType === "video" ? "请输入视频 URL" : "请输入图片 URL" }]}>
                                <Input />
                            </Form.Item>
                        ) : (
                            <Form.Item name="content" label="文本内容" rules={[{ required: true, message: "请输入文本内容" }]}>
                                <Input.TextArea rows={7} />
                            </Form.Item>
                        )}
                    </Form>
                    <AssetFormPreview title={libraryTitle} tags={libraryFormTags} coverUrl={libraryCoverUrl || libraryURL} fallback={libraryContent} />
                </div>
            </Modal>

            <AssetDrawer asset={previewLocalAsset} onClose={() => setPreviewLocalAsset(null)} onCopy={copyLocalAssetText} onDownload={downloadLocalAsset} />
            <LibraryAssetDrawer
                asset={previewLibraryAsset}
                isAdmin={isAdmin}
                onClose={() => setPreviewLibraryAsset(null)}
                onCopy={copyLibraryAssetContent}
                onDownload={downloadLibraryAsset}
                onCopyToMine={(asset) => void copyLibraryAssetToMine(asset)}
                onEdit={(asset) => {
                    setPreviewLibraryAsset(null);
                    openEditLibraryAsset(asset);
                }}
                onDelete={(asset) => {
                    setPreviewLibraryAsset(null);
                    setDeletingLibraryAsset(asset);
                }}
            />

            <Modal title="删除我的素材" open={Boolean(deletingLocalAsset)} onCancel={() => setDeletingLocalAsset(null)} onOk={() => void confirmDeleteLocalAsset()} okText="删除" okButtonProps={{ danger: true }} cancelText="取消">
                确定删除「{deletingLocalAsset?.title}」吗？删除后会从本地工作区我的素材中移除。
            </Modal>

            <Modal title="删除素材库素材" open={Boolean(deletingLibraryAsset)} onCancel={() => setDeletingLibraryAsset(null)} onOk={() => void confirmDeleteLibraryAsset()} okText="删除" okButtonProps={{ danger: true }} cancelText="取消">
                确定删除「{deletingLibraryAsset?.title}」吗？删除后会从服务器素材库中移除。
            </Modal>
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

function AssetCard({ asset, onOpen, onEdit, onCopy, onDownload, onDelete }: { asset: Asset; onOpen: () => void; onEdit: () => void; onCopy: (asset: Asset) => void; onDownload: (asset: Asset) => void; onDelete: () => void }) {
    const cover = asset.coverUrl || (asset.kind === "image" ? asset.data.dataUrl : "");
    const summary = assetSummary(asset);
    return (
        <Card hoverable className="overflow-hidden" styles={{ body: { padding: 0 } }} cover={<AssetCover title={asset.title} cover={cover} fallback={asset.kind === "text" ? asset.data.content : "暂无封面"} onOpen={onOpen} />}>
            <button type="button" className="block w-full text-left" onClick={onOpen}>
                <div className="p-4">
                    <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                            <h2 className="line-clamp-1 text-sm font-semibold text-stone-950 dark:text-stone-100">{asset.title}</h2>
                            <Typography.Text type="secondary" className="mt-1 block text-xs">
                                {asset.source || "未标注来源"}
                            </Typography.Text>
                        </div>
                        <Tag className="m-0 shrink-0 text-[11px]">{asset.kind === "image" ? "图片" : asset.kind === "video" ? "视频" : "文本"}</Tag>
                    </div>
                    <Typography.Paragraph type="secondary" ellipsis={{ rows: 3 }} className="!mb-0 !mt-2 !text-xs !leading-5">
                        {summary}
                    </Typography.Paragraph>
                    <AssetTagPreview tags={asset.tags || []} />
                </div>
            </button>
            <div className="flex flex-wrap items-center gap-2 px-4 pb-4">
                <Button size="small" onClick={onOpen}>
                    查看
                </Button>
                <Button size="small" icon={<PencilLine className="size-3.5" />} onClick={onEdit}>
                    编辑
                </Button>
                {asset.kind === "text" ? (
                    <Button size="small" icon={<Copy className="size-3.5" />} onClick={() => void onCopy(asset)}>
                        复制
                    </Button>
                ) : null}
                {asset.kind === "image" || asset.kind === "video" ? (
                    <Button size="small" icon={<Download className="size-3.5" />} onClick={() => onDownload(asset)}>
                        下载
                    </Button>
                ) : null}
                <Button size="small" danger icon={<Trash2 className="size-3.5" />} onClick={onDelete}>
                    删除
                </Button>
            </div>
        </Card>
    );
}

function LibraryAssetCard({
    asset,
    isAdmin,
    onOpen,
    onCopy,
    onDownload,
    onCopyToMine,
    onEdit,
    onDelete,
}: {
    asset: AssetLibraryItem;
    isAdmin: boolean;
    onOpen: () => void;
    onCopy: () => void;
    onDownload: () => void;
    onCopyToMine: () => void;
    onEdit: () => void;
    onDelete: () => void;
}) {
    const cover = asset.coverUrl || asset.url;
    return (
        <Card hoverable className="overflow-hidden" styles={{ body: { padding: 0 } }} cover={<AssetCover title={asset.title} cover={cover} fallback={asset.type === "text" ? asset.content : "暂无封面"} onOpen={onOpen} />}>
            <button type="button" className="block w-full text-left" onClick={onOpen}>
                <div className="p-4">
                    <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                            <h2 className="line-clamp-1 text-sm font-semibold text-stone-950 dark:text-stone-100">{asset.title}</h2>
                            <Typography.Text type="secondary" className="mt-1 block text-xs">
                                {asset.categoryPath || asset.category || "素材库"} · {assetPurposeLabel(asset.purpose)}
                            </Typography.Text>
                        </div>
                        <Tag className="m-0 shrink-0 text-[11px]">{asset.type === "image" ? "图片" : asset.type === "video" ? "视频" : "文本"}</Tag>
                    </div>
                    <Typography.Paragraph type="secondary" ellipsis={{ rows: 3 }} className="!mb-0 !mt-2 !text-xs !leading-5">
                        {asset.type === "text" ? asset.content : asset.url || asset.coverUrl}
                    </Typography.Paragraph>
                    <AssetTagPreview tags={asset.tags || []} />
                </div>
            </button>
            <div className="flex flex-wrap items-center gap-2 px-4 pb-4">
                <Button size="small" onClick={onOpen}>
                    查看
                </Button>
                {asset.type === "text" ? (
                    <Button size="small" icon={<Copy className="size-3.5" />} onClick={onCopy}>
                        复制
                    </Button>
                ) : (
                    <Button size="small" icon={<Download className="size-3.5" />} onClick={onDownload}>
                        下载
                    </Button>
                )}
                <Button size="small" icon={<Plus className="size-3.5" />} onClick={onCopyToMine}>
                    复制到我的素材
                </Button>
                {isAdmin ? (
                    <>
                        <Button size="small" icon={<PencilLine className="size-3.5" />} onClick={onEdit}>
                            编辑
                        </Button>
                        <Button size="small" danger icon={<Trash2 className="size-3.5" />} onClick={onDelete}>
                            删除
                        </Button>
                    </>
                ) : null}
            </div>
        </Card>
    );
}

function AssetCover({ title, cover, fallback, onOpen }: { title: string; cover: string; fallback: string; onOpen: () => void }) {
    return (
        <button type="button" className="block w-full text-left" onClick={onOpen}>
            {cover ? (
                <img src={cover} alt={title} className="aspect-[4/3] w-full object-cover" />
            ) : (
                <div className="flex aspect-[4/3] items-center justify-center bg-stone-100 p-5 text-center text-sm leading-6 text-stone-600 dark:bg-stone-900 dark:text-stone-300">{fallback || "暂无封面"}</div>
            )}
        </button>
    );
}

function AssetTagPreview({ tags }: { tags: string[] }) {
    return (
        <div className="mt-3 flex flex-wrap gap-1.5">
            {tags.slice(0, 3).map((tag) => (
                <Tag key={tag} className="m-0 text-[11px]">
                    {tag}
                </Tag>
            ))}
            {!tags.length ? <Tag className="m-0 text-[11px]">无标签</Tag> : null}
        </div>
    );
}

function AssetFormPreview({ title, tags, coverUrl, fallback }: { title: string; tags: string[]; coverUrl: string; fallback: string }) {
    return (
        <div className="rounded-xl border border-stone-200 bg-stone-50 p-4 dark:border-stone-800 dark:bg-stone-950">
            <Typography.Text strong>预览</Typography.Text>
            <div className="mt-3 overflow-hidden rounded-lg border border-stone-200 bg-background dark:border-stone-800">
                {coverUrl ? (
                    <img src={coverUrl} alt="" className="aspect-[4/3] w-full object-cover" />
                ) : (
                    <div className="flex aspect-[4/3] items-center justify-center bg-stone-100 p-5 text-center text-sm text-stone-500 dark:bg-stone-900">{fallback || "暂无封面"}</div>
                )}
                <div className="p-4">
                    <Typography.Text strong ellipsis className="block">
                        {title || "未命名素材"}
                    </Typography.Text>
                    <AssetTagPreview tags={tags || []} />
                </div>
            </div>
        </div>
    );
}

function AssetDrawer({ asset, onClose, onCopy, onDownload }: { asset: Asset | null; onClose: () => void; onCopy: (asset: Asset) => void; onDownload: (asset: Asset) => void }) {
    const cover = asset ? asset.coverUrl || (asset.kind === "image" ? asset.data.dataUrl : "") : "";
    return (
        <Drawer title="我的素材详情" open={Boolean(asset)} size="large" onClose={onClose}>
            {asset ? (
                <div className="space-y-5">
                    {cover ? (
                        <Image src={cover} alt={asset.title} className="rounded-lg" />
                    ) : (
                        <div className="rounded-lg border border-stone-200 bg-stone-50 p-5 text-sm leading-6 text-stone-600 dark:border-stone-800 dark:bg-stone-900 dark:text-stone-300">{asset.kind === "text" ? asset.data.content : "暂无封面"}</div>
                    )}
                    <AssetDetailHeader title={asset.title} kind={asset.kind} tags={asset.tags || []} category={asset.source} />
                    <div className="rounded-lg border border-stone-200 p-4 dark:border-stone-800">
                        <Typography.Text type="secondary" className="block text-xs">
                            内容
                        </Typography.Text>
                        {asset.kind === "text" ? (
                            <Typography.Paragraph className="mt-2 whitespace-pre-wrap">{asset.data.content}</Typography.Paragraph>
                        ) : asset.kind === "video" ? (
                            <video src={asset.data.url} controls className="mt-2 aspect-video w-full rounded-lg bg-black" />
                        ) : (
                            <Typography.Text className="mt-2 block">
                                {asset.data.width}x{asset.data.height} · {formatBytes(asset.data.bytes)} · {asset.data.mimeType}
                            </Typography.Text>
                        )}
                    </div>
                    {asset.note ? (
                        <div>
                            <Typography.Text type="secondary">备注</Typography.Text>
                            <Typography.Paragraph className="mt-1">{asset.note}</Typography.Paragraph>
                        </div>
                    ) : null}
                    <Space>
                        {asset.kind === "text" ? (
                            <Button type="primary" icon={<Copy className="size-4" />} onClick={() => onCopy(asset)}>
                                复制文本
                            </Button>
                        ) : null}
                        {asset.kind === "image" || asset.kind === "video" ? (
                            <Button type="primary" icon={<Download className="size-4" />} onClick={() => onDownload(asset)}>
                                {asset.kind === "video" ? "下载视频" : "下载图片"}
                            </Button>
                        ) : null}
                    </Space>
                </div>
            ) : null}
        </Drawer>
    );
}

function LibraryAssetDrawer({
    asset,
    isAdmin,
    onClose,
    onCopy,
    onDownload,
    onCopyToMine,
    onEdit,
    onDelete,
}: {
    asset: AssetLibraryItem | null;
    isAdmin: boolean;
    onClose: () => void;
    onCopy: (asset: AssetLibraryItem) => void;
    onDownload: (asset: AssetLibraryItem) => void;
    onCopyToMine: (asset: AssetLibraryItem) => void;
    onEdit: (asset: AssetLibraryItem) => void;
    onDelete: (asset: AssetLibraryItem) => void;
}) {
    const cover = asset?.coverUrl || asset?.url || "";
    return (
        <Drawer title="素材库详情" open={Boolean(asset)} size="large" onClose={onClose}>
            {asset ? (
                <div className="space-y-5">
                    {cover ? (
                        <Image src={cover} alt={asset.title} className="rounded-lg" />
                    ) : (
                        <div className="rounded-lg border border-stone-200 bg-stone-50 p-5 text-sm leading-6 text-stone-600 dark:border-stone-800 dark:bg-stone-900 dark:text-stone-300">{asset.content || "暂无封面"}</div>
                    )}
                    <AssetDetailHeader title={asset.title} kind={asset.type} tags={asset.tags || []} category={asset.categoryPath || asset.category} />
                    <Space size={[4, 4]} wrap>
                        <Tag>{assetPurposeLabel(asset.purpose)}</Tag>
                        <Tag>{assetSourceLabel(asset.source)}</Tag>
                    </Space>
                    <div className="rounded-lg border border-stone-200 p-4 dark:border-stone-800">
                        <Typography.Text type="secondary" className="block text-xs">
                            内容
                        </Typography.Text>
                        {asset.type === "text" ? <Typography.Paragraph className="mt-2 whitespace-pre-wrap">{asset.content}</Typography.Paragraph> : <Typography.Text className="mt-2 block">{asset.url || asset.coverUrl}</Typography.Text>}
                    </div>
                    {asset.description ? (
                        <div>
                            <Typography.Text type="secondary">描述</Typography.Text>
                            <Typography.Paragraph className="mt-1">{asset.description}</Typography.Paragraph>
                        </div>
                    ) : null}
                    <Space wrap>
                        <Button type="primary" icon={<Copy className="size-4" />} onClick={() => onCopy(asset)}>
                            {asset.type === "text" ? "复制文本" : "复制链接"}
                        </Button>
                        {asset.type !== "text" ? (
                            <Button icon={<Download className="size-4" />} onClick={() => onDownload(asset)}>
                                下载
                            </Button>
                        ) : null}
                        <Button icon={<Plus className="size-4" />} onClick={() => onCopyToMine(asset)}>
                            复制到我的素材
                        </Button>
                        {isAdmin ? (
                            <>
                                <Button icon={<PencilLine className="size-4" />} onClick={() => onEdit(asset)}>
                                    编辑
                                </Button>
                                <Button danger icon={<Trash2 className="size-4" />} onClick={() => onDelete(asset)}>
                                    删除
                                </Button>
                            </>
                        ) : null}
                    </Space>
                </div>
            ) : null}
        </Drawer>
    );
}

function AssetDetailHeader({ title, kind, tags, category }: { title: string; kind: string; tags: string[]; category?: string }) {
    return (
        <div>
            <Typography.Title level={4} className="!mb-2">
                {title}
            </Typography.Title>
            <Space size={[4, 4]} wrap>
                <Tag>{kind === "image" ? "图片" : kind === "video" ? "视频" : "文本"}</Tag>
                {category ? <Tag>{category}</Tag> : null}
                {tags.map((tag) => (
                    <Tag key={tag}>{tag}</Tag>
                ))}
            </Space>
        </div>
    );
}

function assetSummary(asset: Asset) {
    if (asset.kind === "text") return asset.data.content;
    return `${asset.data.width}x${asset.data.height} · ${formatBytes(asset.data.bytes)} · ${asset.data.mimeType}`;
}

function assetSearchText(asset: Asset) {
    return [asset.title, asset.source || "", asset.note || "", (asset.tags || []).join(" "), asset.kind === "text" ? asset.data.content : asset.data.mimeType].join(" ").toLowerCase();
}

function assetPurposeOptionsForMedia(media: MediaFilter) {
    return assetPurposeOptions.filter((item) => !item.media || !media || item.media === media);
}

function categoryPathForMedia(media: AssetKind) {
    if (media === "image") return "通用图片";
    if (media === "video") return "视频素材";
    return "文本素材";
}

function localAssetPurposeValue(asset: Asset) {
    return normalizeAssetPurposeValue(String(asset.metadata?.purpose || "generic"));
}

function localAssetSourceValue(asset: Asset) {
    return normalizeAssetSourceValue(String(asset.metadata?.source || asset.source || "local_upload"));
}

function localAssetCategoryPath(asset: Asset) {
    return String(asset.metadata?.categoryPath || categoryPathForMedia(asset.kind));
}

function normalizeAssetPurposeValue(value?: string) {
    return value === "mockup_base" ? "spec_template" : value || "generic";
}

function normalizeAssetCategoryPath(value?: string, purpose?: string) {
    if (value === "Mockup底版" || purpose === "mockup_base" || purpose === "spec_template") return "规格图模板";
    return value || "通用素材";
}

function normalizeAssetSourceValue(value?: string) {
    switch (value) {
        case "ai_generated":
            return "ai_generated";
        case "local_upload":
            return "local_upload";
        default:
            return "cloud_asset";
    }
}

function libraryAssetToAdminAsset(asset: AssetLibraryItem): AdminAsset {
    return {
        id: asset.id,
        title: asset.title,
        type: asset.type === "video" ? "video" : asset.type === "image" ? "image" : "text",
        mediaType: asset.mediaType || asset.type,
        scope: asset.scope || "library",
        coverUrl: asset.coverUrl,
        tags: asset.tags || [],
        category: asset.category,
        categoryPath: asset.categoryPath || asset.category,
        purpose: normalizeAssetPurposeValue(asset.purpose),
        source: normalizeAssetSourceValue(asset.source),
        description: asset.description,
        content: asset.content,
        url: asset.url,
        metadata: asset.metadata,
        createdAt: asset.createdAt,
        updatedAt: asset.updatedAt,
    };
}

function assetPurposeLabel(value?: string) {
    return assetPurposeOptions.find((item) => item.value === normalizeAssetPurposeValue(value))?.label || "通用素材";
}

function assetSourceLabel(value?: string) {
    return assetSourceOptions.find((item) => item.value === normalizeAssetSourceValue(value))?.label || "云端素材";
}

async function imageDataFromUrl(url: string): Promise<NonNullable<ImageDraft>> {
    const response = await fetch(url);
    if (!response.ok) throw new Error("download image failed");
    const blob = await response.blob();
    const objectUrl = URL.createObjectURL(blob);
    const meta = await readImageMeta(objectUrl);
    return { dataUrl: objectUrl, width: meta.width, height: meta.height, bytes: blob.size, mimeType: blob.type || meta.mimeType };
}

function mimeTypeFromURL(value: string) {
    const clean = value.split("?")[0].toLowerCase();
    if (clean.endsWith(".jpg") || clean.endsWith(".jpeg")) return "image/jpeg";
    if (clean.endsWith(".webp")) return "image/webp";
    if (clean.endsWith(".gif")) return "image/gif";
    if (clean.endsWith(".mp4")) return "video/mp4";
    if (clean.endsWith(".webm")) return "video/webm";
    return "image/png";
}

function mimeExtension(mimeType: string) {
    if (mimeType.includes("jpeg")) return "jpg";
    if (mimeType.includes("webp")) return "webp";
    if (mimeType.includes("gif")) return "gif";
    if (mimeType.includes("mp4")) return "mp4";
    if (mimeType.includes("webm")) return "webm";
    return "png";
}

function readVideoMeta(url: string) {
    return new Promise<{ width: number; height: number }>((resolve) => {
        const video = document.createElement("video");
        const done = () => resolve({ width: video.videoWidth || 1280, height: video.videoHeight || 720 });
        video.onloadedmetadata = done;
        video.onerror = done;
        video.src = url;
    });
}

function sortedUnique(values: string[]) {
    return Array.from(new Set(values.filter(Boolean))).sort((a, b) => a.localeCompare(b, "zh-CN"));
}
