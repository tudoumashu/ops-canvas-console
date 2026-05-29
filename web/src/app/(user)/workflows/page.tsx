"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { App, Button, Card, Input, Modal, Space, Typography } from "antd";
import { FileText, FolderPlus, ImagePlus, Play, RefreshCw, Video, Workflow } from "lucide-react";

type WorkflowFolder = {
    id: string;
    title: string;
    description: string;
    href?: string;
    kind: "pdd" | "article" | "video" | "custom";
};

const builtInFolders: WorkflowFolder[] = [
    { id: "pdd", title: "电商工作流", description: "商品图生成、mockup、主图和待上架目录管理。", href: "/workflows/ecommerce", kind: "pdd" },
];

const storageKey = "ops-canvas-workflow-folders";

export default function WorkflowsPage() {
    const { message } = App.useApp();
    const [folders, setFolders] = useState<WorkflowFolder[]>(builtInFolders);
    const [open, setOpen] = useState(false);
    const [title, setTitle] = useState("");
    const [description, setDescription] = useState("");

    useEffect(() => {
        try {
            const saved = JSON.parse(localStorage.getItem(storageKey) || "[]") as WorkflowFolder[];
            setFolders([...builtInFolders, ...saved.filter((item) => item.id !== "pdd")]);
        } catch {
            setFolders(builtInFolders);
        }
    }, []);

    const customFolders = useMemo(() => folders.filter((item) => item.kind === "custom"), [folders]);

    const saveFolder = () => {
        const nextTitle = title.trim();
        if (!nextTitle) {
            message.error("请输入文件夹名称");
            return;
        }
        const next: WorkflowFolder = {
            id: `custom-${Date.now()}`,
            title: nextTitle,
            description: description.trim() || "自定义工作流文件夹，后续可接入文章、视频或其他自动化流程。",
            kind: "custom",
        };
        const saved = [...customFolders, next];
        localStorage.setItem(storageKey, JSON.stringify(saved));
        setFolders([...builtInFolders, ...saved]);
        setTitle("");
        setDescription("");
        setOpen(false);
        message.success("工作流文件夹已创建");
    };

    return (
        <main className="h-full overflow-auto bg-background text-foreground">
            <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-6 py-8">
                <header className="flex flex-wrap items-end justify-between gap-4 border-b border-stone-200 pb-5 dark:border-stone-800">
                    <div>
                        <Typography.Text type="secondary" className="text-xs">
                            Workflow folders
                        </Typography.Text>
                        <Typography.Title level={2} className="!mb-0 !mt-2">
                            我的工作流
                        </Typography.Title>
                    </div>
                    <Space wrap>
                        <Button icon={<RefreshCw className="size-4" />} onClick={() => location.reload()}>
                            刷新
                        </Button>
                        <Button type="primary" icon={<FolderPlus className="size-4" />} onClick={() => setOpen(true)}>
                            新建工作流文件夹
                        </Button>
                    </Space>
                </header>

                <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                    {folders.map((folder) => (
                        <WorkflowFolderCard key={folder.id} folder={folder} />
                    ))}
                    <Card className="border-dashed">
                        <button type="button" className="flex min-h-40 w-full flex-col items-center justify-center gap-3 text-stone-500 transition hover:text-stone-900 dark:hover:text-stone-100" onClick={() => setOpen(true)}>
                            <FolderPlus className="size-8" />
                            <span className="font-medium">新建工作流文件夹</span>
                        </button>
                    </Card>
                </div>
            </div>

            <Modal title="新建工作流文件夹" open={open} onCancel={() => setOpen(false)} onOk={saveFolder} okText="创建" cancelText="取消">
                <Space direction="vertical" className="w-full">
                    <Input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="例如：文章工作流、视频工作流" />
                    <Input.TextArea value={description} onChange={(event) => setDescription(event.target.value)} rows={4} placeholder="文件夹说明，可留空" />
                    <Typography.Text type="secondary" className="text-xs">
                        当前版本先创建管理入口；具体模板执行器会在对应工作流接入时启用。
                    </Typography.Text>
                </Space>
            </Modal>
        </main>
    );
}

function WorkflowFolderCard({ folder }: { folder: WorkflowFolder }) {
    const Icon = folderIcon(folder.kind);
    const content = (
        <Card className="h-full transition hover:-translate-y-0.5 hover:shadow-lg">
            <div className="flex min-h-40 flex-col justify-between gap-5">
                <div>
                    <div className="mb-4 flex items-center gap-3">
                        <span className="inline-flex size-10 items-center justify-center rounded-xl bg-blue-50 text-blue-600 dark:bg-blue-950/40 dark:text-blue-300">
                            <Icon className="size-5" />
                        </span>
                        <Typography.Title level={4} className="!m-0">
                            {folder.title}
                        </Typography.Title>
                    </div>
                    <Typography.Paragraph type="secondary" className="!mb-0">
                        {folder.description}
                    </Typography.Paragraph>
                </div>
                <Button type={folder.href ? "primary" : "default"} icon={<Play className="size-4" />} disabled={!folder.href}>
                    {folder.href ? "打开工作流" : "待接入"}
                </Button>
            </div>
        </Card>
    );
    if (!folder.href) return content;
    return (
        <Link href={folder.href} className="block h-full">
            {content}
        </Link>
    );
}

function folderIcon(kind: WorkflowFolder["kind"]) {
    if (kind === "article") return FileText;
    if (kind === "video") return Video;
    if (kind === "pdd") return ImagePlus;
    return Workflow;
}
