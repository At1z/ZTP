# Koszyk zakupowy z blokadą produktów — ZTP Lab 3

## 1. Cel

Aplikacja do zarządzania koszykami zakupowymi z tymczasową blokadą (rezerwacją) produktów i składaniem zamówień. Zgodna z kryteriami z `ZTP - Lab 3.pdf`.

**Backend w Go (Golang). Wszystko uruchamiane w Dockerze (`docker compose up --build`).**

Użytkownik może:
- utworzyć nowy koszyk,
- dodać produkt do koszyka (produkt zostaje zablokowany dla innych),
- usunąć produkt z koszyka (blokada zwalniana od razu),
- obejrzeć koszyk i łączną wartość,
- zatwierdzić koszyk, generując zamówienie,
- otrzymać powiadomienie o rozpoczęciu realizacji zamówienia.

## 2. Zakres (decyzje)

- **Bez rejestracji, logowania, sesji i cookies.** PDF wymaga tylko unikalnego identyfikatora użytkownika.
- **Użytkownik = UUID** przesyłany w nagłówku `X-User-ID`. Frontend generuje go przy pierwszej wizycie (`crypto.randomUUID()`) i trzyma w `localStorage`. Brak prawdziwego uwierzytelniania — w produkcji rolę przejąłby token (np. JWT); odnotować w README.
- **Frontend (HTML/CSS/JS) jest opcjonalny**, robimy go na końcu, jeśli zostanie czas. Priorytet: backend, współbieżność, testy.
- Bez Redisa i brokera wiadomości. Wszystko na PostgreSQL i HTTP.

## 3. Stos technologiczny

| Element | Technologia |
|---|---|
| Backend | Go, biblioteka standardowa lub Chi |
| Baza danych | PostgreSQL 16 |
| Dostęp do bazy | pgx (`pgxpool`) |
| Migracje | pliki SQL, montowane do `docker-entrypoint-initdb.d` |
| Konteneryzacja | Docker + Docker Compose |
| CI | GitHub Actions (`.github/workflows/ci-lab3.yml`) |

## 4. Architektura

### 4.1 Trzy niezależne serwisy (jeden moduł Go)

| Serwis | Odpowiedzialność | Baza |
|---|---|---|
| `product` | katalog produktów, stan magazynowy, **blokady (rezerwacje)** | `products` |
| `cart` | koszyki użytkowników, pozycje, wygasanie koszyków | `carts` |
| `order` | zamówienia i powiadomienia | `orders` |

- Jeden kontener PostgreSQL, trzy osobne bazy (skrypt inicjalizujący). Serwis nie sięga do bazy innego serwisu.
- Komunikacja między serwisami **wyłącznie przez HTTP**:
  - `cart` → `product` (rezerwacja, zwolnienie, pobranie produktu),
  - `cart` → `order` (utworzenie zamówienia przy checkout).
- Domena zamówień nie importuje kodu domeny koszyka i odwrotnie. Wymiana danych tylko przez DTO w HTTP.
- `cart` jest bezstanowy, więc można uruchomić wiele jego instancji (skalowanie poziome). Stan trzyma baza.

### 4.2 Warstwy w każdym serwisie (czysta architektura)

```text
cmd/<serwis>/main.go          # składanie zależności, start serwera
internal/<serwis>/
├── domain/                   # encje, błędy domenowe, interfejsy repozytoriów/klientów
├── service/                  # logika aplikacji: komendy, zapytania i walidacja (Use Case'y)
├── repository/               # implementacja interfejsów w PostgreSQL (pgx)
├── client/                   # klienci HTTP do innych serwisów (tylko cart)
└── handler/                  # HTTP: routing, DTO, mapowanie błędów na statusy
internal/platform/            # wspólne: config, db (połączenie), httpx (odpowiedzi JSON)
migrations/<serwis>/          # pliki SQL
```

Przepływ: **handler → service → repository**. Zależności idą do środka: `handler` → `service` → `domain` ← `repository`/`client` (service zna tylko interfejsy z `domain`).

### 4.3 CQRS

Każda operacja zmieniająca stan to **komenda**, każdy odczyt to **zapytanie**. Każda ma osobny typ wejścia (`...Command`, `...Query`) i metodę w serwisie (Use Case): serwis zapisów (komendy) i serwis odczytów (zapytania).

