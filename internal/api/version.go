package api

import (
	"net/http"

	"swipefi/internal/version"
)

func (a *API) GetVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version": version.Version,
	})
}
