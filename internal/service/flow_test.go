package service

// These tests run the real services against a real Postgres, with a fake
// mailer and a fake payment gateway standing in for Brevo and Paystack.
//
//	createdb pas_test
//	(run every migration in internal/db/migrations against it)
//	TEST_DATABASE_URL="postgres://localhost/pas_test?sslmode=disable" go test ./internal/service/
//
// WARNING: these tests empty orders, books, courses, subscribers, users and a
// few other tables first. Point them at a throwaway database, never a real one.
//
// Without TEST_DATABASE_URL the database tests are skipped.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/config"
	"github.com/iv-tunate/PAS/internal/db/sqlc"
	"github.com/iv-tunate/PAS/internal/mailer"
	"github.com/iv-tunate/PAS/internal/payment"
)

// ---- fakes ----------------------------------------------------------

type fakeMailer struct {
	mu   sync.Mutex
	sent []mailer.Message
}

func (f *fakeMailer) Send(m mailer.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}
func (f *fakeMailer) AddContactToList(string, int) error { return nil }

func (f *fakeMailer) to(email string) []mailer.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []mailer.Message
	for _, m := range f.sent {
		if m.ToEmail == email {
			out = append(out, m)
		}
	}
	return out
}

type fakeGateway struct {
	result payment.Result
}

func (g *fakeGateway) Name() string { return "paystack" }
func (g *fakeGateway) Initialize(in payment.InitInput) (payment.InitResult, error) {
	return payment.InitResult{CheckoutURL: "https://pay.example/" + in.Reference, GatewayRef: in.Reference}, nil
}
func (g *fakeGateway) Verify(string) (payment.Result, error) { return g.result, nil }
func (g *fakeGateway) ParseWebhook(http.Header, []byte) (string, error) {
	return "", payment.ErrBadSignature
}

// ---- helpers --------------------------------------------------------

type rig struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *sqlc.Queries
	mail    *fakeMailer
	outbox  *mailer.Outbox
	gateway *fakeGateway
	orders  *OrderService
	cfg     *config.Config
}

func newRig(t *testing.T) *rig {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{FrontendURL: "http://site.test", APIPublicURL: "http://api.test", JWTSecret: strings.Repeat("x", 40)}
	fm := &fakeMailer{}
	outbox := mailer.NewOutbox(fm, log, 2, 100)
	t.Cleanup(func() { outbox.Close(ctx) })
	gw := &fakeGateway{}

	for _, tbl := range []string{"orders", "books", "courses", "subscribers", "newsletter_campaigns", "contact_messages", "refresh_tokens", "users"} {
		if _, err := pool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatal(err)
		}
	}
	return &rig{ctx: ctx, pool: pool, q: sqlc.New(pool), mail: fm, outbox: outbox, gateway: gw,
		orders: NewOrderService(pool, cfg, log, outbox, gw), cfg: cfg}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

var tokenInLink = regexp.MustCompile(`/api/v1/download/([a-f0-9]+)`)

