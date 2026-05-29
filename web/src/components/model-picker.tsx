"use client";

import { useEffect, useId, useMemo, useState } from "react";
import { Cpu } from "lucide-react";

import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectSeparator, SelectTrigger } from "@/components/ui/select";
import { modelOptionsFor, modelDisplayName, type ModelModality } from "@/lib/model-presets";
import { cn } from "@/lib/utils";
import type { AiConfig } from "@/stores/use-config-store";

type ModelPickerProps = {
    config: AiConfig;
    value?: string;
    onChange: (model: string) => void;
    className?: string;
    fullWidth?: boolean;
    placeholder?: string;
    modality?: ModelModality;
    onMissingConfig?: () => void;
};

export function ModelPicker({ config, value, onChange, className, fullWidth = false, placeholder = "选择模型", modality = "any", onMissingConfig }: ModelPickerProps) {
    const pickerId = useId();
    const [open, setOpen] = useState(false);
    const options = useMemo(() => {
        return modelOptionsFor(config, modality, value);
    }, [config.models, modality, value]);
    const grouped = useMemo(
        () => ({
            flow: options.filter((item) => item.group === "flow"),
            common: options.filter((item) => item.group === "common"),
            advanced: options.filter((item) => item.group === "advanced"),
        }),
        [options],
    );
    const current = value || "";

    useEffect(() => {
        const closeOtherPicker = (event: Event) => {
            if ((event as CustomEvent<string>).detail !== pickerId) setOpen(false);
        };
        window.addEventListener("model-picker-open", closeOtherPicker);
        return () => window.removeEventListener("model-picker-open", closeOtherPicker);
    }, [pickerId]);

    return (
        <Select
            open={open}
            value={current}
            onOpenChange={(nextOpen) => {
                if (nextOpen && !options.length && config.channelMode === "local") {
                    onMissingConfig?.();
                    return;
                }
                if (nextOpen) window.dispatchEvent(new CustomEvent("model-picker-open", { detail: pickerId }));
                setOpen(nextOpen);
            }}
            onValueChange={onChange}
        >
            <SelectTrigger
                className={cn(
                    "canvas-composer-model-picker h-8 w-fit max-w-full gap-2 rounded-full border border-input bg-transparent px-3 text-sm font-normal shadow-sm transition-colors",
                    fullWidth ? "w-full min-w-0 justify-start" : "min-w-[9rem] justify-start",
                    "data-[state=open]:border-ring data-[state=open]:ring-2 data-[state=open]:ring-ring/20",
                    className,
                )}
                onMouseDown={(event) => event.stopPropagation()}
                onPointerDown={(event) => event.stopPropagation()}
                title={current || placeholder}
            >
                <ModelIcon model={current} />
                <span className="canvas-model-picker-text min-w-0 flex-1 truncate text-left">{current ? modelDisplayName(current) : placeholder}</span>
            </SelectTrigger>
            <SelectContent
                data-canvas-no-zoom
                className="z-[1200] w-80 max-w-[calc(100vw-24px)] rounded-xl border border-border/70 bg-popover p-1 shadow-xl"
                position="popper"
                align="start"
                side="bottom"
                sideOffset={6}
                onPointerDown={(event) => event.stopPropagation()}
                onMouseDown={(event) => event.stopPropagation()}
            >
                {options.length ? (
                    <>
                        <ModelOptionGroup title="推荐模型" options={grouped.common} />
                        {grouped.common.length && (grouped.flow.length || grouped.advanced.length) ? <SelectSeparator /> : null}
                        <ModelOptionGroup title="Google Flow" options={grouped.flow} />
                        {grouped.flow.length && grouped.advanced.length ? <SelectSeparator /> : null}
                        <ModelOptionGroup title="高级原始模型名" options={grouped.advanced} />
                    </>
                ) : (
                    <SelectItem value="__empty__" disabled>
                        {config.channelMode === "remote" ? "暂无可用模型" : "请先到配置里拉取模型列表"}
                    </SelectItem>
                )}
            </SelectContent>
        </Select>
    );
}

function ModelOptionGroup({ title, options }: { title: string; options: ReturnType<typeof modelOptionsFor> }) {
    if (!options.length) return null;
    return (
        <SelectGroup>
            <SelectLabel>{title}</SelectLabel>
            {options.map((option) => (
                <SelectItem key={option.value} value={option.value} textValue={`${option.label} ${option.value}`}>
                    <ModelLabel model={option.value} label={option.label} description={option.description} />
                </SelectItem>
            ))}
        </SelectGroup>
    );
}

function ModelLabel({ model, label = modelDisplayName(model), description }: { model: string; label?: string; description?: string }) {
    return (
        <span className="flex min-w-0 items-center gap-2">
            <ModelIcon model={model} />
            <span className="grid min-w-0">
                <span className="truncate">{label}</span>
                {description ? <span className="truncate text-[11px] text-muted-foreground">{description}</span> : null}
            </span>
        </span>
    );
}

function ModelIcon({ model }: { model: string }) {
    const icon = resolveModelIcon(model);
    return icon ? <img src={icon} alt="" className="size-4 shrink-0 dark:invert" /> : <Cpu className="size-4 shrink-0 opacity-70" />;
}

function resolveModelIcon(model: string) {
    const name = model.toLowerCase();
    if (name.includes("claude") || name.includes("anthropic")) return "/icons/claude.svg";
    if (name.includes("gemini") || name.includes("google")) return "/icons/gemini.svg";
    if (name.includes("gpt") || name.includes("openai")) return "/icons/openai.svg";
    if (name.includes("grok") || name.includes("grok")) return "/icons/grok.svg";
    if (name.includes("deepseek") || name.includes("deepseek")) return "/icons/deepseek.svg";
    if (name.includes("glm") || name.includes("glm")) return "/icons/glm.svg";
    return "";
}
