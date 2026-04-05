package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func (a *API) PlayerPlay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Folder string `json:"folder"`
		Sort   string `json:"sort"`
		Order  string `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := a.player.PlayFolder(r.Context(), req.Folder, req.Sort, req.Order); err != nil {
		slog.Error("play folder", "folder", req.Folder, "err", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, a.player.GetState())
}

func (a *API) PlayerPause(w http.ResponseWriter, r *http.Request) {
	if err := a.player.Pause(r.Context()); err != nil {
		slog.Error("pause", "err", err)
		writeError(w, http.StatusInternalServerError, "pause failed")
		return
	}
	writeJSON(w, http.StatusOK, a.player.GetState())
}

func (a *API) PlayerResume(w http.ResponseWriter, r *http.Request) {
	if err := a.player.Resume(r.Context()); err != nil {
		slog.Error("resume", "err", err)
		writeError(w, http.StatusInternalServerError, "resume failed")
		return
	}
	writeJSON(w, http.StatusOK, a.player.GetState())
}

func (a *API) PlayerNext(w http.ResponseWriter, r *http.Request) {
	if err := a.player.Next(r.Context()); err != nil {
		slog.Error("next", "err", err)
		writeError(w, http.StatusInternalServerError, "next failed")
		return
	}
	writeJSON(w, http.StatusOK, a.player.GetState())
}

func (a *API) PlayerPrev(w http.ResponseWriter, r *http.Request) {
	if err := a.player.Prev(r.Context()); err != nil {
		slog.Error("prev", "err", err)
		writeError(w, http.StatusInternalServerError, "prev failed")
		return
	}
	writeJSON(w, http.StatusOK, a.player.GetState())
}

func (a *API) PlayerSeek(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PositionMs int64 `json:"position_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := a.player.Seek(r.Context(), req.PositionMs); err != nil {
		slog.Error("seek", "err", err)
		writeError(w, http.StatusInternalServerError, "seek failed")
		return
	}
	writeJSON(w, http.StatusOK, a.player.GetState())
}

func (a *API) PlayerReject(w http.ResponseWriter, r *http.Request) {
	if err := a.player.Reject(r.Context()); err != nil {
		slog.Error("reject", "err", err)
		writeError(w, http.StatusInternalServerError, "reject failed")
		return
	}
	writeJSON(w, http.StatusOK, a.player.GetState())
}

func (a *API) PlayerState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.player.GetState())
}
