#!/bin/sh
# Rebuilds the v2 dashboard from the v1 one using Grafana's own conversion,
# so the two files cannot drift apart by hand.
set -e

cd "$(dirname "$0")"
src=solis-http-exporter.json
out=solis-http-exporter.v2.json
uid=solis-http-exporter
port=13099
image=${GRAFANA_IMAGE:-grafana/grafana:latest}

docker rm -f solis-convert >/dev/null 2>&1 || true
docker run -d --name solis-convert -p "$port:3000" -e GF_LOG_LEVEL=error "$image" >/dev/null
trap 'docker rm -f solis-convert >/dev/null 2>&1' EXIT

api="http://admin:admin@127.0.0.1:$port"
curl -s --retry 60 --retry-all-errors --retry-delay 1 -o /dev/null "$api/api/health"

jq --arg uid "$uid" '{dashboard: (. + {id: null, uid: $uid}), overwrite: true}' "$src" |
	curl -s -X POST -H 'Content-Type: application/json' -d @- "$api/api/dashboards/db" >/dev/null

curl -s "$api/apis/dashboard.grafana.app/v2/namespaces/default/dashboards/$uid" |
	jq --arg uid "$uid" 'del(.status) | .metadata = {name: $uid}' >"$out"

echo "wrote $out ($(jq '.spec.elements | length' "$out") panels)"
