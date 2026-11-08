package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/config"
	"github.com/iv-tunate/PAS/internal/db/sqlc"
	"github.com/iv-tunate/PAS/internal/mailer"
	"github.com/iv-tunate/PAS/internal/payment"
)

var (
	ErrItemNotFound   = errors.New("that item could not be found")
	ErrNotAvailable   = errors.New("this item is not available yet")
	ErrNotForSale     = errors.New("this item has no price for that payment method")
	ErrNoGateway      = errors.New("that payment method is not available")
	ErrGatewayFailed  = errors.New("could not start the payment")
	ErrTooManyOrders  = errors.New("too many requests for this email address, please try again later")
	ErrOrderNotFound  = errors.New("order not found")
	ErrNotPaid        = errors.New("this order has not been paid")
	ErrAmountMismatch = errors.New("paid amount does not match the order")
)

// maxOrdersPerEmailPerHour keeps one address from being used to flood
// somebody else's inbox with download emails.
const maxOrdersPerEmailPerHour = 5

// OrderService runs the whole "someone wants a book or a course" flow:
//
//	checkout -> (payment) -> confirm -> email the download link -> download
//
// There are no buyer accounts. Name, email and phone live on the order.
type OrderService struct {
	queries  *sqlc.Queries
	cfg      *config.Config
	log      *slog.Logger
	outbox   *mailer.Outbox
	gateways map[string]payment.Gateway // only the ones that are configured
}

func NewOrderService(pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger, outbox *mailer.Outbox, gateways ...payment.Gateway) *OrderService {
	byName := make(map[string]payment.Gateway, len(gateways))
	for _, g := range gateways {
		byName[g.Name()] = g
	}
	return &OrderService{queries: sqlc.New(pool), cfg: cfg, log: log, outbox: outbox, gateways: byName}
}

