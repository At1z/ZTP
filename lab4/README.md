# Lab 4: Jaka będzie pogoda?

Aplikacja w Pythonie, która czyta numeryczne prognozy pogody (GFS i ECMWF IFS) w formacie GRIB przez bibliotekę **ecCodes**, ogranicza je do Polski i pokazuje na mapie OpenStreetMap. Jest REST API do odczytu pojedynczych pól oraz prosta strona z mapą.

## 1. Co to znaczy: słownik pojęć

W aplikacji wybierasz cztery rzeczy: model, członka, krok i parametr.

### Model

Program, który liczy pogodę. Mamy trzy zestawy danych:

| Nazwa w aplikacji | Co to jest |
|---|---|
| `gfs` | Model amerykański (NOAA). Jedna prognoza. |
| `ifs-oper` | Model europejski (ECMWF), prognoza główna ("operacyjna"). |
| `ifs-enfo` | Ten sam model europejski, ale **wiązka**: wiele prognoz naraz (patrz: członek). |

Oba modele mogą dawać trochę inną pogodę dla tego samego dnia. Porównanie ich to jeden z celów tego laboratorium.

### Przebieg (`timestamp_init`)

Model uruchamia się o stałych godzinach (00, 06, 12, 18 UTC). Moment startu to *przebieg*. Na mapie widzisz go jako `init: 2026-11-22 00:00 UTC`. Wszystkie kroki odnoszą się do tego momentu.

### Członek (`member`)

Pogoda jest wrażliwa na dane początkowe: malutka zmiana temperatury na starcie może po kilku dniach zmienić wynik. Dlatego model liczy się wiele razy, za każdym razem z lekko zmienionymi danymi początkowymi. Taki zestaw to **wiązka** (ensemble), a każda pojedyncza prognoza w nim to **członek**.

- `oper`: jedyna prognoza, bez zaburzeń (`gfs`, `ifs-oper`).
- `1`, `2`, `3`, ...: kolejni członkowie wiązki (`ifs-enfo`).

Jeśli członkowie dają podobny wynik, prognoza jest pewna. Jeśli się rozjeżdżają, prognoza jest niepewna.

### Krok (`step`)

Na ile godzin do przodu od przebiegu liczy się dany wynik. Przebieg 00:00 i krok 6 h to prognoza na 06:00. W aplikacji krok pokazuje się jako czas, na który obowiązuje prognoza. Krok 0 to stan początkowy modelu, a nie prognoza.

Dostępne kroki zależą od tego, co pobrałeś (`scripts/fetch_data.py --steps ...`). Modele liczą znacznie dalej, niż pobieramy domyślnie (0, 6 i 12 h).

### Parametr (`param`)

Wielkość fizyczna na mapie. Używamy trzech, dla jednej wysokości nad ziemią:

| Parametr | Znaczenie | Jednostka |
|---|---|---|
| `2t` | temperatura powietrza na 2 m | °C (w pliku GRIB jest w kelwinach) |
| `10u` | wiatr na 10 m, składowa wschód–zachód (dodatnia = wieje na wschód) | m/s |
| `10v` | wiatr na 10 m, składowa południe–północ (dodatnia = wieje na północ) | m/s |

### Poziom i pole

- **Poziom** (`level`): wysokość, na której podany jest parametr (np. 2 m nad ziemią, 10 m nad ziemią).
- **Pole** (`field`): siatka wartości jednego parametru na jednym poziomie, dla jednego członka i kroku. Dla Polski to 25 × 45 punktów co 0,25° (około 28 km).

### Co widać na mapie

Kolorowa nakładka na OpenStreetMap: niebieski to wartości niskie, czerwony wysokie. Skala jest dopasowana do wartości w danym polu, więc kolory nie są porównywalne między różnymi polami.

## 2. Uruchomienie

Potrzebne: `uv`, `cmake`, `g++` (Linux) albo Visual Studio Build Tools i `cmake` (Windows).

```bash
./scripts/build_eccodes.sh        # Windows: ./scripts/build_eccodes.ps1
uv sync
uv run python scripts/fetch_data.py
uv run uvicorn weather.api.main:app --app-dir src
```

