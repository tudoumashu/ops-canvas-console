import { FileText, Images, Maximize2, Sparkles, Workflow } from "lucide-react";

export const navigationTools = [
    {
        slug: "workflows",
        label: "我的工作流",
        icon: Workflow,
    },
    {
        slug: "canvas",
        label: "我的画布",
        icon: Maximize2,
    },
    {
        slug: "workbench",
        label: "我的工作台",
        icon: Sparkles,
    },
    {
        slug: "prompts",
        label: "提示词中心",
        icon: FileText,
    },
    {
        slug: "assets",
        label: "素材中心",
        icon: Images,
    },
] as const;

export type NavigationToolSlug = (typeof navigationTools)[number]["slug"];