// Gateways lists the payment methods that can take a payment right now.
func (s *OrderService) Gateways() []string {
	out := []string{}
	for _, name := range []string{"paystack", "flutterwave", "stripe"} {
		if _, ok := s.gateways[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

// ---------------------------------------------------------------------
// Checkout
// ---------------------------------------------------------------------

type CheckoutInput struct {
	Kind      string // "book" | "course"
	ItemID    uuid.UUID
	Name      string
	Email     string
	Phone     string
	Gateway   string // optional; the first available one with a price is used
	Subscribe bool   // buyer ticked "email me new messages and courses"
}

type CheckoutResult struct {
	Reference   string `json:"reference"`
	Free        bool   `json:"free"`
	CheckoutURL string `json:"checkout_url,omitempty"`
}

// item is what a book and a course have in common as far as selling goes.
type item struct {
	Title     string
	FileURL   string
	IsFree    bool
	PriceKobo int64
	PriceUSD  int64
	Published bool
}

func (s *OrderService) loadItem(ctx context.Context, kind string, id uuid.UUID) (item, error) {
	switch kind {
	case "book":
		b, err := s.queries.GetBookByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return item{}, ErrItemNotFound
		}
		if err != nil {
			return item{}, err
		}
		return item{b.Title, b.FileUrl, b.IsFree, b.PriceKobo, b.PriceUsdCents, b.IsPublished}, nil
	case "course":
		c, err := s.queries.GetCourseByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return item{}, ErrItemNotFound
		}
		if err != nil {
			return item{}, err
		}
		return item{c.Title, c.FileUrl, c.IsFree, c.PriceKobo, c.PriceUsdCents, c.IsPublished}, nil
	}
	return item{}, ErrItemNotFound
}

// priceFor returns the amount and currency a gateway would charge.
// Stripe charges in dollars; Paystack and Flutterwave in naira.
func priceFor(it item, gateway string) (int64, string) {
	if gateway == "stripe" {
		return it.PriceUSD, "USD"
	}
	return it.PriceKobo, "NGN"
}

func (s *OrderService) pickGateway(it item, requested string) (payment.Gateway, error) {
	if requested != "" {
		g, ok := s.gateways[requested]
		if !ok {
			return nil, ErrNoGateway
		}
		if amount, _ := priceFor(it, requested); amount <= 0 {
			return nil, ErrNotForSale
		}
		return g, nil
	}
	for _, name := range s.Gateways() {
		if amount, _ := priceFor(it, name); amount > 0 {
			return s.gateways[name], nil
		}
	}
	if len(s.gateways) == 0 {
		return nil, ErrNoGateway
	}
	return nil, ErrNotForSale
}

func (s *OrderService) Checkout(ctx context.Context, in CheckoutInput) (CheckoutResult, error) {
	it, err := s.loadItem(ctx, in.Kind, in.ItemID)
	if err != nil {
		return CheckoutResult{}, err
	}
	// Nothing is sold, or given away, until a file is attached for delivery.
	if !it.Published || it.FileURL == "" {
		return CheckoutResult{}, ErrNotAvailable
	}

	email := normalizeEmail(in.Email)
	recent, err := s.queries.CountRecentOrdersByEmail(ctx, email)
	if err != nil {
		return CheckoutResult{}, err
	}
	if recent >= maxOrdersPerEmailPerHour {
		return CheckoutResult{}, ErrTooManyOrders
	}

	token, err := randomToken(24)
	if err != nil {
		return CheckoutResult{}, err
	}
	reference := "ord_" + uuid.NewString()

	base := sqlc.CreateOrderParams{
		Kind:          in.Kind,
		ItemID:        in.ItemID,
		ItemTitle:     it.Title,
		BuyerName:     oneLine(in.Name),
		BuyerEmail:    email,
		BuyerPhone:    oneLine(in.Phone),
		Reference:     reference,
		DownloadToken: token,
	}

	if in.Subscribe {
		if err := ensureSubscribed(ctx, s.queries, email); err != nil {
			s.log.Warn("could not add buyer to the newsletter", "error", err)
		}
	}

	// Free: no payment step. Record who asked and email the link.
	if it.IsFree {
		base.Gateway, base.Amount, base.Currency, base.Status = "free", 0, "NGN", "success"
		order, err := s.queries.CreateOrder(ctx, base)
		if err != nil {
			return CheckoutResult{}, err
		}
		s.queueDelivery(ctx, order)
		return CheckoutResult{Reference: reference, Free: true}, nil
	}

	gw, err := s.pickGateway(it, in.Gateway)
	if err != nil {
		return CheckoutResult{}, err
	}
	amount, currency := priceFor(it, gw.Name())
	base.Gateway, base.Amount, base.Currency, base.Status = gw.Name(), amount, currency, "pending"

	order, err := s.queries.CreateOrder(ctx, base)
	if err != nil {
		return CheckoutResult{}, err
	}

	res, err := gw.Initialize(payment.InitInput{
		Reference:   reference,
		Email:       email,
		Name:        order.BuyerName,
		Phone:       order.BuyerPhone,
		Description: it.Title,
		Amount:      amount,
		Currency:    currency,
		CallbackURL: s.cfg.FrontendURL + "/checkout/complete?ref=" + reference,
	})
	if err != nil {
		s.log.Error("could not start payment", "gateway", gw.Name(), "reference", reference, "error", err)
		_ = s.queries.MarkOrderFailed(ctx, reference)
		return CheckoutResult{}, ErrGatewayFailed
	}
	if err := s.queries.SetOrderGatewayRef(ctx, sqlc.SetOrderGatewayRefParams{ID: order.ID, GatewayRef: res.GatewayRef}); err != nil {
		s.log.Error("could not save gateway reference", "reference", reference, "error", err)
	}

	return CheckoutResult{Reference: reference, CheckoutURL: res.CheckoutURL}, nil
}

// ---------------------------------------------------------------------
// Confirming a payment
// ---------------------------------------------------------------------

// Confirm asks the payment provider what happened to an order and updates it.
// It is safe to call as often as you like: the webhook, the buyer's return
// page and the background sweeper may all call it for the same order, and
// only the first one that sees "paid" triggers the delivery email.
func (s *OrderService) Confirm(ctx context.Context, reference string) (sqlc.Order, error) {
	order, err := s.queries.GetOrderByReference(ctx, reference)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Order{}, ErrOrderNotFound
	}
	if err != nil {
		return sqlc.Order{}, err
	}
	if order.Status == "success" || order.Gateway == "free" {
		return order, nil
	}

	gw, ok := s.gateways[order.Gateway]
	if !ok {
		return order, nil
	}
	ref := order.GatewayRef
	if ref == "" {
		ref = order.Reference
	}

	res, err := gw.Verify(ref)
	if err != nil {
		return order, err
	}

	switch res.Status {
	case payment.StatusSuccess:
		// Never trust "paid" on its own: check it was paid in full.
		if res.Amount < order.Amount || !strings.EqualFold(res.Currency, order.Currency) {
			s.log.Error("payment does not match order", "reference", reference,
				"expected", order.Amount, "expected_currency", order.Currency,
				"paid", res.Amount, "paid_currency", res.Currency)
			return order, ErrAmountMismatch
		}
		paid, err := s.queries.MarkOrderSuccess(ctx, reference)
		if errors.Is(err, pgx.ErrNoRows) {
			break // another caller got there first
		}
		if err != nil {
			return order, err
		}
		s.queueDelivery(ctx, paid)
	case payment.StatusFailed:
		_ = s.queries.MarkOrderFailed(ctx, reference)
	default:
		_ = s.queries.TouchOrder(ctx, order.ID)
	}

	return s.queries.GetOrderByReference(ctx, reference)
}

