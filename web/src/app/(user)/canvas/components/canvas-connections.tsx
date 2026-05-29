import { canvasThemes, type CanvasTheme } from "@/lib/canvas-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import type { CanvasConnection, CanvasNodeData, ConnectionHandle, Position } from "../types";

export type CanvasConnectionVariant = "default" | "main" | "support" | "pass" | "repair" | "loop";
export type CanvasConnectionShape = "curve" | "orthogonal";
export type CanvasConnectionRoute = {
    startOffsetY?: number;
    endOffsetY?: number;
    railY?: number;
    elbowPadding?: number;
};

export function ConnectionPath({
    connection,
    from,
    to,
    active,
    onSelect,
    variant = "default",
    shape = "curve",
    route,
}: {
    connection: CanvasConnection;
    from: CanvasNodeData;
    to: CanvasNodeData;
    active: boolean;
    onSelect: () => void;
    variant?: CanvasConnectionVariant;
    shape?: CanvasConnectionShape;
    route?: CanvasConnectionRoute;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const startX = from.position.x + from.width;
    const startY = from.position.y + from.height / 2 + (route?.startOffsetY || 0);
    const endX = to.position.x;
    const endY = to.position.y + to.height / 2 + (route?.endOffsetY || 0);
    const pathD = shape === "orthogonal" ? orthogonalPath(startX, startY, endX, endY, route) : curvedPath(startX, startY, endX, endY);
    const style = connectionStyle(theme, variant, active);

    return (
        <g>
            <path
                data-connection-id={connection.id}
                d={pathD}
                stroke="transparent"
                strokeWidth="16"
                fill="none"
                style={{ cursor: "pointer", pointerEvents: "stroke" }}
                onClick={(event) => {
                    event.stopPropagation();
                    onSelect();
                }}
            />
            <path
                d={pathD}
                stroke={style.stroke}
                strokeWidth={style.strokeWidth}
                strokeOpacity={style.strokeOpacity}
                strokeDasharray={style.strokeDasharray}
                fill="none"
                strokeLinejoin="round"
                strokeLinecap="round"
                style={{ filter: active ? `drop-shadow(0 0 8px ${theme.node.activeStroke}66)` : undefined, pointerEvents: "none" }}
            />
        </g>
    );
}

export function ActiveConnectionPath({ node, handle, mouseWorld }: { node?: CanvasNodeData; handle: ConnectionHandle; mouseWorld: Position }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    if (!node) return null;

    const startX = handle.handleType === "source" ? node.position.x + node.width : mouseWorld.x;
    const startY = handle.handleType === "source" ? node.position.y + node.height / 2 : mouseWorld.y;
    const endX = handle.handleType === "source" ? mouseWorld.x : node.position.x;
    const endY = handle.handleType === "source" ? mouseWorld.y : node.position.y + node.height / 2;
    const distance = Math.abs(endX - startX);
    const pathD = `M ${startX} ${startY} C ${startX + distance * 0.5} ${startY}, ${endX - distance * 0.5} ${endY}, ${endX} ${endY}`;

    return <path d={pathD} stroke={theme.node.activeStroke} strokeWidth="2" fill="none" strokeDasharray="5,5" />;
}

function curvedPath(startX: number, startY: number, endX: number, endY: number) {
    const dx = Math.abs(endX - startX);
    const curvature = Math.max(dx * 0.5, 50);
    return `M ${startX} ${startY} C ${startX + curvature} ${startY}, ${endX - curvature} ${endY}, ${endX} ${endY}`;
}

function orthogonalPath(startX: number, startY: number, endX: number, endY: number, route?: CanvasConnectionRoute) {
    const padding = route?.elbowPadding ?? 56;
    if (typeof route?.railY === "number") {
        const startElbowX = startX + padding;
        const endElbowX = endX - padding;
        if (endElbowX > startElbowX) {
            return `M ${startX} ${startY} H ${startElbowX} V ${route.railY} H ${endElbowX} V ${endY} H ${endX}`;
        }
        const loopX = Math.max(startX, endX) + padding * 1.6;
        return `M ${startX} ${startY} H ${loopX} V ${route.railY} H ${endElbowX} V ${endY} H ${endX}`;
    }
    if (endX >= startX) {
        const midX = startX + Math.max((endX - startX) / 2, 48);
        return `M ${startX} ${startY} H ${midX} V ${endY} H ${endX}`;
    }
    const loopX = Math.max(startX, endX) + 96;
    return `M ${startX} ${startY} H ${loopX} V ${endY} H ${endX}`;
}

function connectionStyle(theme: CanvasTheme, variant: CanvasConnectionVariant, active: boolean) {
    const base = {
        stroke: active ? theme.node.activeStroke : theme.node.muted,
        strokeWidth: active ? 3 : 2,
        strokeOpacity: active ? 1 : 0.82,
        strokeDasharray: undefined as string | undefined,
    };
    if (variant === "main") return { ...base, strokeWidth: active ? 3.2 : 2.4, strokeOpacity: active ? 1 : 0.9 };
    if (variant === "support") return { ...base, strokeWidth: active ? 2.6 : 1.7, strokeOpacity: active ? 0.95 : 0.45 };
    if (variant === "pass") return { ...base, stroke: active ? theme.node.activeStroke : "#22c55e", strokeOpacity: active ? 1 : 0.72 };
    if (variant === "repair" || variant === "loop") return { ...base, stroke: active ? theme.node.activeStroke : "#f59e0b", strokeOpacity: active ? 1 : 0.78, strokeDasharray: "8 7" };
    return base;
}
