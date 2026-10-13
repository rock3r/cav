# Cuts the background out of an image for `cav gen cutout` and `cav gen image --alpha`.
# Run by cav through uv, never directly.
#   args: <rembg model> <input image> <output.png>
# rembg downloads the model on its first use into ~/.rembg/models (or $REMBG_HOME/models).
# BiRefNet's general model (birefnet-general) is about 1 GB.
import sys

from PIL import Image
from rembg import new_session, remove


def main():
    model, src, dst = sys.argv[1:4]
    # Decode the image first: a missing or broken file fails before the model download.
    with Image.open(src) as im:
        rgb = im.convert("RGB")
    session = new_session(model)
    out = remove(rgb, session=session)
    out.convert("RGBA").save(dst, "PNG")


if __name__ == "__main__":
    main()
