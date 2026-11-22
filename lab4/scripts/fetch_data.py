"""Pobiera próbki prognoz GFS (NOMADS gribfilter) i ECMWF IFS (open data) dla Polski.

GFS: serwer sam tnie do bboxa Polski.
IFS: pliki są globalne - pobieramy tylko potrzebne wiadomości przez zakresy bajtów z pliku .index.
Przycięcie IFS do Polski robi reader (src/weather/grib/reader.py).

Użycie: uv run python scripts/fetch_data.py [--steps 0,6,12] [--members 5] [--date YYYYMMDD --hour HH]
"""
from __future__ import annotations

import argparse
import json
from datetime import datetime, timedelta, timezone
from pathlib import Path

import requests

DATA_DIR = Path(__file__).resolve().parent.parent / "data"
BBOX = {"leftlon": 14, "rightlon": 25, "toplat": 55, "bottomlat": 49}  # Polska
PARAMS = ("2t", "10u", "10v")  # temperatura 2 m, wiatr 10 m (shortName ecCodes)

GFS_URL = "https://nomads.ncep.noaa.gov/cgi-bin/filter_gfs_0p25.pl"
IFS_URL = "https://data.ecmwf.int/forecasts/{date}/{hour:02d}z/ifs/0p25/{stream}/{date}{hour:02d}0000-{step}h-{stream}-{kind}.grib2"
IFS_STREAMS = {"oper": "fc", "enfo": "ef"}  # stream -> typ pliku

session = requests.Session()


def latest_ifs_run() -> tuple[str, int]:
    """Najnowszy przebieg IFS, który już istnieje na serwerze (00/06/12/18 UTC)."""
    now = datetime.now(timezone.utc)
    for back in range(0, 48, 6):
        t = (now - timedelta(hours=back)).replace(minute=0, second=0, microsecond=0)
        hour = t.hour - t.hour % 6
        date = t.strftime("%Y%m%d")
        url = IFS_URL.format(date=date, hour=hour, stream="oper", step=0, kind="fc")
        if session.head(url, timeout=15).status_code == 200:
            return date, hour
    raise RuntimeError("Brak dostępnego przebiegu IFS")


def fetch_gfs(date: str, hour: int, steps: list[int]) -> None:
    out = DATA_DIR / "gfs"
    out.mkdir(parents=True, exist_ok=True)
    for step in steps:
        params = {
            "dir": f"/gfs.{date}/{hour:02d}/atmos",
            "file": f"gfs.t{hour:02d}z.pgrb2.0p25.f{step:03d}",
            "var_TMP": "on", "var_UGRD": "on", "var_VGRD": "on",
            "lev_2_m_above_ground": "on", "lev_10_m_above_ground": "on",
            "subregion": "", **BBOX,
        }
        r = session.get(GFS_URL, params=params, timeout=120)
        r.raise_for_status()
        path = out / f"gfs_{date}{hour:02d}_f{step:03d}.grib2"
        path.write_bytes(r.content)
        print(f"GFS  {path.name}  {len(r.content)} B")


def fetch_ifs(date: str, hour: int, steps: list[int], max_members: int) -> None:
    out = DATA_DIR / "ifs"
    out.mkdir(parents=True, exist_ok=True)
    for stream, kind in IFS_STREAMS.items():
        for step in steps:
            url = IFS_URL.format(date=date, hour=hour, stream=stream, step=step, kind=kind)
            index = session.get(url.replace(".grib2", ".index"), timeout=60)
            index.raise_for_status()
            entries = [json.loads(line) for line in index.text.splitlines()]
            wanted = [
                e for e in entries
                if e["param"] in PARAMS and e["levtype"] == "sfc" and int(e["step"]) == step
                and int(e.get("number", 0)) < max_members
            ]
            path = out / f"ifs_{stream}_{date}{hour:02d}_f{step:03d}.grib2"
            with path.open("wb") as f:
                for e in wanted:
                    rng = f"bytes={e['_offset']}-{e['_offset'] + e['_length'] - 1}"
                    r = session.get(url, headers={"Range": rng}, timeout=120)
                    r.raise_for_status()
                    f.write(r.content)
            print(f"IFS  {path.name}  {len(wanted)} wiadomości, {path.stat().st_size} B")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--steps", default="0,6,12", help="kroki prognozy w godzinach, np. 0,6,12")
    ap.add_argument("--members", type=int, default=5, help="ilu członków wiązki IFS pobrać")
    ap.add_argument("--date", help="YYYYMMDD (domyślnie najnowszy przebieg IFS)")
    ap.add_argument("--hour", type=int, help="00/06/12/18")
    args = ap.parse_args()

    steps = [int(s) for s in args.steps.split(",")]
    date, hour = (args.date, args.hour) if args.date and args.hour is not None else latest_ifs_run()
    print(f"Przebieg: {date} {hour:02d}z, kroki: {steps}")
    fetch_gfs(date, hour, steps)
    fetch_ifs(date, hour, steps, args.members)


if __name__ == "__main__":
    main()
