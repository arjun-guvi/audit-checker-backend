// Package actions is the feature's use cases: each function loads what it needs through store,
// decides with core, writes back, and records timeline events, notifications and mails. The HTTP
// handlers and the worker jobs both call these functions.
package actions

import (
	"errors"
	"math/rand"
	"net/http"
	"time"

	"auditApp/salesAudit/store"
)

// Now is the clock for every timestamp; tests replace it.
var Now = time.Now

// Random breaks assignment ties; tests replace it with a seeded source.
var Random = rand.New(rand.NewSource(time.Now().UnixNano()))

// Error is a failure the caller should see, with its HTTP status.
type Error struct {
	Status  int
	Message string
}

func (e Error) Error() string { return e.Message }

func badRequest(err error) error { return Error{Status: http.StatusBadRequest, Message: err.Error()} }

func forbidden(message string) error { return Error{Status: http.StatusForbidden, Message: message} }

func notFound(what string) error {
	return Error{Status: http.StatusNotFound, Message: what + " not found"}
}

// orNotFound turns store.ErrNotFound into a 404 for what.
func orNotFound(err error, what string) error {
	if store.IsNotFound(err) {
		return notFound(what)
	}
	return err
}

// StatusOf is the HTTP status for an error from this package: its own, else 500.
func StatusOf(err error) (int, string) {
	var failure Error
	if errors.As(err, &failure) {
		return failure.Status, failure.Message
	}
	return http.StatusInternalServerError, "Something went wrong"
}

func nowSeconds() int64 { return Now().Unix() }
