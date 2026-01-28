# Quick Start Guide

## Prerequisites

- Node.js 18+ and npm
- Go API server running (see main README)

## Setup

1. Install dependencies:
   ```bash
   cd web
   npm install
   ```

2. Start the API server (in project root):
   ```bash
   go run ./cmd/api
   ```

3. Start the frontend dev server:
   ```bash
   npm run dev
   ```

4. Open http://localhost:5173 in your browser

## First Steps

1. **Enter your API Key**: 
   - Get your Grid Status API key from https://gridstatus.io
   - Enter it in the API key input field at the top
   - It will be saved in your browser's localStorage

2. **Configure Backtest**:
   - Select or enter dataset ID (e.g., "caiso_lmp_real_time_5_min")
   - Enter location ID (e.g., "TH_NP15_GEN-APND")
   - Choose date range
   - Select battery preset or configure custom battery
   - Choose strategy and configure parameters
   - Enable "Include Ledger" to see charts (recommended)

3. **Run Backtest**:
   - Click "Run Backtest"
   - Wait for results (may take a few seconds)

4. **View Results**:
   - Large summary metrics at the top
   - Per-day charge/discharge windows
   - Charge/Discharge bar chart (30-minute intervals)
   - Cumulative P&L line chart
   - Full JSON response viewer

## Features

- **API Key Management**: Stored locally, never sent to server except in API requests
- **Dynamic Forms**: Strategy parameters change based on selected strategy
- **Interactive Charts**: Hover for details, zoom, pan
- **JSON Viewer**: Expand/collapse, copy to clipboard
- **Responsive Design**: Works on desktop and mobile
