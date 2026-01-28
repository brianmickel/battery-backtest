# Battery Arbitrage Backtesting Engine

[BUILT WITH CURSOR]

## What I Learned

Prior to this project, I was adamant that battery energy storage systems are now cheap enough that they are the future. We can solve our generation and demand misalignment with short term storage. I thought that private equity would flood the market with battery installations to arbitrage the energy between midday and evening ramp.

While I still believe that batteries are the future, I now understand that the payoff for simple arbitrage (within CAISO) would take hundreds of years--Texas ERCOT may be volatile enough to pay off sooner. It seems batteries make much more money from keeping the voltage up. Coal and Gas provide inertia with spinning metal; solar doesn't. Batteries make much more money on the sub-second voltage maintainence.

I knew that Hornsdale in Australia cost around 150M and paid for itself within 2 years. I now know this isn't simply arbitrage. We are also seeing the spread decrease as more batteries are added to the grid.

Here are some fun tests I ran:
## Moss Landing
```
$ go run ./cmd/cli backtest --data sample_data2.json --config examples/schedule_config.yaml --out results/dispatch.csv --n 288
Wrote 288 rows to results/dispatch.csv
Total PnL=$17992.22 Final SOC=0.100
Backtest window: 2026-01-18 00:00 → 2026-01-19 00:00
Charged from grid: 2553.19 MWh (2026-01-18 11:00 → 2026-01-18 14:25)
Discharged to grid: 2256.00 MWh (2026-01-18 16:00 → 2026-01-18 19:05)
```
- Estimated Yearly Profit: `$6.6M/yr` (from backtest)
- Estimated Cost: `$1B`
- Payoff Time: `154 years`

## Flagstaff
What could a 100MW battery near flagstaff do?
```
$ go run ./cmd/cli backtest --data sample_data.json --config examples/schedule_config.yaml --out results/dispatch.csv --n 288 
Wrote 288 rows to results/dispatch.csv
Total PnL=$469.43 Final SOC=0.200
Backtest window: 2026-01-18 00:00 → 2026-01-19 00:00
Charged from grid: 63.16 MWh (2026-01-18 11:00 → 2026-01-18 11:40)
Discharged to grid: 57.00 MWh (2026-01-18 16:00 → 2026-01-18 16:35)
```
- Estimated Yearly Profit: `$171k/yr` (from backtest)
- Estimated Cost: `$30M`
- Payoff Time: `171 years`

## Some Numbers I learned
- $300k per kwh installed

### Summary
A backtesting system for battery trading strategies that simulates energy arbitrage operations using historical electricity price data along with constraints of the battery system.


**Goal:** given historical LMP price series for a node/zone, simulate a battery’s charge/discharge decisions and compute **profit and utilization**

**Keywords:** time-series handling, optimization vs heuristics, constraint modeling, performance, api design, and testing

### Features
- Configure battery system specification and market rules
- Simulate charge/discharge decisions across price curves
- Multiple strategy implementations (schedule-based, oracle optimizer)
- Strategy comparison and parameter sweeps
- Comprehensive metrics: ROI, degradation costs, utilization, cycles, throughput
- Visualization of SOC, prices, and dispatch decisions

### Significance
- Storage is becoming a critical trading asset in energy markets
- Built on top of Grid Status apis

---

## Technology Stack

### Backend
- **Go 1.21+** - Core language
- **Gin** - HTTP web framework
- **YAML** - Configuration parsing
- **Grid Status API** - Market data source

### Frontend
- **React 18** - UI framework
- **TypeScript** - Type safety
- **Vite** - Build tool
- **Recharts** - Data visualization
- **React Query** - Server state management

### Deployment
- **Docker** - Containerization
- **Alpine Linux** - Minimal base image

## Documentation

- [API Documentation](./API_DOCUMENTATION.md) - Complete API reference
- [Docker Deployment](./README_DOCKER.md) - Docker setup and deployment guide
- [Contributing Guidelines](./CONTRIBUTING.md) - How to contribute to the project
- [Frontend README](./web/README.md) - Frontend development guide

## License

This project is licensed under the MIT License - see the [LICENSE](./LICENSE) file for details.

## Acknowledgments

