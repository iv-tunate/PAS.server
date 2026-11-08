package payment

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const paystackBaseURL = "https://api.paystack.co"

type paystack struct {
	secretKey string
	client    *http.Client
}

func NewPaystack(secretKey string) Gateway {
	return &paystack{secretKey: secretKey, client: newClient()}
}

func (p *paystack) Name() string { return "paystack" }

func (p *paystack) Initialize(in InitInput) (InitResult, error) {
	body, err := json.Marshal(map[string]any{
		"email":        in.Email,
		"amount":       in.Amount,
		"currency":     upper(in.Currency),
		"reference":    in.Reference,
		"callback_url": in.CallbackURL,
		"metadata":     map[string]string{"buyer_name": in.Name, "buyer_phone": in.Phone, "item": in.Description},
	})
	if err != nil {
		return InitResult{}, err
	}

	req, err := http.NewRequest(http.MethodPost, paystackBaseURL+"/transaction/initialize", bytes.NewReader(body))
	if err != nil {
		return InitResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return InitResult{}, err
	}
	defer resp.Body.Close()

	var out struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AuthorizationURL string `json:"authorization_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return InitResult{}, err
	}
	if !out.Status || out.Data.AuthorizationURL == "" {
		return InitResult{}, fmt.Errorf("paystack: %s", out.Message)
	}
	return InitResult{CheckoutURL: out.Data.AuthorizationURL, GatewayRef: in.Reference}, nil
}

func (p *paystack) Verify(ref string) (Result, error) {
	req, err := http.NewRequest(http.MethodGet, paystackBaseURL+"/transaction/verify/"+url.PathEscape(ref), nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	var out struct {
		Status bool `json:"status"`
		Data   struct {
			Status   string `json:"status"` // success | failed | abandoned | pending ...
			Amount   int64  `json:"amount"`
			Currency string `json:"currency"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, err
	}
	if !out.Status {
		return Result{}, fmt.Errorf("paystack: verification request failed")
	}

	status := StatusPending
	switch out.Data.Status {
	case "success":
		status = StatusSuccess
	case "failed", "reversed":
		status = StatusFailed
	}
	return Result{Status: status, Amount: out.Data.Amount, Currency: upper(out.Data.Currency)}, nil
}

func (p *paystack) ParseWebhook(header http.Header, body []byte) (string, error) {
	mac := hmac.New(sha512.New, []byte(p.secretKey))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(header.Get("x-paystack-signature"))) {
		return "", ErrBadSignature
	}

	var event struct {
		Data struct {
			Reference string `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return "", err
	}
	return event.Data.Reference, nil
}
