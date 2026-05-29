"use client";

import { useMemo } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";

import { ALL_PROMPTS_OPTION, fetchPrompts } from "@/services/api/prompts";

export const PROMPT_PAGE_SIZE = 20;

export type PromptListFilters = {
    keyword: string;
    tags: string[];
    category: string;
    domain?: string;
    stage?: string;
    provider?: string;
    model?: string;
    mode?: string;
    inputType?: string;
    outputType?: string;
    status?: string;
    enabled?: boolean;
};

export function usePromptList({ keyword, tags, category, domain = "", stage = "", provider = "", model = "", mode = "", inputType = "", outputType = "", status = "", enabled = true }: PromptListFilters) {
    const query = useInfiniteQuery({
        queryKey: ["prompts", keyword, tags, category, domain, stage, provider, model, mode, inputType, outputType, status],
        queryFn: ({ pageParam }) => fetchPrompts({ keyword, tag: tags, category, domain, stage, provider, model, mode, inputType, outputType, status, page: pageParam, pageSize: PROMPT_PAGE_SIZE }),
        initialPageParam: 1,
        getNextPageParam: (lastPage, pages) => (pages.reduce((total, page) => total + page.items.length, 0) < lastPage.total ? pages.length + 1 : undefined),
        enabled,
    });
    const firstPage = query.data?.pages[0];
    return {
        query,
        items: useMemo(() => query.data?.pages.flatMap((page) => page.items) || [], [query.data?.pages]),
        tags: useMemo(() => [ALL_PROMPTS_OPTION, ...(firstPage?.freeTags || firstPage?.tags || [])], [firstPage?.freeTags, firstPage?.tags]),
        categories: useMemo(() => [ALL_PROMPTS_OPTION, ...(firstPage?.categories || [])], [firstPage?.categories]),
        facets: firstPage?.facets,
        total: firstPage?.total || 0,
    };
}
