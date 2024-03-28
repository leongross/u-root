#!/usr/bin/env python

import sys
from pathlib import Path
import os
import subprocess
import csv

LOGFILE = "tinygo.csv"


def write_error_csv(errordict: dict, logfile: str):
    with open(logfile, 'w', newline='') as file:
        writer = csv.writer(file)
        print(f"Writing errors to {Path(logfile).resolve()}")
        field = ["subcommand", "error"]

        writer.writerow(field)
        for key, value in errordict.items():
            writer.writerow([key, value])


def main():
    errors: dict = {}

    if len(sys.argv) != 2:
        print("Usage: buildall.py <base_dir>")
        sys.exit(1)

    base_dir = Path(sys.argv[1])

    for d in base_dir.iterdir():
        if d.is_dir():
            d = d.resolve()
            print(d)

            os.chdir(d)

            ret = subprocess.run(["tinygo", "build", str(
                d)], check=False, capture_output=True, text=True, env={"CGO_ENABLED": "0"})

            if ret.returncode != 0:
                print(f"[-] Error building {d}: {ret.stderr}")
                errors[d.name] = ret.stderr

    if errors:
        print(f"{len(errors)} tinygo build errors found.")
        os.chroot(base_dir)
        write_error_csv(errors, LOGFILE)


if __name__ == "__main__":
    main()