- Built using the [Grid Status API](https://gridstatus.io) for market data
- Inspired by real-world battery energy storage systems and their role in modern energy markets

---

## Installation & Setup

### Prerequisites
- Go 1.21 or higher

## Quick Start

### Prerequisites

- Go 1.21 or higher
- Node.js 18+ and npm (for frontend)
- Grid Status API key (get one at [gridstatus.io](https://gridstatus.io))

### Installation

```bash
# Clone the repository
git clone https://github.com/your-username/battery-backtest.git
cd battery-backtest

# Install Go dependencies
go mod tidy

# Install frontend dependencies
cd web && npm install && cd ..
```

### Running the API Server

```bash
# Start the API server
go run ./cmd/api

# Server will be available at http://localhost:8080
```

### Running the Web UI

```bash
# In a separate terminal
cd web
npm run dev

# UI will be available at http://localhost:5173
```

### Using Docker

```bash
# Build and run with Docker Compose
docker-compose up -d

# Or build manually
docker build -t battery-backtest .
docker run -p 8080:8080 battery-backtest
```

### CLI Usage

```bash
# Run a backtest (requires sample data file)
mkdir -p results
go run ./cmd/cli backtest --data sample_data.json --config examples/config.yaml --out results/dispatch.csv --n 288

# Rank nodes by arbitrage potential
go run ./cmd/cli rank --data sample_data.json
```

### Using the example batteries

You can point a config at one of the example batteries via `battery_file`:

```yaml
battery_file: examples/batteries/1_moss_landing.yaml

# Optional overrides:
battery:
  initial_soc: 0.20

strategy:
  name: threshold
  params:
    low_threshold: 15.0
    high_threshold: 60.0
```

There’s a ready-to-run example at `examples/moss_landing_threshold.yaml`.

### If you hit Go cache permission errors

Some environments block writing to the default Go build/module caches. You can run with local caches inside the repo:

```bash
mkdir -p .gocache .gopath
GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" GOMODCACHE="$PWD/.gopath/pkg/mod" go mod tidy
GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" GOMODCACHE="$PWD/.gopath/pkg/mod" go run ./cmd/cli backtest --data sample_data.json --config examples/config.yaml --out results/dispatch.csv --n 288
```

### Docker Setup (Alternative)

```bash
# Build and run with Docker
docker-compose up --build

# Or use Dockerfile directly
docker build -t battery-backtest .
docker run -v $(pwd)/examples:/app/examples -v $(pwd)/results:/app/results battery-backtest \
  --prices examples/prices.csv --strategy lookahead --config examples/battery_config.yaml --out results/
```

---

## What the engine does

### Inputs

* **Price series**: timestamps + LMP ($/MWh) for a node 
* **Battery model**:

  * Energy capacity (MWh), power limit (MW)
  * Round-trip efficiency (or charge/discharge efficiency separately)
  * Initial SOC, min/max SOC
  * Optional: cycle limit/day, degradation cost ($/MWh throughput)
* **Market rules / modeling assumptions**:

  * Time resolution (5-min, 15-min, hourly)
  * Whether you can charge/discharge in same interval (usually no)
  * Dispatch latency / ramp (optional, can be a documented simplification)
* **Strategy config**:

  * “Perfect foresight” optimizer OR rule-based strategy with tunable params
  * Risk controls: max cycles/day, min spread threshold, etc.

### Outputs

* **Ledger / dispatch trace** per interval:

  * action: **CHARGING / IDLE / DISCHARGING**
  * power (MW), energy moved (MWh)
  * SOC
  * interval revenue, cumulative PnL
* **Summary metrics**:

  * total profit, profit per MW / per MWh
  * capacity factor / utilization
  * cycles, throughput, efficiency losses
  * max drawdown (optional but nice)
* **Comparisons**:

  * run multiple strategies or param sweeps and compare.

---

## Strategies to implement (choose 2–3)

### 1) Daily schedule strategy (time-based)

This is a “calendar rule” strategy: **start charging at a specific power at 10:00**, then **start discharging at a specific power at 17:00**, repeating every day (using the dataset’s `interval_start_local` timestamps).

**Behavior (in the demo):**
- At `charge_start` (e.g. `10:00`), the mode flips to **charge** and stays charging each interval until the next trigger.
- At `discharge_start` (e.g. `17:00`), the mode flips to **discharge** and stays discharging each interval until the next trigger.
- The battery model still enforces **power limits** and **SOC bounds**, so if you hit `min_soc` or `max_soc`, realized power will be clipped.

**How to test it:**

```bash
go run ./cmd/demo --data sample_data.json --config examples/schedule_config.yaml --n 576
```

`examples/schedule_config.yaml` contains:

```yaml
strategy:
  name: schedule
  params:
    charge_start: "10:00"
    charge_power_mw: 30.0
    discharge_start: "17:00"
    discharge_power_mw: 30.0
```

### 2) Lookahead heuristic (rolling window)

* For each interval, look ahead N hours
* If current price is among lowest X% in window ⇒ charge
* If among highest X% ⇒ discharge
* Otherwise idle

**Pros:** simple, explainable, and effective for predictable daily patterns.
**Cons:** sensitive to N / percentiles.

### 3) “Perfect foresight” optimizer (finite-horizon)

Formulate as linear optimization:
Maximize Σ (p_t * discharge_t − p_t * charge_t − degradation_cost * throughput)

Subject to:

* SOC_{t+1} = SOC_t + η_c * charge_t * Δt − (1/η_d) * discharge_t * Δt
* 0 ≤ charge_t ≤ Pmax
* 0 ≤ discharge_t ≤ Pmax
* SOCmin ≤ SOC_t ≤ SOCmax
* Optionally: charge_t * discharge_t = 0 (nonlinear)
  Practical simplification: allow both but penalize simultaneous behavior heavily, or enforce “either/or” via a binary variable (MILP). For this project, it’s fine to **document** whichever you choose.

**Pros:** shows deep tradeoffs, gives an “upper bound” on profits.
**Cons:** more complex; MILP adds dependency/complexity.

**Implementation approach:** The optimizer uses LP (not MILP) + adds a rule to prevent simultaneous charge/discharge in post-processing. This approach is simpler, faster, and sufficient for backtesting purposes while maintaining realistic dispatch behavior.

---

## Implementation Overview

### Project Scope

This project implements a complete battery arbitrage backtesting system with:

1. A **core library**: battery model + backtest runner + strategies
2. A **CLI**: run a backtest with config file + output CSV/JSON
3. A **simple visualization**: generate a PNG or HTML chart (SOC + prices + actions)
4. **Tests**: battery constraints and a tiny synthetic price series test  

---

## Project Architecture

### Modules

* `data/`

  * `loader.go` (CSV loader + validation + resampling)
  * optional: adapter for Grid Status API (if you want)
* `battery/`

  * `model.go` (BatteryParams, BatteryState structs)
  * `constraints.go` (clamping, energy accounting)
* `strategies/`

  * `threshold.go`
  * `lookahead.go`
  * `optimizer.go` (LP-based “oracle”)
* `backtest/`

  * `engine.go` (runs simulation loop, records ledger)
  * `metrics.go` (PnL, cycles, throughput, drawdown)
* `viz/`

  * `plot.go` (gonum/plot visualization)
* `cmd/cli/main.go` (CLI entry point)
* `README.md`
* `tests/`

### Key interfaces

* `Strategy.Decide(state, price, history, index) *DispatchDecision`
* `BacktestEngine.Run(prices, strategy, params, initialSOC) (*BacktestResult, error)`
* `BacktestResult.ToCSV(path) error` / `BacktestResult.SummaryJSON() ([]byte, error)`

This is clean, discussable, and extensible.

---

## Minimal battery math (the “engine”)

At each timestep:

* decision: `charge_mw`, `discharge_mw`
* energy moved: `charge_mwh = charge_mw * dt`, `discharge_mwh = discharge_mw * dt`
* SOC update:

  * `soc += charge_mwh * η_c`
  * `soc -= discharge_mwh / η_d`
* revenue:

  * `pnl += price * (discharge_mwh - charge_mwh) - degradation_cost * (charge_mwh + discharge_mwh)`

Enforce:

* `0 <= charge_mw, discharge_mw <= Pmax`
* `SOCmin <= soc <= SOCmax` (clamp by reducing power)

---


### CLI Usage Examples

**Basic backtest:**
```bash
./battery-backtest \
  --prices examples/prices.csv \
  --strategy schedule \
  --config examples/battery_config.yaml \
  --out results/
```

**With visualization:**
```bash
./battery-backtest \
  --prices examples/prices.csv \
  --strategy schedule \
  --config examples/battery_config.yaml \
  --out results/ \
  --plot
```

**Compare multiple strategies:**
```bash
for strategy in schedule oracle; do
  ./battery-backtest \
    --prices examples/prices.csv \
    --strategy $strategy \
    --config examples/battery_config.yaml \
    --out results/${strategy}/
done
```

**Or run directly with go run:**
```bash
go run ./cmd/cli \
  --prices examples/prices.csv \
  --strategy schedule \
  --config examples/battery_config.yaml \
  --out results/
```

**Output files:**
- `dispatch.csv` - Per-interval ledger with columns:
  - `timestamp`, `price`, `action`, `charge_mw`, `discharge_mw`, 
  - `soc`, `pnl`, `cumulative_pnl`, `throughput_mwh`
- `summary.json` - Aggregated metrics:
  ```json
  {
    "total_profit": 1234.56,
    "profit_per_mw": 12.34,
    "profit_per_mwh": 1.23,
    "capacity_factor": 0.45,
    "utilization": 0.67,
    "total_cycles": 12.5,
    "total_throughput_mwh": 1000.0,
    "efficiency_losses_mwh": 100.0,
    "max_drawdown": -50.0
  }
  ```
- `plot.png` - Multi-panel visualization (if `--plot` flag used)


---

SV_LNODER6A (Flagstaff)
MOSSLD_2_PSP1 (Moss Landing)

```
curl \
  "https://api.gridstatus.io/v1/datasets/caiso_lmp_real_time_5_min/query/location/?start_time=2026-01-18&\
end_time=2026-01-21&\
download=true&\
timezone=market" \
  -H "x-api-key: [redacted]" > sample_data.json
```

GET /v1/datasets/{dataset_id}/query/location/SV_LNODER6A HTTP/1.1
Host: api.gridstatus.io
Accept: */*

```
func (b *Battery) CalculateIntervalPnL(lmp float64, powerMW float64, durationHours float64) float64 {
    if powerMW < 0 { // Charging
        // Energy pulled from grid = (Power / Efficiency) * Time
        energyMWh := (math.Abs(powerMW) * durationHours) / b.Params.ChargeEfficiency
        return -(lmp * energyMWh) 
    } else if powerMW > 0 { // Discharging
        // Energy delivered to grid = (Power * Efficiency) * Time
        energyMWh := (powerMW * durationHours) * b.Params.DischargeEfficiency
        return lmp * energyMWh
    }
    return 0
}
```

Typical Battery Size	
LNODE (Load Node): 100kW – 10MW	
GNODE (Generation Node): 20MW – 500MW+

```
go run ./cmd/cli backtest --data sample_data2.json --config examples/schedule_config.yaml --out results/dispatch.csv --n 288
Wrote 288 rows to results/dispatch.csv
Total PnL=$897.22 Final SOC=0.100
```



290 views  Aug 4, 2025
Grid-scale batteries in CAISO earned an average of $4.05/kW-month from merchant markets in the first half of 2025. This represents a decrease of 7.7% from $4.39/kW-month over the same period last year.

However, average revenues from capacity payments rose by around $1/kW-month over the same time frame. This meant that all-in, wholesale average revenues for BESS in CAISO rose by 6% year-over-year.


303 views  Jun 9, 2025
Modo Energy’s projections indicate that batteries earned average monthly revenues of $5.4/kW in ERCOT in May 2025 - compared to just $3.8/kW in CAISO. Once settlement data is fully released in August through ERCOT’s 60-day delayed disclosure reports, it’s expected to be confirmed that batteries in Texas outperformed their Californian counterparts for the first time since August 2026.

Texas makes its money in just a few days e.g. heat wave

https://www.youtube.com/watch?v=yU4-APiMsaU



```
{
  "api_key": "[redacted]",
  "data_source": {
    "type": "gridstatus",
    "dataset_id": "caiso_lmp_real_time_5_min",
    "location_id": "TH_NP15_GEN-APND",
    "start_date": "2026-01-01",
    "end_date": "2026-01-07",
    "timezone": "market"
  },
  "config": {
    "battery_file": "examples/batteries/1_moss_landing.yaml",
    "battery": {
      "energy_capacity_mwh": 3000.0,
      "power_capacity_mw": 750.0,
      "charge_efficiency": 0.94,
      "discharge_efficiency": 0.94,
      "min_soc": 0.10,
      "max_soc": 0.90,
      "initial_soc": 0.10,
      "degradation_cost_per_mwh": 1.5
    },
    "strategy": {
      "name": "schedule",
      "params": {
        "charge_start": "11:00",
        "charge_end": "15:00",
        "discharge_start": "17:00",
        "discharge_end": "23:59",
        "charge_power_mw": 750.0,
        "discharge_power_mw": 750.0
      }
    }
  },
  "options": {
    "limit_intervals": 0,
    "include_ledger": false
  }
}
```

```
{
    "status": "completed",
    "summary": {
        "total_pnl": 6570.093537872994,
        "final_soc": 0.1,
        "total_intervals": 1728,
        "backtest_window": {
            "start": "2026-01-01T00:00:00-08:00",
            "end": "2026-01-07T00:00:00-08:00"
        },
        "energy_charged_mwh": 15319.148936170208,
        "energy_discharged_mwh": 13536.000000000002,
        "charge_windows": [
            {
                "start": "2026-01-01T11:00:00-08:00",
                "end": "2026-01-01T14:25:00-08:00",
                "average_cost_per_mwh": 38.012041656250005,
                "energy_mwh": 2553.1914893617013
            },
            {
                "start": "2026-01-02T11:00:00-08:00",
                "end": "2026-01-02T14:25:00-08:00",
                "average_cost_per_mwh": 54.56529599479166,
                "energy_mwh": 2553.1914893617013
            },
            {
                "start": "2026-01-03T11:00:00-08:00",
                "end": "2026-01-03T14:25:00-08:00",
                "average_cost_per_mwh": 54.810834484374986,
                "energy_mwh": 2553.1914893617013
            },
            {
                "start": "2026-01-04T11:00:00-08:00",
                "end": "2026-01-04T14:25:00-08:00",
                "average_cost_per_mwh": 39.68925248958333,
                "energy_mwh": 2553.1914893617013
            },
            {
                "start": "2026-01-05T11:00:00-08:00",
                "end": "2026-01-05T14:25:00-08:00",
                "average_cost_per_mwh": 48.04573207812499,
                "energy_mwh": 2553.1914893617013
            },
            {
                "start": "2026-01-06T11:00:00-08:00",
                "end": "2026-01-06T14:25:00-08:00",
                "average_cost_per_mwh": 67.00585844791665,
                "energy_mwh": 2553.1914893617013
            }
        ],
        "discharge_windows": [
            {
                "start": "2026-01-01T17:00:00-08:00",
                "end": "2026-01-01T20:05:00-08:00",
                "average_price_per_mwh": 45.735104638741134,
                "energy_mwh": 2256.0000000000005
            },
            {
                "start": "2026-01-02T17:00:00-08:00",
                "end": "2026-01-02T20:05:00-08:00",
                "average_price_per_mwh": 62.345536172429085,
                "energy_mwh": 2256.0000000000005
            },
            {
                "start": "2026-01-03T17:00:00-08:00",
                "end": "2026-01-03T20:05:00-08:00",
                "average_price_per_mwh": 62.27897366799646,
                "energy_mwh": 2256.0000000000005
            },
            {
                "start": "2026-01-04T17:00:00-08:00",
                "end": "2026-01-04T20:05:00-08:00",
                "average_price_per_mwh": 60.41692310283687,
                "energy_mwh": 2256.0000000000005
            },
            {
                "start": "2026-01-05T17:00:00-08:00",
                "end": "2026-01-05T20:05:00-08:00",
                "average_price_per_mwh": 65.07670422650709,
                "energy_mwh": 2256.0000000000005
            },
            {
                "start": "2026-01-06T17:00:00-08:00",
                "end": "2026-01-06T20:05:00-08:00",
                "average_price_per_mwh": 68.17426134530143,
                "energy_mwh": 2256.0000000000005
            }
        ]
    }
}
```