# Lab 3: koszyk zakupowy z blokadą produktów

Trzy małe serwisy w Go (`product`, `cart`, `order`), PostgreSQL i prosty sklep w HTML/CSS/JS. Użytkownik tworzy koszyk, dodaje produkty (które zostają zablokowane dla innych na 15 minut), a na końcu zatwierdza koszyk i dostaje zamówienie z powiadomieniem.

## 1. Jak to działa w skrócie

### Połączenia

Przeglądarka rozmawia z jednym adresem. Nginx w kontenerze `frontend` serwuje strony i przekazuje `/api/...` do serwisów:

```text
  przeglądarka ──►  frontend :8080 (nginx)
  nagłówek                │  /api/cart ─────────────────────►  cart    :8082
  X-User-ID: <uuid>       │  /api/products ─────────────────►  product :8081
                          │  /api/orders, /api/notifications ►  order   :8083
```

Serwisy rozmawiają między sobą przez HTTP, a każdy ma własną bazę:

```text
   ┌─────────────────┐  produkt, rezerwacja, zwolnienie, zakup  ┌─────────────────┐
   │  cart   :8082   │ ───────────────────────────────────────► │ product  :8081  │
   │  koszyki,       │                                          │ katalog, stany, │
   │  checkout       │                                          │ blokady         │
   └───────┬─────────┘                                          └────────┬────────┘
           │ utwórz zamówienie                                           │
           ▼                                                             │
   ┌─────────────────┐                                                   │
   │  order  :8083   │                                                   │
   │  zamówienia,    │                                                   │
   │  powiadomienia  │                                                   │
   └───────┬─────────┘                                                   │
           │               ┌───────────────────────────────┐             │
           └────────────►  │ PostgreSQL :5432, trzy bazy:  │  ◄──────────┘
        cart ────────────► │ `carts`, `orders`, `products` │
                           └───────────────────────────────┘
```

- Serwisy rozmawiają ze sobą **wyłącznie przez HTTP**. Żaden nie sięga do bazy innego.
- Użytkownik to **UUID w nagłówku `X-User-ID`**. Nie ma logowania ani sesji.
- Wewnętrzne endpointy rezerwacji `product` nie są wystawione przez nginx na zewnątrz.
- Bez przeglądarki można rozmawiać z serwisami bezpośrednio (`curl` na porty 8081, 8082, 8083).

### Typowy przebieg zakupu

1. `POST /cart` (`cart`): powstaje koszyk użytkownika. Ma jeden otwarty koszyk naraz i wygasa po 15 minutach.
2. `POST /cart/items` (`cart`): `cart` pyta `product` o cenę, potem prosi o **rezerwację**. `product` blokuje produkt w transakcji i sprawdza, czy starcza stanu. Jeśli nie, odpowiada 409. Jeśli tak, `cart` zapisuje pozycję w koszyku.
3. Inny użytkownik, który chce ostatnią sztukę, dostaje 409, dopóki rezerwacja nie wygaśnie albo pierwszy użytkownik nie usunie pozycji (`DELETE /cart/items/{productId}`).
4. `POST /cart/checkout` (`cart`):
   1. `product` kupuje zarezerwowany towar (stan magazynowy maleje na stałe),
   2. `order` tworzy zamówienie ze statusem `processing` i **powiadomienie** "Rozpoczęto realizację zamówienia",
   3. koszyk zostaje zamknięty.
5. `GET /orders/{id}` i `GET /notifications` (`order`): użytkownik widzi swoje zamówienie i powiadomienie.

Wygasła rezerwacja przerywa checkout, zanim cokolwiek zostanie kupione, a odpowiedzią jest 409.

## 2. Uruchomienie

Potrzebny jest tylko Docker (Go ani PostgreSQL lokalnie nie są wymagane).

```bash
cp .env.example .env
make app-up          # buduje i uruchamia postgres + product + cart + order + frontend
```

Potem otwórz **http://localhost:8080**.

| Kontener | Adres |
|---|---|
| frontend (sklep) | http://localhost:8080 |
| product | http://localhost:8081 |
| cart | http://localhost:8082 |
| order | http://localhost:8083 |
| PostgreSQL | localhost:5432 |

Katalog zawiera 15 produktów testowych (migracje `001` i `003`), w tym wyprzedany, jeden z jedną sztuką ("Bilet koncertowy") i kilka z małym stanem do testu blokady.

Inne komendy:

