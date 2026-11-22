"""Prognoza pogody z GRIB (ecCodes). Importuj ten pakiet przed `eccodes`, żeby znaleźć zbudowaną bibliotekę."""
import os
from pathlib import Path

_local = Path(__file__).resolve().parents[2] / ".eccodes"
if _local.is_dir():
    os.environ.setdefault("ECCODES_DIR", str(_local))
