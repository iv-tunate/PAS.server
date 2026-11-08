// Package payment talks to the payment providers. Every provider sits behind
// the same small interface, so the order service does not care which one the
// buyer picked.
//
// Money is always in minor units (kobo for NGN, cents for USD).
package payment

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// Checkout statuses a provider can report for a transaction.
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusPending = "pending"
)

var ErrBadSignature = errors.New("webhook signature did not match")

type InitInput struct {
	Reference   string // our own id for the order
	Email       string
	Name        string
	Phone       string
	Description string
	Amount      int64
	Currency    string // "NGN" | "USD"
	CallbackURL string // where the provider sends the buyer afterwards
}

type InitResult struct {
	CheckoutURL string
	// GatewayRef is the id to ask the provider about later. For Paystack and
	// Flutterwave that is our own reference; Stripe has its own session id.
	GatewayRef string
}

// Result is what the provider says really happened. The order service checks
// Amount and Currency against the order before delivering anything.
type Result struct {
	Status   string
	Amount   int64
	Currency string
}

type Gateway interface {
	Name() string
	Initialize(in InitInput) (InitResult, error)
	Verify(gatewayRef string) (Result, error)
	// ParseWebhook checks the provider's signature on a webhook and returns
	// our order reference from it. The body is only trusted for the reference;
	// the order is always re-verified with Verify before it counts as paid.
	ParseWebhook(header http.Header, body []byte) (reference string, err error)
}

func newClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

func upper(s string) string { return strings.ToUpper(s) }
