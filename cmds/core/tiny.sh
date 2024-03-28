#!/usr/bin/env bash

for d in */;do
    if [[ -d "$d" ]]; then
        tinygoize "$d"
    fi
done
