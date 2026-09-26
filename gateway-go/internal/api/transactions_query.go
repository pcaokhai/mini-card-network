package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/journey"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

// TranLogReader is the read-side port transactions_query.go needs; *store.TranLogRepository
// satisfies it.
type TranLogReader interface {
	List(ctx context.Context, filter store.TransactionFilter) ([]store.TranLogRow, string, error)
	Get(ctx context.Context, rrn string) (store.TranLogRow, error)
	ListStateHistory(ctx context.Context, tranID int64) ([]store.StateTransition, error)
}

// ReversalReader reads a transaction's queued reversal for its journey (nil when none);
// *saf.ReversalLookup satisfies it.
type ReversalReader interface {
	Reversal(ctx context.Context, tranID int64) (*journey.Reversal, error)
}

// MountTransactionsQuery registers the read-only transaction routes (contracts/openapi.yaml, tag
// "transactions"): list, detail, and journey.
func MountTransactionsQuery(r chi.Router, reader TranLogReader, reversals ReversalReader) {
	r.Get("/v1/transactions", handleListTransactions(reader))
	r.Get("/v1/transactions/{rrn}", handleGetTransaction(reader))
	r.Get("/v1/transactions/{rrn}/journey", handleGetTransactionJourney(reader, reversals))
}

func handleListTransactions(reader TranLogReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		filter, errs := parseTransactionFilter(req)
		if len(errs) > 0 {
			validationProblem(w, errs)
			return
		}
		page, nextCursor, err := reader.List(req.Context(), filter)
		if errors.Is(err, store.ErrInvalidCursor) {
			validationProblem(w, fieldErrors{{Field: "cursor", Message: "is not a cursor this API returned"}})
			return
		}
		if err != nil {
			problem(w, http.StatusInternalServerError, "transactions-read-failed", err.Error())
			return
		}
		items := make([]transactionSummaryDTO, len(page))
		for i, row := range page {
			items[i] = toSummaryDTO(row)
		}
		writeJSONBody(w, http.StatusOK, struct {
			Items      []transactionSummaryDTO `json:"items"`
			NextCursor *string                 `json:"nextCursor"`
		}{Items: items, NextCursor: nullableString(nextCursor)})
	}
}

func handleGetTransaction(reader TranLogReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		rrn, ok := pathRRN(w, req)
		if !ok {
			return
		}
		row, err := reader.Get(req.Context(), rrn)
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "unknown-transaction", "no such transaction")
			return
		}
		if err != nil {
			problem(w, http.StatusInternalServerError, "transaction-read-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusOK, toTransactionDTO(row))
	}
}

func handleGetTransactionJourney(reader TranLogReader, reversals ReversalReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		rrn, ok := pathRRN(w, req)
		if !ok {
			return
		}
		row, err := reader.Get(req.Context(), rrn)
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "unknown-transaction", "no such transaction")
			return
		}
		if err != nil {
			problem(w, http.StatusInternalServerError, "transaction-read-failed", err.Error())
			return
		}
		history, err := reader.ListStateHistory(req.Context(), row.ID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "journey-read-failed", err.Error())
			return
		}
		rev, err := findReversal(req.Context(), reversals, row.ID, history)
		if err != nil {
			problem(w, http.StatusInternalServerError, "journey-read-failed", err.Error())
			return
		}
		j := journey.BuildJourney(row, history, rev)
		writeJSONBody(w, http.StatusOK, toJourneyDTO(row, j))
	}
}

const statusReversalPending = "REVERSAL_PENDING"

// findReversal reads saf_queue only for a transaction that ever queued a reversal.
func findReversal(ctx context.Context, reversals ReversalReader, tranID int64, history []store.StateTransition) (*journey.Reversal, error) {
	for _, st := range history {
		if st.ToStatus == statusReversalPending {
			return reversals.Reversal(ctx, tranID)
		}
	}
	return nil, nil
}

