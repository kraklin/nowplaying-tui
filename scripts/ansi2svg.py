#!/usr/bin/env python3
"""Turn a true-colour ANSI frame (as written by TestWriteSnapshots) into an SVG.

usage: ansi2svg.py frame.ansi out.svg [--bg #rrggbb] [--fg #rrggbb]

Half-block cells (▀ ▄) are drawn as rectangles so cover art renders exactly;
every other character is placed on the cell grid one by one, so the picture
does not depend on the viewer's font metrics.
"""
import re
import sys
import unicodedata
from xml.sax.saxutils import escape

CW, CH = 9, 18  # cell size in px
SGR = re.compile(r"\x1b\[([0-9;]*)m")


def width(ch):
    if unicodedata.combining(ch) or ch in "︎️":
        return 0
    return 2 if unicodedata.east_asian_width(ch) in "WF" else 1


def parse(frame, default_fg):
    rows = []
    for line in frame.split("\n"):
        fg, bg, bold = None, None, False
        cells, x = [], 0
        pos = 0
        while pos < len(line):
            m = SGR.match(line, pos)
            if m:
                p = [int(v) if v else 0 for v in m.group(1).split(";")]
                i = 0
                while i < len(p):
                    if p[i] == 0:
                        fg, bg, bold = None, None, False
                    elif p[i] == 1:
                        bold = True
                    elif p[i] == 22:
                        bold = False
                    elif p[i] == 39:
                        fg = None
                    elif p[i] == 49:
                        bg = None
                    elif p[i] in (38, 48) and i + 4 < len(p) and p[i + 1] == 2:
                        c = "#%02x%02x%02x" % tuple(p[i + 2 : i + 5])
                        if p[i] == 38:
                            fg = c
                        else:
                            bg = c
                        i += 4
                    i += 1
                pos = m.end()
                continue
            ch = line[pos]
            pos += 1
            w = width(ch)
            if w == 0:
                continue
            cells.append((x, ch, fg or default_fg, bg, bold))
            x += w
        rows.append(cells)
    return rows


def main():
    args = sys.argv[1:]
    opts = {"--bg": "#1e1e2e", "--fg": "#cdd6f4"}
    pos = []
    while args:
        a = args.pop(0)
        if a in opts:
            opts[a] = args.pop(0)
        else:
            pos.append(a)
    src, dst = pos
    frame = open(src, encoding="utf-8").read().rstrip("\n")
    rows = parse(frame, opts["--fg"])

    used = [(y, c[0]) for y, r in enumerate(rows) for c in r if c[1] != " " or c[3]]
    y0, y1 = min(y for y, _ in used), max(y for y, _ in used)
    x0, x1 = min(x for _, x in used), max(x for _, x in used)
    pad = 2
    cols, nrows = x1 - x0 + 1 + 2 * pad, y1 - y0 + 1 + 2 * pad
    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{cols * CW}" height="{nrows * CH}" '
        f'viewBox="0 0 {cols * CW} {nrows * CH}" role="img">',
        f'<rect width="100%" height="100%" rx="10" fill="{opts["--bg"]}"/>',
        '<g font-family="SF Mono, Menlo, Consolas, DejaVu Sans Mono, monospace" font-size="14">',
    ]
    for y, cells in enumerate(rows[y0 : y1 + 1]):
        for x, ch, fg, bg, bold in cells:
            px, py = (x - x0 + pad) * CW, (y + pad) * CH
            if ch == "▀":
                out.append(f'<rect x="{px}" y="{py}" width="{CW}" height="{CH // 2}" fill="{fg}"/>')
                if bg:
                    out.append(f'<rect x="{px}" y="{py + CH // 2}" width="{CW}" height="{CH // 2}" fill="{bg}"/>')
            elif ch == "▄":
                out.append(f'<rect x="{px}" y="{py + CH // 2}" width="{CW}" height="{CH // 2}" fill="{fg}"/>')
                if bg:
                    out.append(f'<rect x="{px}" y="{py}" width="{CW}" height="{CH // 2}" fill="{bg}"/>')
            else:
                if bg:
                    out.append(f'<rect x="{px}" y="{py}" width="{CW}" height="{CH}" fill="{bg}"/>')
                if ch != " ":
                    weight = ' font-weight="bold"' if bold else ""
                    out.append(
                        f'<text x="{px + CW / 2}" y="{py + CH * 0.74}" text-anchor="middle" fill="{fg}"{weight}>{escape(ch)}</text>'
                    )
    out += ["</g>", "</svg>"]
    open(dst, "w", encoding="utf-8").write("\n".join(out) + "\n")


if __name__ == "__main__":
    main()
