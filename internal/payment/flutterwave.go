package payment

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
)

const flutterwaveBaseURL = "https://api.flutterwave.com/v3"

type flutterwave struct {
	secretKey   string
	webhookHash string
	client      *http.Client
}

func NewFlutterwave(secretKey, webhookHash string) Gateway {
	return &flutterwave{secretKey: secretKey, webhookHash: webhookHash, client: newClient()}
}

func (g *flutterwave) Name() string { return "flutterwave" }

func (g *flutterwave) Initialize(in InitInput) (InitResult, error) {
	body, err := json.Marshal(map[string]any{
		"tx_ref":       in.Reference,
		"amount":       float64(in.Amount) / 100, // Flutterwave takes the major unit
		"currency":     upper(in.Currency),
		"redirect_url": in.CallbackURL,
		"customer":     map[string]string{"email": in.Email, "name": in.Name, "phonenumber": in.Phone},
		"customizations": map[string]string{
			"title":       "Akintola Samuel",
			"description": in.Description,
		},
	})
	if err != nil {
		return InitResult{}, err
	}

	req, err := http.NewRequest(http.MethodPost, flutterwaveBaseURL+"/payments", bytes.NewReader(body))
	if err != nil {
		return InitResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return InitResult{}, err
	}
	defer resp.Body.Close()

	var out struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Link string `json:"link"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return InitResult{}, err
	}
	if out.Status != "success" || out.Data.Link == "" {
		return InitResult{}, fmt.Errorf("flutterwave: %s", out.Message)
	}
	return InitResult{CheckoutURL: out.Data.Link, GatewayRef: in.Reference}, nil
}

func (g *flutterwave) Verify(ref string) (Result, error) {
	req, err := http.NewRequest(http.MethodGet,
		flutterwaveBaseURL+"/transactions/verify_by_reference?tx_ref="+url.QueryEscape(ref), nil)
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
		Status string `json:"status"`
		Data   struct {
			Status   string  `json:"status"` // successful | failed | pending
			Amount   float64 `json:"amount"`
			Currency string  `json:"currency"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, err
	}
	// "error" with no data means the transaction has not been started yet.
	if out.Status != "success" {
		return Result{Status: StatusPending}, nil
	}

	status := StatusPending
	switch out.Data.Status {
	case "successful":
		status = StatusSuccess
	case "failed":
		status = StatusFailed
	}
	return Result{
		Status:   status,
		Amount:   int64(math.Round(out.Data.Amount * 100)),
		Currency: upper(out.Data.Currency),
	}, nil
}

func (g *flutterwave) ParseWebhook(header http.Header, body []byte) (string, error) {
	// An unset secret hash must never match an empty header.
	if g.webhookHash == "" ||
		subtle.ConstantTimeCompare([]byte(header.Get("verif-hash")), []byte(g.webhookHash)) != 1 {
		return "", ErrBadSignature
	}

	var event struct {
		Data struct {
			TxRef string `json:"tx_ref"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return "", err
	}
	return event.Data.TxRef, nil
}
