package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/BoilerMake/new-backend/internal/http/middleware"
	"github.com/BoilerMake/new-backend/internal/mail"
	"github.com/BoilerMake/new-backend/internal/models"
	"github.com/rollbar/rollbar-go"

	jwt "github.com/dgrijalva/jwt-go"
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
}

// NewHandler creates a handler for API requests.
func NewHandler(us models.UserService, mailer mail.Mailer) *Handler {
	h := Handler{
		UserService: us,
		Mailer:      mailer,
	}

	r := chi.NewRouter()

	r.Use(middleware.SetContentTypeJSON) // All responses from here will be JSON
	r.Use(middleware.WithJWT)

	r.Post("/signup", h.postSignup())
	r.Post("/login", h.postLogin())
	r.Post("/activate/{code}", h.postActivate())
	r.Post("/forgot", h.postForgotPassword())
	r.Post("/reset-password", h.postResetPassword())

	r.Route("/users", func(r chi.Router) {
		r.Get("/", h.getSelf())
	})

	h.Mux = r
	return &h
}

// postSignup tries to sign up a user.
func (h *Handler) postSignup() http.HandlerFunc {
	domain := mustGetEnv("DOMAIN")

	mode := mustGetEnv("ENV_MODE")
	if mode == "development" {
		domain += ":" + mustGetEnv("PORT")
	}

	jwtIssuer := mustGetEnv("JWT_ISSUER")
	jwtSigningKey := []byte(mustGetEnv("JWT_SIGNING_KEY"))
	jwtCookie := mustGetEnv("JWT_COOKIE_NAME")

	return func(w http.ResponseWriter, r *http.Request) {
		// TODO check if login is valid (i.e. account exists), if so log them in

		var u models.User
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&u)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		id, confirmationCode, err := h.UserService.Signup(&u)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		u.ID = id

		// Build confirmation email
		to := u.Email
		subject := "Confirm your email"
		link := domain + "/api/activate/" + confirmationCode
		body := "Please click the following link to confirm your email address: " + link

		err = h.Mailer.Send(to, subject, body)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		jwt, err := u.GetJWT(jwtIssuer, jwtSigningKey)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO right now this is only valid on the domain it's sent from, if we do
		// subdomains (seems likely) then we'll need to change that.
		http.SetCookie(w, &http.Cookie{
			Name:       jwtCookie,
			Value:      jwt,
			Path:       "/",
			RawExpires: "0",
		})
	}
}

// postLogin tries to log in a user.
func (h *Handler) postLogin() http.HandlerFunc {
	jwtIssuer := mustGetEnv("JWT_ISSUER")
	jwtSigningKey := []byte(mustGetEnv("JWT_SIGNING_KEY"))
	jwtCookie := mustGetEnv("JWT_COOKIE_NAME")

	return func(w http.ResponseWriter, r *http.Request) {
		// Convert JSON user details in request to a user struct
		var u models.User
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&u)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = h.UserService.Login(&u)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		jwt, err := u.GetJWT(jwtIssuer, jwtSigningKey)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO Right now this is only valid on the domain it's sent from, if we do
		// subdomains (seems likely) then we'll need to change that.
		http.SetCookie(w, &http.Cookie{
			Name:       jwtCookie,
			Value:      jwt,
			Path:       "/",
			RawExpires: "0",
		})
	}
}

// postForgotPassword sends the password reset email
func (h *Handler) postForgotPassword() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get info from body
		var emailModel models.EmailModel
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&emailModel)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		token, err := h.UserService.GetPasswordReset(emailModel.Email)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO This will need to be formatted better once the front end is setup for the link
		to := emailModel.Email
		subject := "Password Reset"
		body := "Your reset token is: " + token

		err = h.Mailer.Send(to, subject, body)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// postResetPassword resets the password with a valid token
func (h *Handler) postResetPassword() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get info from body
		var passwordResetInfo models.PasswordResetPayload
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&passwordResetInfo)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = h.UserService.ResetPassword(passwordResetInfo.UserToken, passwordResetInfo.NewPassword)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// getSelf returns the user as JSON.  It will redact some fields (like
// password).
// TODO Right now it sets the password value to blank but keeps the now blank
// field in the JSON response.  Consider even removing that field.
// TODO Same as above, but for passwordConfirm
func (h *Handler) getSelf() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := getClaimsFromCtx(r.Context())
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		u, err := h.UserService.GetByEmail(claims["email"].(string))
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			// This can fail either because the DB is messed up or nothing is found
			// So be sure to deal with that
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(u)
		return
	}
}

// postActivate activates the account that corresponds to the activation code
// if there is such an account.
func (h *Handler) postActivate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Temporarily ignoring claims returned from getClaimsFromCtx
		_, err := getClaimsFromCtx(r.Context())
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		code := chi.URLParam(r, "code")

		u, err := h.UserService.GetByCode(code)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		u.IsActive = true
		u.ConfirmationCode = ""
		err = h.UserService.Update(u)
		if err != nil {
			rollbar.Error(err)
			rollbar.Wait()
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// getClaimsFromCtx returns the claims of a Context's JWT or an error.
func getClaimsFromCtx(ctx context.Context) (claims jwt.MapClaims, err error) {
	// Always make sure the error field is nil
	err, _ = ctx.Value(middleware.JWTErrorCtxKey).(error)
	if err != nil {
		return nil, err
	}

	// Make sure the token is not nil
	token, ok := ctx.Value(middleware.JWTCtxKey).(*jwt.Token)
	if !ok {
		return nil, fmt.Errorf("missing jwt in context")
	}

	claims = token.Claims.(jwt.MapClaims)
	if err = claims.Valid(); err != nil {
		return nil, err
	}

	return claims, err
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