- Komendy: `CreateCart`, `AddItem`, `RemoveItem`, `Checkout`, `ExpireCarts` (job), `ReserveProduct`, `ReleaseReservation`, `CreateOrder`.
- Zapytania: `GetCart`, `GetProduct`, `ListProducts`, `GetNotifications`.

### 4.4 Testowalność i jakość kodu

- Serwisy zależą od interfejsów (repozytoria, klient HTTP, zegar), więc testy jednostkowe używają fake'ów, **bez serwera i bez bazy**.
- Testy integracyjne na prawdziwym PostgreSQL (osobna baza testowa).
- Zasady SOLID, DRY, KISS. Brak powtórzeń (wspólne helpery w `platform`), brak zbędnych abstrakcji.

## 5. Tożsamość użytkownika i koszyki

- **Użytkownik** to UUID z nagłówka `X-User-ID`. Brak lub niepoprawny UUID → `400`/`401`.
- **Koszyk** ma własne `id` (UUID), `user_id`, `status` (`open`, `checked_out`, `expired`) i `expires_at`. Jeden użytkownik może mieć wiele koszyków w czasie, ale **tylko jeden otwarty** naraz (częściowy indeks unikalny `UNIQUE (user_id) WHERE status = 'open'`).
- Koszyk jest zawsze wybierany po `user_id` z nagłówka, więc użytkownik nie ma dostępu do cudzych koszyków (zwracamy `404`).
- Frontend nie zna `cartId`. Endpointy działają na "moim otwartym koszyku".

## 6. Blokada produktów (rezerwacje)

### 6.1 Wymagania
- Produkt jest blokowany od momentu dodania do koszyka. Nikt inny nie może go zarezerwować równolegle.
- Blokada wygasa po **15 minutach** (`RESERVATION_TTL`, zmienna środowiskowa, domyślnie `15m`; na demo można ustawić np. `20s`). Po tym czasie koszyk jest usuwany, a przedmioty zwalniane.
- Usunięcie pozycji z koszyka natychmiast zwalnia blokadę.
- Nie można zarezerwować więcej niż dostępny stan. Brak stanu → `409 Conflict`.

### 6.2 Gdzie umieszczamy blokadę: analiza

| | Blokada w domenie **koszyka** | Blokada w domenie **produktów** (wybrana) |
|---|---|---|
| Zalety | mniej wywołań HTTP, prostsza logika w jednym miejscu | produkt jest jedynym właścicielem stanu magazynowego, jedno źródło prawdy; `SELECT ... FOR UPDATE` na wierszu produktu daje poprawność przy wielu instancjach koszyka; blokadę widać przy każdym zapytaniu o dostępność, niezależnie od tego, kto ją założył |
| Wady | każda instancja koszyka musiałaby znać rezerwacje innych koszyków, więc stan magazynowy rozjeżdża się między domenami; trudno o poprawną współbieżność przy skalowaniu | dodatkowe wywołanie HTTP przy dodaniu i usunięciu pozycji; trzeba obsłużyć niedostępność serwisu produktów |

**Decyzja: blokada w domenie produktów.** Uzasadnienie: stan magazynowy należy do produktów, a poprawność przy równoległych żądaniach i wielu replikach koszyka zapewnia jedna transakcja w jednej bazie. Szczegółową analizę przenieść do README.

### 6.3 Mechanizm (serwis `product`)

Tabela `reservations`: `id`, `product_id`, `cart_id`, `user_id`, `quantity`, `expires_at`, `created_at`. Rezerwacja jest aktywna, gdy `expires_at > NOW()`. Poprawność nie zależy od żadnego joba czyszczącego.

Rezerwacja w jednej transakcji:
1. `BEGIN`.
2. `SELECT ... FROM products WHERE id = $1 FOR UPDATE`.
3. Policz aktywne rezerwacje: `SUM(quantity) WHERE product_id = $1 AND expires_at > NOW()`.
4. `dostępne = stock - zarezerwowane`.
5. Jeśli `quantity > dostępne` → `ROLLBACK`, `409`.
6. W przeciwnym razie wstaw lub zwiększ rezerwację dla `(cart_id, product_id)`, `COMMIT`.

