#!/usr/bin/env python3
"""tblLevel3.txt (WGSRPD 2nd Edition) -> wgsrpd_areas.csv.

The source is a `*`-separated, CRLF-terminated, cp1252-encoded export of the
TDWG geography database ("Føroyar", "Galápagos" carry the non-ASCII bytes).
Only level 3 is emitted: situs stores exactly one area scheme (wgsrpd_l3),
and the level 1/2 hierarchy has no reader yet.

Writes a report.json next to the CSV with counted, not assumed, figures.
"""
import csv
import json
import os
import sys

SCHEME = "wgsrpd_l3"
EXPECTED_HEADER = ["L3 code", "L3 area", "L2 code", "L3 ISOcode", "Ed2status", "Notes"]


def rows(path):
    with open(path, encoding="cp1252", newline="") as f:
        for line in f:
            line = line.rstrip("\r\n")
            if line:
                yield line.split("*")


def main():
    in_path, out_path = sys.argv[1:3]

    it = rows(in_path)
    header = next(it)
    if header[: len(EXPECTED_HEADER)] != EXPECTED_HEADER:
        sys.exit(f"unexpected header in {in_path}: {header!r}")

    written, skipped = 0, 0
    seen = set()
    with open(out_path, "w", encoding="utf-8", newline="") as outf:
        w = csv.writer(outf)
        w.writerow(["area_scheme", "area_code", "name_en"])
        for row in it:
            # A truncated line is what `skipped` is for — indexing it blindly
            # would abort the whole run over one malformed record.
            if len(row) < 2:
                skipped += 1
                continue
            code = row[0].strip()
            name = row[1].strip()
            if not code or not name or code in seen:
                skipped += 1
                continue
            seen.add(code)
            w.writerow([SCHEME, code, name])
            written += 1

    report = {"source": os.path.basename(in_path), "scheme": SCHEME,
              "areas": written, "skipped_rows": skipped}
    with open(os.path.join(os.path.dirname(out_path), "report.json"), "w", encoding="utf-8") as rf:
        json.dump(report, rf, indent=2, ensure_ascii=False)
        rf.write("\n")
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()
