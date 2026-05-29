import axios from "axios";

import { dataUrlToFile } from "@/lib/image-utils";
import { isFlowVideoModel } from "@/lib/model-presets";
import { imageToDataUrl } from "@/services/image-storage";
import { buildApiUrl, type AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import type { ReferenceImage } from "@/types/image";

type VideoResponse = { id: string; status?: string; error?: { message?: string } };
type ApiVideoResponse = VideoResponse | { code?: number; data?: VideoResponse | null; msg?: string };

function aiApiUrl(config: AiConfig, path: string) {
    return config.channelMode === "remote" ? `/api/v1${path}` : buildApiUrl(config.baseUrl, path);
}

function aiHeaders(config: AiConfig) {
    const token = useUserStore.getState().token;
    return config.channelMode === "remote" ? (token ? { Authorization: `Bearer ${token}` } : undefined) : { Authorization: `Bearer ${config.apiKey}` };
}

function refreshRemoteUser(config: AiConfig) {
    if (config.channelMode === "remote") void useUserStore.getState().hydrateUser();
}

export async function requestVideoGeneration(config: AiConfig, prompt: string, references: ReferenceImage[] = []) {
    const model = config.model || config.videoModel;
    const flowVideo = isFlowVideoModel(model);
    const body = new FormData();
    body.append("model", model);
    body.append("prompt", prompt);
    const seconds = normalizeVideoSeconds(config.videoSeconds, flowVideo);
    body.append("seconds", seconds);
    const size = normalizeVideoSize(config.size, flowVideo);
    if (size) body.append("size", size);
    const resolution = normalizeVideoResolution(config.vquality, flowVideo);
    const referenceMode = normalizeReferenceMode(config.videoReferenceMode);
    body.append("resolution_name", resolution);
    body.append("reference_mode", referenceMode);
    body.append("generation_config", JSON.stringify({ aspectRatio: videoAspectRatio(size), resolution, duration: Number(seconds) || 6, referenceMode }));
    body.append("preset", "normal");
    const files = await Promise.all(videoReferencesForMode(references, referenceMode).map(async (image) => dataUrlToFile({ ...image, dataUrl: await imageToDataUrl(image) })));
    files.forEach((file) => body.append("input_reference[]", file));
    try {
        const created = unwrapVideoResponse((await axios.post<ApiVideoResponse>(aiApiUrl(config, "/videos"), body, { headers: aiHeaders(config) })).data);
        if (!created.id) throw new Error("视频接口没有返回任务 ID");
        for (;;) {
            const video = unwrapVideoResponse((await axios.get<ApiVideoResponse>(aiApiUrl(config, `/videos/${created.id}`), { headers: aiHeaders(config), params: config.channelMode === "remote" ? { model } : undefined })).data);
            if (video.status === "completed") break;
            if (video.status === "failed" || video.status === "cancelled") throw new Error(video.error?.message || "视频生成失败");
            await new Promise((resolve) => setTimeout(resolve, 2500));
        }
        const content = await axios.get<Blob>(aiApiUrl(config, `/videos/${created.id}/content`), { headers: aiHeaders(config), params: config.channelMode === "remote" ? { model } : undefined, responseType: "blob" });
        await assertVideoBlob(content.data);
        refreshRemoteUser(config);
        return content.data;
    } catch (error) {
        throw new Error(readAxiosError(error, "视频生成失败"));
    }
}

function normalizeReferenceMode(value?: string) {
    return ["text", "frame", "asset", "extend"].includes(value || "") ? value || "text" : "text";
}

function videoReferencesForMode(references: ReferenceImage[], mode: string) {
    if (mode === "text") return [];
    if (mode === "asset") return references.slice(0, 3);
    if (mode === "frame") return references.slice(0, 2);
    if (mode === "extend") return references.slice(0, 1);
    return references.slice(0, 2);
}

function videoAspectRatio(size: string | null) {
    if (!size) return "";
    const match = size.match(/^(\d+)x(\d+)$/);
    if (!match) return "";
    const width = Number(match[1]);
    const height = Number(match[2]);
    if (!width || !height) return "";
    return width >= height ? "16:9" : "9:16";
}

function normalizeVideoSeconds(value: string, flowVideo = false) {
    const seconds = Math.floor(Number(value) || 6);
    if (flowVideo) return String([4, 6].includes(seconds) ? seconds : 6);
    return String(Math.max(1, Math.min(20, seconds)));
}

function normalizeVideoSize(value: string, flowVideo = false) {
    if (value === "auto") return null;
    const size = value || "1280x720";
    if (flowVideo) return ["720x1280", "9:16", "2:3", "3:4", "portrait"].includes(size) ? "720x1280" : "1280x720";
    if (/^\d+x\d+$/.test(size)) return size;
    return ["9:16", "2:3", "3:4"].includes(size) ? "720x1280" : "1280x720";
}

function normalizeVideoResolution(value: string, flowVideo = false) {
    if (value === "4k" || value === "4K") return "4k";
    if (value === "1080" || value === "1080p") return "1080p";
    if (flowVideo) return "720p";
    if (value === "low") return "480p";
    if (value === "auto" || value === "high" || value === "medium") return "720p";
    const resolution = value.replace(/p$/i, "") || "720";
    return `${resolution}p`;
}

function unwrapVideoResponse(payload: ApiVideoResponse): VideoResponse {
    if (!payload) throw new Error("接口没有返回视频任务");
    if ("code" in payload && typeof payload.code === "number") {
        if (payload.code !== 0) throw new Error(payload.msg || "请求失败");
        if (!payload.data) throw new Error("接口没有返回视频任务");
        return payload.data;
    }
    return payload as VideoResponse;
}

function readAxiosError(error: unknown, fallback: string) {
    if (axios.isAxiosError<{ error?: { message?: string }; msg?: string; code?: number }>(error)) {
        const responseData = error.response?.data;
        return responseData?.msg || responseData?.error?.message || (error.response?.status ? `${fallback}：${error.response.status}` : fallback);
    }
    return error instanceof Error ? error.message : fallback;
}

async function assertVideoBlob(blob: Blob) {
    if (!blob.type.includes("json")) return;
    let payload: { code?: number; msg?: string };
    try {
        payload = JSON.parse(await blob.text()) as { code?: number; msg?: string };
    } catch {
        return;
    }
    if (typeof payload.code === "number" && payload.code !== 0) throw new Error(payload.msg || "视频下载失败");
}