// HandleWebhook verifies a provider's signature, then confirms the order it
// names. A bad signature is an error; an unknown order is quietly ignored so
// the provider stops retrying.
func (s *OrderService) HandleWebhook(ctx context.Context, gateway string, header http.Header, body []byte) error {
	gw, ok := s.gateways[gateway]
	if !ok {
		return ErrNoGateway
	}
	reference, err := gw.ParseWebhook(header, body)
	if err != nil {
		return err
	}
	if reference == "" {
		return nil
	}
	if _, err := s.Confirm(ctx, reference); err != nil && !errors.Is(err, ErrOrderNotFound) {
		s.log.Error("webhook could not confirm order", "gateway", gateway, "reference", reference, "error", err)
	}
	return nil
}

type OrderStatus struct {
	Status    string `json:"status"` // pending | success | failed
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Free      bool   `json:"free"`
	Delivered bool   `json:"delivered"`
}

// Status is what the "thank you" page shows. It re-checks with the provider
// while the order is still pending, so the page updates without a webhook.
func (s *OrderService) Status(ctx context.Context, reference string) (OrderStatus, error) {
	order, err := s.Confirm(ctx, reference)
	if err != nil && !errors.Is(err, ErrAmountMismatch) {
		// A provider hiccup should not break the page: show the order as it is.
		existing, getErr := s.queries.GetOrderByReference(ctx, reference)
		if getErr != nil {
			return OrderStatus{}, ErrOrderNotFound
		}
		order = existing
	}
	return OrderStatus{
		Status:    order.Status,
		Kind:      order.Kind,
		Title:     order.ItemTitle,
		Free:      order.Gateway == "free",
		Delivered: order.DeliveredAt.Valid,
	}, nil
}

// ---------------------------------------------------------------------
// Delivery
// ---------------------------------------------------------------------

// queueDelivery builds the download email and hands it to the outbox. It
// never blocks: if the queue is full the order simply stays undelivered and
// the sweeper tries again in a few minutes.
func (s *OrderService) queueDelivery(ctx context.Context, order sqlc.Order) {
	// The title on the order is a snapshot, but if the item has since been
	// deleted there is nothing left to download, so do not send a dead link.
	if _, err := s.loadItem(ctx, order.Kind, order.ItemID); err != nil {
		s.log.Error("could not prepare delivery email", "order_id", order.ID, "error", err)
		return
	}

	link := s.cfg.APIPublicURL + "/api/v1/download/" + order.DownloadToken
	what := "book"
	if order.Kind == "course" {
		what = "course"
	}

	body := `<p style="margin:0 0 16px;line-height:1.65">Hi ` + esc(firstName(order.BuyerName)) + `,</p>` +
		`<p style="margin:0 0 16px;line-height:1.65">Thank you. Your ` + what + `, <strong>` + esc(order.ItemTitle) + `</strong>, is ready.</p>` +
		emailButton(link, "Download your "+what) +
		`<p style="margin:0;line-height:1.65;font-size:14px;color:#555">This link is for you. Please do not share it. If the button does not work, copy this address into your browser:<br>` + esc(link) + `</p>`

	msg := mailer.Message{
		ToEmail: order.BuyerEmail,
		ToName:  order.BuyerName,
		Subject: "Your " + what + " is ready: " + oneLine(order.ItemTitle),
		HTML:    emailPage("Your "+what+" is ready", body, ""),
	}

	if err := s.queries.BumpDeliveryAttempt(ctx, order.ID); err != nil {
		s.log.Error("could not record delivery attempt", "order_id", order.ID, "error", err)
	}

	orderID := order.ID
	queued := s.outbox.TrySend(mailer.Job{
		Msg: msg,
		OnResult: func(err error) {
			if err != nil {
				return // the sweeper will try again, up to a few times
			}
			bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.queries.MarkOrderDelivered(bg, orderID); err != nil {
				s.log.Error("email sent but could not be recorded", "order_id", orderID, "error", err)
			}
		},
	})
	if !queued {
		s.log.Warn("email queue is full; order will be retried by the sweeper", "order_id", order.ID)
	}
}