Potem otwórz **http://localhost:8000**. Dokumentacja API (Swagger) jest pod `/docs`.

| Komenda | Działanie |
|---|---|
| `./scripts/build_eccodes.sh` | buduje libaec i ecCodes z CMake do `.eccodes/` (pierwszy raz kilka minut) |
| `uv run python scripts/fetch_data.py` | pobiera GFS i IFS do `data/` |
| `uv run python scripts/fetch_data.py --steps 0,3,6,12,24,48,72 --members 5` | więcej kroków i członków wiązki |
| `uv run pytest` | testy |

Opcje `fetch_data.py`: `--steps` (godziny od przebiegu), `--members` (ile członków wiązki IFS), `--date YYYYMMDD --hour HH` (konkretny przebieg; domyślnie najnowszy dostępny). Po pobraniu nowych danych zrestartuj serwer, bo API trzyma wczytane prognozy w pamięci.

## 3. Architektura

### 3.1 Przepływ danych

```text
  scripts/fetch_data.py                       przeglądarka (Leaflet + OSM)
  ┌──────────────────────┐                         ▲  PNG + JSON
  │ GFS  (NOMADS)        │──► data/gfs/*.grib2     │
  │ IFS  (data.ecmwf.int)│──► data/ifs/*.grib2     │
  └──────────────────────┘          │         FastAPI (api/main.py)
                                    ▼              ▲
                           sources ─► GribReader ─► domain ─► render (Mercator → PNG)
                          (pliki)    (ecCodes,      (Forecast…
                                      przycięcie)    Field)
```

Pobieranie jest osobnym krokiem (skrypt), a aplikacja czyta tylko lokalne pliki. Dzięki temu API działa bez internetu i szybko.

### 3.2 Moduły (`src/weather/`)

| Moduł | Odpowiedzialność |
|---|---|
| `domain/models.py` | struktura danych: `Forecast → Member → Step → Level → Field` |
| `grib/reader.py` | `GribReader`: czyta GRIB przez ecCodes, przycina do `BBox` (Polska), buduje drzewo |
| `sources/sources.py` | `ForecastSource` (Protocol) i `GribFilesSource`: znajduje pliki modelu i woła reader |
| `render/mercator.py` | rasteryzacja pola do PNG w EPSG:3857 |
| `api/main.py` | endpointy REST i strona z mapą |
| `web/index.html` | mapa Leaflet + OpenStreetMap, nakładka PNG |

### 3.3 Model obiektowy

Hierarchia z diagramu na tablicy (`GSGCVJN5.jpeg`):

```text
Forecast(model, timestamp_init)
 └─ Member(id)                       "oper" | "1" | "2" ...
     └─ Step(time)                   czas ważności prognozy
         └─ Level(type, value)       np. heightAboveGround, 2
             └─ Field(param_name, values[lat][lon], lats, lons)
```

`Forecast.find_field(member, time, param)` zwraca pole. Dla GRIB-a wszystkie wiadomości mają te same atrybuty (data, krok, poziom, parametr, numer członka), więc reader przechodzi plik raz i wkłada każdą wiadomość w odpowiednie miejsce drzewa.

### 3.4 Dlaczego GFS i IFS dzielą kod (DRY)

- Jeden `GribReader` dla obu modeli: różnice formatów (kolejność siatki, długości 0–360 vs −180–180, kierunek szerokości) są wyrównane w jednym miejscu (`_crop`).
- Jedna klasa `GribFilesSource` z parametrem `pattern`. Nowy model to jedna linijka w `default_sources()`.
- Wiązka to zwykły model z wieloma członkami. `_member_id` bierze `perturbationNumber` z GRIB-a, a jeśli go nie ma, daje `oper`.
- Renderer, API i UI znają tylko `Field`, nie wiedzą nic o GFS ani IFS.

### 3.5 ecCodes

