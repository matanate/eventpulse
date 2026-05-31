package api

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error   string       `json:"error"`
	Code    string       `json:"code"`
	Details []FieldError `json:"details,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{Error: message, Code: code})
}

func WriteValidationError(w http.ResponseWriter, errs []FieldError) {
	WriteJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
		Error:   "validation failed",
		Code:    "VALIDATION_FAILED",
		Details: errs,
	})
}
