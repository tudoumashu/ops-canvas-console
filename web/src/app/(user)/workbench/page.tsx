"use client";

import { Suspense } from "react";
import { useMemo } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Select } from "antd";

import { ImageWorkbench } from "./components/image-workbench";
import { TextWorkbench } from "./components/text-workbench";
import { VideoWorkbench } from "./components/video-workbench";

type WorkbenchMode = "image" | "text" | "video";

const workbenchModeOptions = [
    { label: "图片创作", value: "image" },
    { label: "文本创作", value: "text" },
    { label: "视频创作", value: "video" },
];

export default function WorkbenchPage() {
    return (
        <Suspense fallback={<div className="flex h-full items-center justify-center text-sm text-stone-500">加载工作台...</div>}>
            <WorkbenchBody />
        </Suspense>
    );
}

function WorkbenchBody() {
    const router = useRouter();
    const searchParams = useSearchParams();
    const mode = useMemo<WorkbenchMode>(() => {
        const value = searchParams.get("mode");
        return value === "text" || value === "video" ? value : "image";
    }, [searchParams]);

    const modeSwitcher = (
        <Select
            className="min-w-32 text-xl font-semibold"
            value={mode}
            options={workbenchModeOptions}
            variant="borderless"
            onChange={(value) => {
                router.replace(`/workbench?mode=${value}`);
            }}
        />
    );

    return (
        <div className="h-full min-h-0 overflow-hidden">
            {mode === "image" ? <ImageWorkbench modeSwitcher={modeSwitcher} /> : null}
            {mode === "text" ? <TextWorkbench modeSwitcher={modeSwitcher} /> : null}
            {mode === "video" ? <VideoWorkbench modeSwitcher={modeSwitcher} /> : null}
        </div>
    );
}
