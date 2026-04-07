package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"swipefi/internal/dlna"
)

func (a *API) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices := a.discovery.ListDevices()
	if devices == nil {
		devices = []dlna.Device{}
	}
	writeJSON(w, http.StatusOK, devices)
}

func (a *API) SelectDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UDN string `json:"udn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	renderer, ok := a.discovery.GetRenderer(req.UDN)
	if !ok {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	transport := dlna.NewTransport(renderer.Transport)
	a.player.SetTransport(transport)

	// Persist selected device for auto-reconnect
	if err := a.store.SetConfig("selected_device_udn", req.UDN); err != nil {
		slog.Warn("failed to persist selected device", "err", err)
	}

	slog.Info("selected renderer", "name", renderer.Name, "udn", renderer.UDN)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"device": renderer.Name,
	})
}

func (a *API) DisconnectDevice(w http.ResponseWriter, r *http.Request) {
	a.player.Disconnect(r.Context())
	a.store.SetConfig("selected_device_udn", "")
	slog.Info("device disconnected")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) RescanDevices(w http.ResponseWriter, r *http.Request) {
	if err := a.discovery.Scan(r.Context()); err != nil {
		slog.Error("rescan devices", "err", err)
		writeError(w, http.StatusInternalServerError, "discovery failed")
		return
	}
	devices := a.discovery.ListDevices()
	writeJSON(w, http.StatusOK, devices)
}
