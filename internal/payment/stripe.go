package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const stripeBaseURL = "https://api.stripe.com/v1"

type stripe struct {
	secretKey     string
	webhookSecret string
	client        *http.Client
}

func NewStripe(secretKey, webhookSecret string) Gateway {
	return &stripe{secretKey: secretKey, webhookSecret: webhookSecret, client: newClient()}
}

func (g *stripe) Name() string { return "stripe" }

func (g *stripe) Initialize(in InitInput) (InitResult, error) {
	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("customer_email", in.Email)
	form.Set("client_reference_id", in.Reference)
	form.Set("success_url", in.CallbackURL)
	form.Set("cancel_url", in.CallbackURL+"&cancelled=1")
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(in.Currency))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(in.Amount, 10))
	form.Set("line_items[0][price_data][product_data][name]", in.Description)
	form.Set("metadata[reference]", in.Reference)

	req, err := http.NewRequest(http.MethodPost, stripeBaseURL+"/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return InitResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.secretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.client.Do(req)
	if err != nil {
		return InitResult{}, err
	}
	defer resp.Body.Close()

	var out struct {
		ID    string `json:"id"`
		URL   string `json:"url"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return InitResult{}, err
	}
	if out.URL == "" || out.ID == "" {
		return InitResult{}, fmt.Errorf("stripe: %s", out.Error.Message)
	}
	return InitResult{CheckoutURL: out.URL, GatewayRef: out.ID}, nil
}

// Verify looks the checkout session up by the id saved when it was created.
func (g *stripe) Verify(sessionID string) (Result, error) {
	req, err := http.NewRequest(http.MethodGet, stripeBaseURL+"/checkout/sessions/"+url.PathEscape(sessionID), nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.secretKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	var out struct {
		Status        string `json:"status"`         // open | complete | expired
		PaymentStatus string `json:"payment_status"` // paid | unpaid | no_payment_required
		AmountTotal   int64  `json:"amount_total"`
		Currency      string `json:"currency"`
		Error         struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, err
	}
	if out.Error.Message != "" {
		return Result{}, fmt.Errorf("stripe: %s", out.Error.Message)
	}

	status := StatusPending
	switch {
	case out.PaymentStatus == "paid":
		status = StatusSuccess
	case out.Status == "expired":
		status = StatusFailed
	}
	return Result{Status: status, Amount: out.AmountTotal, Currency: upper(out.Currency)}, nil
}

func (g *stripe) ParseWebhook(header http.Header, body []byte) (string, error) {
	if g.webhookSecret == "" {
		return "", ErrBadSignature
	}

	var timestamp string
	var signatures []string
	for _, part := range strings.Split(header.Get("Stripe-Signature"), ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}
	if timestamp == "" || len(signatures) == 0 {
		return "", ErrBadSignature
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(ts, 0)) > 5*time.Minute {
		return "", ErrBadSignature
	}

	mac := hmac.New(sha256.New, []byte(g.webhookSecret))
	mac.Write([]byte(timestamp + "." + string(body)))
	expected := hex.EncodeToString(mac.Sum(nil))

	valid := false
	for _, s := range signatures {
		if hmac.Equal([]byte(expected), []byte(s)) {
			valid = true
		}
	}
	if !valid {
		return "", ErrBadSignature
	}

	var event struct {
		Data struct {
			Object struct {
				ClientReferenceID string `json:"client_reference_id"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return "", err
	}
	return event.Data.Object.ClientReferenceID, nil
}
