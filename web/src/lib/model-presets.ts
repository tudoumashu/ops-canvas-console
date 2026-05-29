"use client";

import type { AiConfig } from "@/stores/use-config-store";

export type ModelModality = "any" | "image" | "text" | "video";

export type ModelOption = {
    value: string;
    label: string;
    description?: string;
    group: "common" | "flow" | "advanced";
};

export type FlowImageFamily = "nano-pro" | "nano-2" | "imagen-4";
export type FlowVideoFamily = "veo-lite" | "veo-fast" | "veo-quality";

export type FlowOption = {
    value: string;
    label: string;
    hint?: string;
};

export type FlowAspectOption = FlowOption & {
    width: number;
    height: number;
    icon: "landscape" | "portrait" | "square";
};

const flowImageLabels: Array<{ match: RegExp; label: string; description: string }> = [
    { match: /^gemini-3\.0-pro-image/i, label: "Nano Banana Pro", description: "Google Flow 图片模型" },
    { match: /^gemini-3\.1-flash-image/i, label: "Nano Banana 2", description: "Google Flow 快速图片模型" },
    { match: /^imagen-4/i, label: "Imagen 4", description: "Google Imagen 图片模型" },
];

const flowVideoLabels: Array<{ match: RegExp; label: string }> = [
    { match: /omni.*flash|veo_3_1_omni/i, label: "Omni Flash" },
    { match: /veo_3_1_.*lite/i, label: "Veo 3.1 Lite" },
    { match: /veo_3_1_.*fast/i, label: "Veo 3.1 Fast" },
    { match: /veo_3_1/i, label: "Veo 3.1 Quality" },
];

export function modelMatchesModality(model: string, modality: Exclude<ModelModality, "any">) {
    const name = model.toLowerCase();
    const isVideo = name.includes("video") || name.includes("veo") || name.includes("sora") || name.includes("_t2v") || name.includes("_i2v") || name.includes("_r2v") || name.includes("grok-imagine");
    const isImage = name.includes("image") || name.includes("imagen") || name.includes("gpt-image") || name.includes("nano-banana") || name.includes("gemini-3.0-pro-image") || name.includes("gemini-3.1-flash-image");
    if (modality === "video") return isVideo;
    if (modality === "image") return isImage && !isVideo;
    return !isImage && !isVideo;
}

export function modelDisplayName(model: string) {
    const image = flowImageLabels.find((item) => item.match.test(model));
    if (image) return image.label;
    const video = flowVideoLabels.find((item) => item.match.test(model));
    if (video) return video.label;
    return model;
}

export function modelDescription(model: string) {
    const image = flowImageLabels.find((item) => item.match.test(model));
    if (image) return image.description;
    const video = flowVideoLabels.find((item) => item.match.test(model));
    if (video) return "Google Flow 视频模型";
    return "";
}

export function modelOptionsFor(config: AiConfig, modality: ModelModality, current?: string): ModelOption[] {
    const raw = Array.from(new Set([current, ...config.models].filter((item): item is string => Boolean(item))));
    const filtered = modality === "any" ? raw : raw.filter((model) => modelMatchesModality(model, modality));
    const options = filtered.map((model) => {
        const label = modelDisplayName(model);
        const description = modelDescription(model);
        const isFriendly = label !== model;
        const isFlow = isFriendly || /veo|imagen|gemini-3\.[01].*image/i.test(model);
        return {
            value: model,
            label,
            description,
            group: isFlow ? ("flow" as const) : isFriendly ? ("common" as const) : ("advanced" as const),
        };
    });
    return dedupeFriendlyOptions(options);
}

function dedupeFriendlyOptions(options: ModelOption[]) {
    const seen = new Set<string>();
    const result: ModelOption[] = [];
    for (const option of options) {
        const key = option.group === "advanced" ? `${option.group}:${option.value}` : `${option.group}:${option.label}`;
        if (option.group !== "advanced" && seen.has(key)) continue;
        seen.add(key);
        result.push(option);
    }
    return result;
}

