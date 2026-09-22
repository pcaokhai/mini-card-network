package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/journey"
	"github.com/mcn/gateway-go/internal/store"
)

// TranLogReader is the read-side port transactions_query.go needs; *store.TranLogRepository
// satisfies it.
type TranLogReader interface {
	List(ctx context.Context, filter store.TransactionFilter) ([]store.TranLogRow, string, error)
	Get(ctx context.Context, rrn string) (store.TranLogRow, error)
	ListStateHistory(ctx context.Context, tranID int64) ([]store.StateTransition, error)
}

// MountTransactionsQuery registers the read-only transaction routes (contracts/openapi.yaml, tag
// "transactions"): list, detail, and journey.
func MountTransactionsQuery(r chi.Router, reader TranLogReader) {
	r.Get("/v1/transactions", handleListTransactions(reader))
	r.Get("/v1/transactions/{rrn}", handleGetTransaction(reader))
	r.Get("/v1/transactions/{rrn}/journey", handleGetTransactionJourney(reader))
}

func handleListTransactions(reader TranLogReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		filter, err := parseTransactionFilter(req)
		if err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		page, nextCursor, err := reader.List(req.Context(), filter)
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
		row, err := reader.Get(req.Context(), chi.URLParam(req, "rrn"))
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

func handleGetTransactionJourney(reader TranLogReader) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		row, err := reader.Get(req.Context(), chi.URLParam(req, "rrn"))
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
		j := journey.BuildJourney(row, history)
		writeJSONBody(w, http.StatusOK, toJourneyDTO(row, j))
	}
}

func parseTransactionFilter(req *http.Request) (store.TransactionFilter, error) {
	q := req.URL.Query()
	filter := store.TransactionFilter{Cursor: q.Get("cursor")}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return store.TransactionFilter{}, errors.New("invalid limit")
		}
		filter.Limit = n
	}
	if v := q.Get("status"); v != "" {
		filter.Status = &v
	}
	if v := q.Get("rc"); v != "" {
		filter.RC = &v
	}
	if v := q.Get("last4"); v != "" {
		filter.Last4 = &v
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return store.TransactionFilter{}, errors.New("invalid from")
		}
		filter.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return store.TransactionFilter{}, errors.New("invalid to")
		}
		filter.To = &t
	}
	return filter, nil
}

// transactionSummaryDTO mirrors contracts/openapi.yaml's TransactionSummary schema.
type transactionSummaryDTO struct {
	RRN           string    `json:"rrn"`
	STAN          string    `json:"stan,omitempty"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	ResponseCode  *string   `json:"responseCode"`
	ResponseLabel *string   `json:"responseLabel"`
	Amount        moneyDTO  `json:"amount"`
	MaskedPAN     string    `json:"maskedPan"`
	TerminalID    string    `json:"terminalId"`
	MerchantName  string    `json:"merchantName"`
	LatencyMs     *int      `json:"latencyMs"`
	CreatedAt     time.Time `json:"createdAt"`
}

// transactionDTO mirrors contracts/openapi.yaml's Transaction schema (TransactionSummary + detail
// fields).
type transactionDTO struct {
	transactionSummaryDTO
	AuthCode       *string `json:"authCode"`
	ApprovedAmount *string `json:"approvedAmount"`
	Balance        *string `json:"balance"`
	BusinessDate   string  `json:"businessDate"`
	OriginalRRN    *string `json:"originalRrn"`
	TraceID        string  `json:"traceId"`
}

type moneyDTO struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func toSummaryDTO(row store.TranLogRow) transactionSummaryDTO {
	return transactionSummaryDTO{
		RRN:           row.RRN,
		STAN:          row.NetworkSTAN,
		Type:          row.Type,
		Status:        row.Status,
		ResponseCode:  nullableString(row.ResponseCode),
		ResponseLabel: responseLabel(row.ResponseCode),
		Amount:        moneyDTO{Amount: row.Amount, Currency: row.Currency},
		MaskedPAN:     row.MaskedPAN,
		TerminalID:    row.TerminalID,
		MerchantName:  row.MerchantName,
		CreatedAt:     row.CreatedAt,
	}
}

func toTransactionDTO(row store.TranLogRow) transactionDTO {
	return transactionDTO{
		transactionSummaryDTO: toSummaryDTO(row),
		AuthCode:              nullableString(row.AuthCode),
		BusinessDate:          row.CreatedAt.Format("2006-01-02"),
		// ponytail: tran_log has no trace_id column yet (docs/03 §10 describes joining traces by
		// RRN in Tempo/Grafana, not a stored trace_id) - RRN doubles as the correlation id here
		// until a future story adds real trace_id persistence.
		TraceID: row.RRN,
	}
}

// journeyDTO mirrors contracts/openapi.yaml's Journey schema.
type journeyDTO struct {
	Transaction transactionDTO   `json:"transaction"`
	Steps       []journeyStepDTO `json:"steps"`
	Money       []moneyRowDTO    `json:"money"`
}

type journeyStepDTO struct {
	Seq           int    `json:"seq"`
	Actor         string `json:"actor"`
	OffsetMs      int    `json:"offsetMs"`
	Title         string `json:"title"`
	EasyText      string `json:"easyText"`
	TechnicalText string `json:"technicalText"`
	Kind          string `json:"kind"`
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
			Seq: s.Seq, Actor: string(s.Actor), OffsetMs: s.OffsetMs, Title: s.Title,
			EasyText: s.EasyText, TechnicalText: s.TechnicalText, Kind: string(s.Kind),
		}
	}
	money := make([]moneyRowDTO, len(j.Money))
	for i, m := range j.Money {
		money[i] = moneyRowDTO{Label: m.Label, Delta: m.Delta, BalanceAfter: m.BalanceAfter, AtStep: m.AtStep}
	}
	return journeyDTO{Transaction: toTransactionDTO(row), Steps: steps, Money: money}
}

func responseLabel(rc string) *string {
	if rc == "" {
		return nil
	}
	label := journey.EasyTextForRC(rc)
	return &label
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
