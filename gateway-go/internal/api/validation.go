package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

// Patterns and enums from contracts/openapi.yaml.
var (
	rrnPattern      = regexp.MustCompile(`^[0-9A-Z]{12}$`)
	currencyPattern = regexp.MustCompile(`^[0-9]{3}$`)
	rcPattern       = regexp.MustCompile(`^[0-9A-Z]{2}$`)
	last4Pattern    = regexp.MustCompile(`^[0-9]{4}$`)

	entryModes         = set("CHIP_PIN", "CHIP_NO_PIN", "MANUAL_PIN", "MANUAL_NO_PIN")
	transactionStatus  = set("CREATED", "SENT", "APPROVED", "DECLINED", "TIMED_OUT", "REVERSAL_PENDING", "REVERSED", "FAILED")
	reversalReasonEnum = set(store.ReversalCustomerCancellation, store.ReversalTimeout, store.ReversalMACFailure, store.ReversalSendFailure)
)

const (
	// maxAmount is the largest amount DE 4 (n 12) can carry.
	maxAmount = 999_999_999_999
	// maxBodyBytes bounds a POST body: every transaction request is well under 1 KiB.
	maxBodyBytes     = 16 << 10
	maxListLimit     = 200
	terminalIDLength = 8
)

func set(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

// fieldError is one entry of a validation-error problem's errors[] (docs/04 §3).
type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// fieldErrors collects the violations of one request.
type fieldErrors []fieldError

func (e *fieldErrors) check(ok bool, field, message string) {
	if !ok {
		*e = append(*e, fieldError{Field: field, Message: message})
	}
}

// validationProblem writes a 400 validation-error with errors[] filled (docs/04 §3).
func validationProblem(w http.ResponseWriter, req *http.Request, errs fieldErrors) {
	writeProblem(w, req, problemBody{
		Status: http.StatusBadRequest, Type: problemValidation, Detail: errs[0].Field + " " + errs[0].Message, Errors: errs,
	})
}

// idempotencyKey returns the request's Idempotency-Key, or writes the 400 a missing or non-UUID
// key gets (docs/04 §2).
func idempotencyKey(w http.ResponseWriter, req *http.Request) (string, bool) {
	key := req.Header.Get("Idempotency-Key")
	if _, err := uuid.Parse(key); err != nil {
		problem(w, req, http.StatusBadRequest, problemIdempotencyKey, "Idempotency-Key header must be a UUID")
		return "", false
	}
	return key, true
}

// decodeBody decodes req's JSON body into dst and validates it with check, writing the 400
// validation-error when either fails.
func decodeBody[T any](w http.ResponseWriter, req *http.Request, dst *T, check func(T) fieldErrors) bool {
	req.Body = http.MaxBytesReader(w, req.Body, maxBodyBytes)
	if err := json.NewDecoder(req.Body).Decode(dst); err != nil {
		validationProblem(w, req, fieldErrors{{Field: fieldBody, Message: "must be a JSON object matching the schema"}})
		return false
	}
	if errs := check(*dst); len(errs) > 0 {
		validationProblem(w, req, errs)
		return false
	}
	return true
}

// pathRRN returns the {rrn} path parameter, or writes the 400 a malformed one gets.
func pathRRN(w http.ResponseWriter, req *http.Request) (string, bool) {
	rrn := chi.URLParam(req, "rrn")
	if !rrnPattern.MatchString(rrn) {
		validationProblem(w, req, fieldErrors{{Field: "rrn", Message: "must match ^[0-9A-Z]{12}$"}})
		return "", false
	}
	return rrn, true
}

func checkMoney(errs *fieldErrors, m purchase.Money) {
	errs.check(m.Amount > 0 && m.Amount <= maxAmount, "amount.amount", "must be > 0 and fit DE 4 (at most 999999999999)")
	errs.check(currencyPattern.MatchString(m.Currency), "amount.currency", "must be an ISO 4217 numeric code")
}

// checkCardPresent validates contracts/openapi.yaml's CardPresentData.
func checkCardPresent(errs *fieldErrors, terminalID, cardToken, entryMode string) {
	errs.check(len(terminalID) == terminalIDLength, "terminalId", "must be 8 characters")
	errs.check(cardToken != "", "cardToken", "is required")
	errs.check(entryModes[entryMode], "entryMode", "must be one of CHIP_PIN, CHIP_NO_PIN, MANUAL_PIN, MANUAL_NO_PIN")
}

// checkAmount validates a request that carries only an amount (a completion).
func checkAmount(m purchase.Money) fieldErrors {
	var errs fieldErrors
	checkMoney(&errs, m)
	return errs
}

// parseTransactionFilter reads GET /v1/transactions' query (contracts/openapi.yaml).
func parseTransactionFilter(req *http.Request) (store.TransactionFilter, fieldErrors) {
	q := req.URL.Query()
	filter := store.TransactionFilter{Cursor: q.Get("cursor")}
	var errs fieldErrors
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		errs.check(err == nil && n >= 1 && n <= maxListLimit, "limit", "must be an integer from 1 to 200")
		filter.Limit = n
	}
	filter.Status = enumParam(&errs, q.Get("status"), "status", transactionStatus)
	filter.ReversalReason = enumParam(&errs, q.Get("reversalReason"), "reversalReason", reversalReasonEnum)
	filter.RC = patternParam(&errs, q.Get("rc"), "rc", rcPattern)
	filter.Last4 = patternParam(&errs, q.Get("last4"), "last4", last4Pattern)
	filter.From = timeParam(&errs, q.Get("from"), "from")
	filter.To = timeParam(&errs, q.Get("to"), "to")
	return filter, errs
}

func enumParam(errs *fieldErrors, v, field string, allowed map[string]bool) *string {
	if v == "" {
		return nil
	}
	errs.check(allowed[v], field, "is not an allowed value")
	return &v
}

func patternParam(errs *fieldErrors, v, field string, pattern *regexp.Regexp) *string {
	if v == "" {
		return nil
	}
	errs.check(pattern.MatchString(v), field, "must match "+pattern.String())
	return &v
}

func timeParam(errs *fieldErrors, v, field string) *time.Time {
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	errs.check(err == nil, field, "must be an RFC 3339 date-time")
	return &t
}
