#!/bin/sh
# Compress the man pages in man/ into man/*.gz for the Linux packages.
#
# nfpm copies files as they are, and Debian policy wants gzipped man pages.
# -n leaves the file name and timestamp out of the archive, so the output is
# reproducible. The output cannot go to dist/: goreleaser runs the before hooks
# first and refuses to continue if dist/ already has files in it.
set -e

rm -f man/*.gz

for page in man/*.[0-9]; do
    gzip -9nc "$page" > "$page.gz"
done
