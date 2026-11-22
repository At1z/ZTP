# Lab 4 – Jaka będzie pogoda?

Prognozy GFS (NOAA) i ECMWF IFS (open data, w tym wiązka) odczytywane przez **ecCodes**, ograniczone do Polski (lon 14–25, lat 49–55). REST API (FastAPI), PNG w Mercatorze (EPSG:3857) i mapa Leaflet + OpenStreetMap.

## Struktura danych
`Forecast(timestamp_init) → Member(id) → Step(time) → Level(type, value) → Field(param_name, values)` (`src/weather/domain/models.py`).

## Uruchomienie (Linux)
```bash
./scripts/build_eccodes.sh                    # CMake: libaec + ecCodes -> .eccodes/ (Windows: scripts/build_eccodes.ps1)
uv sync
uv run python scripts/fetch_data.py           # GFS (gribfilter) + IFS (data.ecmwf.int) -> data/
uv run uvicorn weather.api.main:app --app-dir src
```
Mapa: http://localhost:8000/ · dokumentacja API: http://localhost:8000/docs

Wymagane: `cmake`, `g++`, `uv`.

## API
- `GET /api/models` · `GET /api/{model}` (init, członkowie, kroki, parametry)
- `GET /api/{model}/field?member=&step=&param=` – pole jako JSON
- `GET /api/{model}/png?member=&step=&param=` – PNG w Mercatorze
- modele: `gfs`, `ifs-oper`, `ifs-enfo`; parametry: `2t`, `10u`, `10v`

## Testy
`uv run pytest`

## Źródła
[ecCodes](https://sites.ecmwf.int/docs/eccodes/) · [ECMWF IFS](https://data.ecmwf.int/forecasts/) · [GFS](https://nomads.ncep.noaa.gov/gribfilter.php?ds=gfs_0p25) · [Mercator](https://proj.org/en/stable/operations/projections/merc.html)
