package http

import (
	"github.com/BoilerMake/new-backend/internal/http/middleware"
	"github.com/BoilerMake/new-backend/internal/http/web"
	"github.com/BoilerMake/new-backend/internal/mail"
	"github.com/BoilerMake/new-backend/internal/models"
	"github.com/BoilerMake/new-backend/internal/s3"

	"github.com/go-chi/chi"
)

// A Handler ties together the handlers found in api and web.
type Handler struct {
	*chi.Mux

	WebHandler *web.Handler
}

// NewHandler creates a handler that wraps the subhandlers for the entire app.
func NewHandler(us models.UserService, as models.ApplicationService, mailer mail.Mailer, S3 s3.S3) *Handler {
	h := Handler{
		WebHandler: web.NewHandler(us, as, mailer, S3),
	}
	r := chi.NewRouter()

	// Limit body request size
	r.Use(middleware.LimitRequestSize)

	r.Mount("/", h.WebHandler)

	h.Mux = r
	return &h
}
