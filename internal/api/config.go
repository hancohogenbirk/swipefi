package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// BrowseFilesystem lists directories at a given path for the music dir picker.
func (a *API) BrowseFilesystem(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	// Clean and resolve the path
	path = filepath.Clean(path)

	entries, err := os.ReadDir(path)
	if err != nil {
		slog.Error("browse filesystem", "path", path, "err", err)
		writeError(w, http.StatusBadRequest, "cannot read directory: "+err.Error())
		return
	}

	var dirs []DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden directories
		if strings.HasPrefix(name, ".") {
			continue
		}
		dirs = append(dirs, DirEntry{
			Name:  name,
			Path:  filepath.Join(path, name),
			IsDir: true,
		})
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	if dirs == nil {
		dirs = []DirEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"current": path,
		"parent":  filepath.Dir(path),
		"entries": dirs,
	})
}

func (a *API) GetAppConfig(w http.ResponseWriter, r *http.Request) {
	musicDir, _ := a.store.GetConfig("music_dir")
	deleteDir, _ := a.store.GetConfig("delete_dir")

	writeJSON(w, http.StatusOK, map[string]string{
		"music_dir":  musicDir,
		"delete_dir": deleteDir,
	})
}

func (a *API) SetMusicDir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate directory exists
	info, err := os.Stat(req.Path)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "directory does not exist")
		return
	}

	if err := a.store.SetConfig("music_dir", req.Path); err != nil {
		slog.Error("save music dir", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	deleteDir := filepath.Join(req.Path, "to_delete")
	a.store.SetConfig("delete_dir", deleteDir)

	slog.Info("music directory set", "path", req.Path, "delete_dir", deleteDir)

	// Notify the app to reconfigure (via callback)
	if a.onMusicDirChanged != nil {
		a.onMusicDirChanged(req.Path, deleteDir)
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"music_dir":  req.Path,
		"delete_dir": deleteDir,
	})
}
