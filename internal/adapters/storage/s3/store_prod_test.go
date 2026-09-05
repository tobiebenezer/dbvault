//go:build !restricted

package s3

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/aws/smithy-go"

	"github.com/dbvault/dbvault/internal/domain"
)

type mockSmithyError struct {
	code    string
	message string
}

func (m *mockSmithyError) Error() string        { return m.code + ": " + m.message }
func (m *mockSmithyError) ErrorCode() string    { return m.code }
func (m *mockSmithyError) ErrorMessage() string { return m.message }
func (m *mockSmithyError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}

func TestClassifyErrors(t *testing.T) {
	// Nil error
	if classify(nil) != nil {
		t.Fatal("expected nil for nil error")
	}

	// NotFound smithy error
	errNotFound := classify(&mockSmithyError{code: "NoSuchKey", message: "The specified key does not exist."})
	var appErr *domain.AppError
	if !errors.As(errNotFound, &appErr) || appErr.Code != domain.ErrChunkMissing {
		t.Fatalf("expected ErrChunkMissing, got %v", errNotFound)
	}

	// NoSuchBucket
	errBucket := classify(&mockSmithyError{code: "NoSuchBucket", message: "The bucket does not exist."})
	if !errors.As(errBucket, &appErr) || appErr.Code != domain.ErrStorageUnavailable {
		t.Fatalf("expected ErrStorageUnavailable, got %v", errBucket)
	}

	// AccessDenied
	errAccess := classify(&mockSmithyError{code: "AccessDenied", message: "Access Denied."})
	if !errors.As(errAccess, &appErr) || appErr.Code != domain.ErrStorageUnavailable {
		t.Fatalf("expected ErrStorageUnavailable, got %v", errAccess)
	}

	// PreconditionFailed
	errPrecond := classify(&mockSmithyError{code: "PreconditionFailed", message: "At least one precondition failed."})
	if !errors.As(errPrecond, &appErr) || appErr.Code != domain.ErrStorageUnavailable {
		t.Fatalf("expected ErrStorageUnavailable, got %v", errPrecond)
	}

	// EOF / Timeout
	errEOF := classify(io.EOF)
	if !errors.As(errEOF, &appErr) || appErr.Code != domain.ErrStorageUnavailable {
		t.Fatalf("expected ErrStorageUnavailable for EOF, got %v", errEOF)
	}

	errTimeout := classify(http.ErrHandlerTimeout)
	if !errors.As(errTimeout, &appErr) || appErr.Code != domain.ErrStorageUnavailable {
		t.Fatalf("expected ErrStorageUnavailable for timeout, got %v", errTimeout)
	}
}
