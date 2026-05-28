package abyss

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestError_Error_WithoutCause(t *testing.T) {
	e := &Error{Code: "not_found", Message: "resource not found"}
	assert.Equal(t, "not_found: resource not found", e.Error())
}

func TestError_Error_WithCause(t *testing.T) {
	cause := errors.New("db timeout")
	e := &Error{Code: "internal", Message: "internal server error", Cause: cause}
	assert.Contains(t, e.Error(), "internal")
	assert.Contains(t, e.Error(), "db timeout")
}

func TestError_Error_Nil(t *testing.T) {
	var e *Error
	assert.Equal(t, "", e.Error())
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("original")
	e := &Error{Code: "internal", Message: "wrapped", Cause: cause}
	assert.Equal(t, cause, errors.Unwrap(e))
}

func TestError_Unwrap_Nil(t *testing.T) {
	var e *Error
	assert.Nil(t, e.Unwrap())
}

func TestError_Unwrap_NoCause(t *testing.T) {
	e := &Error{Code: "not_found", Message: "not found"}
	assert.Nil(t, e.Unwrap())
}

func TestSentinels_Defined(t *testing.T) {
	require.NotNil(t, ErrNotFound)
	require.NotNil(t, ErrUnauthorized)
	require.NotNil(t, ErrForbidden)
	require.NotNil(t, ErrInvalidInput)
	require.NotNil(t, ErrConflict)
	require.NotNil(t, ErrInternal)

	assert.Equal(t, "not_found", ErrNotFound.Code)
	assert.Equal(t, "unauthorized", ErrUnauthorized.Code)
	assert.Equal(t, "forbidden", ErrForbidden.Code)
	assert.Equal(t, "invalid_input", ErrInvalidInput.Code)
	assert.Equal(t, "conflict", ErrConflict.Code)
	assert.Equal(t, "internal", ErrInternal.Code)
}

func TestWrapError_OverridesMessage(t *testing.T) {
	cause := errors.New("low-level")
	wrapped := WrapError(ErrNotFound, cause, "custom message")
	assert.Equal(t, "not_found", wrapped.Code)
	assert.Equal(t, "custom message", wrapped.Message)
	assert.Equal(t, cause, wrapped.Cause)
}

func TestWrapError_PreservesBaseMessage(t *testing.T) {
	cause := errors.New("low-level")
	wrapped := WrapError(ErrInternal, cause, "")
	assert.Equal(t, "internal", wrapped.Code)
	assert.Equal(t, ErrInternal.Message, wrapped.Message)
}

func TestWrapError_NilBase_UsesErrInternal(t *testing.T) {
	wrapped := WrapError(nil, errors.New("x"), "msg")
	assert.Equal(t, ErrInternal.Code, wrapped.Code)
}

func TestError_ErrorsAs(t *testing.T) {
	e := WrapError(ErrNotFound, nil, "item missing")
	var target *Error
	require.True(t, errors.As(e, &target))
	assert.Equal(t, "not_found", target.Code)
}