// Resend lets an admin push the download email out again.
func (s *OrderService) Resend(ctx context.Context, id uuid.UUID) error {
	order, err := s.queries.GetOrderByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return err
	}
	if order.Status != "success" {
		return ErrNotPaid
	}
	s.queueDelivery(ctx, order)
	return nil
}

// Download turns the secret token from an email into the real file address.
// The file address itself is never shown anywhere on the website.
func (s *OrderService) Download(ctx context.Context, token string) (string, error) {
	order, err := s.queries.GetPaidOrderByDownloadToken(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOrderNotFound
	}
	if err != nil {
		return "", err
	}
	it, err := s.loadItem(ctx, order.Kind, order.ItemID)
	if err != nil {
		return "", err
	}
	if it.FileURL == "" {
		return "", ErrNotAvailable
	}
	if err := s.queries.IncrementDownloadCount(ctx, order.ID); err != nil {
		s.log.Warn("could not count download", "order_id", order.ID, "error", err)
	}
	return forceDownloadURL(it.FileURL), nil
}

// forceDownloadURL asks Cloudinary to send the file as a download instead of
// playing it in the browser. Other hosts' links are left alone.
func forceDownloadURL(u string) string {
	if !strings.Contains(u, "res.cloudinary.com") || !strings.Contains(u, "/upload/") || strings.Contains(u, "fl_attachment") {
		return u
	}
	return strings.Replace(u, "/upload/", "/upload/fl_attachment/", 1)
}

// RunSweeper is the safety net. Every few minutes it
//  1. re-checks payments that were started but never confirmed (a missed
//     webhook, a closed tab), and
//  2. re-sends delivery emails that did not go out (Brevo down, restart).
//
// It stops when ctx is cancelled.
func (s *OrderService) RunSweeper(ctx context.Context) {
	s.sweep(ctx)
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep(ctx)
		}
	}
}

func (s *OrderService) sweep(ctx context.Context) {
	pending, err := s.queries.ListStalePendingOrders(ctx)
	if err != nil {
		s.log.Error("sweeper: could not list pending orders", "error", err)
	}
	for _, o := range pending {
		if _, err := s.Confirm(ctx, o.Reference); err != nil {
			s.log.Warn("sweeper: could not confirm order", "reference", o.Reference, "error", err)
		}
	}

	undelivered, err := s.queries.ListOrdersAwaitingDelivery(ctx)
	if err != nil {
		s.log.Error("sweeper: could not list undelivered orders", "error", err)
		return
	}
	for _, o := range undelivered {
		s.queueDelivery(ctx, o)
	}
	if len(undelivered) > 0 {
		s.log.Info("sweeper: re-queued delivery emails", "count", len(undelivered))
	}
}

// ---------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------

// AdminOrder is an order as the dashboard sees it. The secret download token
// is left out on purpose.
type AdminOrder struct {
	ID            uuid.UUID `json:"id"`
	Kind          string    `json:"kind"`
	ItemTitle     string    `json:"item_title"`
	BuyerName     string    `json:"buyer_name"`
	BuyerEmail    string    `json:"buyer_email"`
	BuyerPhone    string    `json:"buyer_phone"`
	Gateway       string    `json:"gateway"`
	Reference     string    `json:"reference"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	DownloadCount int32     `json:"download_count"`
	Delivered     bool      `json:"delivered"`
	CreatedAt     time.Time `json:"created_at"`
}

func (s *OrderService) ListForAdmin(ctx context.Context, status string, limit, offset int32) ([]AdminOrder, int64, error) {
	rows, err := s.queries.ListOrders(ctx, sqlc.ListOrdersParams{Status: status, PageSize: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.queries.CountOrders(ctx, status)
	if err != nil {
		return nil, 0, err
	}
	out := make([]AdminOrder, 0, len(rows))
	for _, o := range rows {
		out = append(out, AdminOrder{
			ID: o.ID, Kind: o.Kind, ItemTitle: o.ItemTitle,
			BuyerName: o.BuyerName, BuyerEmail: o.BuyerEmail, BuyerPhone: o.BuyerPhone,
			Gateway: o.Gateway, Reference: o.Reference, Amount: o.Amount, Currency: o.Currency,
			Status: o.Status, DownloadCount: o.DownloadCount, Delivered: o.DeliveredAt.Valid,
			CreatedAt: o.CreatedAt.Time,
		})
	}
	return out, total, nil
}
