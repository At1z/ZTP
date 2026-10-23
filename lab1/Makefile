.PHONY: db-up db-down run test test-unit test-integ

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

run:
	go run ./cmd/api

# Unit tests use in-memory fakes and need no database.
test-unit:
	go test -count=1 ./tests/unit/...

# Integration tests need the database from `make db-up`; they use a separate
# database (lab1_test) that is created and reset automatically.
test-integ:
	go test -count=1 -v ./tests/integ/...

test: test-unit test-integ
