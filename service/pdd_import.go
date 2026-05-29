package service

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
)

const (
	referenceMaterialAssetCategory = "角色参考图"
	skuArtworkBaseAssetID          = "pdd-mockup-sku-artwork-base"
	skuArtworkBaseAssetRel         = "pdd-mockup/sku_artwork_base.png"
)

func SyncPDDLocalLibraries() {
	if count, err := SyncPDDMockupAssetsToAssets(); err != nil {
		log.Printf("sync pdd mockup assets failed err=%v", err)
	} else if count > 0 {
		log.Printf("sync pdd mockup assets done count=%d", count)
	}
	if count, err := SyncPDDMaterialsToAssets(); err != nil {
		log.Printf("sync pdd materials failed err=%v", err)
	} else if count > 0 {
		log.Printf("sync pdd materials done count=%d", count)
	}
}

func SyncPDDMockupAssetsToAssets() (int, error) {
	root := consoleAssetsRoot()
	path := filepath.Join(root, filepath.FromSlash(skuArtworkBaseAssetRel))
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0, nil
	}
	if _, err := SaveAsset(model.Asset{
		ID:           skuArtworkBaseAssetID,
		Title:        "SKU 抱枕底版 - sku_artwork_base",
		Type:         model.AssetTypeImage,
		CoverURL:     consoleAssetURL(skuArtworkBaseAssetRel),
		MediaType:    string(model.AssetTypeImage),
		Scope:        assetScopeLibrary,
		Tags:         []string{"抱枕底版"},
		Category:     assetCategorySpecTemplate,
		CategoryPath: assetCategorySpecTemplate,
		Purpose:      assetPurposeSpecTemplate,
		Source:       assetSourceCloud,
		Description:  "Image 2 mockup 底版：" + skuArtworkBaseAssetRel,
		URL:          consoleAssetURL(skuArtworkBaseAssetRel),
	}); err != nil {
		return 0, err
	}
	return 1, nil
}

func SyncPDDMaterialsToAssets() (int, error) {
	root := existingPDDLocalRoot(config.Cfg.PDDMaterialsRoot, "materials")
	if root == "" {
		return 0, nil
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		log.Printf("skip pdd materials sync root=%s err=%v", root, err)
		return 0, nil
	}
	files := []string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isImagePath(path) {
			return err
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return 0, err
	}
	sort.Strings(files)
	for _, path := range files {
		rel, ok := relativeToBase(root, path)
		if !ok {
			continue
		}
		item := pddMaterialAsset(root, rel)
		if _, err := SaveAsset(item); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

func ResolvePDDMaterialFile(relativePath string) (string, error) {
	root := existingPDDLocalRoot(config.Cfg.PDDMaterialsRoot, "materials")
	relativePath = strings.TrimSpace(relativePath)
	if root == "" || relativePath == "" {
		return "", safeMessageError{message: "缺少素材文件路径"}
	}
	if filepath.IsAbs(relativePath) {
		if rel, ok := relativeToBase(root, relativePath); ok {
			relativePath = rel
		} else {
			return "", safeMessageError{message: "素材文件路径不在允许目录内"}
		}
	}
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(relativePath)))
	if _, ok := relativeToBase(root, target); !ok {
		return "", safeMessageError{message: "素材文件路径不在允许目录内"}
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() || !isImagePath(target) {
		return "", safeMessageError{message: "素材文件不是图片"}
	}
	return target, nil
}

func ResolveConsoleAssetFile(relativePath string) (string, error) {
	root := consoleAssetsRoot()
	relativePath = strings.TrimSpace(relativePath)
	if root == "" || relativePath == "" {
		return "", safeMessageError{message: "缺少控制台素材文件路径"}
	}
	if filepath.IsAbs(relativePath) {
		if rel, ok := relativeToBase(root, relativePath); ok {
			relativePath = rel
		} else {
			return "", safeMessageError{message: "控制台素材文件路径不在允许目录内"}
		}
	}
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(relativePath)))
	if _, ok := relativeToBase(root, target); !ok {
		return "", safeMessageError{message: "控制台素材文件路径不在允许目录内"}
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() || !isImagePath(target) {
		return "", safeMessageError{message: "控制台素材文件不是图片"}
	}
	return target, nil
}

func consoleAssetsRoot() string {
	root := filepath.Clean(config.Cfg.ConsoleAssetsRoot)
	if root == "." || root == "" {
		return filepath.Clean("data/assets")
	}
	return root
}

func existingPDDLocalRoot(configured string, child string) string {
	root := filepath.Clean(configured)
	if isDir(root) {
		return root
	}
	if cwd, err := os.Getwd(); err == nil {
		for _, base := range []string{
			filepath.Join(cwd, "..", "pdd", child),
			filepath.Join(cwd, "..", "..", "pdd", child),
		} {
			if isDir(base) {
				return filepath.Clean(base)
			}
		}
	}
	return root
}

func pddMaterialAsset(root, rel string) model.Asset {
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	ip := ""
	role := ""
	if len(parts) >= 4 && parts[0] == "reference_library" && parts[1] == "anime_ip" {
		ip = parts[2]
		role = parts[3]
	}
	kind := "素材图片"
	index := ""
	if strings.Contains(rel, "/official_references/") {
		kind = "官方参考图"
		index = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		index = strings.TrimPrefix(index, "official_")
	} else if strings.HasPrefix(filepath.Base(rel), "reference.") {
		kind = "标准参考图"
	}
	titleParts := []string{}
	if ip != "" {
		titleParts = append(titleParts, ip)
	}
	if role != "" {
		titleParts = append(titleParts, role)
	}
	titleParts = append(titleParts, kind)
	if index != "" {
		titleParts = append(titleParts, index)
	}
	if len(titleParts) == 1 {
		titleParts[0] = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	}
	purpose := assetPurposeGeneric
	categoryPath := assetCategoryGenericImage
	if kind == "官方参考图" {
		purpose = assetPurposeOfficialReference
		categoryPath = assetCategoryOfficialReference
	} else if kind == "标准参考图" {
		purpose = assetPurposeStandardReference
		categoryPath = assetCategoryStandardReference
	}
	metadata := map[string]any{"originPath": rel}
	if ip != "" {
		metadata["ip"] = ip
	}
	if role != "" {
		metadata["character"] = role
	}
	return model.Asset{
		ID:           "pdd-material-" + shortHash(rel),
		Title:        strings.Join(titleParts, " - "),
		Type:         model.AssetTypeImage,
		MediaType:    string(model.AssetTypeImage),
		Scope:        assetScopeLibrary,
		CoverURL:     pddMaterialURL(rel),
		Tags:         []string{},
		Category:     referenceMaterialAssetCategory,
		CategoryPath: categoryPath,
		Purpose:      purpose,
		Source:       assetSourceCloud,
		Description:  "素材库：" + rel,
		URL:          pddMaterialURL(rel),
		Metadata:     metadata,
	}
}

func pddMaterialURL(rel string) string {
	return "/api/assets/pdd-materials/file?path=" + url.QueryEscape(filepath.ToSlash(rel))
}

func consoleAssetURL(rel string) string {
	return "/api/assets/local/file?path=" + url.QueryEscape(filepath.ToSlash(rel))
}

func shortHash(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}
