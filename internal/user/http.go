package user

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxRegistrationBodySize = 1 << 20 // 1 MiB

type registerRequest struct {
	Nickname   string  `json:"nickname"`
	Email      *string `json:"email"`
	FirstName  string  `json:"first_name"`
	LastName   string  `json:"last_name"`
	Patronymic *string `json:"patronymic"`
	Password   string  `json:"password"`
}

// NewHTTPHandler creates HTTP handlers for user operations.
func NewHTTPHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) {
		handleRegister(w, r, service)
	})
	return mux
}

func handleRegister(w http.ResponseWriter, r *http.Request, service *Service) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRegistrationBodySize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var input registerRequest
	if err := decoder.Decode(&input); err != nil {
		writeUserJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON request",
		})
		return
	}

	// Reject trailing JSON values and other content after the first object.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeUserJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request must contain a single JSON object",
		})
		return
	}

	created, err := service.Register(r.Context(), RegisterInput{
		Nickname:   input.Nickname,
		Email:      input.Email,
		FirstName:  input.FirstName,
		LastName:   input.LastName,
		Patronymic: input.Patronymic,
		Password:   input.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidUser):
			writeUserJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid user data",
			})
		case errors.Is(err, ErrConflict):
			writeUserJSON(w, http.StatusConflict, map[string]string{
				"error": "nickname or email already exists",
			})
		default:
			writeUserJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}
		return
	}

	writeUserJSON(w, http.StatusCreated, created.ToPublic())
}

func writeUserJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
