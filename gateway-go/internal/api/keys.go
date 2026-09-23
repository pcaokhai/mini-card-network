package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/store"
)

// keyLifetimeDays is the ceiling for daysRemaining (real HSM rotation policy is out of scope
// here; MCN-502+ can wire a per-key-type policy if that's ever needed).
// ponytail: fixed lifetime, add per-key-type policy if rotation requirements diverge.
const keyLifetimeDays = 365

// KeyLister is the port keys.go needs; *store.KeyStoreRepository satisfies it.
type KeyLister interface {
	List(ctx context.Context) ([]store.KeyRow, error)
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

// MountKeys registers GET /v1/keys/acquirer (contracts/openapi.yaml, tag "keys").
func MountKeys(r chi.Router, svc KeyLister) {
	r.Get("/v1/keys/acquirer", handleListAcquirerKeys(svc))
}

func handleListAcquirerKeys(svc KeyLister) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		rows, err := svc.List(req.Context())
		if err != nil {
			problem(w, http.StatusInternalServerError, "list-keys-failed", err.Error())
			return
		}
		infos := make([]keyInfo, 0, len(rows))
		for _, row := range rows {
			infos = append(infos, toKeyInfo(row))
		}
		writeJSONBody(w, http.StatusOK, infos)
	}
}

func toKeyInfo(row store.KeyRow) keyInfo {
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
		DaysRemaining: daysRemaining(row.ActivatedAt),
		LifetimeDays:  keyLifetimeDays,
	}
}

func daysRemaining(activatedAt *time.Time) int {
	if activatedAt == nil {
		return keyLifetimeDays
	}
	remaining := keyLifetimeDays - int(time.Since(*activatedAt).Hours()/24)
	if remaining < 0 {
		return 0
	}
	return remaining
}
