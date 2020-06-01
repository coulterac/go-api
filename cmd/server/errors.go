package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type apiError struct {
	Message string `json:"message,omitempty"`
}

func sendError(w http.ResponseWriter, status int, msg string) {
	err := apiError{
		Message: msg,
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(err)
}

// ErrorValidation will return a nice JSON response when sent back to the user.
// We should use this when sending error responses back over HTTP and should
// usually be occupanied by 400 Bad Request
type errorValidation struct {
	// Field is the field that caused the validation error
	Field string `json:"field"`

	// Reason is the reason the field is invalid.
	Reason string `json:"reason"`
}

// Error implements the error interface so that we can return this error from
// functions in a nice mannor.
func (e errorValidation) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}
