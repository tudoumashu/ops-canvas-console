package handler

import (
	"encoding/json"
	"net/http"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
)

func Assets(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListAssets(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminAssets(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListAssets(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminSaveAsset(w http.ResponseWriter, r *http.Request) {
	var item model.Asset
	_ = json.NewDecoder(r.Body).Decode(&item)
	result, err := service.SaveAsset(item)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminDeleteAsset(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeleteAsset(id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func PDDMaterialFile(w http.ResponseWriter, r *http.Request) {
	path, err := service.ResolvePDDMaterialFile(r.URL.Query().Get("path"))
	if err != nil {
		FailError(w, err)
		return
	}
	http.ServeFile(w, r, path)
}

func LocalAssetFile(w http.ResponseWriter, r *http.Request) {
	path, err := service.ResolveConsoleAssetFile(r.URL.Query().Get("path"))
	if err != nil {
		FailError(w, err)
		return
	}
	http.ServeFile(w, r, path)
}
