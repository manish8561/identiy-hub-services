#!/usr/bin/env bash
set -uo pipefail

services=(auth-service user-service notification-service api-gateway)
severity="UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"
trivy_image="${TRIVY_IMAGE:-aquasec/trivy:latest}"
trivy_cache_volume="${TRIVY_CACHE_VOLUME:-backend-trivy-cache}"
scan_id="trivy-$$-${RANDOM}"
failed=0
images=()

usage() {
  echo "usage: $0 [all|fs|image]" >&2
}

mode="${1:-all}"
case "$mode" in
  all|fs|image) ;;
  *)
    usage
    exit 2
    ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  echo "error: Docker is required to run Trivy from the ${trivy_image} container" >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "error: the Docker daemon is unavailable; start Docker before scanning" >&2
  exit 1
fi
if ! docker pull "$trivy_image"; then
  echo "error: could not pull Trivy image ${trivy_image}" >&2
  exit 1
fi

backend_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
trivy_ignore_file="${backend_dir}/.trivyignore"

trivy_fs() {
  local service_dir="$1"
  shift
  docker run --rm \
    --mount "type=volume,source=${trivy_cache_volume},target=/root/.cache/" \
    --mount "type=bind,source=${service_dir},target=/src,readonly" \
    --mount "type=bind,source=${trivy_ignore_file},target=/etc/trivy/.trivyignore,readonly" \
    --workdir /src \
    "$trivy_image" fs --ignorefile /etc/trivy/.trivyignore "$@"
}

trivy_image_scan() {
  docker run --rm \
    --mount "type=volume,source=${trivy_cache_volume},target=/root/.cache/" \
    --mount "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock" \
    --mount "type=bind,source=${trivy_ignore_file},target=/etc/trivy/.trivyignore,readonly" \
    "$trivy_image" image --ignorefile /etc/trivy/.trivyignore "$@"
}

cleanup() {
  for image in "${images[@]}"; do
    if ! docker image rm "$image" >/dev/null; then
      echo "warning: could not remove temporary scan image $image" >&2
    fi
  done
}

if [[ "$mode" == "all" || "$mode" == "fs" ]]; then
  for service in "${services[@]}"; do
    echo "Scanning ${service} Go dependencies with Trivy..."
    if ! trivy_fs "${backend_dir}/${service}" \
      --scanners vuln --severity "$severity" --exit-code 1 --format table .; then
      failed=1
    fi
  done
fi

if [[ "$mode" == "all" || "$mode" == "image" ]]; then
  trap cleanup EXIT
  for service in "${services[@]}"; do
    image="backend-trivy/${service}:${scan_id}"

    echo "Building ${service} image for Trivy..."
    if ! docker build --pull --no-cache --file "${backend_dir}/${service}/Dockerfile" --tag "$image" "${backend_dir}/${service}"; then
      echo "error: could not build ${service} image for Trivy scanning" >&2
      failed=1
      continue
    fi
    images+=("$image")

    echo "Scanning ${service} image with Trivy..."
    if ! trivy_image_scan \
      --scanners vuln --severity "$severity" --exit-code 1 --format table "$image"; then
      failed=1
    fi
  done
fi

exit "$failed"
