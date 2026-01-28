# Battery Backtest UI

React + TypeScript frontend for the Battery Backtest API.

## Setup

```bash
cd web
npm install
```

## Development

1. Make sure the API server is running:
   ```bash
   # In the project root
   go run ./cmd/api
   ```

2. Start the frontend dev server:
   ```bash
   npm run dev
   ```

The UI will be available at http://localhost:5173

The frontend is configured to proxy API requests to http://localhost:8080 (see `vite.config.ts`).

## Build

```bash
npm run build
```

The built files will be in the `dist` directory and can be served by any static file server.

## Features

- **API Key Input**: Secure input field with localStorage persistence
- **Comprehensive Backtest Form**: 
  - Data source configuration (dataset, location, dates, timezone)
  - Battery configuration (preset selector or custom inputs)
  - Strategy configuration with dynamic parameters
  - Backtest options (limit intervals, include ledger)
- **Large Summary Metrics**: Prominent display of key metrics (Total P&L, Final SOC, Energy, etc.)
- **Postman-style JSON Viewer**: Expandable/collapsible JSON response with syntax highlighting
- **Charge/Discharge Bar Chart**: 
  - 30-minute interval grouping
  - Charge bars above x-axis, discharge bars below
  - Time on x-axis
- **Cumulative P&L Line Chart**: Shows profit accumulation over time
- **Per-Day Windows Display**: Shows charge/discharge windows for each day with average costs/prices

## Requirements

- Node.js 18+ and npm
- API server running on port 8080 (or configure via `VITE_API_URL` environment variable)