- ecCodes to biblioteka C od ECMWF. Python (`pip install eccodes`) dostaje tylko bindingi, **bez biblioteki**, dlatego budujemy ją z CMake (`scripts/build_eccodes.sh` / `.ps1`).
- Dodatkowo budujemy **libaec** (≥ 1.1.4): IFS open data używa kompresji CCSDS (`grid_ccsds`), której ecCodes bez libaec nie rozpakuje.
- Wynik instalacji trafia do `.eccodes/`. `weather/__init__.py` ustawia `ECCODES_DIR`, więc nie trzeba niczego eksportować.
- Opcje CMake: `-DENABLE_FORTRAN=OFF -DENABLE_NETCDF=OFF -DENABLE_PYTHON=OFF -DENABLE_AEC=ON`.

### 3.6 Przycięcie do Polski

Bounding box: długość 14–25°, szerokość 49–55° (`grib/reader.py`, `POLAND`).

- **GFS**: serwer NOMADS tnie plik za nas (`subregion=&leftlon=14&rightlon=25&toplat=55&bottomlat=49`), plik ma kilka KB.
- **IFS**: pliki są globalne (1440 × 721). Pobieramy tylko potrzebne wiadomości przez zakresy bajtów (`Range`) z pliku `.index`, a przycinamy w readerze. Wiązka ma około 7 MB na krok przy 5 członkach.

### 3.7 PNG w układzie Mercatora

Mapa OSM i Leaflet używają EPSG:3857, w którym odstępy między równoleżnikami rosną z szerokością. Siatka modelu jest równomierna w stopniach, więc surowy obraz byłby zniekształcony.

1. Wiersze obrazu rozmieszczamy równomiernie w `y = ln(tan(π/4 + φ/2))`, a kolumny w długości geograficznej.
2. Dla każdego piksela interpolujemy dwuliniowo wartość z siatki.
3. Kolor z liniowej palety (7 przystanków), półprzezroczysty (alpha 170).
4. Wysokość obrazu wynika z proporcji bboxa w Mercatorze.

Dzięki temu `L.imageOverlay` rozciąga obraz tylko liniowo i nakładka pasuje do mapy. Wzór jest z dokumentacji PROJ (`merc`).

## 4. API

Błędy: `404` gdy brak modelu, członka, kroku, parametru albo plików danych.

| Endpoint | Zwraca |
|---|---|
| `GET /api/models` | lista modeli: `gfs`, `ifs-enfo`, `ifs-oper` |
| `GET /api/{model}` | `timestamp_init`, `members`, `steps`, `params`, `bbox` |
| `GET /api/{model}/field?member=&step=&param=` | JSON: `param`, `unit`, `lats`, `lons`, `values` (2D) |
| `GET /api/{model}/png?member=&step=&param=` | PNG w Mercatorze |
| `GET /` | strona z mapą |

`member` domyślnie `oper`. `step` to czas ważności w ISO, np. `2026-11-22T06:00:00` (listę daje `GET /api/{model}`).

```bash
curl -s localhost:8000/api/ifs-enfo
curl -s "localhost:8000/api/gfs/field?step=2026-11-22T06:00:00&param=2t"
curl -s -o t.png "localhost:8000/api/ifs-enfo/png?member=3&step=2026-11-22T12:00:00&param=2t"
```

## 5. Testy

`uv run pytest`

| Plik | Co sprawdza |
|---|---|
| `tests/test_render.py` | wzór Mercatora (odwracalność), rozmiar i format PNG (nie wymaga danych) |
| `tests/test_reader.py` | przycięcie do Polski dla wszystkich modeli, członkowie wiązki |
| `tests/test_api.py` | endpointy opisu, pola i PNG, 404 |

Testy `test_reader` i `test_api` są pomijane, gdy w `data/` nie ma próbek (najpierw `fetch_data.py`).

## 6. CI

`.github/workflows/ci-lab4.yml` buduje ecCodes (CMake) i uruchamia testy na `ubuntu-latest` i `windows-latest`. CI nie pobiera danych z sieci, więc testy czytające GRIB są tam pomijane.

## 7. Poza zakresem

- Tylko trzy parametry (`2t`, `10u`, `10v`) i jeden poziom na parametr.
- Brak szybkości wiatru ani innych wielkości pochodnych, brak statystyk wiązki (średnia, rozrzut).
- Skala kolorów jest liczona z jednego pola, nie jest stała między polami.
- Dane nie odświeżają się same: trzeba uruchomić `fetch_data.py`.
