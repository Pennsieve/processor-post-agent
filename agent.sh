#!/bin/sh

pennsieve agent
pennsieve whoami
pennsieve dataset use $1

ls -alh $3
timestamp=$(date +%Y%m%d_%H%M%S%Z)
target_path="${TARGET_PATH:-"output-$timestamp-$2"}"

pennsieve manifest create $3 -t $target_path
pennsieve manifest list 1
pennsieve upload manifest 1