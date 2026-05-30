package localworkspace

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultAssetUploadFileKey = "original"

func (api *serveAPI) createAssetImport(w http.ResponseWriter, r *http.Request) {
	data, file, header, fileKey, contentType, err := decodeAssetImportRequest(r)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	defer file.Close()
	document, err := NewAsset(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	relPath, err := assetUploadRelPath(fileKey, contentType)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if document.Data.Files == nil {
		document.Data.Files = map[string]string{}
	}
	document.Data.Files[fileKey] = relPath
	if document.Data.MIME == "" {
		document.Data.MIME = contentType
	}
	if document.Data.Metadata == nil {
		document.Data.Metadata = map[string]any{}
	}
	if header != nil {
		document.Data.Metadata["fileName"] = filepath.Base(header.Filename)
		document.Data.Metadata["bytes"] = header.Size
	}
	filePath := filepath.Join(AssetRepository(api.workspace).Dir(document.ID), filepath.FromSlash(relPath))
	if err := AtomicWriteFromReader(filePath, file, 0o600); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteAsset(api.workspace, document); err != nil {
		_ = os.RemoveAll(AssetRepository(api.workspace).Dir(document.ID))
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) updateAssetImport(w http.ResponseWriter, r *http.Request, id string) {
	data, file, header, fileKey, contentType, err := decodeAssetImportRequest(r)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	defer file.Close()
	revision, err := parseAssetImportRevision(r)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadAsset(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if data.Files == nil {
		data.Files = map[string]string{}
		for key, value := range existing.Data.Files {
			data.Files[key] = value
		}
	}
	relPath, err := assetUploadRelPath(fileKey, contentType)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	data.Files[fileKey] = relPath
	if data.MIME == "" {
		data.MIME = contentType
	}
	if data.Metadata == nil {
		data.Metadata = map[string]any{}
	}
	if header != nil {
		data.Metadata["fileName"] = filepath.Base(header.Filename)
		data.Metadata["bytes"] = header.Size
	}
	document := nextEnvelopeRevision(existing, data)
	filePath := filepath.Join(AssetRepository(api.workspace).Dir(document.ID), filepath.FromSlash(relPath))
	if err := AtomicWriteFromReader(filePath, file, 0o600); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteAsset(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func decodeAssetImportRequest(r *http.Request) (AssetData, multipart.File, *multipart.FileHeader, string, string, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return AssetData{}, nil, nil, "", "", WrapError(ErrorInvalidArgument, "multipart request is invalid", 1, err)
	}
	raw := strings.TrimSpace(r.FormValue("data"))
	if raw == "" {
		return AssetData{}, nil, nil, "", "", NewError(ErrorInvalidArgument, "request data is required", 1, nil)
	}
	dataRaw := json.RawMessage(raw)
	if err := validateRawNoPlaintextSecrets(dataRaw, "asset"); err != nil {
		return AssetData{}, nil, nil, "", "", err
	}
	var data AssetData
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return AssetData{}, nil, nil, "", "", WrapError(ErrorInvalidArgument, "request data is invalid", 1, err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return AssetData{}, nil, nil, "", "", NewError(ErrorInvalidArgument, "asset file is required", 1, nil)
	}
	contentType, err := sniffMultipartFile(file, header)
	if err != nil {
		_ = file.Close()
		return AssetData{}, nil, nil, "", "", err
	}
	fileKey := strings.TrimSpace(r.FormValue("fileKey"))
	if fileKey == "" {
		fileKey = defaultAssetUploadFileKey
	}
	if err := validatePathComponent("asset file key", fileKey); err != nil {
		_ = file.Close()
		return AssetData{}, nil, nil, "", "", err
	}
	return data, file, header, fileKey, contentType, nil
}

func parseAssetImportRevision(r *http.Request) (int, error) {
	value := strings.TrimSpace(r.FormValue("revision"))
	if value == "" {
		return 0, NewError(ErrorInvalidArgument, "revision is required for update", 1, nil)
	}
	revision, err := strconv.Atoi(value)
	if err != nil {
		return 0, NewError(ErrorInvalidArgument, "revision is invalid", 1, nil)
	}
	return revision, nil
}

func sniffMultipartFile(file multipart.File, header *multipart.FileHeader) (string, error) {
	contentType := ""
	if header != nil {
		contentType = strings.TrimSpace(header.Header.Get("Content-Type"))
	}
	if contentType != "" && contentType != "application/octet-stream" {
		return contentType, nil
	}
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", WrapError(ErrorInvalidArgument, "read asset file header", 1, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", WrapError(ErrorInvalidArgument, "rewind asset file", 1, err)
	}
	if n > 0 {
		return http.DetectContentType(buffer[:n]), nil
	}
	return "application/octet-stream", nil
}

func assetUploadRelPath(fileKey string, contentType string) (string, error) {
	if err := validatePathComponent("asset file key", fileKey); err != nil {
		return "", err
	}
	ext := extensionForContentType(contentType)
	return path.Join("files", fileKey+ext), nil
}

func extensionForContentType(contentType string) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	}
	extensions, err := mime.ExtensionsByType(contentType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	return ".bin"
}
