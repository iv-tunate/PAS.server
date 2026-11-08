.PHONY: run build test migrate-up migrate-down migrate-status sqlc tidy create-admin

run:
	go run .

build:
	go build -o bin/server .

# Needs a throwaway database with the migrations applied; see internal/service/flow_test.go
test:
	go test ./...

# The database url here is read from the terminal... not the env file itself
# To add the database url to the terminal, run
# export DATABASE_URL= $(grep DATABASE_URL .env | cut -d "=" -f2-) ... This is for gitbash
# For powershell, 
# $env:DATABASE_URL=(Get-Content .env | Select-String "DATABASE_URL").ToString().Split("=")[1]
migrate-up:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" down

migrate-status:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" status

sqlc:
	sqlc generate

tidy:
	go mod tidy

# make create-admin EMAIL=you@example.com PASSWORD='a long password' NAME='Your Name'
create-admin:
	go run . create-admin "$(EMAIL)" "$(PASSWORD)" "$(NAME)"
