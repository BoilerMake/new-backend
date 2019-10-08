package web

import (
	"net/http"
	"strings"

	"github.com/BoilerMake/new-backend/internal/postgres"
)

type ExecInfo struct {
	Announcement   string `json:"announcement"`
	AcceptedEmails string `json:"acceptedEmails"`
}

// FromFormData converts an application from a request's FormData to a
// models.Application struct.
func (a *ExecInfo) FromExecData(r *http.Request) error {
	a.Announcement = r.FormValue("announcement")
	a.AcceptedEmails = r.FormValue("acceptedEmails")

	return nil
}

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

func changeAcceptStatus(email string) error {
	var s postgres.UserService
	s.GetByEmail(email)

	return nil
}

// Post Exec for announcements
func (h *Handler) postExecAnnouncement() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}

// Post Exec for accepting emails
func (h *Handler) postExecAccepted() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var execInfo ExecInfo
		execInfo.FromExecData(r)
		emails := strings.Split(execInfo.AcceptedEmails, ", ")
		for _, email := range emails {
			changeAcceptStatus(email)
		}
	}
}
