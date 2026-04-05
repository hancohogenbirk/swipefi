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

// BrowseShortcuts returns mount points and top-level directories that likely contain music.
// Detected dynamically from /proc/mounts and well-known paths.
func (a *API) BrowseShortcuts(w http.ResponseWriter, r *http.Request) {
	type shortcut struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	var results []shortcut
	seen := make(map[string]bool)

	// Parse /proc/mounts for real filesystem mounts (excludes virtual filesystems)
	mountData, err := os.ReadFile("/proc/mounts")
	if err == nil {
		virtualFS := map[string]bool{
			"proc": true, "sysfs": true, "devpts": true, "tmpfs": true,
			"cgroup": true, "cgroup2": true, "overlay": true, "devtmpfs": true,
			"securityfs": true, "debugfs": true, "mqueue": true, "hugetlbfs": true,
			"fusectl": true, "binfmt_misc": true, "tracefs": true, "pstore": true,
			"configfs": true, "efivarfs": true, "autofs": true, "nsfs": true,
		}
		skipPaths := map[string]bool{
			"/": true, "/boot": true, "/dev": true, "/proc": true,
			"/sys": true, "/run": true, "/tmp": true, "/etc": true,
		}

		for _, line := range strings.Split(string(mountData), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			mountPoint := fields[1]
			fsType := fields[2]

			if virtualFS[fsType] || skipPaths[mountPoint] {
				continue
			}
			if strings.HasPrefix(mountPoint, "/dev") || strings.HasPrefix(mountPoint, "/sys") || strings.HasPrefix(mountPoint, "/proc") {
				continue
			}

			// Check it actually exists and is readable
			if info, err := os.Stat(mountPoint); err == nil && info.IsDir() {
				if !seen[mountPoint] {
					seen[mountPoint] = true
					name := filepath.Base(mountPoint)
					results = append(results, shortcut{Name: name + " (mount)", Path: mountPoint})
				}
			}
		}
	}

	// Also scan /run/user/*/gvfs for GVFS mounts (local Linux dev)
	gvfsDirs, _ := filepath.Glob("/run/user/*/gvfs/*")
	for _, p := range gvfsDirs {
		if info, err := os.Stat(p); err == nil && info.IsDir() && !seen[p] {
			seen[p] = true
			results = append(results, shortcut{Name: filepath.Base(p), Path: p})
		}
	}

	if results == nil {
		results = []shortcut{}
	}

	writeJSON(w, http.StatusOK, results)
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
	// Return the active music dir (scanner knows the truth)
	musicDir := a.scanner.MusicDir()
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
