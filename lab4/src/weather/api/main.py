"""REST API: odczyt pojedynczych pól prognostycznych (JSON) i ich wizualizacji (PNG, Mercator)."""
from __future__ import annotations

from datetime import datetime
from functools import lru_cache
from pathlib import Path

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import FileResponse, Response

from weather.domain.models import Forecast
from weather.grib.reader import POLAND
from weather.render.mercator import render_png, to_display
from weather.sources.sources import default_sources

WEB_DIR = Path(__file__).resolve().parents[3] / "web"
SOURCES = default_sources()

app = FastAPI(title="Jaka będzie pogoda?")


@lru_cache(maxsize=None)
def _forecast(model: str) -> Forecast:
    if model not in SOURCES:
        raise HTTPException(404, f"nieznany model {model}; dostępne: {sorted(SOURCES)}")
    try:
        return SOURCES[model].load()
    except FileNotFoundError as e:
        raise HTTPException(404, str(e)) from e


def _field(model: str, member: str, step: datetime, param: str):
    try:
        return _forecast(model).find_field(member, step, param)
    except LookupError as e:
        raise HTTPException(404, str(e)) from e


@app.get("/api/models")
def models() -> list[str]:
    return sorted(SOURCES)


@app.get("/api/{model}")
def describe(model: str) -> dict:
    """Drzewo prognozy: init, członkowie, kroki, poziomy i parametry."""
    f = _forecast(model)
    first = next(iter(f.members.values()))
    steps = sorted(first.steps.values(), key=lambda s: s.time)
    return {
        "model": f.model,
        "timestamp_init": f.timestamp_init.isoformat(),
        "members": sorted(f.members, key=lambda m: (not m.isdigit(), int(m) if m.isdigit() else 0, m)),
        "steps": [s.time.isoformat() for s in steps],
        "params": sorted({p for lv in steps[0].levels.values() for p in lv.fields}),
        "bbox": {"lon_min": POLAND.lon_min, "lon_max": POLAND.lon_max, "lat_min": POLAND.lat_min, "lat_max": POLAND.lat_max},
    }


@app.get("/api/{model}/field")
def get_field(model: str, member: str = "oper", step: datetime = Query(...), param: str = Query(...)) -> dict:
    fld = _field(model, member, step, param)
    values, unit = to_display(fld)
    return {"param": param, "unit": unit, "lats": fld.lats.tolist(), "lons": fld.lons.tolist(), "values": values.tolist()}


@app.get("/api/{model}/png")
def get_png(model: str, member: str = "oper", step: datetime = Query(...), param: str = Query(...)) -> Response:
    return Response(render_png(_field(model, member, step, param)), media_type="image/png")


@app.get("/")
def index() -> FileResponse:
    return FileResponse(WEB_DIR / "index.html")