func (r *rig) freeBook(t *testing.T, file string) sqlc.Book {
	t.Helper()
	b, err := r.q.CreateBook(r.ctx, sqlc.CreateBookParams{
		Title: "Free Book", Author: "A", FileUrl: file, IsFree: true, IsPublished: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ---- tests ----------------------------------------------------------

func TestFreeBookIsEmailedAndDownloads(t *testing.T) {
	r := newRig(t)
	book := r.freeBook(t, "https://res.cloudinary.com/demo/raw/upload/v1/books/b.pdf")

	res, err := r.orders.Checkout(r.ctx, CheckoutInput{
		Kind: "book", ItemID: book.ID, Name: "Ada Obi", Email: "Ada@Example.com", Phone: "0801 234 5678", Subscribe: true,
	})
	if err != nil || !res.Free {
		t.Fatalf("checkout: %+v %v", res, err)
	}

	eventually(t, "delivery email", func() bool { return len(r.mail.to("ada@example.com")) == 1 })
	msg := r.mail.to("ada@example.com")[0]
	m := tokenInLink.FindStringSubmatch(msg.HTML)
	if m == nil {
		t.Fatalf("no download link in email: %s", msg.HTML)
	}

	url, err := r.orders.Download(r.ctx, m[1])
	if err != nil || !strings.Contains(url, "/upload/fl_attachment/") {
		t.Fatalf("download: %q %v", url, err)
	}
	if _, err := r.orders.Download(r.ctx, "nope"); err == nil {
		t.Fatal("a made-up token must not download anything")
	}

	// recorded as delivered, and the buyer was put on the newsletter
	eventually(t, "delivered_at", func() bool {
		o, _ := r.q.GetOrderByReference(r.ctx, res.Reference)
		return o.DeliveredAt.Valid
	})
	if n, _ := r.q.CountSubscribers(r.ctx); n != 1 {
		t.Fatalf("expected 1 subscriber, got %d", n)
	}
}

func TestBuyerNameCannotInjectHTML(t *testing.T) {
	r := newRig(t)
	book := r.freeBook(t, "https://example.com/f.pdf")
	_, err := r.orders.Checkout(r.ctx, CheckoutInput{
		Kind: "book", ItemID: book.ID, Name: `<script>alert(1)</script>`, Email: "x@example.com", Phone: "08012345678",
	})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "email", func() bool { return len(r.mail.to("x@example.com")) == 1 })
	if strings.Contains(r.mail.to("x@example.com")[0].HTML, "<script>") {
		t.Fatal("name was not escaped in the email")
	}
}

func TestNothingIsSoldWithoutAFile(t *testing.T) {
	r := newRig(t)
	book := r.freeBook(t, "")
	_, err := r.orders.Checkout(r.ctx, CheckoutInput{Kind: "book", ItemID: book.ID, Name: "Ada", Email: "a@example.com", Phone: "08012345678"})
	if err != ErrNotAvailable {
		t.Fatalf("want ErrNotAvailable, got %v", err)
	}
}

func TestPaidCourseDeliversOnceEvenIfConfirmedTwice(t *testing.T) {
	r := newRig(t)
	course, err := r.q.CreateCourse(r.ctx, sqlc.CreateCourseParams{
		Title: "Paid Course", Slug: "paid-course", MediaKind: "video",
		FileUrl: "https://res.cloudinary.com/demo/video/upload/v1/c.mp4", PriceKobo: 500000, IsPublished: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := r.orders.Checkout(r.ctx, CheckoutInput{Kind: "course", ItemID: course.ID, Name: "Tunde", Email: "tunde@example.com", Phone: "+234 801 234 5678"})
	if err != nil || res.Free || !strings.HasPrefix(res.CheckoutURL, "https://pay.example/") {
		t.Fatalf("checkout: %+v %v", res, err)
	}
	if len(r.mail.to("tunde@example.com")) != 0 {
		t.Fatal("nothing may be delivered before payment")
	}

	// Underpaid: must not deliver.
	r.gateway.result = payment.Result{Status: payment.StatusSuccess, Amount: 1000, Currency: "NGN"}
	if _, err := r.orders.Confirm(r.ctx, res.Reference); err != ErrAmountMismatch {
		t.Fatalf("want ErrAmountMismatch, got %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if len(r.mail.to("tunde@example.com")) != 0 {
		t.Fatal("an underpaid order was delivered")
	}

	// Paid in full, confirmed from several places at once.
	r.gateway.result = payment.Result{Status: payment.StatusSuccess, Amount: 500000, Currency: "NGN"}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = r.orders.Confirm(r.ctx, res.Reference) }()
	}
	wg.Wait()
	eventually(t, "one email", func() bool { return len(r.mail.to("tunde@example.com")) >= 1 })
	time.Sleep(150 * time.Millisecond)
	if n := len(r.mail.to("tunde@example.com")); n != 1 {
		t.Fatalf("expected exactly one delivery email, got %d", n)
	}
}

func TestStripeUsesDollarPrice(t *testing.T) {
	it := item{PriceKobo: 500000, PriceUSD: 700}
	if a, c := priceFor(it, "stripe"); a != 700 || c != "USD" {
		t.Fatalf("stripe price: %d %s", a, c)
	}
	if a, c := priceFor(it, "paystack"); a != 500000 || c != "NGN" {
		t.Fatalf("paystack price: %d %s", a, c)
	}
}

func TestOneEmailAddressCannotFloodInbox(t *testing.T) {
	r := newRig(t)
	book := r.freeBook(t, "https://example.com/f.pdf")
	var last error
	for i := 0; i < maxOrdersPerEmailPerHour+1; i++ {
		_, last = r.orders.Checkout(r.ctx, CheckoutInput{Kind: "book", ItemID: book.ID, Name: "Spam", Email: "victim@example.com", Phone: "08012345678"})
	}
	if last != ErrTooManyOrders {
		t.Fatalf("want ErrTooManyOrders, got %v", last)
	}
}

func TestSweeperResendsUndeliveredOrders(t *testing.T) {
	r := newRig(t)
	book := r.freeBook(t, "https://example.com/f.pdf")
	// An order that is paid but whose email never went out, a few minutes ago.
	o, err := r.q.CreateOrder(r.ctx, sqlc.CreateOrderParams{
		Kind: "book", ItemID: book.ID, ItemTitle: book.Title, BuyerName: "Late", BuyerEmail: "late@example.com",
		BuyerPhone: "08012345678", Gateway: "free", Reference: "ord_" + uuid.NewString(), Currency: "NGN",
		Status: "success", DownloadToken: "tok" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(r.ctx, "UPDATE orders SET updated_at = now() - interval '10 minutes' WHERE id = $1", o.ID); err != nil {
		t.Fatal(err)
	}
	r.orders.sweep(r.ctx)
	eventually(t, "sweeper email", func() bool { return len(r.mail.to("late@example.com")) == 1 })
}

func TestNewsletterSendsOneEmailPerSubscriberWithTheirOwnUnsubscribeLink(t *testing.T) {
	r := newRig(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	news := NewNewsletterService(r.ctx, r.pool, r.mail, r.outbox, r.cfg, log)

	if _, err := news.StartCampaign(r.ctx, "Hello", "Body"); err != ErrNoSubscribers {
		t.Fatalf("want ErrNoSubscribers, got %v", err)
	}

	for _, e := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		if err := news.Subscribe(r.ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := news.Subscribe(r.ctx, "A@example.com"); err != nil {
		t.Fatalf("subscribing twice must not fail: %v", err)
	}

	c, err := news.StartCampaign(r.ctx, "Hello friends", "Line one\n\nSee https://example.com/x?a=1&b=2")
	if err != nil || c.Total != 3 {
		t.Fatalf("campaign: %+v %v", c, err)
	}
	eventually(t, "campaign done", func() bool {
		list, _ := news.ListCampaigns(r.ctx)
		return len(list) == 1 && list[0].Status == "done" && list[0].Sent == 3
	})

	links := map[string]bool{}
	for _, e := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		msgs := r.mail.to(e)
		if len(msgs) != 1 {
			t.Fatalf("%s got %d emails", e, len(msgs))
		}
		m := regexp.MustCompile(`/unsubscribe/([a-f0-9]+)`).FindStringSubmatch(msgs[0].HTML)
		if m == nil {
			t.Fatalf("no unsubscribe link for %s", e)
		}
		links[m[1]] = true
	}
	if len(links) != 3 {
		t.Fatal("unsubscribe links must be unique per subscriber")
	}

	// unsubscribe by token removes only that person
	for tok := range links {
		_ = news.Unsubscribe(r.ctx, tok)
		break
	}
	if n, _ := r.q.CountSubscribers(r.ctx); n != 2 {
		t.Fatalf("expected 2 subscribers left, got %d", n)
	}
}

func TestOnlyAdminsCanLogInAndTheLastAdminStays(t *testing.T) {
	r := newRig(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := NewAuthService(r.pool, r.cfg, log, r.outbox)

	// A leftover non-admin account from the old sign-up flow cannot log in.
	if _, err := r.q.CreateUser(r.ctx, sqlc.CreateUserParams{FullName: "Old Student", Email: "old@example.com", PasswordHash: "$2a$12$abcdefghijklmnopqrstuuF3PfC.0lG6pP5bF3N7zQeYhXq3zPhGe", Role: "student"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(r.ctx, "old@example.com", "whatever12345"); err != ErrInvalidCredentials {
		t.Fatalf("student login must fail, got %v", err)
	}

	admin, err := auth.CreateAdmin(r.ctx, "", "Boss@Example.com", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if admin.Email != "boss@example.com" || admin.FullName != "boss" || admin.Role != "admin" {
		t.Fatalf("unexpected admin: %+v", admin)
	}
	if _, err := auth.CreateAdmin(r.ctx, "", "boss@example.com", "correct horse battery"); err != ErrEmailTaken {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
	if _, _, err := auth.Login(r.ctx, "boss@example.com", "wrong password!!"); err != ErrInvalidCredentials {
		t.Fatalf("wrong password must fail, got %v", err)
	}
	user, tokens, err := auth.Login(r.ctx, "boss@example.com", "correct horse battery")
	if err != nil || tokens.AccessToken == "" || user.Role != "admin" {
		t.Fatalf("admin login: %v", err)
	}

	if err := auth.DeleteAdmin(r.ctx, admin.ID, admin.ID); err != ErrCannotDeleteSelf {
		t.Fatalf("want ErrCannotDeleteSelf, got %v", err)
	}
	other, _ := auth.CreateAdmin(r.ctx, "Second", "second@example.com", "another long password")
	if err := auth.DeleteAdmin(r.ctx, admin.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if err := auth.DeleteAdmin(r.ctx, uuid.New(), admin.ID); err != ErrLastAdmin {
		t.Fatalf("want ErrLastAdmin, got %v", err)
	}
}

func TestContactMessageIsSavedAndAdminsAreEmailedWithReplyTo(t *testing.T) {
	r := newRig(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := NewAuthService(r.pool, r.cfg, log, r.outbox)
	if _, err := auth.CreateAdmin(r.ctx, "Pastor", "pastor@example.com", "correct horse battery"); err != nil {
		t.Fatal(err)
	}

	contact := NewContactService(r.pool, r.outbox, log)
	err := contact.Create(r.ctx, ContactInput{
		Topic: "appointment", Name: "Ngozi <b>", Email: "ngozi@example.com", Phone: "08012345678",
		PreferredTime: "2026-11-02 at 15:00", Body: "Please may we meet?",
	})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "notification", func() bool { return len(r.mail.to("pastor@example.com")) == 1 })
	m := r.mail.to("pastor@example.com")[0]
	if m.ReplyToMail != "ngozi@example.com" || strings.Contains(m.HTML, "<b>") {
		t.Fatalf("reply-to / escaping wrong: %+v", m)
	}
	if n, _ := r.q.CountUnreadContactMessages(r.ctx); n != 1 {
		t.Fatalf("expected 1 unread message, got %d", n)
	}
}

func TestEventsAreSeeded(t *testing.T) {
	r := newRig(t)
	events, err := NewEventService(r.pool).ListPublished(r.ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range events {
		names = append(names, fmt.Sprintf("%s %d/%d-%d", e.Name, e.Month, e.DayStart, e.DayEnd))
	}
	t.Log(names)
	if len(events) < 4 {
		t.Fatalf("expected the seeded events, got %v", names)
	}
}

func TestContentRejectsUnsafeLinks(t *testing.T) {
	r := newRig(t)
	c := NewContentService(r.pool)
	if err := c.Set(r.ctx, "social.instagram", "javascript:alert(1)"); err != ErrContentNotLink {
		t.Fatalf("want ErrContentNotLink, got %v", err)
	}
	if err := c.Set(r.ctx, "social.instagram", "https://www.instagram.com/akintolasamuel/"); err != nil {
		t.Fatal(err)
	}
	if err := c.Set(r.ctx, "social.instagram", ""); err != nil { // reset
		t.Fatal(err)
	}
	if err := c.Set(r.ctx, "not.a.key", "x"); err != ErrUnknownContentKey {
		t.Fatalf("want ErrUnknownContentKey, got %v", err)
	}
}

// ---- no database needed ----------------------------------------------

func TestParseYouTubeID(t *testing.T) {
	good := map[string]string{
		"https://www.youtube.com/watch?v=aRnOuC8nSRc":            "aRnOuC8nSRc",
		"https://youtu.be/aRnOuC8nSRc?si=abc":                    "aRnOuC8nSRc",
		"https://m.youtube.com/watch?v=aRnOuC8nSRc&t=30s":        "aRnOuC8nSRc",
		"https://www.youtube.com/embed/aRnOuC8nSRc":              "aRnOuC8nSRc",
		"https://www.youtube.com/shorts/aRnOuC8nSRc":             "aRnOuC8nSRc",
		"https://www.youtube.com/live/aRnOuC8nSRc?feature=share": "aRnOuC8nSRc",
		"aRnOuC8nSRc": "aRnOuC8nSRc",
	}
	for in, want := range good {
		if got, err := ParseYouTubeID(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "https://example.com/watch?v=aRnOuC8nSRc", "https://youtube.com.evil.com/watch?v=aRnOuC8nSRc", "javascript:alert(1)", "https://www.youtube.com/watch?v=short"} {
		if _, err := ParseYouTubeID(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestParagraphsEscapesAndLinks(t *testing.T) {
	out := paragraphs("Hi <b>there</b>\n\nVisit https://example.com/a?x=1&y=2 now")
	if strings.Contains(out, "<b>") {
		t.Fatal("html was not escaped")
	}
	if !strings.Contains(out, `<a href="https://example.com/a?x=1&amp;y=2"`) {
		t.Fatalf("link not made clickable: %s", out)
	}
	if strings.Count(out, "<p ") != 2 {
		t.Fatalf("expected two paragraphs: %s", out)
	}
}

func TestForceDownloadURL(t *testing.T) {
	if got := forceDownloadURL("https://res.cloudinary.com/x/raw/upload/v1/a.pdf"); got != "https://res.cloudinary.com/x/raw/upload/fl_attachment/v1/a.pdf" {
		t.Fatal(got)
	}
	if got := forceDownloadURL("https://example.com/upload/a.pdf"); got != "https://example.com/upload/a.pdf" {
		t.Fatal("non-cloudinary links must be left alone:", got)
	}
}
