"""Odczyt plików GRIB przez ecCodes i budowa drzewa Forecast -> ... -> Field (z przycięciem do bboxa)."""
from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path

import numpy as np

import weather  # noqa: F401  (ustawia ECCODES_DIR)
import eccodes

from weather.domain.models import Field, Forecast, Level, Member, Step


@dataclass(frozen=True)
class BBox:
    lon_min: float = 14.0
    lon_max: float = 25.0
    lat_min: float = 49.0
    lat_max: float = 55.0


POLAND = BBox()


def _member_id(handle) -> str:
    """Wiązka: perturbationNumber; prognoza deterministyczna (oper/GFS): 'oper'."""
    if eccodes.codes_is_defined(handle, "perturbationNumber"):
        return str(eccodes.codes_get_long(handle, "perturbationNumber"))
    return "oper"


def _crop(handle, bbox: BBox) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    ni, nj = eccodes.codes_get_long(handle, "Ni"), eccodes.codes_get_long(handle, "Nj")
    lats = eccodes.codes_get_array(handle, "latitudes").reshape(nj, ni)[:, 0]
    lons = eccodes.codes_get_array(handle, "longitudes").reshape(nj, ni)[0, :]
    values = eccodes.codes_get_values(handle).reshape(nj, ni)
    lons = np.where(lons > 180, lons - 360, lons)
    rows = np.where((lats >= bbox.lat_min) & (lats <= bbox.lat_max))[0]
    cols = np.where((lons >= bbox.lon_min) & (lons <= bbox.lon_max))[0]
    lats, lons = lats[rows], lons[cols]
    values = values[np.ix_(rows, cols)]
    if lats[0] < lats[-1]:  # północ u góry
        lats, values = lats[::-1], values[::-1]
    return values, lats, lons


class GribReader:
    def __init__(self, bbox: BBox = POLAND):
        self._bbox = bbox

    def read(self, model: str, paths: list[Path]) -> Forecast:
        forecast: Forecast | None = None
        for path in sorted(paths):
            with open(path, "rb") as f:
                while (handle := eccodes.codes_grib_new_from_file(f)) is not None:
                    try:
                        forecast = self._add_message(forecast, model, handle)
                    finally:
                        eccodes.codes_release(handle)
        if forecast is None:
            raise ValueError(f"brak wiadomości GRIB w {paths}")
        return forecast

    def _add_message(self, forecast: Forecast | None, model: str, handle) -> Forecast:
        date, time = eccodes.codes_get_long(handle, "dataDate"), eccodes.codes_get_long(handle, "dataTime")
        init = datetime.strptime(f"{date:08d}{time:04d}", "%Y%m%d%H%M")
        forecast = forecast or Forecast(model=model, timestamp_init=init)

        valid = init + timedelta(hours=eccodes.codes_get_long(handle, "step"))
        level_key = (eccodes.codes_get(handle, "typeOfLevel"), eccodes.codes_get_long(handle, "level"))
        param = eccodes.codes_get(handle, "shortName")

        member = forecast.members.setdefault(_member_id(handle), Member(id=_member_id(handle)))
        step = member.steps.setdefault(valid, Step(time=valid))
        level = step.levels.setdefault(level_key, Level(type=level_key[0], value=level_key[1]))
        values, lats, lons = _crop(handle, self._bbox)
        level.fields[param] = Field(param_name=param, values=values, lats=lats, lons=lons)
        return forecast
