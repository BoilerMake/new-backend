package web

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/BoilerMake/new-backend/internal/http/middleware"
	"github.com/BoilerMake/new-backend/internal/mail"
	"github.com/BoilerMake/new-backend/internal/models"
	"github.com/BoilerMake/new-backend/pkg/template"
	"github.com/gorilla/sessions"

	"github.com/go-chi/chi"
)

// A Handler will route requests to their appropriate HandlerFunc.
type Handler struct {
	// A Mux handles all routing and middleware.
	*chi.Mux

	// A UserService is the interface with the database.
	UserService models.UserService

	// A Mailer is used to send emails
	Mailer mail.Mailer

	Templates *template.Template

	// Cookiestore for session
	CookieStore *sessions.CookieStore
}

// NewHandler creates a handler for web requests.
func NewHandler(us models.UserService, mailer mail.Mailer) *Handler {
	h := Handler{
		UserService: us,
		Mailer:      mailer,
	}

	r := chi.NewRouter()

	// Set up templates
	mode := mustGetEnv("ENV_MODE")
	tmplPath := mustGetEnv("TEMPLATES_PATH")
	tmplFuncs := map[string]interface{}{
		"static_path": staticFileReplace(mode),
	}

	tmpls, err := template.NewTemplate(tmplPath, tmplFuncs)
	if err != nil {
		log.Fatalf("failed to load templates: %s", err)
	}
	h.Templates = tmpls

	if mode == "development" {
		// In development mode, reload templates on every request
		r.Use(h.Templates.ReloadTemplates)
	}

	// Set up pool of buffers used for rendering templates.
	r.Use(middleware.WithJWT)
	r.Use(middleware.SessionMiddleware)

	r.Get("/", h.getRoot())

	r.Get("/signup", h.getSignup())
	r.Post("/signup", h.postSignup())

	r.Get("/activate/{code}", h.getActivate())

	r.Get("/forgot", h.getForgotPassword())
	r.Post("/forgot", h.postForgotPassword())
	r.Get("/reset", h.getResetPassword())
	r.Get("/reset/{token}", h.getResetPasswordWithToken())
	r.Post("/reset/{token}", h.postResetPassword())

	r.Get("/login", h.getLogin())
	r.Post("/login", h.postLogin())

	r.Get("/account", h.getAccount())

	if mode == "development" {
		// In prod we serve static items through a CDN, in development just serve
		// out of web/static/ at /static/
		fs := http.StripPrefix("/static", http.FileServer(http.Dir("web/static")))
		r.Get("/static/*", fs.ServeHTTP)
	}

	r.NotFound(h.get404())

	sessionSecret := mustGetEnv("SESSION_SECRET")

	// key must be 16, 24 or 32 bytes long (AES-128, AES-192 or AES-256)
	var (
		key   = []byte(sessionSecret)
		store = sessions.NewCookieStore(key)
	)

	h.CookieStore = store

	h.Mux = r
	return &h
}

// getRoot renders the index template.
func (h *Handler) getRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.Templates.RenderTemplate(w, "index", nil)
	}
}

// getSignup renders the signup template.
func (h *Handler) getSignup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.Templates.RenderTemplate(w, "signup", nil)
	}
}

