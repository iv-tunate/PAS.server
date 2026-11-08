# Akintola Samuel: backend

Go (Echo, pgx, sqlc, goose). Visitors have no accounts: they enter a name, email and phone to get a book or teaching. Only admins sign in.

## First run

1. `cp env.example .env` and fill it in (JWT_SECRET needs 32+ characters).
2. `make migrate-up`
3. `make create-admin EMAIL=you@example.com PASSWORD='a long password' NAME='Your Name'`
4. `make run`

After changing SQL in `internal/db/queries`, run `make sqlc`.

## How buying works

- Free item: `POST /checkout` queues an email with a download link.
- Paid item: `POST /checkout` returns a payment link. Paystack, Flutterwave or Stripe call
  `{API_PUBLIC_URL}/api/v1/payments/webhook/<gateway>` when the payment clears; the amount is
  checked against the order, then the link is emailed. A sweeper re-checks stuck payments every 5 minutes.
- The email link is `{API_PUBLIC_URL}/api/v1/download/<token>`, which redirects to the Cloudinary file. The file address is never in any public response.

## Deploying

`Dockerfile` builds a small non-root image. Set `RUN_MIGRATIONS=true` to apply new migrations on start.
`.github/workflows/backend.yml` tests, builds the image to GHCR and deploys over SSH using `docker-compose.prod.yml`.

Repository secrets: `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY`. On the server, put your filled-in `.env` at `/opt/pas/.env`
and put Caddy or nginx in front of port 8080 for HTTPS. Set `TRUST_PROXY=true` only when you do.

## Tests

`go test ./...` runs the flow tests only when `TEST_DATABASE_URL` points at a throwaway database. **They wipe its tables.**
