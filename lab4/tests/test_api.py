import pytest
from fastapi.testclient import TestClient

from weather.api.main import app
from weather.sources.sources import DATA_DIR

pytestmark = pytest.mark.skipif(not (DATA_DIR / "gfs").exists(), reason="brak próbek GRIB")
client = TestClient(app)


def test_describe_and_field_and_png():
    meta = client.get("/api/gfs").json()
    q = {"step": meta["steps"][1], "param": "2t"}
    assert client.get("/api/gfs/field", params=q).json()["unit"] == "°C"
    r = client.get("/api/gfs/png", params=q)
    assert r.status_code == 200 and r.headers["content-type"] == "image/png"


def test_unknown_model_is_404():
    assert client.get("/api/nope").status_code == 404
