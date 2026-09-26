package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/store"
)

// defaultKeyLifetimeDays applies to a key type the lifetime policy (KEY_LIFETIME_DAYS) leaves out.
const defaultKeyLifetimeDays = 365

// KeyLister is the port keys.go needs; *store.KeyStoreRepository satisfies it.
type KeyLister interface {
	ListCurrent(ctx context.Context) ([]store.KeyRow, error)
}

// keyInfo mirrors contracts/openapi.yaml's KeyInfo schema. It never carries the wrapped key.
type keyInfo struct {
	KeyType       string     `json:"keyType"`
	Counterparty  *string    `json:"counterparty"`
	KCV           string     `json:"kcv"`
	Status        string     `json:"status"`
	ActivatedAt   *time.Time `json:"activatedAt"`
	DaysRemaining int        `json:"daysRemaining"`
	LifetimeDays  int        `json:"lifetimeDays"`
}

// MountKeys registers GET /v1/keys/acquirer (contracts/openapi.yaml, tag "keys"). lifetimes is
// the per-type lifetime policy in days (SEC-G9); a type it leaves out gets 365.
func MountKeys(r chi.Router, svc KeyLister, lifetimes map[string]int) {
	r.Get("/v1/keys/acquirer", handleListAcquirerKeys(svc, lifetimes))
}

func handleListAcquirerKeys(svc KeyLister, lifetimes map[string]int) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		rows, err := svc.ListCurrent(req.Context())
		if err != nil {
			problem(w, http.StatusInternalServerError, problemInternal, "could not read the key inventory")
			return
		}
		infos := make([]keyInfo, 0, len(rows))
		for _, row := range rows {
			lifetime, ok := lifetimes[row.KeyType]
			if !ok {
				lifetime = defaultKeyLifetimeDays
			}
			infos = append(infos, toKeyInfo(row, lifetime))
		}
		writeJSONBody(w, http.StatusOK, infos)
	}
}

func toKeyInfo(row store.KeyRow, lifetimeDays int) keyInfo {
	var counterparty *string
	if row.OwnerRef != "" {
		counterparty = &row.OwnerRef
	}
	return keyInfo{
		KeyType:       row.KeyType,
		Counterparty:  counterparty,
		KCV:           row.KCV,
		Status:        row.Status,
		ActivatedAt:   row.ActivatedAt,
		DaysRemaining: daysRemaining(row.ActivatedAt, lifetimeDays),
		LifetimeDays:  lifetimeDays,
	}
}

func daysRemaining(activatedAt *time.Time, lifetimeDays int) int {
	if activatedAt == nil {
		return lifetimeDays
	}
	remaining := lifetimeDays - int(time.Since(*activatedAt).Hours()/24)
	if remaining < 0 {
		return 0
	}
	return remaining
}
