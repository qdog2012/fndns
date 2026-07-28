from pathlib import Path
from PIL import Image, ImageDraw


ROOT = Path(__file__).resolve().parents[1]


def create_icon(size: int, destination: Path) -> None:
    scale = 4
    canvas_size = size * scale
    image = Image.new("RGBA", (canvas_size, canvas_size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)
    def p(value: float) -> int:
        return round(value * canvas_size / 256)

    draw.rounded_rectangle((p(8), p(8), p(248), p(248)), radius=p(64), fill=(39, 117, 230, 255))
    draw.rounded_rectangle((p(10), p(10), p(246), p(246)), radius=p(62), outline=(255, 255, 255, 55), width=max(1, p(4)))
    draw.ellipse((p(-23), p(53), p(129), p(205)), outline=(255, 255, 255, 70), width=max(1, p(5)))
    draw.ellipse((p(146), p(-20), p(276), p(110)), outline=(255, 255, 255, 62), width=max(1, p(5)))
    links = [((73, 77), (128, 128)), ((128, 128), (193, 93)), ((128, 128), (83, 187)), ((128, 128), (189, 181))]
    for start, end in links:
        draw.line((p(start[0]), p(start[1]), p(end[0]), p(end[1])), fill=(255, 255, 255, 185), width=max(2, p(7)))
    for x, y in ((73, 77), (193, 93), (83, 187), (189, 181)):
        draw.ellipse((p(x - 14), p(y - 14), p(x + 14), p(y + 14)), fill=(255, 255, 255, 255))
    draw.ellipse((p(104), p(104), p(152), p(152)), fill=(161, 243, 255, 255))
    draw.ellipse((p(115), p(115), p(141), p(141)), fill=(255, 255, 255, 255))
    clip = Image.new("L", (canvas_size, canvas_size), 0)
    ImageDraw.Draw(clip).rounded_rectangle((p(8), p(8), p(248), p(248)), radius=p(64), fill=255)
    image.putalpha(clip)
    image = image.resize((size, size), Image.Resampling.LANCZOS)
    destination.parent.mkdir(parents=True, exist_ok=True)
    image.save(destination, "PNG", optimize=True)


create_icon(64, ROOT / "packaging" / "fnos" / "ICON.PNG")
create_icon(256, ROOT / "packaging" / "fnos" / "ICON_256.PNG")
create_icon(64, ROOT / "packaging" / "fnos" / "ui" / "images" / "64.png")
create_icon(256, ROOT / "packaging" / "fnos" / "ui" / "images" / "256.png")
