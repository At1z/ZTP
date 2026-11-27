import pytest

from weather.sources.sources import DATA_DIR, default_sources

pytestmark = pytest.mark.skipif(not (DATA_DIR / "gfs").exists(), reason="brak próbek GRIB (scripts/fetch_data.py)")


@pytest.mark.parametrize("name", ["gfs", "ifs-oper", "ifs-enfo"])
def test_tree_is_cropped_to_poland(name):
    forecast = default_sources()[name].load()
    assert forecast.members
    for member in forecast.members.values():
        for step in member.steps.values():
            for level in step.levels.values():
                for fld in level.fields.values():
                    assert fld.lats.max() <= 55 and fld.lats.min() >= 49
                    assert fld.lons.max() <= 25 and fld.lons.min() >= 14
                    assert fld.values.shape == (len(fld.lats), len(fld.lons))


def test_ensemble_has_many_members_and_deterministic_one():
    assert len(default_sources()["ifs-enfo"].load().members) > 1
    assert list(default_sources()["gfs"].load().members) == ["oper"]
