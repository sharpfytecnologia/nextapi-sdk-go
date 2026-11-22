package nextapi

import (
	"errors"
	"fmt"
	"time"
)

// Códigos de erro da NextAPI, no corpo { status, code, message }.
const (
	CodeNotFound                 = 1000
	CodeTokenNotProvided         = 1001
	CodeTokenInvalid             = 1002
	CodeTooManyRequests          = 1003
	CodeMissingCredits           = 1004
	CodeLiveSearchNotAllowed     = 1005
	CodeLiveSearchMissingCredits = 1006
	CodeInvalidCep               = 1007
	CodeRequiresPaidPlan         = 1012
	CodeExternalProviderFailure  = 1014
	CodeSandboxDailyLimit        = 1015
	CodeSandboxDisabled          = 1016
	// Monitoramento: o saldo não cobre o mês dos CNPJs novos (402). Nada foi incluído.
	CodeMonitoringInsufficientCredits = 1024
	// Monitoramento: o plano gratuito acompanha até 10 CNPJs (402). Nada foi incluído.
	CodeMonitoringFreePlanLimit = 1025
)

// Kind classifica a falha; compare com errors.Is(err, nextapi.ErrNotFound) etc.
type Kind int

const (
	KindUnknown Kind = iota
	KindAuthentication
	KindInsufficientCredits
	KindPaidPlanRequired
	KindNotFound
	KindValidation
	KindRateLimit
	KindServer
	KindConnection
)

// Sentinelas para errors.Is.
var (
	ErrAuthentication      = &Error{Kind: KindAuthentication}
	ErrInsufficientCredits = &Error{Kind: KindInsufficientCredits}
	ErrPaidPlanRequired    = &Error{Kind: KindPaidPlanRequired}
	ErrNotFound            = &Error{Kind: KindNotFound}
	ErrValidation          = &Error{Kind: KindValidation}
	ErrRateLimit           = &Error{Kind: KindRateLimit}
	ErrServer              = &Error{Kind: KindServer}
	ErrConnection          = &Error{Kind: KindConnection}
)

// Error é toda falha do SDK. Use errors.As para ler Status, Code e RequestID.
//
// Code distingue casos com o mesmo status: 405 vale para "sem créditos" (1004, 1006) e para
// "plano pago obrigatório" (1012, 1005); 402 vale para os créditos (1024) e o limite do plano gratuito (1025) do
// monitoramento.
type Error struct {
	Kind       Kind
	Status     int // 0 quando a requisição nem chegou à API
	Code       int // 0 quando a API não informou
	Message    string
	Body       []byte
	RequestID  string
	RetryAfter time.Duration
	Err        error // causa (rede, tempo esgotado)
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return "nextapi: " + e.Message
	}
	if e.Code != 0 {
		return fmt.Sprintf("nextapi: %s (HTTP %d, código %d)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("nextapi: %s (HTTP %d)", e.Message, e.Status)
}

func (e *Error) Unwrap() error { return e.Err }

// Is faz errors.Is(err, nextapi.ErrNotFound) comparar pelo tipo da falha.
func (e *Error) Is(target error) bool {
	var t *Error
	return errors.As(target, &t) && t.Status == 0 && t.Message == "" && t.Kind == e.Kind
}

func kindOf(status, code int) Kind {
	switch code {
	case CodeTokenNotProvided, CodeTokenInvalid, CodeSandboxDisabled:
		return KindAuthentication
	case CodeMissingCredits, CodeLiveSearchMissingCredits, CodeMonitoringInsufficientCredits:
		return KindInsufficientCredits
	case CodeRequiresPaidPlan, CodeLiveSearchNotAllowed, CodeMonitoringFreePlanLimit:
		return KindPaidPlanRequired
	}
	switch {
	case status == 401 || status == 403:
		return KindAuthentication
	case status == 404:
		return KindNotFound
	case status == 429:
		return KindRateLimit
	case status >= 500:
		return KindServer
	case status == 400 || status == 422:
		return KindValidation
	}
	return KindUnknown
}

func invalid(format string, args ...any) error {
	return &Error{Kind: KindValidation, Message: fmt.Sprintf(format, args...)}
}
