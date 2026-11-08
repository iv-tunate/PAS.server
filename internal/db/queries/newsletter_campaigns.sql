-- name: CreateCampaign :one
INSERT INTO newsletter_campaigns (subject, body, total)
VALUES ($1, $2, $3)
RETURNING *;

-- name: IncrementCampaignSent :exec
UPDATE newsletter_campaigns SET sent = sent + 1 WHERE id = $1;

-- name: IncrementCampaignFailed :exec
UPDATE newsletter_campaigns SET failed = failed + 1 WHERE id = $1;

-- name: FinishCampaign :exec
UPDATE newsletter_campaigns SET status = 'done', finished_at = now() WHERE id = $1;

-- name: ListCampaigns :many
SELECT * FROM newsletter_campaigns
ORDER BY created_at DESC
LIMIT 20;

-- name: InterruptStaleCampaigns :exec
-- A restart kills any broadcast that was mid-send; say so instead of
-- leaving it "sending" forever.
UPDATE newsletter_campaigns SET status = 'interrupted', finished_at = now() WHERE status = 'sending';
