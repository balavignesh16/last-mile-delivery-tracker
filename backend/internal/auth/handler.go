package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	// No Role field. This is deliberate, not an oversight: public
	// registration always creates a CUSTOMER account (see RegisterHandler).
	// Decoding uses DisallowUnknownFields, so a client that sends
	// {"role":"ADMIN", ...} gets an explicit 422 rejection rather than
	// having the field silently ignored — the attempt fails closed and
	// visibly, not quietly.
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// userResponse is the only shape a User is ever allowed to leave this
// package as. It has no PasswordHash field — constructing this via
// toUserResponse, rather than marshaling users.User directly, is what
// makes "password hashes never appear in an API response" true by
// construction rather than by remembering to omit a field each time.
type userResponse struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	FullName  string  `json:"full_name"`
	Phone     *string `json:"phone"`
	Role      string  `json:"role"`
	CreatedAt string  `json:"created_at"`
}

func toUserResponse(u users.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		FullName:  u.FullName,
		Phone:     u.Phone,
		Role:      string(u.Role),
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
	}
}

// RegisterHandler handles POST /api/v1/auth/register. Every account it
// creates is a CUSTOMER — there is no code path here that can produce an
// ADMIN or DELIVERY_AGENT account from public input. Agent accounts are
// provisioned by an admin (M03's POST /agents); the two demo non-customer
// accounts that exist for evaluation come from SeedDemoUsers, not this
// endpoint.
func RegisterHandler(repo users.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		email := NormalizeEmail(req.Email)
		if problems := ValidateRegistration(email, req.Password, req.FullName); len(problems) > 0 {
			server.WriteError(w, http.StatusUnprocessableEntity, strings.Join(problems, "; "))
			return
		}

		hash, err := HashPassword(req.Password)
		if err != nil {
			slog.Error("password hashing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not process registration")
			return
		}

		var phone *string
		if trimmed := strings.TrimSpace(req.Phone); trimmed != "" {
			phone = &trimmed
		}

		created, err := repo.Create(r.Context(), users.NewUser{
			Email:        email,
			PasswordHash: hash,
			FullName:     strings.TrimSpace(req.FullName),
			Phone:        phone,
			Role:         users.RoleCustomer, // server-controlled; never from client input
		})
		if err != nil {
			if errors.Is(err, users.ErrEmailTaken) {
				server.WriteError(w, http.StatusConflict, "an account with this email already exists")
				return
			}
			slog.Error("user creation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not complete registration")
			return
		}

		server.WriteJSON(w, http.StatusCreated, toUserResponse(created))
	}
}

// LoginHandler handles POST /api/v1/auth/login. Unknown email and wrong
// password both produce the exact same response — this function has only
// one path to a 401, by construction, so there is no risk of the two
// cases accidentally being distinguishable to a caller.
func LoginHandler(repo users.Repository, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body")
			return
		}
		if req.Email == "" || req.Password == "" {
			server.WriteError(w, http.StatusUnprocessableEntity, "email and password are required")
			return
		}

		email := NormalizeEmail(req.Email)

		user, err := repo.FindByEmail(r.Context(), email)
		if err != nil {
			if errors.Is(err, users.ErrNotFound) {
				unauthorizedInvalidCredentials(w)
				return
			}
			slog.Error("login lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not process login")
			return
		}

		if err := VerifyPassword(user.PasswordHash, req.Password); err != nil {
			unauthorizedInvalidCredentials(w)
			return
		}

		token, err := GenerateToken(jwtSecret, user.ID, user.Role, TokenExpiry)
		if err != nil {
			slog.Error("token generation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not process login")
			return
		}

		server.WriteJSON(w, http.StatusOK, loginResponse{Token: token})
	}
}

func unauthorizedInvalidCredentials(w http.ResponseWriter) {
	server.WriteError(w, http.StatusUnauthorized, "invalid email or password")
}

// GetMeHandler and UpdateMeHandler handle GET/PUT /api/v1/users/me. They
// stay here rather than moving to internal/users — considered in M03 and
// decided against — specifically to avoid a real import cycle:
// internal/auth already depends on internal/users for the Repository, and
// these handlers' only job is "look up the identity RequireAuth already
// put in context" — which needs auth's Identity type. Putting them in
// internal/users would require users to import auth right back.
// internal/agents (M03's new package) instead depends on both auth and
// users with no cycle, since nothing imports agents — that is the
// pattern this project uses to add identity-aware handlers without
// relocating existing ones.
func GetMeHandler(repo users.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		user, err := repo.FindByID(r.Context(), identity.UserID)
		if err != nil {
			if errors.Is(err, users.ErrNotFound) {
				server.WriteError(w, http.StatusUnauthorized, "user no longer exists")
				return
			}
			slog.Error("failed to load current user", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load profile")
			return
		}

		server.WriteJSON(w, http.StatusOK, toUserResponse(user))
	}
}

// updateMeRequest deliberately has only the two fields the frozen
// architecture permits a user to edit. There is no id/email/role/
// password_hash field here at all — not "ignored", never decoded — which
// is what makes mass-assignment onto those protected fields structurally
// impossible rather than merely filtered out. Combined with
// DisallowUnknownFields, a client that tries to sneak in
// {"role":"ADMIN"} gets the whole request rejected (422), the same
// fail-closed behavior as RegisterHandler.
type updateMeRequest struct {
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
}

// UpdateMeHandler handles PUT /api/v1/users/me. A user can only ever
// update their own profile — identity.UserID (from the verified JWT, not
// any client input) is the only source of which row gets written.
func UpdateMeHandler(repo users.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		var req updateMeRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		fullName := strings.TrimSpace(req.FullName)
		if fullName == "" {
			server.WriteError(w, http.StatusUnprocessableEntity, "full name is required")
			return
		}

		var phone *string
		if trimmed := strings.TrimSpace(req.Phone); trimmed != "" {
			phone = &trimmed
		}

		updated, err := repo.Update(r.Context(), identity.UserID, users.ProfileUpdate{
			FullName: fullName,
			Phone:    phone,
		})
		if err != nil {
			if errors.Is(err, users.ErrNotFound) {
				server.WriteError(w, http.StatusUnauthorized, "user no longer exists")
				return
			}
			slog.Error("profile update failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update profile")
			return
		}

		server.WriteJSON(w, http.StatusOK, toUserResponse(updated))
	}
}

func NormalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func ValidateRegistration(email, password, fullName string) []string {
	var problems []string
	if email == "" {
		problems = append(problems, "email is required")
	} else if _, err := mail.ParseAddress(email); err != nil {
		problems = append(problems, "email must be a valid address")
	}
	if len(password) < minPasswordLength {
		problems = append(problems, fmt.Sprintf("password must be at least %d characters", minPasswordLength))
	}
	if strings.TrimSpace(fullName) == "" {
		problems = append(problems, "full name is required")
	}
	return problems
}
