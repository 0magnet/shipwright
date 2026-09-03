#!/bin/sh
# Harvest the js/wasm std-package archives a hello-world needs, from the host
# build cache, into pkg/<importpath>.a — plus the two importcfg files the
# in-tab compiler and linker read. Run after: (cd hello && GOOS=js GOARCH=wasm go build .)
set -eu
cd "$(dirname "$0")"
rm -rf pkg importcfg importcfg.link
mkdir -p pkg
: > importcfg
: > importcfg.link
while IFS== read -r ip file; do
	[ "$ip" = "hello" ] && continue
	dest="pkg/$ip.a"
	mkdir -p "$(dirname "$dest")"
	cp "$file" "$dest"
	printf 'packagefile %s=/pkg/%s.a\n' "$ip" "$ip" >> importcfg
	printf 'packagefile %s=/pkg/%s.a\n' "$ip" "$ip" >> importcfg.link
done < deps.txt
du -sh pkg
wc -l importcfg importcfg.link
