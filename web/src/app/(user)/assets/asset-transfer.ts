import { saveAs } from "file-saver";

import { createZip, readZip } from "@/lib/zip";
import { getMediaBlob } from "@/services/file-storage";
import { getImageBlob } from "@/services/image-storage";
import type { Asset } from "@/stores/use-asset-store";

type AssetExportFile = {
    app: "infinite-canvas";
    version: 1;
    exportedAt: string;
    assets: Asset[];
    files: AssetExportItem[];
};

type AssetExportItem = {
    assetId?: string;
    storageKey: string;
    path: string;
    mimeType: string;
    bytes: number;
};

type AssetImportResult = {
    imported: number;
    failed: number;
};

type AssetPayload = Omit<Asset, "id" | "createdAt" | "updatedAt">;
type AddAsset = (asset: AssetPayload) => Promise<string>;

export async function exportAssets(assets: Asset[]) {
    const files: AssetExportItem[] = [];
    const zipFiles: { name: string; data: BlobPart }[] = [];
    const exportedFileByAssetId = new Map<string, string>();

    await Promise.all(
        assets.map(async (asset) => {
            if (asset.kind !== "image" && asset.kind !== "video") return;
            const storageKey = asset.data.storageKey;
            const blob = await assetBlobForExport(asset);
            if (!blob) return;
            const fileId = storageKey || `${asset.kind}:${asset.id}:original`;
            const path = `files/${safeFileName(fileId)}.${fileExtension(blob.type || asset.data.mimeType, asset.kind)}`;
            files.push({ assetId: asset.id, storageKey: fileId, path, mimeType: blob.type || asset.data.mimeType || "application/octet-stream", bytes: blob.size });
            exportedFileByAssetId.set(asset.id, path);
            zipFiles.push({ name: path, data: blob });
        }),
    );

    const data: AssetExportFile = { app: "infinite-canvas", version: 1, exportedAt: new Date().toISOString(), assets: assets.map((asset) => assetSnapshotForExport(asset, exportedFileByAssetId.get(asset.id))), files };
    const zip = await createZip([{ name: "assets.json", data: JSON.stringify(data, null, 2) }, ...zipFiles]);
    saveAs(zip, "我的素材.zip");
}

export async function importAssetPackage(file: File, addAsset: AddAsset): Promise<AssetImportResult> {
    const zip = await readZip(file);
    const assetFile = zip.get("assets.json");
    if (!assetFile) throw new Error("missing assets.json");
    const data = JSON.parse(await assetFile.text()) as AssetExportFile;
    let imported = 0;
    let failed = 0;

    for (const asset of data.assets) {
        const packaged = assetPayloadFromPackage(asset, data.files, zip);
        if (!packaged) {
            failed += 1;
            continue;
        }
        try {
            const id = await addAsset(packaged.asset);
            if (id) imported += 1;
            else failed += 1;
        } catch {
            failed += 1;
        } finally {
            if (packaged.objectUrl) URL.revokeObjectURL(packaged.objectUrl);
        }
    }

    return { imported, failed };
}

async function assetBlobForExport(asset: Asset) {
    const storageKey = asset.kind === "image" || asset.kind === "video" ? asset.data.storageKey : undefined;
    if (storageKey) {
        const blob = asset.kind === "image" ? await getImageBlob(storageKey) : await getMediaBlob(storageKey);
        if (blob) return blob;
    }
    const url = asset.kind === "image" ? asset.data.dataUrl : asset.kind === "video" ? asset.data.url : "";
    if (!url) return null;
    try {
        const response = await fetch(url);
        if (!response.ok) return null;
        return response.blob();
    } catch {
        return null;
    }
}

function assetSnapshotForExport(asset: Asset, filePath?: string): Asset {
    if (asset.kind === "text") return asset;
    if (asset.kind === "image") {
        const url = filePath || (isHttpUrl(asset.data.dataUrl) ? asset.data.dataUrl : "");
        return { ...asset, coverUrl: filePath && !isHttpUrl(asset.coverUrl) ? filePath : asset.coverUrl, data: { ...asset.data, dataUrl: url, storageKey: undefined } };
    }
    const url = filePath || (isHttpUrl(asset.data.url) ? asset.data.url : "");
    return { ...asset, coverUrl: filePath && !isHttpUrl(asset.coverUrl) ? filePath : asset.coverUrl, data: { ...asset.data, url, storageKey: undefined } };
}

function isHttpUrl(value?: string) {
    return Boolean(value && /^https?:\/\//i.test(value));
}

function safeFileName(value: string) {
    return value.replace(/[\\/:*?"<>|]/g, "_");
}

function fileExtension(mimeType: string, kind: Asset["kind"]) {
    if (mimeType.includes("png")) return "png";
    if (mimeType.includes("jpeg")) return "jpg";
    if (mimeType.includes("webp")) return "webp";
    if (mimeType.includes("gif")) return "gif";
    if (mimeType.includes("mp4")) return "mp4";
    if (mimeType.includes("webm")) return "webm";
    return kind === "image" ? "png" : "bin";
}

function assetPayloadFromPackage(asset: Asset, files: AssetExportItem[], zip: Map<string, Blob>): { asset: AssetPayload; objectUrl?: string } | null {
    const base = {
        title: asset.title,
        coverUrl: asset.coverUrl,
        tags: asset.tags || [],
        source: asset.source || "素材包导入",
        note: asset.note,
        metadata: { ...(asset.metadata || {}), source: "local_upload", packageSource: "asset_package_import" },
    };

    if (asset.kind === "text") return { asset: { ...base, kind: "text", data: { content: asset.data.content } } };

    const fileItem = findPackagedFile(asset, files);
    const blob = fileItem ? typedZipBlob(zip.get(fileItem.path), fileItem.mimeType) : null;
    const objectUrl = blob ? URL.createObjectURL(blob) : "";

    if (asset.kind === "image") {
        const dataUrl = objectUrl || asset.data.dataUrl || "";
        if (!dataUrl) return null;
        return {
            objectUrl,
            asset: {
                ...base,
                kind: "image",
                coverUrl: objectUrl || asset.coverUrl || dataUrl,
                data: {
                    dataUrl,
                    width: asset.data.width || 0,
                    height: asset.data.height || 0,
                    bytes: blob?.size || fileItem?.bytes || asset.data.bytes || 0,
                    mimeType: blob?.type || fileItem?.mimeType || asset.data.mimeType || "image/png",
                },
            },
        };
    }

    const url = objectUrl || asset.data.url || "";
    if (!url) return null;
    return {
        objectUrl,
        asset: {
            ...base,
            kind: "video",
            coverUrl: objectUrl || asset.coverUrl || url,
            data: {
                url,
                width: asset.data.width || 0,
                height: asset.data.height || 0,
                bytes: blob?.size || fileItem?.bytes || asset.data.bytes || 0,
                mimeType: blob?.type || fileItem?.mimeType || asset.data.mimeType || "video/mp4",
            },
        },
    };
}

function findPackagedFile(asset: Asset, files: AssetExportItem[]) {
    const storageKey = asset.kind === "image" || asset.kind === "video" ? asset.data.storageKey : "";
    return files.find((item) => item.assetId === asset.id) || (storageKey ? files.find((item) => item.storageKey === storageKey) : undefined);
}

function typedZipBlob(blob: Blob | undefined, mimeType: string) {
    if (!blob) return null;
    return blob.type ? blob : blob.slice(0, blob.size, mimeType || "application/octet-stream");
}
