"""Źródła prognoz: pliki GRIB pobrane przez scripts/fetch_data.py. GFS i IFS różnią się tylko wzorcem plików (DRY)."""
from __future__ import annotations

from pathlib import Path
from typing import Protocol

from weather.domain.models import Forecast
from weather.grib.reader import GribReader

DATA_DIR = Path(__file__).resolve().parents[3] / "data"


class ForecastSource(Protocol):
    name: str

    def load(self) -> Forecast: ...


class GribFilesSource:
    def __init__(self, name: str, pattern: str, reader: GribReader | None = None, data_dir: Path = DATA_DIR):
        self.name, self._pattern, self._dir = name, pattern, data_dir
        self._reader = reader or GribReader()

    def load(self) -> Forecast:
        paths = list(self._dir.glob(self._pattern))
        if not paths:
            raise FileNotFoundError(f"brak plików {self._pattern} w {self._dir} (uruchom scripts/fetch_data.py)")
        return self._reader.read(self.name, paths)


def default_sources() -> dict[str, ForecastSource]:
    sources = [
        GribFilesSource("gfs", "gfs/gfs_*.grib2"),
        GribFilesSource("ifs-oper", "ifs/ifs_oper_*.grib2"),
        GribFilesSource("ifs-enfo", "ifs/ifs_enfo_*.grib2"),
    ]
    return {s.name: s for s in sources}
