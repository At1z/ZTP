import io

import numpy as np
from PIL import Image

from weather.domain.models import Field
from weather.render.mercator import inverse_mercator_y, mercator_y, render_png


def _field():
    lats = np.linspace(55, 49, 25)
    lons = np.linspace(14, 25, 45)
    return Field("2t", 270 + np.add.outer(np.linspace(0, 10, 25), np.linspace(0, 10, 45)), lats, lons)


def test_mercator_roundtrip():
    lat = np.array([49.0, 52.0, 55.0])
    assert np.allclose(inverse_mercator_y(mercator_y(lat)), lat)


def test_png_is_rgba_with_mercator_aspect():
    img = Image.open(io.BytesIO(render_png(_field(), width=440)))
    assert img.mode == "RGBA"
    expected_h = round(440 * (mercator_y(55) - mercator_y(49)) / np.radians(11))
    assert img.size == (440, expected_h)