Stan magazynowy (`stock`) jest zmniejszany dopiero przy potwierdzeniu zamówienia (checkout).

### 6.4 Wygasanie koszyka (serwis `cart`)

Prosty job w tle (ticker) znajduje otwarte koszyki z `expires_at < NOW()`, oznacza je jako `expired` (lub usuwa) i zwalnia ich rezerwacje przez HTTP w `product`. Rezerwacja w `product` i tak wygasa sama po `expires_at`, więc job jest tylko porządkiem.

## 7. Spójność operacyjna (wymóg PDF — udokumentować w kodzie)

Użytkownik może jednocześnie dodawać i usuwać pozycje z różnych urządzeń. Żadna operacja nie może zostać utracona ani pominięta.

- **Dodawanie:** atomowy upsert `INSERT ... ON CONFLICT (cart_id, product_id) DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity`. Bez wzorca "odczytaj, zmień, zapisz", więc dwa równoległe dodania się sumują.
- **Usuwanie:** atomowy `DELETE ... WHERE cart_id = $1 AND product_id = $2`. Operacja jest idempotentna.
- **Checkout:** `SELECT ... FROM carts WHERE id = $1 FOR UPDATE` blokuje koszyk na czas zatwierdzenia, więc dodanie pozycji w trakcie nie zgubi się ani nie trafi do zamówienia po zamknięciu koszyka. Po zatwierdzeniu status koszyka to `checked_out` i kolejne zmiany zwracają `409`.
- **Jeden otwarty koszyk na użytkownika** (częściowy indeks unikalny), więc równoległe żądania z różnych urządzeń trafiają do tego samego koszyka.
- Tworzenie i zamykanie koszyka dzieje się na jednym urządzeniu naraz (zgodnie z uwagą w PDF), więc nie wymaga równoległego przetwarzania.

