package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/config"
	"github.com/iv-tunate/PAS/internal/db/sqlc"
	"github.com/iv-tunate/PAS/internal/mailer"
)

var ErrNoSubscribers = errors.New("there is nobody subscribed yet")

// ensureSubscribed adds an address to the mailing list. An address that is
// already on the list is fine: the unique index turns that into "no row".
func ensureSubscribed(ctx context.Context, q *sqlc.Queries, email string) error {
	_, err := q.CreateSubscriber(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

type NewsletterService struct {
	appCtx  context.Context // cancelled on shutdown, stops a running broadcast cleanly
	queries *sqlc.Queries
	mailer  mailer.Mailer
	outbox  *mailer.Outbox
	cfg     *config.Config
	log     *slog.Logger
}

func NewNewsletterService(appCtx context.Context, pool *pgxpool.Pool, m mailer.Mailer, outbox *mailer.Outbox, cfg *config.Config, log *slog.Logger) *NewsletterService {
	return &NewsletterService{appCtx: appCtx, queries: sqlc.New(pool), mailer: m, outbox: outbox, cfg: cfg, log: log}
}

func (s *NewsletterService) Subscribe(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	if err := ensureSubscribed(ctx, s.queries, email); err != nil {
		return err
	}

	// Mirroring into a Brevo list is optional and must not slow the visitor
	// down, so it runs in the background.
	if listID, err := strconv.Atoi(s.cfg.BrevoListID); err == nil {
		go func() {
			if err := s.mailer.AddContactToList(email, listID); err != nil {
				s.log.Error("brevo list sync failed", "error", err)
				return
			}
			bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.queries.MarkSubscriberSynced(bg, email)
		}()
	}
	return nil
}

// Unsubscribe is silent about whether the token existed, so it cannot be
// used to probe for valid ones.
func (s *NewsletterService) Unsubscribe(ctx context.Context, token string) error {
	return s.queries.DeleteSubscriberByToken(ctx, token)
}

func (s *NewsletterService) ListSubscribers(ctx context.Context, limit, offset int32) ([]sqlc.Subscriber, int64, error) {
	rows, err := s.queries.ListSubscribers(ctx, sqlc.ListSubscribersParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.queries.CountSubscribers(ctx)
	return rows, total, err
}

func (s *NewsletterService) DeleteSubscriber(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteSubscriber(ctx, id)
}

func (s *NewsletterService) ListCampaigns(ctx context.Context) ([]sqlc.NewsletterCampaign, error) {
	return s.queries.ListCampaigns(ctx)
}

// MarkInterrupted is called once at startup. A broadcast that was in the
// middle of sending when the server stopped cannot resume, so say so.
func (s *NewsletterService) MarkInterrupted(ctx context.Context) {
	if err := s.queries.InterruptStaleCampaigns(ctx); err != nil {
		s.log.Error("could not mark interrupted newsletters", "error", err)
	}
}

func (s *NewsletterService) message(subject, body, toEmail, unsubscribeToken string) mailer.Message {
	footer := ""
	if unsubscribeToken != "" {
		link := s.cfg.FrontendURL + "/unsubscribe/" + unsubscribeToken
		footer = emailFooter(`You are receiving this because you subscribed on the Akintola Samuel website. <a href="` + esc(link) + `" style="color:#666">Unsubscribe</a>`)
	}
	return mailer.Message{
		ToEmail: toEmail,
		Subject: oneLine(subject),
		HTML:    emailPage(oneLine(subject), paragraphs(body), footer),
	}
}

// SendTest sends the draft to the admin alone, so they can see it first.
func (s *NewsletterService) SendTest(ctx context.Context, toEmail, subject, body string) error {
	msg := s.message("[Test] "+subject, body, toEmail, "")
	if !s.outbox.TrySend(mailer.Job{Msg: msg}) {
		return errors.New("the email queue is busy, try again in a moment")
	}
	return nil
}

// StartCampaign records the broadcast and starts sending it in the
// background. It returns straight away; the dashboard watches progress by
// reloading the campaign list.
func (s *NewsletterService) StartCampaign(ctx context.Context, subject, body string) (sqlc.NewsletterCampaign, error) {
	recipients, err := s.queries.ListAllSubscribersForSend(ctx)
	if err != nil {
		return sqlc.NewsletterCampaign{}, err
	}
	if len(recipients) == 0 {
		return sqlc.NewsletterCampaign{}, ErrNoSubscribers
	}

	campaign, err := s.queries.CreateCampaign(ctx, sqlc.CreateCampaignParams{
		Subject: subject, Body: body, Total: int32(len(recipients)),
	})
	if err != nil {
		return sqlc.NewsletterCampaign{}, err
	}

	go s.runCampaign(campaign, recipients)
	return campaign, nil
}

// runCampaign feeds one email per subscriber into the outbox and waits for
// every one of them to be reported back, then closes the campaign. Each
// subscriber gets their own message so the unsubscribe link is theirs.
func (s *NewsletterService) runCampaign(c sqlc.NewsletterCampaign, recipients []sqlc.ListAllSubscribersForSendRow) {
	var wg sync.WaitGroup

	record := func(err error) {
		bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err != nil {
			_ = s.queries.IncrementCampaignFailed(bg, c.ID)
		} else {
			_ = s.queries.IncrementCampaignSent(bg, c.ID)
		}
	}

	for _, r := range recipients {
		wg.Add(1)
		queued := s.outbox.Send(s.appCtx, mailer.Job{
			Msg: s.message(c.Subject, c.Body, r.Email, r.UnsubscribeToken),
			OnResult: func(err error) {
				defer wg.Done()
				record(err)
			},
		})
		if !queued {
			wg.Done()
			if s.appCtx.Err() != nil {
				// Shutting down: leave the campaign as "sending"; the next
				// start-up marks it interrupted.
				return
			}
			record(errors.New("could not queue"))
		}
	}

	wg.Wait()
	bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.queries.FinishCampaign(bg, c.ID); err != nil {
		s.log.Error("could not close campaign", "campaign_id", c.ID, "error", err)
	}
}
