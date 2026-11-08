package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Env  string // development | production
	Port string

	DatabaseURL string

	JWTSecret string

	PaystackSecretKey string
	PaystackPublicKey string

	FlutterwaveSecretKey     string
	FlutterwavePublicKey     string
	FlutterwaveWebhookSecret string

	StripeSecretKey      string
	StripePublishableKey string
	StripeWebhookSecret  string

	BrevoAPIKey      string
	BrevoSenderEmail string
	BrevoSenderName  string
	BrevoListID      string

	CloudinaryCloudName string
	CloudinaryAPIKey    string
	CloudinaryAPISecret string

	// FrontendURL is the website (used for CORS and links back to the site).
	// APIPublicURL is where this server is reachable from the internet; the
	// download links in emails point at it.
	FrontendURL  string
	APIPublicURL string

	// TrustProxy: set TRUST_PROXY=true only when the API sits behind a
	// reverse proxy or platform load balancer. It lets the server read the
	// visitor's real address from X-Forwarded-For, which the rate limits use.
	// Left off, that header is ignored, so nobody can fake their address.
	TrustProxy bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	port := getEnv("PORT", "8080")

	cfg := &Config{
		Env:  getEnv("APP_ENV", "development"),
		Port: port,

		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),

		PaystackSecretKey: os.Getenv("PAYSTACK_SECRET_KEY"),
		PaystackPublicKey: os.Getenv("PAYSTACK_PUBLIC_KEY"),

		FlutterwaveSecretKey:     os.Getenv("FLUTTERWAVE_SECRET_KEY"),
		FlutterwavePublicKey:     os.Getenv("FLUTTERWAVE_PUBLIC_KEY"),
		FlutterwaveWebhookSecret: os.Getenv("FLUTTERWAVE_WEBHOOK_SECRET_HASH"),

		StripeSecretKey:      os.Getenv("STRIPE_SECRET_KEY"),
		StripePublishableKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		StripeWebhookSecret:  os.Getenv("STRIPE_WEBHOOK_SECRET"),

		BrevoAPIKey:      os.Getenv("BREVO_API_KEY"),
		BrevoSenderEmail: os.Getenv("BREVO_SENDER_EMAIL"),
		BrevoSenderName:  getEnv("BREVO_SENDER_NAME", "Akintola Samuel"),
		BrevoListID:      os.Getenv("BREVO_LIST_ID"),

		CloudinaryCloudName: os.Getenv("CLOUDINARY_CLOUD_NAME"),
		CloudinaryAPIKey:    os.Getenv("CLOUDINARY_API_KEY"),
		CloudinaryAPISecret: os.Getenv("CLOUDINARY_API_SECRET"),

		FrontendURL:  strings.TrimRight(getEnv("FRONTEND_URL", "http://localhost:3000"), "/"),
		APIPublicURL: strings.TrimRight(getEnv("API_PUBLIC_URL", "http://localhost:"+port), "/"),

		TrustProxy: getEnv("TRUST_PROXY", "false") == "true",
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	// The secret signs every admin session, so a short one is a real risk.
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET is required and must be at least 32 characters")
	}

	return cfg, nil
}

// PaymentsReady reports which gateways have a secret key configured, so the
// website only offers the ones that can actually take a payment.
func (c *Config) PaymentsReady() (paystack, flutterwave, stripe bool) {
	return c.PaystackSecretKey != "", c.FlutterwaveSecretKey != "", c.StripeSecretKey != ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
