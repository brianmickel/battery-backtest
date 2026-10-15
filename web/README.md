# Battery Backtest interface

React 19, TypeScript, and Vite provide the editor and result views. Go owns configuration defaults, physical execution, and market-data access. See the [quickstart](QUICKSTART.md) for a runnable synthetic example and the [HTTP contract](../API_DOCUMENTATION.md) for wire fields.

Use Node 24:

```sh
npm ci
npm run dev
```

Run `go run ./cmd/api` from the repository root in another terminal. Vite serves port 5173 and proxies `/api` to the Go server on port 8080. Production builds use same-origin API requests.

```sh
npm test -- --run
npm run typecheck
npm run lint
npm run build
npx playwright install chromium
npm run test:e2e
```

Browser tests need Go and its downloaded modules. The system suite builds a temporary local Go server, serves `dist`, and exercises the production router with a local synthetic HTTP provider. Other editor tests stub responses deliberately to isolate editor behavior. Playwright starts its own Vite server on port 4173; leave that port free. Reports and traces go under `playwright-report` and `test-results`.

Request JSON has no credentials. The separate key input stores the key in this browser and sends it in `X-GridStatus-API-Key`. Valid legacy JSON imports move `api_key` into that input and remove it from the draft. Invalid imports commit neither draft nor key.