| Komenda | Działanie |
|---|---|
| `make up` | tylko PostgreSQL (do pracy z `go run`) |
| `make run-product` / `run-cart` / `run-order` | serwis na hoście (wymaga `make up`) |
| `make app-down` | zatrzymuje serwisy aplikacji |
| `make down` | zatrzymuje wszystko |
| `make reset` | zatrzymuje i **usuwa dane** (migracje wykonają się od nowa) |
| `make test-unit` / `make test-integ` / `make test` | testy |

Migracje wykonują się tylko przy pierwszym starcie pustego wolumenu. Po zmianie migracji zrób `make reset`.

### Frontend

Prosty sklep w czystym HTML, CSS i JavaScript (bez frameworków), katalog `frontend/`, serwowany przez nginx:

| Strona | Co robi |
|---|---|
| `index.html` | lista produktów ze stanem i przyciskiem "Do koszyka" |
| `product.html?id=...` | szczegóły produktu, wybór ilości |
| `cart.html` | pozycje, suma, **odliczanie do wygaśnięcia rezerwacji** (wg czasu serwera), usuwanie, złożenie zamówienia |
| `orders.html` | powiadomienia o zamówieniach i ich szczegóły |

- Identyfikator użytkownika (UUID) jest generowany przy pierwszej wizycie i trzymany w `localStorage`, a każde żądanie niesie go w `X-User-ID`.
- Przycisk **"Zmień"** w nagłówku pozwala wkleić inny UUID. Dzięki temu test blokady da się zrobić w dwóch kartach: w jednej wklej UUID drugiego użytkownika.
- Odliczanie jest tylko informacją. O wygaśnięciu decyduje serwer.

### Szybki test ręczny

```bash
U=$(uuidgen)
curl -s localhost:8081/products                                    # katalog, weź id
curl -s -XPOST localhost:8082/cart -H "X-User-ID: $U"              # utwórz koszyk
curl -s -XPOST localhost:8082/cart/items -H "X-User-ID: $U" \
     -d '{"product_id":"<id>","quantity":1}'                       # dodaj i zablokuj
curl -s localhost:8082/cart -H "X-User-ID: $U"                     # podgląd z sumą
curl -s -XPOST localhost:8082/cart/checkout -H "X-User-ID: $U"     # zamówienie
curl -s localhost:8083/notifications -H "X-User-ID: $U"            # powiadomienie
```

**Test blokady:** weź drugi `uuidgen` i spróbuj dodać do jego koszyka ten sam "Bilet koncertowy". Dostaniesz 409. Do demo ustaw w `.env` `RESERVATION_TTL=20s` i `CART_TTL=20s`, żeby nie czekać 15 minut.

### Konfiguracja (`.env`)

| Zmienna | Znaczenie | Domyślnie |
|---|---|---|
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | dostęp do PostgreSQL | `lab3` |
| `FRONTEND_PORT` / `PRODUCT_PORT` / `CART_PORT` / `ORDER_PORT` | porty na hoście | 8080 / 8081 / 8082 / 8083 |
| `RESERVATION_TTL` | jak długo produkt jest zablokowany | `15m` |
| `CART_TTL` | jak długo żyje koszyk | `15m` |

Serwisy czytają też `SERVICE_DB`, `PRODUCT_SERVICE_URL` i `ORDER_SERVICE_URL` (compose ustawia je sam) oraz `CART_SWEEP_INTERVAL`, jak często `cart` usuwa wygasłe koszyki (domyślnie `10s`).

## 3. Architektura

### 3.1 Trzy niezależne domeny

| Serwis | Odpowiada za | Baza |
|---|---|---|
| `product` | katalog, stan magazynowy, **blokady (rezerwacje)** | `products` |
| `cart` | koszyki użytkowników, pozycje, wygasanie koszyków, checkout | `carts` |
| `order` | zamówienia i powiadomienia | `orders` |

- Odwołania między serwisami (`user_id`, `cart_id`, `product_id`) to zwykłe UUID bez kluczy obcych, bo każdy serwis ma własną bazę.
- `cart` zna `product` i `order` tylko przez interfejsy (`ProductClient`, `OrderClient`) zaimplementowane w warstwie `client`. Domena zamówień nie zawiera ani linijki kodu koszyka.
- `cart` jest bezstanowy, więc można uruchomić wiele jego instancji (skalowanie poziome): stan i blokady są w bazie.

### 3.2 Warstwy: handler → service → repository (+ domain)

Każdy serwis ma ten sam układ:

