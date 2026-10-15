#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image="${SMOKE_IMAGE:-battery-backtest:smoke}"
artifacts="${SMOKE_ARTIFACT_DIR:-$(mktemp -d)}"
mkdir -p "$artifacts"
container=""
cleanup() {
  if [[ -n "$container" ]]; then
    docker logs "$container" >"$artifacts/container.log" 2>&1 || true
    docker rm -f "$container" >/dev/null
  fi
}
trap cleanup EXIT
docker build -t "$image" "$root"
container="$(docker run -d --read-only --cap-drop ALL --security-opt no-new-privileges -p 127.0.0.1::8080 "$image")"
port="$(docker port "$container" 8080/tcp)"
base="http://$port"
for ((attempt=0; attempt<40; attempt++)); do
  if curl -fsS "$base/health" >"$artifacts/health.json" 2>/dev/null; then break; fi
  sleep 1
done
curl -fsS "$base/health" >"$artifacts/health.json"
curl -fsS "$base/" >"$artifacts/index.html"
curl -fsS "$base/api/v1/batteries" >"$artifacts/batteries.json"
curl -fsS "$base/api/v1/strategies" >"$artifacts/strategies.json"
curl -fsS "$base/api/v1/locations?dataset_id=caiso_lmp_real_time_5_min" >"$artifacts/locations.json"
asset="$(python3 - "$artifacts/index.html" <<'PY'
import pathlib, re, sys
html = pathlib.Path(sys.argv[1]).read_text()
match = re.search(r'src="(/assets/[^\"]+\.js)"', html)
assert match, 'built JavaScript asset missing'
print(match.group(1))
PY
)"
curl -fsS "$base$asset" >"$artifacts/bundle.js"
status="$(curl -sS -o "$artifacts/invalid.json" -w '%{http_code}' -H 'Content-Type: application/json' -H 'X-GridStatus-API-Key: container-smoke-secret' --data '{}' "$base/api/v1/backtest")"
[[ "$status" == 400 ]]
status="$(curl -sS -o "$artifacts/legacy.json" -w '%{http_code}' "$base/api/v1/rank?api_key=container-smoke-secret")"
[[ "$status" == 400 ]]
status="$(curl -sS -o "$artifacts/removed.json" -w '%{http_code}' "$base/api/v1/backtest/old/ledger")"
[[ "$status" == 404 ]]
python3 - "$artifacts" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
assert json.loads((root/'health.json').read_text()) == {'status': 'ok'}
assert len(json.loads((root/'batteries.json').read_text())['batteries']) > 0
assert {s['name'] for s in json.loads((root/'strategies.json').read_text())['strategies']} == {'schedule', 'oracle'}
assert 'locations' in json.loads((root/'locations.json').read_text())
for name in ['invalid', 'legacy', 'removed']:
    raw = (root/f'{name}.json').read_text()
    assert 'container-smoke-secret' not in raw
    assert json.loads(raw)['error']['code']
PY
[[ "$(docker exec "$container" id -u)" == 1000 ]]
docker exec "$container" sh -c 'test ! -e /app/.env && test ! -e /app/.git && test ! -e /app/.agent-outputs && test ! -e /app/testdata && test ! -e /app/node_modules && test ! -e /app/web/src'
docker logs "$container" >"$artifacts/container.log" 2>&1
if grep -q 'container-smoke-secret' "$artifacts/container.log"; then
  echo 'Credential appeared in container logs' >&2
  exit 1
fi
printf 'Container health, UI/assets, metadata, safe errors, nonroot read-only runtime: PASS\nEvidence: %s\n' "$artifacts"
