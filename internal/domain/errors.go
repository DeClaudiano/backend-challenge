package domain

import "fmt"

type ErrorCode string

const (
	ErrInvalidValue           ErrorCode = "INVALID_VALUE"
	ErrInvalidCurrency        ErrorCode = "INVALID_CURRENCY"
	ErrCurrencyMismatch       ErrorCode = "CURRENCY_MISMATCH"
	ErrOverflow               ErrorCode = "MONEY_OVERFLOW"
	ErrNegativeAmount         ErrorCode = "NEGATIVE_AMOUNT"
	ErrNonPositiveAmount      ErrorCode = "NON_POSITIVE_AMOUNT"
	ErrInsufficientBalance    ErrorCode = "INSUFFICIENT_BALANCE"
	ErrInsufficientReversal   ErrorCode = "INSUFFICIENT_REVERSAL_BALANCE"
	ErrInvalidTransition      ErrorCode = "INVALID_STATE_TRANSITION"
	ErrTerminalTransaction    ErrorCode = "TERMINAL_TRANSACTION"
	ErrInvalidOperation       ErrorCode = "INVALID_OPERATION"
	ErrReferenceRequired      ErrorCode = "REFERENCE_REQUIRED"
	ErrReferenceNotFound      ErrorCode = "REFERENCE_NOT_FOUND"
	ErrReferenceIncompatible  ErrorCode = "REFERENCE_INCOMPATIBLE"
	ErrReversalAlreadyApplied ErrorCode = "REVERSAL_ALREADY_APPLIED"
	ErrIdentityConflict       ErrorCode = "IDENTITY_CONFLICT"
	ErrInvalidOpening         ErrorCode = "INVALID_OPENING"
	ErrNotFound               ErrorCode = "NOT_FOUND"
	ErrOptimisticConflict     ErrorCode = "OPTIMISTIC_CONFLICT"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && e.Code == other.Code
}
func NewError(code ErrorCode, message string) *Error { return &Error{Code: code, Message: message} }
func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}