```text
internal/<serwis>/
├── domain/       encje, błędy domenowe, interfejsy (repozytoria, klienci)
├── service/      logika aplikacji i walidacja: komendy i zapytania (Use Case'y)
├── repository/   implementacja interfejsów domeny w PostgreSQL (pgx)
├── client/       klienci HTTP do innych serwisów (tylko cart)
└── handler/      HTTP: routing, DTO, mapowanie błędów na kody statusu
cmd/<serwis>/main.go   składa zależności i uruchamia serwer
internal/platform/     wspólne: config, połączenie z bazą, odpowiedzi JSON, X-User-ID
```

```text
handler ──► service ──► domain ◄── repository / client
 (HTTP)    (Use Case)  (interfejsy)   (PostgreSQL, HTTP)
```

- **handler** zamienia HTTP na wywołanie serwisu i błąd domenowy na kod statusu. Nie ma w nim logiki biznesowej.
- **service** to Use Case'y. Zależy tylko od interfejsów z `domain`, więc testy jednostkowe używają fake'ów i **nie potrzebują serwera ani bazy**.
- **repository** i **client** implementują te interfejsy. Zależności wskazują do środka (Dependency Inversion).

### 3.3 CQRS

Operacje zmieniające stan to **komendy**, odczyty to **zapytania**. Każda ma osobny typ wejścia (`...Command`, `...Query`) i metodę w serwisie. Serwisy są rozdzielone na stronę zapisu i odczytu:

| Serwis | Komendy | Zapytania |
|---|---|---|
| `cart` | `OpenCart`, `AddItem`, `RemoveItem`, `Checkout`, `ExpireCarts` | `GetCart` |
| `product` | `Reserve`, `Release`, `ReleaseCart`, `Confirm` | `ListProducts`, `GetProduct` |
| `order` | `CreateOrder` | `GetOrder`, `ListNotifications` |

Repozytoria `product` też są rozdzielone: `ProductRepository` (odczyt) i `ReservationRepository` (zapis).

### 3.4 Tożsamość użytkownika i koszyki

- **Użytkownik** to UUID z nagłówka `X-User-ID`. Brak lub zły UUID daje 401. To nie jest prawdziwe uwierzytelnianie (nagłówek można podrobić). PDF wymaga tylko unikalnego identyfikatora. W produkcji tę rolę przejąłby token (np. JWT).
- **Koszyk** ma własne `id`, `user_id`, `status` i `expires_at`. Częściowy indeks unikalny `UNIQUE (user_id) WHERE status = 'open'` gwarantuje **jeden otwarty koszyk na użytkownika**. Żądania z kilku urządzeń trafiają więc do tego samego koszyka.
- Każda operacja wybiera koszyk po `user_id` z nagłówka, więc użytkownik nie dotknie cudzego koszyka ani zamówienia (zwracamy 404).

### 3.5 Gdzie jest blokada produktów: analiza

PDF wymaga uzasadnienia, czy blokada ma być w domenie koszyka, czy produktów.

| | Blokada w domenie **koszyka** | Blokada w domenie **produktów** (wybrana) |
|---|---|---|
| Zalety | mniej wywołań HTTP, wszystko w jednym miejscu | produkt jest jedynym właścicielem stanu magazynowego, jedno źródło prawdy; `SELECT ... FOR UPDATE` na wierszu produktu poprawnie szereguje żądania nawet przy wielu replikach koszyka; dostępność zawsze uwzględnia wszystkie blokady |
| Wady | każda instancja koszyka musiałaby znać rezerwacje innych koszyków, więc stan rozjeżdża się między domenami; trudna poprawność przy skalowaniu | dodatkowe wywołanie HTTP przy dodaniu i usunięciu pozycji; trzeba obsłużyć niedostępność serwisu produktów |

**Decyzja: blokada w domenie produktów.** Stan magazynowy należy do produktów, a poprawność przy równoległych żądaniach daje jedna transakcja w jednej bazie. Koszt (dodatkowe wywołanie HTTP) jest tu mniejszym problemem niż ryzyko sprzedania towaru dwa razy.

### 3.6 Jak działa rezerwacja

Tabela `reservations` (w bazie `products`): produkt, koszyk, ilość, `expires_at`. Rezerwacja jest **aktywna, gdy `expires_at > now()`**, więc wygasłe nie liczą się do zajętego stanu, nawet jeśli żaden job ich nie posprzątał.

Rezerwacja w jednej transakcji:

1. `SELECT ... FROM products WHERE id = $1 FOR UPDATE` (blokada wiersza produktu),
2. suma aktywnych rezerwacji produktu,
3. jeśli `ilość > stock − suma`, odpowiedź 409 i wycofanie,
4. inaczej upsert rezerwacji i zatwierdzenie.

