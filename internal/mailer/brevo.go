package mailer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Message is one email. ReplyTo is optional; the contact form uses it so
// "reply" in the admin's inbox goes straight to the person who wrote.
type Message struct {
	ToEmail     string
	ToName      string
	Subject     string
	HTML        string
	ReplyToMail string
	ReplyToName string
}

type Mailer interface {
	Send(msg Message) error
	AddContactToList(email string, listID int) error
}

type brevoMailer struct {
	apiKey      string
	senderEmail string
	senderName  string
	client      *http.Client
}

func NewBrevoMailer(apiKey, senderEmail, senderName string) Mailer {
	return &brevoMailer{
		apiKey:      apiKey,
		senderEmail: senderEmail,
		senderName:  senderName,
		client:      &http.Client{Timeout: 20 * time.Second},
	}
}

const brevoBaseURL = "https://api.brevo.com/v3"

type sendEmailRequest struct {
	Sender      contact   `json:"sender"`
	To          []contact `json:"to"`
	ReplyTo     *contact  `json:"replyTo,omitempty"`
	Subject     string    `json:"subject"`
	HTMLContent string    `json:"htmlContent"`
}

type contact struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

func (b *brevoMailer) Send(msg Message) error {
	if b.apiKey == "" || b.senderEmail == "" {
		return fmt.Errorf("brevo is not configured (BREVO_API_KEY / BREVO_SENDER_EMAIL)")
	}

	payload := sendEmailRequest{
		Sender:      contact{Email: b.senderEmail, Name: b.senderName},
		To:          []contact{{Email: msg.ToEmail, Name: msg.ToName}},
		Subject:     msg.Subject,
		HTMLContent: msg.HTML,
	}
	if msg.ReplyToMail != "" {
		payload.ReplyTo = &contact{Email: msg.ReplyToMail, Name: msg.ReplyToName}
	}
	return b.post("/smtp/email", payload)
}

type addContactRequest struct {
	Email         string `json:"email"`
	ListIDs       []int  `json:"listIds"`
	UpdateEnabled bool   `json:"updateEnabled"`
}

func (b *brevoMailer) AddContactToList(email string, listID int) error {
	if b.apiKey == "" {
		return fmt.Errorf("brevo is not configured (BREVO_API_KEY)")
	}
	return b.post("/contacts", addContactRequest{
		Email:         email,
		ListIDs:       []int{listID},
		UpdateEnabled: true,
	})
}

func (b *brevoMailer) post(path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, brevoBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", b.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		// Brevo explains what went wrong in the body; keep a short slice of it.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return &StatusError{Path: path, Code: resp.StatusCode, Detail: string(detail)}
	}
	return nil
}

// StatusError is a non-2xx answer from Brevo.
type StatusError struct {
	Path   string
	Code   int
	Detail string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("brevo: %s failed with status %d: %s", e.Path, e.Code, e.Detail)
}
