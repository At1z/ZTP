"""Struktura danych prognozy zgodna z diagramem: Forecast -> Member -> Step -> Level -> Field."""
from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime

import numpy as np


@dataclass
class Field:
    param_name: str
    values: np.ndarray  # (lat, lon), lat malejąco (północ u góry)
    lats: np.ndarray
    lons: np.ndarray


@dataclass
class Level:
    type: str  # np. heightAboveGround
    value: int
    fields: dict[str, Field] = field(default_factory=dict)


@dataclass
class Step:
    time: datetime  # czas ważności
    levels: dict[tuple[str, int], Level] = field(default_factory=dict)


@dataclass
class Member:
    id: str  # "oper", "1", "2", ...
    steps: dict[datetime, Step] = field(default_factory=dict)


@dataclass
class Forecast:
    model: str  # "gfs" | "ifs-oper" | "ifs-enfo"
    timestamp_init: datetime
    members: dict[str, Member] = field(default_factory=dict)

    def find_field(self, member: str, time: datetime, param: str) -> Field:
        """Zwraca pole `param` z pierwszego poziomu, który je zawiera."""
        try:
            step = self.members[member].steps[time]
        except KeyError as e:
            raise LookupError(f"brak {e} w prognozie {self.model}") from e
        for level in step.levels.values():
            if param in level.fields:
                return level.fields[param]
        raise LookupError(f"brak parametru {param}")