Dodatkowe zasady:
- Blokada trwa `RESERVATION_TTL` od utworzenia. Ponowne dodanie tego samego produktu **nie przedłuża** czasu.
- Stan (`stock`) maleje dopiero przy checkout.
- Czas pochodzi z bazy (`now()`), więc kilka instancji serwisu nie ma rozbieżności zegarów.
- Operacje zmieniające stan blokują produkty **zawsze w kolejności `id`**. Dzięki temu dwa równoległe checkouty nie zakleszczą się.

### 3.7 Spójność przy wielu urządzeniach

Użytkownik może dodawać i usuwać pozycje z kilku urządzeń naraz, a żadna operacja nie może zginąć.

- `AddItem` i `RemoveItem` idą przez `CartRepository.Mutate`. Ta metoda blokuje wiersz koszyka (`SELECT ... FOR UPDATE`) na czas całej operacji, razem z wywołaniem HTTP do `product`. Operacje na jednym koszyku wykonują się więc **po kolei**, a rezerwacja w `product` zawsze zgadza się z pozycją w koszyku.
- Koszyki różnych użytkowników nie blokują się nawzajem, a blokada leży w bazie, więc działa przy dowolnej liczbie instancji `cart`.
- Dodanie pozycji to jeden atomowy upsert (`quantity = quantity + n`), a nie odczyt-zmiana-zapis, więc dwa równoległe dodania się sumują.
- Jeśli zwolnienie rezerwacji się nie uda, usunięcie pozycji jest cofane (transakcja się wycofuje).

Opis jest też w komentarzach w kodzie (`internal/cart/service/commands.go`, `internal/product/repository/reservation.go`).

### 3.8 Wygasanie

- **Rezerwacja** wygasa sama po `RESERVATION_TTL`.
- **Koszyk** wygasa po `CART_TTL`. Job w `cart` co 10 sekund usuwa wygasłe koszyki i zwalnia ich rezerwacje, żeby towar wrócił szybciej. Poprawność od niego **nie zależy**: wygasły koszyk jest traktowany jak nieistniejący na podstawie zegara bazy. Przy tworzeniu nowego koszyka wygasłe są sprzątane od razu, żeby użytkownik mógł znów zarezerwować ten sam produkt.
- Kilka instancji może uruchamiać job jednocześnie: każdy wygasły koszyk trafia do dokładnie jednej z nich.

### 3.9 Checkout i awarie

`Checkout` działa pod blokadą koszyka: kupuje rezerwacje w `product` (z jawną listą pozycji), tworzy zamówienie w `order`, zamyka koszyk.

- Wygasła rezerwacja przerywa checkout **przed** zakupem. Koszyk zostaje bez zmian, stan i zamówienia też.
- Jeśli `order` padnie już po kupnie towaru, koszyk zostaje otwarty, a **ponowienie checkoutu jest bezpieczne**: potwierdzenie rezerwacji (tabela `confirmed_carts`) i utworzenie zamówienia (unikalny `cart_id`) są idempotentne, więc nic nie zostanie kupione ani zamówione dwa razy.
- Znane ograniczenie: gdyby użytkownik zmienił koszyk między nieudanym checkoutem a ponowieniem, nowa pozycja trafiłaby do zamówienia bez zmniejszenia stanu. PDF zakłada, że zamykanie koszyka robi się z jednego urządzenia naraz, więc zostawiamy to bez dodatkowej ochrony.

## 4. API

Wszystkie endpointy `cart` i `order` (poza `healthz`) wymagają nagłówka `X-User-ID`. Błędy mają postać `{"error": "..."}`. Kwoty są podawane w groszach (`*_cents`).

### product (:8081)

| Metoda | Endpoint | Opis |
|---|---|---|
| GET | `/products` | lista produktów (`stock` i `available`) |
| GET | `/products/{id}` | szczegóły produktu |
| POST | `/reservations` | zarezerwuj dla koszyka (wewnętrzne), 409 przy braku stanu |
| DELETE | `/reservations/{cartId}/{productId}` | zwolnij rezerwację (wewnętrzne) |
| DELETE | `/reservations/{cartId}` | zwolnij wszystkie rezerwacje koszyka (wewnętrzne) |
| POST | `/reservations/{cartId}/confirm` | kup: `{"items":[{"product_id","quantity"}]}` (wewnętrzne, idempotentne) |
| GET | `/healthz` | stan serwisu i bazy |

