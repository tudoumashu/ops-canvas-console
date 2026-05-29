import { apiGet, compactApiParams } from "@/services/api/request";

export type AssetLibraryItem = {
    id: string;
    title: string;
    type: "text" | "image" | "video";
    mediaType: "text" | "image" | "video" | string;
    scope: "local" | "library" | string;
    categoryPath: string;
    purpose: string;
    source: string;
    coverUrl: string;
    tags: string[];
    category: string;
    description: string;
    content: string;
    url: string;
    metadata?: Record<string, unknown>;
    createdAt: string;
    updatedAt: string;
};

export type AssetLibraryResponse = {
    items: AssetLibraryItem[];
    tags: string[];
    freeTags: string[];
    facets: {
        mediaTypes: string[];
        categoryPaths: string[];
        purposes: string[];
        sources: string[];
    };
    total: number;
};

export type AssetLibraryQuery = {
    keyword?: string;
    type?: string;
    mediaType?: string;
    scope?: string;
    categoryPath?: string;
    purpose?: string;
    source?: string;
    tag?: string[];
    page?: number;
    pageSize?: number;
};

export async function fetchAssetLibrary(query: AssetLibraryQuery = {}) {
    return apiGet<AssetLibraryResponse>("/api/assets", compactApiParams(query));
}
