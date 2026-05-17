package errors

import (
	stderrors "errors"
	"fmt"
	"reflect"
	"testing"
)

func TestNew(t *testing.T) {
	err := New(418, "TEAPOT")

	if err.Code != 418 {
		t.Fatalf("Code = %d, want 418", err.Code)
	}
	if err.Status != "TEAPOT" {
		t.Fatalf("Status = %q, want TEAPOT", err.Status)
	}
	if err.Message != "" {
		t.Fatalf("Message = %q, want empty", err.Message)
	}
	if err.Details != nil {
		t.Fatalf("Details = %#v, want nil", err.Details)
	}
}

func TestNewfAndErrorf(t *testing.T) {
	err := Newf(400, StatusBadRequest, "invalid %s", "name")
	if err.Message != "invalid name" {
		t.Fatalf("Newf Message = %q, want invalid name", err.Message)
	}

	var typed *Error
	if !stderrors.As(Errorf(404, StatusNotFound, "missing %d", 42), &typed) {
		t.Fatal("Errorf result does not unwrap/as to *Error")
	}
	if typed.Code != 404 || typed.Status != StatusNotFound || typed.Message != "missing 42" {
		t.Fatalf("Errorf result = (%d, %q, %q), want (404, NOT_FOUND, missing 42)", typed.Code, typed.Status, typed.Message)
	}
}

func TestErrorString(t *testing.T) {
	err := New(500, StatusInternalServerError).WithMessage("boom")
	want := "error: code = 500, status = INTERNAL_SERVER_ERROR, message = boom"

	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestNilReceiverMethods(t *testing.T) {
	var err *Error

	if got := err.Error(); got != "<nil>" {
		t.Fatalf("nil Error() = %q, want <nil>", got)
	}
	if got := err.Unwrap(); got != nil {
		t.Fatalf("nil Unwrap() = %#v, want nil", got)
	}
	if stderrors.Is(err, New(500, StatusInternalServerError)) {
		t.Fatal("errors.Is(nil *Error, target) = true, want false")
	}
}

func TestWithMethodsMutateAndReturnReceiver(t *testing.T) {
	details := map[string]string{"field": "name"}
	cause := stderrors.New("root cause")
	err := New(400, StatusBadRequest)

	got := err.
		WithCode(422).
		WithStatus("VALIDATION_FAILED").
		WithMessage("invalid payload").
		WithDetails(details).
		WithCause(cause)

	if got != err {
		t.Fatal("With* methods did not return receiver")
	}
	if err.Code != 422 || err.Status != "VALIDATION_FAILED" || err.Message != "invalid payload" {
		t.Fatalf("mutated error = (%d, %q, %q), want (422, VALIDATION_FAILED, invalid payload)", err.Code, err.Status, err.Message)
	}
	gotDetails, ok := err.Details.(map[string]string)
	if !ok {
		t.Fatalf("Details type = %T, want map[string]string", err.Details)
	}
	if gotDetails["field"] != details["field"] {
		t.Fatalf("Details = %#v, want %#v", gotDetails, details)
	}
	if !stderrors.Is(err, cause) {
		t.Fatal("errors.Is(err, cause) = false, want true")
	}
}

func TestIsComparesCodeAndStatus(t *testing.T) {
	err := New(404, StatusNotFound).WithMessage("user missing")
	same := New(404, StatusNotFound).WithMessage("project missing")
	differentCode := New(400, StatusNotFound)
	differentStatus := New(404, StatusBadRequest)

	if !stderrors.Is(err, same) {
		t.Fatal("errors.Is(err, same) = false, want true")
	}
	if stderrors.Is(err, differentCode) {
		t.Fatal("errors.Is(err, differentCode) = true, want false")
	}
	if stderrors.Is(err, differentStatus) {
		t.Fatal("errors.Is(err, differentStatus) = true, want false")
	}
	if stderrors.Is(err, stderrors.New("plain")) {
		t.Fatal("errors.Is(err, plain error) = true, want false")
	}
}

func TestIsDoesNotUnwrapTarget(t *testing.T) {
	err := New(404, StatusNotFound)
	wrappedTarget := fmt.Errorf("wrapped: %w", New(404, StatusNotFound))

	if stderrors.Is(err, wrappedTarget) {
		t.Fatal("errors.Is(err, wrappedTarget) = true, want false")
	}
}

func TestClone(t *testing.T) {
	cause := stderrors.New("root cause")
	original := New(500, StatusInternalServerError).
		WithMessage("boom").
		WithDetails([]string{"a"}).
		WithCause(cause)

	cloned := original.Clone()
	if cloned == nil {
		t.Fatal("Clone() = nil, want copy")
	}
	if cloned == original {
		t.Fatal("Clone() returned original pointer")
	}
	if cloned.Code != original.Code || cloned.Status != original.Status || cloned.Message != original.Message {
		t.Fatalf("clone = (%d, %q, %q), want original values", cloned.Code, cloned.Status, cloned.Message)
	}
	if !reflect.DeepEqual(cloned.Details, original.Details) {
		t.Fatal("Clone() should preserve Details value")
	}
	if !stderrors.Is(cloned, cause) {
		t.Fatal("cloned error does not unwrap to original cause")
	}

	cloned.WithMessage("changed")
	if original.Message != "boom" {
		t.Fatalf("original Message = %q, want boom", original.Message)
	}

	if (*Error)(nil).Clone() != nil {
		t.Fatal("nil Clone() != nil")
	}
}

func TestAsError(t *testing.T) {
	if got := AsError(nil); got != nil {
		t.Fatalf("AsError(nil) = %#v, want nil", got)
	}
	if got := AsError(stderrors.New("plain")); got != nil {
		t.Fatalf("AsError(plain) = %#v, want nil", got)
	}

	target := BadRequest("bad")
	wrapped := fmt.Errorf("wrapped: %w", target)
	if got := AsError(wrapped); got != target {
		t.Fatalf("AsError(wrapped) = %#v, want original *Error", got)
	}
}

func TestConvenienceConstructors(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		code    int
		status  string
		message string
	}{
		{"BadRequest", BadRequest("bad"), 400, StatusBadRequest, "bad"},
		{"NotFound", NotFound("missing"), 404, StatusNotFound, "missing"},
		{"InternalServerError", InternalServerError("boom"), 500, StatusInternalServerError, "boom"},
		{"Unauthorized", Unauthorized("login"), 401, StatusUnauthorized, "login"},
		{"Forbidden", Forbidden("denied"), 403, StatusForbidden, "denied"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.code || tt.err.Status != tt.status || tt.err.Message != tt.message {
				t.Fatalf("%s = (%d, %q, %q), want (%d, %q, %q)",
					tt.name, tt.err.Code, tt.err.Status, tt.err.Message, tt.code, tt.status, tt.message)
			}
		})
	}
}