export function flowVideoModeLabel(model: string) {
    const name = model.toLowerCase();
    if (name.includes("_r2v")) return "素材模式";
    if (name.includes("_i2v") || name.includes("interpolation")) return "帧模式";
    if (name.includes("extend")) return "续写模式";
    if (name.includes("_t2v")) return "文本模式";
    return "";
}

export function flowVideoRatioLabel(model: string) {
    const name = model.toLowerCase();
    if (name.includes("portrait")) return "竖屏";
    if (name.includes("landscape")) return "横屏";
    return "";
}

export function isFlowVideoModel(model: string) {
    return /veo_3_1|omni.*flash/i.test(model);
}

export function isFlowImageModel(model: string) {
    return /^gemini-3\.[01].*image/i.test(model) || /^imagen-4/i.test(model);
}

export function flowImageFamily(model: string): FlowImageFamily | null {
    const name = model.toLowerCase();
    if (name.startsWith("gemini-3.0-pro-image")) return "nano-pro";
    if (name.startsWith("gemini-3.1-flash-image")) return "nano-2";
    if (name.startsWith("imagen-4")) return "imagen-4";
    return null;
}

export function flowVideoFamily(model: string): FlowVideoFamily | null {
    const name = model.toLowerCase();
    if (!isFlowVideoModel(name)) return null;
    if (name.includes("lite")) return "veo-lite";
    if (name.includes("fast")) return "veo-fast";
    return "veo-quality";
}

export function flowImageAspectOptions(model: string): FlowAspectOption[] {
    const common: FlowAspectOption[] = [
        { value: "landscape", label: "横屏", hint: "16:9", width: 16, height: 9, icon: "landscape" },
        { value: "portrait", label: "竖屏", hint: "9:16", width: 9, height: 16, icon: "portrait" },
    ];
    if (flowImageFamily(model) === "imagen-4") return common;
    return [
        ...common,
        { value: "square", label: "方图", hint: "1:1", width: 1, height: 1, icon: "square" },
        { value: "four-three", label: "4:3", width: 4, height: 3, icon: "landscape" },
        { value: "three-four", label: "3:4", width: 3, height: 4, icon: "portrait" },
    ];
}

export function flowImageQualityOptions(model: string): FlowOption[] {
    if (flowImageFamily(model) === "imagen-4") return [];
    return [
        { value: "low", label: "1x", hint: "标准" },
        { value: "medium", label: "2K" },
        { value: "high", label: "4K" },
    ];
}

export function flowVideoReferenceModeOptions(): FlowOption[] {
    return [
        { value: "text", label: "文本", hint: "不上传参考" },
        { value: "frame", label: "帧", hint: "首帧/首尾帧" },
        { value: "asset", label: "素材", hint: "1-3 个参考" },
        { value: "extend", label: "续写", hint: "1 个视频参考" },
    ];
}

export function flowVideoSizeOptions(): FlowAspectOption[] {
    return [
        { value: "1280x720", label: "横屏", hint: "16:9", width: 16, height: 9, icon: "landscape" },
        { value: "720x1280", label: "竖屏", hint: "9:16", width: 9, height: 16, icon: "portrait" },
    ];
}

export function flowVideoSecondOptions(): FlowOption[] {
    return [
        { value: "4", label: "4s" },
        { value: "6", label: "6s" },
    ];
}

export function flowVideoResolutionOptions(): FlowOption[] {
    return [
        { value: "720", label: "默认", hint: "720p" },
        { value: "1080p", label: "1080p" },
        { value: "4k", label: "4K" },
    ];
}

export function flowImageSizeLabel(model: string, value: string) {
    return flowImageAspectOptions(model).find((item) => item.value === value)?.label || value;
}

export function flowImageQualityLabel(model: string, value: string) {
    return flowImageQualityOptions(model).find((item) => item.value === value)?.label || value;
}

export function flowVideoSizeLabel(value: string) {
    return flowVideoSizeOptions().find((item) => item.value === value)?.label || value;
}

export function flowVideoResolutionLabel(value: string) {
    return flowVideoResolutionOptions().find((item) => item.value === value)?.label || value;
}
