package web

import (
	"net/http"
)

// getExec renders the exec home page.
func (h *Handler) getExec() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := NewPage("BoilerMake - Execs", r)
		if !ok {
			// TODO Error Handling, this state should never be reached
			http.Error(w, "creating page failed", http.StatusInternalServerError)
			return
		}

		h.Templates.RenderTemplate(w, "exec", p)
	}
}