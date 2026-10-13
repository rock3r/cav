"""Tests for remove_background.py, the rembg helper behind `cav gen cutout`.

They need only Pillow, not rembg or its model. Run from the repository root:

  uv run --no-project --python 3.12 --with pillow python -m unittest assets/python/test/test_remove_background.py
"""
import sys
import tempfile
import unittest
from pathlib import Path

from PIL import Image

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))

import remove_background as rb  # noqa: E402


class ApplyMask(unittest.TestCase):
    def test_existing_transparency_is_kept(self):
        # Left half fully transparent, right half half transparent, all red.
        im = Image.new("RGBA", (4, 1), (255, 0, 0, 255))
        for x, a in enumerate([0, 0, 128, 128]):
            im.putpixel((x, 0), (255, 0, 0, a))
        # The mask keeps everything except the last pixel.
        mask = Image.new("L", (4, 1), 255)
        mask.putpixel((3, 0), 0)
        out = rb.apply_mask(im, mask)
        self.assertEqual([out.getpixel((x, 0))[3] for x in range(4)], [0, 0, 128, 0])
        self.assertEqual(out.getpixel((2, 0))[:3], (255, 0, 0))

    def test_opaque_image_takes_the_mask(self):
        im = Image.new("RGB", (2, 1), (0, 0, 255))
        mask = Image.new("L", (2, 1), 0)
        mask.putpixel((1, 0), 200)
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / "in.png"
            im.save(path)
            out = rb.apply_mask(rb.load(path), mask)
        self.assertEqual(out.mode, "RGBA")
        self.assertEqual([out.getpixel((x, 0))[3] for x in range(2)], [0, 200])


if __name__ == "__main__":
    unittest.main()
