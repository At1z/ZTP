"""Rasteryzacja pola do PNG w układzie Web Mercator (EPSG:3857), gotowego pod nakładkę na mapę OSM."""
from __future__ import annotations

import io
import math

import numpy as np
from PIL import Image

from weather.domain.models import Field

# paleta: niebieski -> cyjan -> zielony -> żółty -> czerwony
_STOPS = np.array([[49, 54, 149], [69, 117, 180], [116, 173, 209], [171, 217, 233], [254, 224, 144], [244, 109, 67], [165, 0, 38]], dtype=float)
_ALPHA = 170
_UNITS = {"2t": ("°C", -273.15)}  # param -> (jednostka, przesunięcie)


def mercator_y(lat_deg: np.ndarray | float) -> np.ndarray | float:
    """y = ln(tan(pi/4 + phi/2)) (jednostkowa kula; skala R nie wpływa na rozmieszczenie wierszy)."""
    return np.log(np.tan(math.pi / 4 + np.radians(lat_deg) / 2))


def inverse_mercator_y(y: np.ndarray) -> np.ndarray:
    return np.degrees(2 * np.arctan(np.exp(y)) - math.pi / 2)


def to_display(field: Field) -> tuple[np.ndarray, str]:
    unit, shift = _UNITS.get(field.param_name, ("m/s" if field.param_name in ("10u", "10v") else "", 0.0))
    return field.values + shift, unit


def _colorize(values: np.ndarray, vmin: float, vmax: float) -> np.ndarray:
    t = np.clip((values - vmin) / (vmax - vmin or 1.0), 0, 1) * (len(_STOPS) - 1)
    lo = np.floor(t).astype(int).clip(0, len(_STOPS) - 2)
    frac = (t - lo)[..., None]
    rgb = _STOPS[lo] * (1 - frac) + _STOPS[lo + 1] * frac
    return np.dstack([rgb, np.full(values.shape, _ALPHA)]).astype(np.uint8)


def render_png(field: Field, width: int = 660, vmin: float | None = None, vmax: float | None = None) -> bytes:
    """Wiersze obrazu są równomierne w y Merkatora, kolumny w długości geograficznej."""
    values, _ = to_display(field)
    lat_top, lat_bottom = float(field.lats[0]), float(field.lats[-1])
    y_top, y_bottom = mercator_y(lat_top), mercator_y(lat_bottom)
    lon_span = float(field.lons[-1] - field.lons[0])
    height = max(2, round(width * (y_top - y_bottom) / math.radians(lon_span)))

    row_lats = inverse_mercator_y(np.linspace(y_top, y_bottom, height))
    col_lons = np.linspace(field.lons[0], field.lons[-1], width)

    # interpolacja dwuliniowa na siatce źródłowej (lats malejąco -> odwracamy do interp)
    lat_idx = np.interp(row_lats, field.lats[::-1], np.arange(len(field.lats))[::-1])
    lon_idx = np.interp(col_lons, field.lons, np.arange(len(field.lons)))
    r0, c0 = np.floor(lat_idx).astype(int), np.floor(lon_idx).astype(int)
    r1, c1 = np.minimum(r0 + 1, len(field.lats) - 1), np.minimum(c0 + 1, len(field.lons) - 1)
    fr, fc = (lat_idx - r0)[:, None], (lon_idx - c0)[None, :]
    grid = (values[np.ix_(r0, c0)] * (1 - fr) * (1 - fc) + values[np.ix_(r0, c1)] * (1 - fr) * fc
            + values[np.ix_(r1, c0)] * fr * (1 - fc) + values[np.ix_(r1, c1)] * fr * fc)

    rgba = _colorize(grid, values.min() if vmin is None else vmin, values.max() if vmax is None else vmax)
    buf = io.BytesIO()
    Image.fromarray(rgba, "RGBA").save(buf, format="PNG")
    return buf.getvalue()