// postSignup tries to signup a user from a post request.
func (h *Handler) postSignup() http.HandlerFunc {

	domain := mustGetEnv("DOMAIN")

	mode := mustGetEnv("ENV_MODE")
	if mode == "development" {
		domain = "http://" + domain + ":" + mustGetEnv("PORT")
	} else {
		domain = "https://" + domain
	}

	return func(w http.ResponseWriter, r *http.Request) {

		// TODO check if login is valid (i.e. account exists), if so log them in
		var u models.User
		u.FromFormData(r)

		id, confirmationCode, err := h.UserService.Signup(&u)

		if err != nil {
			// TODO error handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		session, ok := r.Context().Value(middleware.SessionCtxKey).(*sessions.Session)

		if !ok {
			session, err = h.CookieStore.Get(r, middleware.SessionCookieName)
			if err != nil {
				// TODO error handling
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		u.ID = id

		// Build confirmation email
		to := u.Email
		subject := "Confirm your email"

		link := domain + "/activate/" + confirmationCode
		data := map[string]interface{}{
			"Name":        u.FirstName,
			"ConfirmLink": link,
		}

		err = h.Mailer.SendTemplate(to, subject, "email confirm", data)
		if err != nil {
			// TODO error handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		u.SetSession(session)

		// Saving Session
		err = session.Save(r, w)
		if err != nil {
			// TODO Error Handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		// Redirect to homepage if signup was successful
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// getActivate activates the account that corresponds to the activation code
// if there is such an account.
func (h *Handler) getActivate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Temporarily ignoring claims returned from getClaimsFromCtx
		_, ok := r.Context().Value(middleware.SessionCtxKey).(*sessions.Session)
		if !ok {
			// TODO error handling
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		code := chi.URLParam(r, "code")

		u, err := h.UserService.GetByCode(code)
		if err != nil {
			// TODO error handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		u.IsActive = true
		u.ConfirmationCode = ""
		err = h.UserService.Update(u)
		if err != nil {
			// TODO error handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		// TODO once session tokens are updated this should show a success flash
		// Redirect to homepage if activation was successful
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// getForgotPassword renders the forgot password page.
func (h *Handler) getForgotPassword() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.Templates.RenderTemplate(w, "forgot", nil)
	}
}

// postForgotPassword sends the password reset email.
func (h *Handler) postForgotPassword() http.HandlerFunc {
	domain := mustGetEnv("DOMAIN")

	mode := mustGetEnv("ENV_MODE")
	if mode == "development" {
		domain = "http://" + domain + ":" + mustGetEnv("PORT")
	} else {
		domain = "https://" + domain
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var u models.User
		u.FromFormData(r)

		token, err := h.UserService.GetPasswordReset(u.Email)
		if err != nil {
			// TODO error handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		to := u.Email
		subject := "Password Reset"

		link := domain + "/reset/" + token
		data := map[string]interface{}{
			"Name":      u.FirstName,
			"ResetLink": link,
		}

		err = h.Mailer.SendTemplate(to, subject, "email reset", data)
		if err != nil {
			// TODO error handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO once session tokens are updated this should show a success flash
		// Redirect to homepage if activation was successful
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// getResetPassword renders the reset password template.
func (h *Handler) getResetPassword() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.Templates.RenderTemplate(w, "reset", nil)
	}
}

// getResetPasswordWithToken renders the reset password template with the token filled in.
func (h *Handler) getResetPasswordWithToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		h.Templates.RenderTemplate(w, "reset", token)
	}
}

// postResetPassword resets the password with a valid token
func (h *Handler) postResetPassword() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var passwordResetInfo models.PasswordResetPayload
		passwordResetInfo.UserToken = r.FormValue("token")
		passwordResetInfo.NewPassword = r.FormValue("new-password")

		err := h.UserService.ResetPassword(passwordResetInfo.UserToken, passwordResetInfo.NewPassword)
		if err != nil {
			// TODO error handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO once session tokens are updated this should show a success flash
		// Redirect to homepage if activation was successful
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// getLogin renders the login template.
func (h *Handler) getLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.Templates.RenderTemplate(w, "login", nil)
	}
}

// postLogin tries to log in a user.
func (h *Handler) postLogin() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		var u models.User
		u.FromFormData(r)

		err := h.UserService.Login(&u)
		if err != nil {
			// TODO error handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		session, ok := r.Context().Value(middleware.SessionCtxKey).(*sessions.Session)
		if !ok {
			session, err = h.CookieStore.Get(r, middleware.SessionCookieName)
			if err != nil {
				// TODO error handling
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// Setting session values
		u.SetSession(session)

		// Save session
		err = session.Save(r, w)
		if err != nil {
			// TODO Error Handling
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Redirect to homepage if login was successful
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// getAccount shows a user their account.
func (h *Handler) getAccount() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := r.Context().Value(middleware.SessionCtxKey).(*sessions.Session)
		if !ok {
			// TODO error handling
			// TODO once session tokens are updated this should show a success flash

			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		if _, ok := session.Values["EMAIL"]; !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		u, err := h.UserService.GetByEmail(session.Values["EMAIL"].(string))

		if err != nil {
			// TODO error handling
			// This can fail either because the DB is messed up or nothing is found
			// So be sure to deal with that

			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		println("FOUND 2")
		data := map[string]interface{}{
			"Email":       u.Email,
			"FirstName":   u.FirstName,
			"LastName":    u.LastName,
			"Phone":       u.Phone,
			"ProjectIdea": u.ProjectIdea,
			"TeamMembers": u.TeamMembers,
		}

		h.Templates.RenderTemplate(w, "account", data)
	}
}

// get404 handles requests that couldn't find a valid route by rendering the
// 404 template.
func (h *Handler) get404() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.Templates.RenderTemplate(w, "404", nil)
	}
}

// mustGetEnv looks up and sets an environment variable.  If the environment
// variable is not found, it panics.
func mustGetEnv(var_name string) (value string) {
	value, ok := os.LookupEnv(var_name)
	if !ok {
		log.Fatalf("environment variable not set: %v", var_name)
	}
	return value
}

// staticFileReplace is a template helper that reads in a manifest file and
// reroutes requests accordingly.  It's useful when you upload versioned files
// to a CDN for cache invalidation.  The manifest file used is made by gulp
// when running the prod config.
func staticFileReplace(mode string) func(path string) string {
	if mode == "development" {
		// No need to change path in development
		return func(path string) string {
			return "/static/" + path
		}
	}

	// In prod change path to cloudfront
	cloudfrontURL := mustGetEnv("CLOUDFRONT_URL")

	file, err := ioutil.ReadFile("web/static/rev-manifest.json")
	if err != nil {
		log.Fatalf("failed to read static manifest file: %v", err)
	}

	var manifest map[string]interface{}
	err = json.Unmarshal(file, &manifest)
	if err != nil {
		log.Fatalf("failed to parse static manifest file: %v", err)
	}

	return func(path string) string {
		if manifest[path] != nil {
			return cloudfrontURL + "/" + manifest[path].(string)
		} else {
			return "/404"
		}
	}
}