### cart (:8082)

| Metoda | Endpoint | Opis |
|---|---|---|
| POST | `/cart` | utwórz koszyk (`201`) albo zwróć istniejący (`200`) |
| GET | `/cart` | pozycje, suma, czas wygaśnięcia (`404`, gdy brak otwartego koszyka) |
| POST | `/cart/items` | dodaj i zablokuj: `{"product_id":"...","quantity":1}` (`201`; `409` brak stanu) |
| DELETE | `/cart/items/{productId}` | usuń pozycję i zwolnij blokadę |
| POST | `/cart/checkout` | zatwierdź koszyk (`201`; `409` pusty koszyk lub wygasła rezerwacja; `502` serwis niedostępny) |
| GET | `/healthz` | stan serwisu i bazy |

### order (:8083)

| Metoda | Endpoint | Opis |
|---|---|---|
| POST | `/orders` | utwórz zamówienie (wołane przez `cart`), `201` nowe lub `200` istniejące dla koszyka |
| GET | `/orders/{id}` | zamówienie (tylko właściciel, inaczej `404`) |
| GET | `/notifications` | powiadomienia użytkownika, najnowsze pierwsze |
| GET | `/healthz` | stan serwisu i bazy |

### Kody odpowiedzi

`200` odczyt lub zmiana, `201` utworzono, `204` zwolniono, `400` zła walidacja, `401` brak `X-User-ID`, `404` brak zasobu lub cudzy koszyk, `409` brak stanu, pusty koszyk, wygasła rezerwacja, `500` błąd serwera, `502` niedostępny serwis, z którym rozmawia `cart`.

## 5. Baza danych

Jeden kontener PostgreSQL 16 z trzema bazami. Skrypt `docker/postgres/init-databases.sh` tworzy je i stosuje migracje z `migrations/<serwis>/` przy pierwszym starcie.

| Baza | Tabele |
|---|---|
| `products` | `products`, `reservations`, `confirmed_carts` |
| `carts` | `carts`, `cart_items` |
| `orders` | `orders`, `order_items`, `notifications` |

Zastosowano klucze główne UUID, klucze obce wewnątrz bazy serwisu, ograniczenia `CHECK` (`quantity > 0`, `stock >= 0`, `price >= 0`, dozwolone statusy), unikalności (`(cart_id, product_id)`, jeden otwarty koszyk na użytkownika, jedno zamówienie na koszyk) i indeksy na kolumnach filtrowanych. Wszystkie zapytania są parametryzowane.

## 6. Testy

```bash
make up              # integracyjne potrzebują PostgreSQL
make test-unit       # fake'i, bez bazy i bez serwera
make test-integ      # prawdziwe serwisy na tymczasowych bazach
```

- **Jednostkowe** (`tests/unit/`): Use Case'y koszyka, produktów i zamówień na fake'ach: dodawanie, usuwanie, walidacja, suma, izolacja użytkowników, checkout (sukces, pusty koszyk, wygasła rezerwacja, awaria serwisu zamówień), wygasanie koszyków.
- **Integracyjne** (`tests/integ/`): każdy test tworzy własne bazy z prawdziwymi migracjami i usuwa je na końcu. Serwisy rozmawiają ze sobą przez HTTP (`httptest`). Sprawdzają m.in.:
  - **współbieżność:** 50 równoległych koszyków walczy o ostatnią sztukę, wygrywa dokładnie jeden. Przy stanie 10 i 40 próbach przechodzi dokładnie 10.
  - równoległe dodawania z jednego koszyka są wszystkie policzone, a po mieszanym wyścigu dodań i usunięć ilość w koszyku równa się rezerwacji w `product`,
  - wygasanie rezerwacji i koszyka oraz zwolnienie blokady po usunięciu pozycji,
  - checkout end-to-end (stan, zamknięcie koszyka, zamówienie, powiadomienie, izolacja użytkowników),
  - bezpieczne ponowienie checkoutu po awarii serwisu zamówień (jedno zamówienie, stan zmniejszony raz),
  - brak zakleszczeń przy równoległych potwierdzeniach.

CI (`.github/workflows/ci-lab3.yml`) uruchamia gofmt, `go vet`, walidację compose i wszystkie testy z usługą PostgreSQL.

## 7. Poza zakresem

Rejestracja, logowanie, sesje i cookies, prawdziwa płatność, broker wiadomości, testy automatyczne frontendu. Dalszy przebieg realizacji zamówienia (status poza `processing`) też nie jest zaimplementowany.
