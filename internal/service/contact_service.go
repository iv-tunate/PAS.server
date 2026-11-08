package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/db/sqlc"
	"github.com/iv-tunate/PAS/internal/mailer"
)

// ContactService handles the "write to Pastor Akin / book an appointment"
// form. Each message is saved (so nothing is lost if email hiccups) and then
// emailed to every admin, with Reply-To set to the sender.
type ContactService struct {
	queries *sqlc.Queries
	outbox  *mailer.Outbox
	log     *slog.Logger
}

func NewContactService(pool *pgxpool.Pool, outbox *mailer.Outbox, log *slog.Logger) *ContactService {
	return &ContactService{queries: sqlc.New(pool), outbox: outbox, log: log}
}

type ContactInput struct {
	Topic         string // message | appointment | prayer
	Name          string
	Email         string
	Phone         string
	PreferredTime string
	Body          string
}

var topicLabels = map[string]string{
	"message":     "Message",
	"appointment": "Appointment request",
	"prayer":      "Prayer or personal request",
}

func ValidTopic(t string) bool { _, ok := topicLabels[t]; return ok }

func (s *ContactService) Create(ctx context.Context, in ContactInput) error {
	msg, err := s.queries.CreateContactMessage(ctx, sqlc.CreateContactMessageParams{
		Topic: in.Topic, Name: oneLine(in.Name), Email: normalizeEmail(in.Email),
		Phone: oneLine(in.Phone), PreferredTime: oneLine(in.PreferredTime), Body: in.Body,
	})
	if err != nil {
		return err
	}
	s.notifyAdmins(ctx, msg)
	return nil
}

func (s *ContactService) notifyAdmins(ctx context.Context, m sqlc.ContactMessage) {
	admins, err := s.queries.ListAdmins(ctx)
	if err != nil {
		s.log.Error("could not load admins to notify", "error", err)
		return
	}

	label := topicLabels[m.Topic]
	rows := `<p style="margin:0 0 4px"><strong>` + esc(m.Name) + `</strong></p>` +
		`<p style="margin:0 0 4px">` + esc(m.Email) + `</p>`
	if m.Phone != "" {
		rows += `<p style="margin:0 0 4px">` + esc(m.Phone) + `</p>`
	}
	if m.PreferredTime != "" {
		rows += `<p style="margin:12px 0 0"><strong>Preferred time:</strong> ` + esc(m.PreferredTime) + `</p>`
	}
	body := rows + `<div style="margin-top:20px">` + paragraphs(m.Body) + `</div>` +
		`<p style="margin:20px 0 0;font-size:14px;color:#555">Reply to this email to answer ` + esc(firstName(m.Name)) + ` directly. It is also saved in the dashboard under Inbox.</p>`

	for _, a := range admins {
		if !s.outbox.TrySend(mailer.Job{Msg: mailer.Message{
			ToEmail:     a.Email,
			ToName:      a.FullName,
			Subject:     label + " from " + oneLine(m.Name),
			HTML:        emailPage(label, body, ""),
			ReplyToMail: m.Email,
			ReplyToName: m.Name,
		}}) {
			s.log.Warn("email queue full; contact notification skipped (message is saved in the inbox)", "message_id", m.ID)
		}
	}
}

func (s *ContactService) List(ctx context.Context, limit, offset int32) ([]sqlc.ContactMessage, error) {
	return s.queries.ListContactMessages(ctx, sqlc.ListContactMessagesParams{Limit: limit, Offset: offset})
}

func (s *ContactService) SetRead(ctx context.Context, id uuid.UUID, read bool) error {
	return s.queries.SetContactMessageRead(ctx, sqlc.SetContactMessageReadParams{ID: id, IsRead: read})
}

func (s *ContactService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteContactMessage(ctx, id)
}
