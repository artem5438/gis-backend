#!/bin/sh
set -eu

i=0
until mc alias set local "${MINIO_ENDPOINT}" "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}"; do
  i=$((i + 1))
  if [ "$i" -ge 30 ]; then
    echo "minio is not ready" >&2
    exit 1
  fi
  sleep 1
done

mc mb --ignore-existing "local/${MINIO_BUCKET}"
echo "bucket ${MINIO_BUCKET} ready"
