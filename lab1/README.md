# Lab 1 – Products REST API (Go + PostgreSQL)

English version first, Polish version ([PL version](#pl-version)) below.

## Project structure / Struktura projektu

```
ZTP/
├── .github/workflows/ci.yml       # CI: gofmt, vet, tests on PR to main
└── lab1/
    ├── cmd/api/main.go            # entry point, wiring of layers
    ├── internal/
    │   ├── config/                # env configuration
    │   ├── db/                    # PostgreSQL connection pool
    │   ├── domain/                # shared types, errors, price limits
    │   ├── handler/               # presentation layer (HTTP, JSON)
    │   ├── service/               # business logic, validation
    │   └── repository/            # data access (SQL, change history)
    ├── migrations/001_init.sql    # database schema
    ├── tests/
    │   ├── unit/                  # service tests with in-memory fakes
    │   └── integ/                 # HTTP tests on a real PostgreSQL
    ├── docker-compose.yml         # PostgreSQL 16
    ├── .env.example               # database settings template
    ├── Makefile                   # db-up, run, test-unit, test-integ, test
    ├── go.mod / go.sum
    └── README.md
```

## Step 1 Docker environment with PostgreSQL

**Requirements:** Docker + Docker Compose.

1. Copy the config file: `cp .env.example .env` (adjust values if needed).
2. Start the database: `docker compose up -d`.
3. Check its state: `docker compose ps` – the status should be `healthy`.
4. Open `psql`: `docker compose exec postgres psql -U lab1 -d lab1`, then `\dt` lists the `products`, `product_history` and `banned_phrases` tables.

The schema comes from `migrations/001_init.sql` – Postgres runs it automatically **only on first start** (empty volume). To start over: `docker compose down -v && docker compose up -d`.

To stop: `docker compose down` (data stays in the `pgdata` volume).

## Step 2 Go connection to PostgreSQL

**Requirements:** Go 1.22+ (`brew install go`), the database from Step 1 running.

1. Start the database: `docker compose up -d`.
2. Run the API: `go run ./cmd/api`.
3. Check the connection: `curl -i localhost:8080/health` – expect `200` and `{"status":"ok"}`.
4. Stop the database (`docker compose stop`) and call `/health` again – expect `503 database unavailable`.

Configuration is read from environment variables (defaults match `.env.example`): `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_HOST`, `POSTGRES_PORT`, or a full `DATABASE_URL`; the HTTP address is set with `HTTP_ADDR` (default `:8080`).

Structure: `cmd/api/main.go` (startup, graceful shutdown on Ctrl+C), `internal/config` (env loading), `internal/db` (`pgxpool` connection + ping). The schema is still created by Docker on first start (Step 1), so no separate migration runner is needed.

## Step 3 Endpoints and business logic

Run: `docker compose up -d && go run ./cmd/api`.

**Layers (N-Layer):** `handler` (HTTP/JSON, status codes) → `service` (domain rules, validation, forbidden phrases) → `repository` (SQL, change history). `domain` holds the shared types. Layers depend on interfaces (`service.ProductRepository`, `handler.ProductService`), wired in `cmd/api/main.go`.

| Method | Path | Description | Success |
|---|---|---|---|
| POST | `/api/v1/products` | create a product | 201 |
| GET | `/api/v1/products` | list products | 200 |
| GET | `/api/v1/products/{id}` | get one product | 200 |
| PUT | `/api/v1/products/{id}` | replace a product | 200 |
| DELETE | `/api/v1/products/{id}` | delete a product | 204 |
| GET | `/api/v1/products/{id}/history` | change history (name, category, price, quantity) | 200 |
| GET | `/api/v1/banned-phrases` | list forbidden phrases | 200 |
| POST | `/api/v1/banned-phrases` | add a phrase `{"phrase": "spam"}` | 201 |
| DELETE | `/api/v1/banned-phrases/{id}` | remove a phrase | 204 |

**Product body:** `{"name": "Laptop1", "category": "electronics", "price": 3000, "quantity": 5}`. Categories: `electronics` (50–50000 PLN), `books` (5–500), `clothing` (10–5000).

**Rules:** name unique, 3–20 characters, letters and digits only, must not contain a forbidden phrase (case-insensitive, checked on create and update); `price` and `quantity` are required; price within the category range; quantity an integer ≥ 0.

**Errors** always use the format `{"errors": [{"field": "price", "message": "..."}]}`: `400` malformed JSON / bad id, `413` body over 1 MiB, `404` not found, `409` duplicate name or phrase, `422` rule violation (all invalid fields listed at once).

**History:** every `PUT` that changes a tracked field writes one row per changed field (old and new value) to `product_history`, in the same transaction as the update. Creating or deleting a product writes no history entry; deleting a product also deletes its history (`ON DELETE CASCADE`). Databases created before this change need `docker compose down -v` to pick up the new schema.

Example:
```
curl -i -X POST localhost:8080/api/v1/products -H 'Content-Type: application/json' \
  -d '{"name":"Laptop1","category":"electronics","price":3000,"quantity":5}'
```

## Step 4 Tests

Run: `make test-unit` (no database needed), `make db-up && make test-integ`, or `make test` for both.

`tests/unit` checks the service layer against in-memory fakes (all validation rules, forbidden phrases, conflicts, not-found).

`tests/integ` starts the real router on a real PostgreSQL and talks to it over HTTP (`net/http` client). It uses a separate database, `lab1_test`, created and re-migrated automatically on every run, so development data is never touched; each test starts from empty tables.

| File (`tests/integ/`) | Covered requirements |
|---|---|
| `product_test.go` | name rules (3–20 chars, letters/digits only, unique), price limits per category (boundaries), unknown category, quantity ≥ 0 and integer, errors name the invalid field (all at once), CRUD with 201/200/204/404/409/422/400 |
| `banned_phrase_test.go` | forbidden phrases block create and update (case-insensitive), list is dynamic (add/remove at runtime), duplicate and empty phrases |
| `history_test.go` | changed fields written to history (old/new value), history accumulates, unchanged or rejected updates leave no entries, 404 for unknown product |

Configuration: the database connection comes from the same env variables as the API (`POSTGRES_*` / `DATABASE_URL`); the test database name can be changed with `TEST_DB_NAME`.

## Workflow (GitHub Actions)

`.github/workflows/ci.yml` (repository root) runs on every pull request to `main`: it starts PostgreSQL 16 as a service container, checks `gofmt`, runs `go vet` and the unit and integration tests.

The tests use the default connection settings (`lab1`/`lab1`@`localhost:5432`), which match the service container.

Run the same checks locally with `gofmt -l .`, `go vet ./...` and `make test`.

---

## PL version

Wersja polska tego samego README.

## Krok 1 Środowisko Docker z PostgreSQL

**Wymagania:** Docker + Docker Compose.

1. Skopiuj plik konfiguracyjny: `cp .env.example .env` (w razie potrzeby zmień wartości).
2. Uruchom bazę: `docker compose up -d`.
3. Sprawdź stan: `docker compose ps` – status ma być `healthy`.
4. Wejdź do `psql`: `docker compose exec postgres psql -U lab1 -d lab1`, a w nim `\dt` pokaże tabele `products`, `product_history`, `banned_phrases`.

Schemat bazy tworzy `migrations/001_init.sql` – Postgres wykonuje go automatycznie **tylko przy pierwszym starcie** (pusty wolumen). Aby zacząć od zera: `docker compose down -v && docker compose up -d`.

Zatrzymanie: `docker compose down` (dane zostają w wolumenie `pgdata`).

## Krok 2 Połączenie Go z PostgreSQL

**Wymagania:** Go 1.22+ (`brew install go`), działająca baza z kroku 1.

1. Uruchom bazę: `docker compose up -d`.
2. Uruchom API: `go run ./cmd/api`.
3. Sprawdź połączenie: `curl -i localhost:8080/health` – oczekiwane `200` i `{"status":"ok"}`.
4. Zatrzymaj bazę (`docker compose stop`) i wywołaj `/health` ponownie – oczekiwane `503 database unavailable`.

Konfiguracja pochodzi ze zmiennych środowiskowych (domyślne wartości zgodne z `.env.example`): `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_HOST`, `POSTGRES_PORT` albo pełny `DATABASE_URL`; adres HTTP ustawia `HTTP_ADDR` (domyślnie `:8080`).

Struktura: `cmd/api/main.go` (start, graceful shutdown po Ctrl+C), `internal/config` (wczytanie env), `internal/db` (połączenie `pgxpool` + ping). Schemat nadal tworzy Docker przy pierwszym starcie (krok 1), więc osobny runner migracji nie jest potrzebny.

## Krok 3 Endpointy i logika biznesowa

Uruchomienie: `docker compose up -d && go run ./cmd/api`.

**Warstwy (N-Layer):** `handler` (HTTP/JSON, kody statusów) → `service` (reguły domenowe, walidacja, zabronione frazy) → `repository` (SQL, historia zmian). Pakiet `domain` zawiera wspólne typy. Warstwy zależą od interfejsów (`service.ProductRepository`, `handler.ProductService`), składanych w `cmd/api/main.go`.

| Metoda | Ścieżka | Opis | Sukces |
|---|---|---|---|
| POST | `/api/v1/products` | utwórz produkt | 201 |
| GET | `/api/v1/products` | lista produktów | 200 |
| GET | `/api/v1/products/{id}` | jeden produkt | 200 |
| PUT | `/api/v1/products/{id}` | zastąp produkt | 200 |
| DELETE | `/api/v1/products/{id}` | usuń produkt | 204 |
| GET | `/api/v1/products/{id}/history` | historia zmian (name, category, price, quantity) | 200 |
| GET | `/api/v1/banned-phrases` | lista zabronionych fraz | 200 |
| POST | `/api/v1/banned-phrases` | dodaj frazę `{"phrase": "spam"}` | 201 |
| DELETE | `/api/v1/banned-phrases/{id}` | usuń frazę | 204 |

**Body produktu:** `{"name": "Laptop1", "category": "electronics", "price": 3000, "quantity": 5}`. Kategorie: `electronics` (50–50000 PLN), `books` (5–500), `clothing` (10–5000).

**Reguły:** nazwa unikalna, 3–20 znaków, tylko litery i cyfry, bez zabronionej frazy (bez względu na wielkość liter, sprawdzane przy dodaniu i edycji); `price` i `quantity` są wymagane; cena w zakresie kategorii; ilość to liczba całkowita ≥ 0.

**Błędy** zawsze w formacie `{"errors": [{"field": "price", "message": "..."}]}`: `400` błędny JSON / id, `413` body powyżej 1 MiB, `404` brak zasobu, `409` duplikat nazwy lub frazy, `422` naruszenie reguł (wszystkie błędne pola naraz).

**Historia:** każdy `PUT`, który zmienia śledzone pole, zapisuje po jednym wierszu na zmienione pole (wartość stara i nowa) do `product_history`, w tej samej transakcji co aktualizacja. Utworzenie ani usunięcie produktu nie zapisuje wpisu historii; usunięcie produktu kasuje też jego historię (`ON DELETE CASCADE`). Bazy utworzone przed tą zmianą wymagają `docker compose down -v`, żeby przyjąć nowy schemat.

Przykład:
```
curl -i -X POST localhost:8080/api/v1/products -H 'Content-Type: application/json' \
  -d '{"name":"Laptop1","category":"electronics","price":3000,"quantity":5}'
```

## Krok 4 Testy

Uruchomienie: `make test-unit` (bez bazy), `make db-up && make test-integ` albo `make test` dla obu.

`tests/unit` sprawdza warstwę serwisów na fake'ach w pamięci (wszystkie reguły walidacji, zabronione frazy, konflikty, brak zasobu).

`tests/integ` uruchamia prawdziwy router na prawdziwym PostgreSQL i komunikuje się z nim przez HTTP (klient `net/http`). Używa osobnej bazy `lab1_test`, tworzonej i migrowanej od nowa przy każdym uruchomieniu, więc dane developerskie zostają nietknięte; każdy test startuje z pustych tabel.

| Plik (`tests/integ/`) | Pokryte wymagania |
|---|---|
| `product_test.go` | reguły nazwy (3–20 znaków, tylko litery i cyfry, unikalność), limity cen per kategoria (wartości graniczne), nieznana kategoria, ilość ≥ 0 i całkowita, błędy wskazują błędne pole (wszystkie naraz), CRUD z kodami 201/200/204/404/409/422/400 |
| `banned_phrase_test.go` | zabronione frazy blokują dodanie i edycję (bez względu na wielkość liter), lista jest dynamiczna (dodawanie i usuwanie w trakcie działania), duplikaty i puste frazy |
| `history_test.go` | zapis zmienionych pól ze starą i nową wartością, historia się kumuluje, brak zmian lub odrzucony PUT nie zostawia wpisów, 404 dla nieznanego produktu |

Konfiguracja: połączenie z bazą pochodzi z tych samych zmiennych środowiskowych co API (`POSTGRES_*` / `DATABASE_URL`); nazwę bazy testowej zmienisz przez `TEST_DB_NAME`.

## Workflow (GitHub Actions)

`.github/workflows/ci.yml` (w korzeniu repozytorium) uruchamia się przy każdym pull requeście do `main`: startuje PostgreSQL 16 jako service container, sprawdza `gofmt`, uruchamia `go vet` oraz testy jednostkowe i integracyjne.

Testy używają domyślnych ustawień połączenia (`lab1`/`lab1`@`localhost:5432`), zgodnych z kontenerem.

Te same sprawdzenia lokalnie: `gofmt -l .`, `go vet ./...` i `make test`.
