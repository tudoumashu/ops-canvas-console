"use client";

import { BookOpen, CheckSquare, ClipboardCopy, History, LoaderCircle, Plus, Sparkles, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { App, Button, Checkbox, Drawer, Empty, Input, Modal, Tag, Typography } from "antd";
import localforage from "localforage";
import { nanoid } from "nanoid";

import { ModelPicker } from "@/components/model-picker";
import { PromptSelectDialog } from "@/components/prompts/prompt-select-dialog";
import { formatDuration } from "@/lib/image-utils";
import { requestImageQuestion, type ChatCompletionMessage } from "@/services/api/image";
import { useConfigStore, useEffectiveConfig, type AiConfig } from "@/stores/use-config-store";

type TextGenerationLog = {
    id: string;
    createdAt: number;
    title: string;
    prompt: string;
    result: string;
    error?: string;
    time: string;
    model: string;
    config: Pick<AiConfig, "model" | "textModel">;
    durationMs: number;
    status: "成功" | "失败";
};

const logStore = localforage.createInstance({ name: "infinite-canvas", storeName: "text_generation_logs" });

export function TextWorkbench({ modeSwitcher }: { modeSwitcher?: ReactNode }) {
    const { message } = App.useApp();
    const effectiveConfig = useEffectiveConfig();
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const isAiConfigReady = useConfigStore((state) => state.isAiConfigReady);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const [prompt, setPrompt] = useState("");
    const [result, setResult] = useState("");
    const [logs, setLogs] = useState<TextGenerationLog[]>([]);
    const [running, setRunning] = useState(false);
    const [logsOpen, setLogsOpen] = useState(false);
    const [startedAt, setStartedAt] = useState(0);
    const [elapsedMs, setElapsedMs] = useState(0);
    const [promptDialogOpen, setPromptDialogOpen] = useState(false);
    const [selectedLogIds, setSelectedLogIds] = useState<string[]>([]);
    const [previewLog, setPreviewLog] = useState<TextGenerationLog | null>(null);
    const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);

    const model = effectiveConfig.textModel || effectiveConfig.model;

    useEffect(() => {
        void refreshLogs();
    }, []);

    useEffect(() => {
        if (!running || !startedAt) return;
        const timer = window.setInterval(() => setElapsedMs(performance.now() - startedAt), 1000);
        return () => window.clearInterval(timer);
    }, [running, startedAt]);

    const runTextGeneration = async () => {
        const text = prompt.trim();
        if (!text) {
            message.warning("请先输入提示词");
            return;
        }
        if (!isAiConfigReady(effectiveConfig, model)) {
            openConfigDialog(true);
            return;
        }
        setRunning(true);
        setResult("");
        setElapsedMs(0);
        setStartedAt(performance.now());
        setPreviewLog(null);
        const start = performance.now();
        try {
            const messages: ChatCompletionMessage[] = [{ role: "user", content: text }];
            const answer = await requestImageQuestion({ ...effectiveConfig, model }, messages, (delta) => setResult((current) => current + delta));
            setResult(answer);
            saveLog(buildLog({ prompt: text, result: answer, model, durationMs: performance.now() - start, status: "成功" }));
            message.success("文本已生成");
        } catch (error) {
            const errorMessage = error instanceof Error ? error.message : "文本生成失败";
            saveLog(buildLog({ prompt: text, result: "", error: errorMessage, model, durationMs: performance.now() - start, status: "失败" }));
            message.error(errorMessage);
        } finally {
            setRunning(false);
        }
    };

    const copyResult = async () => {
        if (!result.trim()) return;
        await navigator.clipboard.writeText(result);
        message.success("已复制结果");
    };

    const createSession = () => {
        setPrompt("");
        setResult("");
        setElapsedMs(0);
        setStartedAt(0);
        setSelectedLogIds([]);
        setPreviewLog(null);
    };

    const deleteSelectedLogs = () => {
        void Promise.all(selectedLogIds.map((id) => logStore.removeItem(id))).then(refreshLogs);
        if (previewLog && selectedLogIds.includes(previewLog.id)) {
            setPreviewLog(null);
            setResult("");
        }
        setSelectedLogIds([]);
        setDeleteConfirmOpen(false);
    };

    const previewGenerationLog = (log: TextGenerationLog) => {
        setPreviewLog(log);
        setLogsOpen(false);
        setPrompt(log.prompt);
        setResult(log.result || log.error || "");
        if (log.config.textModel || log.model) updateConfig("textModel", log.config.textModel || log.model);
    };

    const saveLog = (log: TextGenerationLog) => {
        void logStore.setItem(log.id, log).then(refreshLogs);
    };

    const refreshLogs = async () => setLogs(await readStoredLogs());

    return (
        <div className="flex h-full flex-col overflow-hidden bg-stone-50 text-stone-900 dark:bg-stone-950 dark:text-stone-100">
            <main className="grid min-h-0 flex-1 grid-cols-1 gap-3 overflow-y-auto p-3 lg:grid-cols-[300px_minmax(0,1fr)] lg:overflow-hidden xl:grid-cols-[320px_minmax(0,1fr)]">
                <aside className="thin-scrollbar hidden min-h-0 overflow-y-auto rounded-lg border border-stone-200 bg-card p-4 shadow-sm dark:border-stone-800 lg:block">
                    <LogPanel logs={logs} selectedLogIds={selectedLogIds} activeLogId={previewLog?.id} onSelectedLogIdsChange={setSelectedLogIds} onCreateSession={createSession} onDeleteSelected={() => setDeleteConfirmOpen(true)} onPreviewLog={previewGenerationLog} />
                </aside>

                <section className="grid gap-3 lg:min-h-0 lg:overflow-hidden xl:grid-cols-[420px_minmax(0,1fr)]">
                    <div className="thin-scrollbar flex flex-col rounded-lg border border-stone-200 bg-card p-4 shadow-sm dark:border-stone-800 lg:min-h-0 lg:overflow-y-auto">
                        <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">{modeSwitcher || <h1 className="text-2xl font-semibold text-stone-950 dark:text-stone-100">文本创作</h1>}</div>
                            <Button className="lg:hidden" icon={<History className="size-4" />} onClick={() => setLogsOpen(true)}>
                                记录
                            </Button>
                        </div>

                        <div className="mt-6 space-y-5">
                            <div>
                                <div className="mb-2 flex items-center justify-between gap-3">
                                    <span className="text-base font-semibold">提示词</span>
                                    <Button size="small" icon={<BookOpen className="size-3.5" />} onClick={() => setPromptDialogOpen(true)}>
                                        查看提示词中心
                                    </Button>
                                </div>
                                <Input.TextArea value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={18} placeholder="输入文本生成要求" />
                            </div>
                            <label className="block min-w-0">
                                <span className="mb-2 block text-base font-semibold">模型</span>
                                <ModelPicker config={effectiveConfig} value={model} onChange={(value) => updateConfig("textModel", value)} fullWidth modality="text" onMissingConfig={() => openConfigDialog(false)} />
                            </label>
                        </div>

                        <div className="mt-auto pt-6">
                            <Button type="primary" size="large" block icon={running ? <LoaderCircle className="size-4 animate-spin" /> : <Sparkles className="size-4" />} disabled={running || !prompt.trim()} onClick={() => void runTextGeneration()}>
                                {running ? "生成中" : "开始生成"}
                            </Button>
                        </div>
                    </div>

                    <div className="thin-scrollbar rounded-lg border border-stone-200 bg-card p-4 shadow-sm dark:border-stone-800 lg:min-h-0 lg:overflow-y-auto lg:p-5">
                        <div className="mb-4 flex items-center justify-between gap-3">
                            <h2 className="text-xl font-semibold">生成结果</h2>
                            <div className="flex items-center gap-2">
                                {running ? <Tag className="m-0 px-2 py-1">等待 {formatDuration(elapsedMs)}</Tag> : null}
                                <Button icon={<ClipboardCopy className="size-4" />} disabled={!result.trim()} onClick={() => void copyResult()}>
                                    复制
                                </Button>
                            </div>
                        </div>
                        <div className="thin-scrollbar min-h-[320px] overflow-auto whitespace-pre-wrap rounded-lg border border-stone-200 bg-stone-50 p-4 text-sm leading-7 dark:border-stone-800 dark:bg-stone-900/70 lg:min-h-[560px]">
                            {result ? result : running ? <span className="text-stone-400">等待模型返回...</span> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无结果" />}
                        </div>
                    </div>
                </section>
            </main>
            <Drawer title="生成记录" placement="bottom" size="large" open={logsOpen} onClose={() => setLogsOpen(false)}>
                <LogPanel logs={logs} selectedLogIds={selectedLogIds} activeLogId={previewLog?.id} onSelectedLogIdsChange={setSelectedLogIds} onCreateSession={createSession} onDeleteSelected={() => setDeleteConfirmOpen(true)} onPreviewLog={previewGenerationLog} />
            </Drawer>
            <PromptSelectDialog open={promptDialogOpen} onOpenChange={setPromptDialogOpen} onSelect={(value) => setPrompt((current) => (current ? `${current}\n\n${value}` : value))} />
            <Modal title="删除生成记录" open={deleteConfirmOpen} onCancel={() => setDeleteConfirmOpen(false)} onOk={deleteSelectedLogs} okText="删除" okButtonProps={{ danger: true }} cancelText="取消">
                确定删除选中的 {selectedLogIds.length} 条生成记录吗？
            </Modal>
        </div>
    );
}

function LogPanel({
    logs,
    selectedLogIds,
    activeLogId,
    onSelectedLogIdsChange,
    onCreateSession,
    onDeleteSelected,
    onPreviewLog,
}: {
    logs: TextGenerationLog[];
    selectedLogIds: string[];
    activeLogId?: string;
    onSelectedLogIdsChange: (ids: string[]) => void;
    onCreateSession: () => void;
    onDeleteSelected: () => void;
    onPreviewLog: (log: TextGenerationLog) => void;
}) {
    const allSelected = Boolean(logs.length) && selectedLogIds.length === logs.length;
    const toggleAll = () => onSelectedLogIdsChange(allSelected ? [] : logs.map((log) => log.id));

    return (
        <>
            <div className="mb-3 flex items-center justify-between gap-3">
                <h2 className="text-base font-semibold">生成记录</h2>
                <Tag className="m-0">{logs.length}</Tag>
            </div>
            <div className="mb-4 flex flex-wrap gap-2">
                <Button size="small" icon={<Plus className="size-3.5" />} onClick={onCreateSession}>
                    新建
                </Button>
                <Button size="small" icon={<CheckSquare className="size-3.5" />} disabled={!logs.length} onClick={toggleAll}>
                    {allSelected ? "取消" : "全选"}
                </Button>
                <Button size="small" danger icon={<Trash2 className="size-3.5" />} disabled={!selectedLogIds.length} onClick={onDeleteSelected}>
                    删除
                </Button>
            </div>
            <div className="space-y-3">
                {logs.map((log) => (
                    <button key={log.id} type="button" className={`block w-full rounded-lg border p-2 text-left transition ${activeLogId === log.id ? "border-stone-900 bg-blue-50 dark:border-stone-100 dark:bg-blue-950/20" : "border-stone-200 bg-background hover:bg-stone-50 dark:border-stone-800 dark:hover:bg-stone-900"}`} onClick={() => onPreviewLog(log)}>
                        <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-2">
                            <Checkbox className="mt-0.5" checked={selectedLogIds.includes(log.id)} onClick={(event) => event.stopPropagation()} onChange={(event) => onSelectedLogIdsChange(event.target.checked ? [...selectedLogIds, log.id] : selectedLogIds.filter((id) => id !== log.id))} />
                            <div className="min-w-0">
                                <div className="truncate text-sm font-semibold leading-5">{log.title}</div>
                                <Typography.Paragraph ellipsis={{ rows: 2 }} className="!mb-0 !mt-1 !text-xs !text-stone-500 dark:!text-stone-400">
                                    {log.result || log.error || log.prompt}
                                </Typography.Paragraph>
                            </div>
                            <div className="grid justify-items-end gap-2">
                                <Tag className="m-0 flex h-6 items-center rounded-md px-1.5 text-xs leading-none" color={log.status === "成功" ? "blue" : "red"}>
                                    {log.status}
                                </Tag>
                                <Tag className="m-0 flex h-6 items-center rounded-md px-1.5 text-xs leading-none" color="green">
                                    {formatDuration(log.durationMs)}
                                </Tag>
                            </div>
                        </div>
                    </button>
                ))}
                {!logs.length ? <div className="flex min-h-48 items-center justify-center rounded-lg border border-dashed border-stone-300 text-center text-sm text-stone-500 dark:border-stone-700">暂无生成记录</div> : null}
            </div>
        </>
    );
}

async function readStoredLogs() {
    if (typeof window === "undefined") return [];
    try {
        const logs: TextGenerationLog[] = [];
        await logStore.iterate<TextGenerationLog, void>((value) => {
            logs.push(normalizeLog(value));
        });
        return logs.sort((a, b) => (b.createdAt || 0) - (a.createdAt || 0));
    } catch {
        return [];
    }
}

function normalizeLog(log: Partial<TextGenerationLog>): TextGenerationLog {
    const config = {
        model: log.config?.model || log.model || "",
        textModel: log.config?.textModel || log.model || "",
    };
    return {
        id: log.id || nanoid(),
        createdAt: log.createdAt || Date.now(),
        title: log.title || log.prompt?.slice(0, 16) || "未命名",
        prompt: log.prompt || "",
        result: log.result || "",
        error: log.error,
        time: log.time || new Date().toLocaleString("zh-CN", { hour12: false }),
        model: log.model || config.textModel,
        config,
        durationMs: log.durationMs || 0,
        status: log.status || "成功",
    };
}

function buildLog({ prompt, result, error, model, durationMs, status }: { prompt: string; result: string; error?: string; model: string; durationMs: number; status: TextGenerationLog["status"] }): TextGenerationLog {
    return {
        id: nanoid(),
        createdAt: Date.now(),
        title: prompt.slice(0, 16) || "未命名",
        prompt,
        result,
        error,
        time: new Date().toLocaleString("zh-CN", { hour12: false }),
        model,
        config: { model, textModel: model },
        durationMs,
        status,
    };
}
