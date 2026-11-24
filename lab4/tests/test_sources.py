from pathlib import Path

from weather.sources.sources import GribFilesSource, latest_run


class FakeReader:
    def read(self, model, paths):
        self.paths = paths
        return model


def test_latest_run_keeps_only_newest_run():
    paths = [Path(n) for n in ("gfs_2026112200_f000.grib2", "gfs_2026112412_f000.grib2", "gfs_2026112412_f006.grib2")]
    assert {p.name for p in latest_run(paths)} == {"gfs_2026112412_f000.grib2", "gfs_2026112412_f006.grib2"}


def test_source_does_not_mix_runs(tmp_path):
    for n in ("gfs_2026112200_f000.grib2", "gfs_2026112412_f000.grib2"):
        (tmp_path / n).touch()
    reader = FakeReader()
    GribFilesSource("gfs", "gfs_*.grib2", reader=reader, data_dir=tmp_path).load()
    assert [p.name for p in reader.paths] == ["gfs_2026112412_f000.grib2"]
