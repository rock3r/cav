# Cuts the background out of an image for `cav gen cutout` and `cav gen image --alpha`.
# Run by cav through uv, never directly.
#   args: <rembg model> <input image> <output.png>
# rembg downloads the model on its first use into ~/.rembg/models (or $REMBG_HOME/models).
# BiRefNet's general model (birefnet-general) is about 1 GB.
import sys

from PIL import Image, ImageChops, ImageOps


def load(src):
    """Opens the image upright (as its EXIF orientation says), as RGBA."""
    with Image.open(src) as im:
        return ImageOps.exif_transpose(im).convert("RGBA")


def apply_mask(rgba, mask):
    """Multiplies the image's own alpha by rembg's mask, so pixels that were already
    transparent or partly transparent stay so."""
    out = rgba.copy()
    out.putalpha(ImageChops.multiply(rgba.getchannel("A"), mask.convert("L")))
    return out


def main():
    model, src, dst = sys.argv[1:4]
    # Decode the image first: a missing or broken file fails before the model download.
    rgba = load(src)
    from rembg import new_session, remove

    session = new_session(model)
    mask = remove(rgba.convert("RGB"), session=session, only_mask=True)
    apply_mask(rgba, mask).save(dst, "PNG")


if __name__ == "__main__":
    main()