// transactionSummaryDTO mirrors contracts/openapi.yaml's TransactionSummary schema.
type transactionSummaryDTO struct {
	RRN            string    `json:"rrn"`
	STAN           string    `json:"stan,omitempty"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	ResponseCode   *string   `json:"responseCode"`
	ResponseLabel  *string   `json:"responseLabel"`
	Amount         moneyDTO  `json:"amount"`
	MaskedPAN      string    `json:"maskedPan"`
	TerminalID     string    `json:"terminalId"`
	MerchantName   string    `json:"merchantName"`
	LatencyMs      *int      `json:"latencyMs"`
	CreatedAt      time.Time `json:"createdAt"`
	ReversalReason *string   `json:"reversalReason"`
}

// transactionDTO mirrors contracts/openapi.yaml's Transaction schema (TransactionSummary + detail
// fields).
type transactionDTO struct {
	transactionSummaryDTO
	AuthCode       *string   `json:"authCode"`
	ApprovedAmount *moneyDTO `json:"approvedAmount"`
	Balance        *moneyDTO `json:"balance"`
	BusinessDate   string    `json:"businessDate"`
	OriginalRRN    *string   `json:"originalRrn"`
	TraceID        string    `json:"traceId,omitempty"`
}

type moneyDTO struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func toSummaryDTO(row store.TranLogRow) transactionSummaryDTO {
	return transactionSummaryDTO{
		RRN:            row.RRN,
		STAN:           row.NetworkSTAN,
		Type:           row.Type,
		Status:         row.Status,
		ResponseCode:   nullableString(row.ResponseCode),
		ResponseLabel:  responseLabel(row.ResponseCode),
		Amount:         moneyDTO{Amount: row.Amount, Currency: row.Currency},
		MaskedPAN:      row.MaskedPAN,
		TerminalID:     row.TerminalID,
		MerchantName:   row.MerchantName,
		LatencyMs:      journey.LatencyMs(row),
		CreatedAt:      row.CreatedAt,
		ReversalReason: nullableString(store.ReversalReasonOf(row.ReversalReasonCode)),
	}
}

func toTransactionDTO(row store.TranLogRow) transactionDTO {
	return transactionDTO{
		transactionSummaryDTO: toSummaryDTO(row),
		AuthCode:              nullableString(row.AuthCode),
		ApprovedAmount:        approvedAmountDTO(row),
		Balance:               balanceDTO(row.Balance),
		BusinessDate:          row.CreatedAt.Format("2006-01-02"),
		OriginalRRN:           nullableString(row.OriginalRRN),
		// Rows created before tran_log.trace_id existed have none; traceId is then omitted.
		TraceID: row.TraceID,
	}
}

func approvedAmountDTO(row store.TranLogRow) *moneyDTO {
	if row.ApprovedAmount == nil {
		return nil
	}
	return &moneyDTO{Amount: *row.ApprovedAmount, Currency: row.Currency}
}

func balanceDTO(balance *store.Money) *moneyDTO {
	if balance == nil {
		return nil
	}
	return &moneyDTO{Amount: balance.Amount, Currency: balance.Currency}
}

// journeyDTO mirrors contracts/openapi.yaml's Journey schema.
type journeyDTO struct {
	Transaction transactionDTO   `json:"transaction"`
	Steps       []journeyStepDTO `json:"steps"`
	Money       []moneyRowDTO    `json:"money"`
}

type journeyStepDTO struct {
	Seq           int                 `json:"seq"`
	Code          string              `json:"code"`
	Actor         string              `json:"actor"`
	OffsetMs      int                 `json:"offsetMs"`
	Title         string              `json:"title"`
	EasyText      string              `json:"easyText"`
	TechnicalText string              `json:"technicalText"`
	Kind          string              `json:"kind"`
	Message       *journey.IsoMessage `json:"message"`
}

type moneyRowDTO struct {
	Label        string `json:"label"`
	Delta        int64  `json:"delta"`
	BalanceAfter *int64 `json:"balanceAfter"`
	AtStep       int    `json:"atStep"`
}

func toJourneyDTO(row store.TranLogRow, j journey.Journey) journeyDTO {
	steps := make([]journeyStepDTO, len(j.Steps))
	for i, s := range j.Steps {
		steps[i] = journeyStepDTO{
			Seq: s.Seq, Code: string(s.Code), Actor: string(s.Actor), Message: s.Message, OffsetMs: s.OffsetMs, Title: s.Title,
			EasyText: s.EasyText, TechnicalText: s.TechnicalText, Kind: string(s.Kind),
		}
	}
	money := make([]moneyRowDTO, len(j.Money))
	for i, m := range j.Money {
		money[i] = moneyRowDTO{Label: m.Label, Delta: m.Delta, BalanceAfter: m.BalanceAfter, AtStep: m.AtStep}
	}
	return journeyDTO{Transaction: toTransactionDTO(row), Steps: steps, Money: money}
}

func responseLabel(rc string) *string { return purchase.ResponseLabel(rc) }

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
