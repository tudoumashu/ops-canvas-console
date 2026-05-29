import { apiGet, compactApiParams } from "@/services/api/request";

export type Prompt = {
    id: string;
    title: string;
    coverUrl: string;
    prompt: string;
    tags: string[];
    category: string;
    domain: string;
    stage: string;
    provider: string;
    model: string;
    mode: string;
    inputType: string;
    outputType: string;
    status: string;
    metadata?: Record<string, unknown>;
    githubUrl: string;
    preview: string;
    createdAt: string;
    updatedAt: string;
};

export const ALL_PROMPTS_OPTION = "全部";

export type PromptListResponse = {
    items: Prompt[];
    tags: string[];
    freeTags: string[];
    categories: string[];
    facets: {
        categories: string[];
        domains: string[];
        stages: string[];
        providers: string[];
        models: string[];
        modes: string[];
        inputTypes: string[];
        outputTypes: string[];
        statuses: string[];
    };
    total: number;
};

export async function fetchPrompts({
    keyword = "",
    tag = [],
    category = ALL_PROMPTS_OPTION,
    domain = "",
    stage = "",
    provider = "",
    model = "",
    mode = "",
    inputType = "",
    outputType = "",
    status = "",
    page,
    pageSize,
}: {
    keyword?: string;
    tag?: string[];
    category?: string;
    domain?: string;
    stage?: string;
    provider?: string;
    model?: string;
    mode?: string;
    inputType?: string;
    outputType?: string;
    status?: string;
    page?: number;
    pageSize?: number;
} = {}) {
    return apiGet<PromptListResponse>(
        "/api/prompts",
        compactApiParams({
            ...(keyword ? { keyword } : {}),
            ...(tag.length ? { tag } : {}),
            ...(category !== ALL_PROMPTS_OPTION ? { category } : {}),
            ...(domain ? { domain } : {}),
            ...(stage ? { stage } : {}),
            ...(provider ? { provider } : {}),
            ...(model ? { model } : {}),
            ...(mode ? { mode } : {}),
            ...(inputType ? { inputType } : {}),
            ...(outputType ? { outputType } : {}),
            ...(status ? { status } : {}),
            ...(page ? { page } : {}),
            ...(pageSize ? { pageSize } : {}),
        }),
    );
}

export function formatPromptDate(value: string) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "" : new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).format(date);
}