Opis tego mechanizmu ma być w komentarzach w kodzie (repozytorium koszyka i Use Case'y).

## 8. Zamówienia i powiadomienia (serwis `order`)

- `Checkout` w `cart`: blokuje koszyk, odczytuje pozycje, wysyła `POST /orders` do `order` (migawka pozycji, cen i `user_id`), potwierdza rezerwacje w `product` (zmniejszenie `stock`), zamyka koszyk.
- `order` zapisuje zamówienie ze statusem `processing` i tworzy powiadomienie "Rozpoczęto realizację zamówienia".
- Powiadomienia odczytuje się przez `GET /notifications` (dla użytkownika z `X-User-ID`).
- Brak prawdziwej płatności. Realizacja jest symulowana.

## 9. API REST

Wszystkie endpointy koszyka i zamówień wymagają nagłówka `X-User-ID`.

### product (publiczne)
| Metoda | Endpoint | Opis |
|---|---|---|
| GET | `/products` | lista produktów z dostępnym stanem |
| GET | `/products/{id}` | szczegóły produktu |

### product (wewnętrzne, wołane przez cart)
| Metoda | Endpoint | Opis |
|---|---|---|
| POST | `/reservations` | zarezerwuj produkt dla koszyka (`409` przy braku stanu) |
| DELETE | `/reservations/{cartId}/{productId}` | zwolnij rezerwację |
| POST | `/reservations/{cartId}/confirm` | potwierdź rezerwacje i zmniejsz stan |

### cart
| Metoda | Endpoint | Opis |
|---|---|---|
| POST | `/cart` | utwórz koszyk (zwraca istniejący otwarty, jeśli jest) |
| GET | `/cart` | pozycje, suma, czas wygaśnięcia |
| POST | `/cart/items` | dodaj produkt i zablokuj (`{"product_id": "...", "quantity": 1}`) |
| DELETE | `/cart/items/{productId}` | usuń pozycję i zwolnij blokadę |
| POST | `/cart/checkout` | zatwierdź koszyk, utwórz zamówienie |

### order
| Metoda | Endpoint | Opis |
|---|---|---|
| POST | `/orders` | utwórz zamówienie (wołane przez cart) |
| GET | `/orders/{id}` | szczegóły zamówienia |
| GET | `/notifications` | powiadomienia użytkownika |

### Kody odpowiedzi
`200`, `201`, `400` (zła walidacja lub brak `X-User-ID`), `404` (brak zasobu lub cudzy koszyk), `409` (brak stanu, koszyk zamknięty lub wygasły), `500`. Błędy zwracane w spójnym formacie JSON `{"error": "..."}`. Wszystkie zapytania SQL parametryzowane.

## 10. Baza danych

Zasady: UUID jako klucze główne, klucze obce w obrębie bazy serwisu, ograniczenia `CHECK` (`quantity > 0`, `stock >= 0`, `price >= 0`), indeksy na kolumnach filtrowanych. Referencje między serwisami (np. `product_id` w koszyku) to zwykłe UUID bez klucza obcego.

- **products:** `products(id, name, description, price, stock, created_at)`, `reservations(id, product_id FK, cart_id, user_id, quantity, expires_at, created_at)`, unikalność `(cart_id, product_id)`. Seed: min. dwa produkty (w tym jeden z `stock = 1` do demo blokady).
- **carts:** `carts(id, user_id, status, created_at, expires_at)` + częściowy indeks unikalny na otwarte koszyki; `cart_items(id, cart_id FK, product_id, quantity, unit_price)` z unikalnością `(cart_id, product_id)`.
- **orders:** `orders(id, user_id, cart_id, total, status, created_at)`, `order_items(...)`, `notifications(id, user_id, order_id, message, created_at)`.

## 11. Docker

- `Dockerfile` wieloetapowy z argumentem `APP` (`product`, `cart`, `order`), wzorowany na lab2.
- `docker-compose.yml`: `postgres` (wolumen, healthcheck, skrypt tworzący trzy bazy i migracje), `product`, `cart`, `order` (`depends_on: service_healthy`). Wiele replik `cart` możliwe przez `--scale`.
- Konfiguracja przez zmienne środowiskowe, plik `.env.example`, brak zahardkodowanych haseł.
- `Makefile` ze skrótami (`up`, `down`, `reset`, `test-unit`, `test-integ`) jak w lab2.
- Start: `docker compose up --build`.

## 12. Testy

**Jednostkowe (fake'i, bez serwera i bazy):**
- Use Case'y koszyka: utworzenie, dodanie, usunięcie, podgląd z sumą, checkout.
- Odrzucenie dodania, gdy `product` zwraca brak stanu.
- Izolacja: użytkownik nie widzi cudzego koszyka.
- Checkout pustego lub zamkniętego koszyka.

**Integracyjne (PostgreSQL):**
- Rezerwacja ostatniej sztuki: **N równoległych żądań, przechodzi dokładnie jedno** (kluczowy test współbieżności).
- Wygasanie rezerwacji po TTL i ponowna rezerwacja przez innego użytkownika.
- Zwolnienie rezerwacji po usunięciu pozycji.
- Równoległe dodawanie i usuwanie pozycji z tego samego koszyka (żadna operacja nie ginie).
- Checkout: zmniejszenie stanu, zamówienie i powiadomienie.

**CI:** `.github/workflows/ci-lab3.yml` (gofmt, `go vet`, testy jednostkowe i integracyjne z usługą Postgres), jak `ci-lab2.yml`.

## 13. Kolejność implementacji

1. Docker Compose z PostgreSQL (trzy bazy), migracje, połączenie z bazą w Go (`config`, `db`).
2. Workflow CI (`ci-lab3.yml`).
3. Serwis `product`: lista, szczegóły, rezerwacje (z locking i testem współbieżności).
4. Serwis `cart`: komendy i zapytania, klient HTTP do `product`, wygasanie koszyków.
5. Serwis `order`: zamówienia, powiadomienia, checkout łączący `cart` z `order`.
6. Pozostałe testy, README (architektura, analiza blokady, opis spójności, przykłady żądań).
7. Opcjonalnie: prosty frontend (HTML/CSS/JS).

## 14. Poza zakresem (chyba że zostanie czas)

Rejestracja i logowanie, sesje cookie, bcrypt, scalanie koszyków anonimowych, prawdziwe płatności, broker wiadomości. Prezentacja +1.0 (15 min demo technologii) wymaga wcześniejszej zgody prowadzącego i jest osobnym zadaniem.
