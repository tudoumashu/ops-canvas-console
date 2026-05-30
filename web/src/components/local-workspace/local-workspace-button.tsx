"use client";

import { HardDrive, PlugZap, RefreshCw } from "lucide-react";
import { useState } from "react";
import { Alert, App, Button, Input, Modal, Space, Typography } from "antd";

import { DEFAULT_LOCAL_WORKSPACE_BASE_URL } from "@/services/local-workspace";
import { useConfigStore } from "@/stores/use-config-store";
import { useLocalWorkspaceStore } from "@/stores/use-local-workspace-store";
import { LocalProjectsPanel } from "./local-projects-panel";

export function LocalWorkspaceButton() {
    const { message } = App.useApp();
    const baseUrl = useLocalWorkspaceStore((state) => state.baseUrl);
    const status = useLocalWorkspaceStore((state) => state.status);
    const serveAvailable = useLocalWorkspaceStore((state) => state.serveAvailable);
    const workspace = useLocalWorkspaceStore((state) => state.workspace);
    const lastError = useLocalWorkspaceStore((state) => state.lastError);
    const connect = useLocalWorkspaceStore((state) => state.connect);
    const refresh = useLocalWorkspaceStore((state) => state.refresh);
    const disconnect = useLocalWorkspaceStore((state) => state.disconnect);
    const loadLocalProfile = useConfigStore((state) => state.loadLocalProfile);
    const clearLocalProfile = useConfigStore((state) => state.clearLocalProfile);

    const [open, setOpen] = useState(false);
    const [draftBaseUrl, setDraftBaseUrl] = useState(baseUrl || DEFAULT_LOCAL_WORKSPACE_BASE_URL);
    const [launchSecret, setLaunchSecret] = useState("");
    const [submitting, setSubmitting] = useState(false);

    const connected = status === "connected" && Boolean(workspace);

    const handleOpen = () => {
        setDraftBaseUrl(baseUrl || DEFAULT_LOCAL_WORKSPACE_BASE_URL);
        setLaunchSecret("");
        setOpen(true);
    };

    const handleConnect = async () => {
        setSubmitting(true);
        try {
            const workspace = await connect(draftBaseUrl, launchSecret);
            setLaunchSecret("");
            try {
                await loadLocalProfile(useLocalWorkspaceStore.getState().baseUrl, workspace.defaultProfileId);
            } catch (error) {
                clearLocalProfile();
                message.warning(error instanceof Error ? error.message : "读取本地 profile 失败");
            }
            message.success("本地工作区已连接");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "连接本地工作区失败");
        } finally {
            setSubmitting(false);
        }
    };

    const handleRefresh = async () => {
        const next = await refresh();
        if (next) {
            try {
                await loadLocalProfile(useLocalWorkspaceStore.getState().baseUrl, next.defaultProfileId);
            } catch (error) {
                clearLocalProfile();
                message.warning(error instanceof Error ? error.message : "读取本地 profile 失败");
            }
            message.success("本地工作区连接正常");
        } else {
            clearLocalProfile();
            message.warning("本地工作区未连接");
        }
    };

    const handleDisconnect = () => {
        disconnect();
        clearLocalProfile();
    };

    return (
        <>
            <button
                type="button"
                className="inline-flex size-8 shrink-0 items-center justify-center text-stone-600 transition hover:text-stone-950 dark:text-stone-300 dark:hover:text-white [&_svg]:size-4"
                onClick={handleOpen}
                aria-label="本地工作区"
                title={connected ? `本地工作区：${workspace?.name || workspace?.id}` : "连接本地工作区"}
            >
                {connected ? <HardDrive className="size-4" /> : <PlugZap className="size-4" />}
            </button>

            <Modal title="本地工作区" open={open} onCancel={() => setOpen(false)} footer={null} width={760} destroyOnHidden>
                <div className="space-y-5">
                    {connected ? (
                        <Alert
                            type="success"
                            showIcon
                            message={workspace?.name || "已连接"}
                            description={
                                <span>
                                    workspaceId: <Typography.Text code>{workspace?.id}</Typography.Text>
                                </span>
                            }
                        />
                    ) : serveAvailable ? (
                        <Alert type="warning" showIcon message="opsc serve 已启动，等待授权" description={lastError || "请输入本次启动生成的 launch secret 建立浏览器 session。"} />
                    ) : (
                        <Alert type="warning" showIcon message="未检测到本地工作区服务" description={lastError || "启动 opsc serve 后，使用 launch secret 建立浏览器 session。"} />
                    )}

                    <div className="space-y-2">
                        <Typography.Text strong>服务地址</Typography.Text>
                        <Input value={draftBaseUrl} placeholder={DEFAULT_LOCAL_WORKSPACE_BASE_URL} onChange={(event) => setDraftBaseUrl(event.target.value)} />
                    </div>

                    <div className="space-y-2">
                        <Typography.Text strong>Launch secret</Typography.Text>
                        <Input.Password value={launchSecret} autoComplete="off" placeholder="只用于一次性建立本地 session，不会保存到浏览器状态" onChange={(event) => setLaunchSecret(event.target.value)} />
                    </div>

                    <Typography.Paragraph type="secondary" className="!mb-0 !text-xs">
                        启动示例：<Typography.Text code>opsc serve --origin {typeof window === "undefined" ? "http://127.0.0.1:3000" : window.location.origin}</Typography.Text>，然后读取 runtime state 目录里的 <Typography.Text code>launch.secret</Typography.Text>。
                    </Typography.Paragraph>

                    <Space wrap>
                        <Button type="primary" icon={<PlugZap className="size-4" />} loading={submitting} onClick={() => void handleConnect()}>
                            连接
                        </Button>
                        <Button icon={<RefreshCw className="size-4" />} onClick={() => void handleRefresh()}>
                            刷新状态
                        </Button>
                        {connected ? <Button onClick={handleDisconnect}>断开</Button> : null}
                    </Space>

                    {connected ? <LocalProjectsPanel baseUrl={baseUrl} /> : null}
                </div>
            </Modal>
        </>
    );
}
