"use client";

import { Alert } from "antd";

import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";

export function LocalWorkspaceStatusAlert({ message }: { message: string }) {
    const status = useLocalWorkspaceStore((state) => state.status);
    const serveAvailable = useLocalWorkspaceStore((state) => state.serveAvailable);
    const lastError = useLocalWorkspaceStore((state) => state.lastError);

    if (status === "connected") return null;

    const description = lastError || (serveAvailable ? "opsc serve 已启动，但当前浏览器 session 未建立或已过期，请在顶部本地工作区入口输入 launch secret。" : "未检测到 opsc serve，请先启动本地工作区服务。");

    return <Alert type="warning" showIcon title={message} description={description} />;
}
