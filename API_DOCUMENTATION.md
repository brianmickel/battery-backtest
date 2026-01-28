# Battery Backtest API Documentation

## Overview

The Battery Backtest API provides endpoints for simulating battery energy storage arbitrage strategies using historical electricity market price data. The API allows you to:

- Run backtests with different battery configurations and trading strategies
- Compare multiple backtest variations
- Rank locations by arbitrage potential
- List available batteries, strategies, datasets, and locations

## Base URL

```
http://localhost:8080/api/v1
```

The default port is `8080`, but can be configured via the `API_PORT` environment variable.

## Authentication

All endpoints that fetch data from Grid Status require a Grid Status API key. The API key should be provided in the request body (for POST requests) or as a query parameter (for GET requests).

**Note:** The API key is passed through to Grid Status API - you must provide your own valid Grid Status API key.

## Endpoints

### Health Check

#### `GET /health`

Check if the API server is running.

**Response:**
```json
{
  "status": "ok"
}
```

---

### Run Backtest

#### `POST /api/v1/backtest`

Run a backtest simulation with specified battery configuration and trading strategy.

**Request Body:**
```json
{
  "api_key": "{{API_KEY}}",
  "data_source": {
    "type": "gridstatus",
    "dataset_id": "caiso_lmp_real_time_5_min",
    "location_id": "TH_NP15_GEN-APND",
    "start_date": "2026-01-01",
    "end_date": "2026-01-07",
    "timezone": "market"
  },
  "config": {
    "battery_file": "1_moss_landing",
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
        "charge_start": "10:00",
        "charge_end": "17:00",
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

**Request Fields:**

- `api_key` (string, required): Your Grid Status API key
- `data_source` (object, required):
  - `type` (string, required): Currently only `"gridstatus"` is supported
  - `dataset_id` (string, required): Grid Status dataset ID (e.g., `"caiso_lmp_real_time_5_min"`)
  - `location_id` (string, required): Grid Status location/node ID
  - `start_date` (string, required): Start date in `YYYY-MM-DD` format
  - `end_date` (string, required): End date in `YYYY-MM-DD` format
  - `timezone` (string, optional): Timezone for data (default: `"market"`)
- `config` (object, required):
  - `battery_file` (string, optional): Battery preset filename without extension (e.g., `"1_moss_landing"`). Files are looked up in the `examples/batteries/` directory with `.yaml` extension automatically appended.
  - `battery` (object, optional if `battery_file` is provided):
    - `name` (string, optional): Battery name
    - `energy_capacity_mwh` (float, required): Energy capacity in MWh
    - `power_capacity_mw` (float, required): Power capacity in MW
    - `charge_efficiency` (float, required): Charge efficiency (0.0-1.0)
    - `discharge_efficiency` (float, required): Discharge efficiency (0.0-1.0)
    - `min_soc` (float, required): Minimum state of charge (0.0-1.0)
    - `max_soc` (float, required): Maximum state of charge (0.0-1.0)
    - `initial_soc` (float, optional): Initial state of charge (default: `min_soc`)
    - `degradation_cost_per_mwh` (float, optional): Degradation cost per MWh throughput
  - `strategy` (object, required):
    - `name` (string, required): Strategy name (`"schedule"` or `"oracle"`)
    - `params` (object, optional): Strategy-specific parameters (see Strategy section)
- `options` (object, optional):
  - `limit_intervals` (int, optional): Limit number of intervals to process (0 = all)
  - `include_ledger` (bool, optional): Include detailed ledger in response (default: `false`)

**Response:**
```json
{
  "status": "completed",
  "summary": {
    "total_pnl": 125430.50,
    "final_soc": 0.15,
    "total_intervals": 2016,
    "backtest_window": {
      "start": "2026-01-01T00:00:00-08:00",
      "end": "2026-01-07T23:55:00-08:00"
    },
    "energy_charged_mwh": 12500.0,
    "energy_discharged_mwh": 11875.0,
    "charge_windows": [
      {
        "start": "2026-01-01T10:00:00-08:00",
        "end": "2026-01-01T17:00:00-08:00",
        "average_cost_per_mwh": 45.25,
        "energy_mwh": 2625.0
      },
      {
        "start": "2026-01-02T10:00:00-08:00",
        "end": "2026-01-02T17:00:00-08:00",
        "average_cost_per_mwh": 48.50,
        "energy_mwh": 2625.0
      }
    ],
    "discharge_windows": [
      {
        "start": "2026-01-01T17:00:00-08:00",
        "end": "2026-01-01T23:55:00-08:00",
        "average_price_per_mwh": 65.75,
        "energy_mwh": 2250.0
      },
      {
        "start": "2026-01-02T17:00:00-08:00",
        "end": "2026-01-02T23:55:00-08:00",
        "average_price_per_mwh": 68.25,
        "energy_mwh": 2250.0
      }
    ]
  },
  "ledger": []
}
```

**Response Fields:**
- `charge_windows` (array): Per-day charge windows showing when the battery charged each day. Each window contains:
  - `start` (time): First interval where charging occurred on this day
  - `end` (time): Last interval where charging occurred on this day
  - `average_cost_per_mwh` (float): Weighted average LMP (price) during charging periods, weighted by energy charged in each interval
  - `energy_mwh` (float): Total energy charged during this window
- `discharge_windows` (array): Per-day discharge windows showing when the battery discharged each day. Each window contains:
  - `start` (time): First interval where discharging occurred on this day
  - `end` (time): Last interval where discharging occurred on this day
  - `average_price_per_mwh` (float): Weighted average LMP (price) during discharging periods, weighted by energy discharged in each interval
  - `energy_mwh` (float): Total energy discharged during this window

**Response with Ledger** (when `include_ledger: true`):
```json
{
  "status": "completed",
  "summary": { ... },
  "ledger": [
    {
      "index": 0,
      "interval_start_local": "2026-01-01T00:00:00-08:00",
      "interval_end_local": "2026-01-01T00:05:00-08:00",
      "interval_start_utc": "2026-01-01T08:00:00Z",
      "interval_end_utc": "2026-01-01T08:05:00Z",
      "location": "TH_NP15_GEN-APND",
      "market": "CAISO",
      "lmp": 45.25,
      "action": "IDLE",
      "requested_power_mw": 0.0,
      "power_mw": 0.0,
      "energy_from_grid_mwh": 0.0,
      "energy_to_grid_mwh": 0.0,
      "throughput_mwh": 0.0,
      "soc_start": 0.10,
      "soc_end": 0.10,
      "pnl": 0.0,
      "cum_pnl": 0.0
    }
  ]
}
```

**Example using cURL:**
```bash
curl -X POST http://localhost:8080/api/v1/backtest \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-api-key",
    "data_source": {
      "type": "gridstatus",
      "dataset_id": "caiso_lmp_real_time_5_min",
      "location_id": "TH_NP15_GEN-APND",
      "start_date": "2026-01-01",
      "end_date": "2026-01-07"
    },
    "config": {
      "battery_file": "1_moss_landing",
      "strategy": {
        "name": "schedule",
        "params": {
          "charge_start": "10:00",
          "discharge_start": "17:00"
        }
      }
    }
  }'
```

---

### Compare Backtests

#### `POST /api/v1/backtest/compare`

Run multiple backtest variations and compare results. Useful for parameter sweeps or strategy comparisons.

**Request Body:**
```json
{
  "api_key": "{{API_KEY}}",
  "data_source": {
    "type": "gridstatus",
    "dataset_id": "caiso_lmp_real_time_5_min",
    "location_id": "TH_NP15_GEN-APND",
    "start_date": "2026-01-01",
    "end_date": "2026-01-07"
  },
  "base_config": {
    "battery_file": "1_moss_landing",
    "strategy": {
      "name": "schedule",
      "params": {
        "charge_start": "10:00",
        "discharge_start": "17:00"
      }
    }
  },
  "variations": [
    {
      "name": "Early Charge (8am)",
      "config": {
        "strategy": {
          "name": "schedule",
          "params": {
            "charge_start": "08:00",
            "discharge_start": "17:00"
          }
        }
      }
    },
    {
      "name": "Late Charge (12pm)",
      "config": {
        "strategy": {
          "name": "schedule",
          "params": {
            "charge_start": "12:00",
            "discharge_start": "17:00"
          }
        }
      }
    },
    {
      "name": "Oracle Strategy",
      "config": {
        "strategy": {
          "name": "oracle",
          "params": {
            "soc_steps": 200,
            "power_steps": 10
          }
        }
      }
    }
  ]
}
```

**Response:**
```json
{
  "comparison": [
    {
      "name": "Early Charge (8am)",
      "summary": {
        "total_pnl": 120000.0,
        "final_soc": 0.15,
        "total_intervals": 2016,
        "backtest_window": { ... },
        "energy_charged_mwh": 13000.0,
        "energy_discharged_mwh": 12350.0
      }
    },
    {
      "name": "Late Charge (12pm)",
      "summary": {
        "total_pnl": 115000.0,
        "final_soc": 0.12,
        "total_intervals": 2016,
        "backtest_window": { ... },
        "energy_charged_mwh": 11000.0,
        "energy_discharged_mwh": 10450.0
      }
    },
    {
      "name": "Oracle Strategy",
      "summary": {
        "total_pnl": 150000.0,
        "final_soc": 0.10,
        "total_intervals": 2016,
        "backtest_window": { ... },
        "energy_charged_mwh": 15000.0,
        "energy_discharged_mwh": 14250.0
      }
    }
  ]
}
```

**Example using cURL:**
```bash
curl -X POST http://localhost:8080/api/v1/backtest/compare \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-api-key",
    "data_source": {
      "type": "gridstatus",
      "dataset_id": "caiso_lmp_real_time_5_min",
      "location_id": "TH_NP15_GEN-APND",
      "start_date": "2026-01-01",
      "end_date": "2026-01-07"
    },
    "base_config": {
      "battery_file": "1_moss_landing",
      "strategy": {
        "name": "schedule",
        "params": {
          "charge_start": "10:00",
          "discharge_start": "17:00"
        }
      }
    },
    "variations": [
      {
        "name": "Oracle",
        "config": {
          "strategy": {
            "name": "oracle"
          }
        }
      }
    ]
  }'
```

---

### Get Ledger

#### `GET /api/v1/backtest/:id/ledger`

**Note:** This endpoint is not yet implemented. Use `include_ledger: true` in the backtest request to get ledger data.

**Response:**
```json
{
  "error": {
    "code": "NOT_IMPLEMENTED",
    "message": "Ledger retrieval not yet implemented. Use include_ledger=true in backtest request."
  }
}
```

---

### List Batteries

#### `GET /api/v1/batteries`

List all available battery presets.

**Response:**
```json
{
  "batteries": [
    {
      "id": "moss_landing",
      "name": "Moss Landing Phase I & II",
      "file": "./examples/batteries/1_moss_landing.yaml",
      "specs": {
        "energy_capacity_mwh": 3000.0,
        "power_capacity_mw": 750.0
      }
    },
    {
      "id": "victorian_big_battery",
      "name": "Victorian Big Battery",
      "file": "./examples/batteries/2_victorian_big_battery.yaml",
      "specs": {
        "energy_capacity_mwh": 300.0,
        "power_capacity_mw": 300.0
      }
    }
  ]
}
```

**Example using cURL:**
```bash
curl http://localhost:8080/api/v1/batteries
```

---

### List Strategies

#### `GET /api/v1/strategies`

List all available trading strategies with their parameters.

**Response:**
```json
{
  "strategies": [
    {
      "name": "schedule",
      "description": "Time-based schedule strategy. Charges and discharges at specific times each day.",
      "parameters": [
        {
          "name": "charge_start",
          "type": "string",
          "description": "Start time for charging (HH:MM format, e.g., '10:00')",
          "default": "10:00"
        },
        {
          "name": "charge_end",
          "type": "string",
          "description": "End time for charging (HH:MM format)",
          "default": "17:00"
        },
        {
          "name": "discharge_start",
          "type": "string",
          "description": "Start time for discharging (HH:MM format, e.g., '17:00')",
          "default": "17:00"
        },
        {
          "name": "discharge_end",
          "type": "string",
          "description": "End time for discharging (HH:MM format)",
          "default": "23:59"
        },
        {
          "name": "charge_power_mw",
          "type": "float",
          "description": "Charge power in MW",
          "default": 0.0
        },
        {
          "name": "discharge_power_mw",
          "type": "float",
          "description": "Discharge power in MW",
          "default": 0.0
        }
      ]
    },
    {
      "name": "oracle",
      "description": "Perfect foresight optimizer. Uses dynamic programming to find optimal dispatch with full knowledge of future prices.",
      "parameters": [
        {
          "name": "soc_steps",
          "type": "int",
          "description": "Number of SOC discretization steps (higher = more accurate but slower)",
          "default": 200
        },
        {
          "name": "power_steps",
          "type": "int",
          "description": "Number of power discretization steps",
          "default": 10
        }
      ]
    }
  ]
}
```

**Example using cURL:**
```bash
curl http://localhost:8080/api/v1/strategies
```

---

### List Datasets

#### `GET /api/v1/datasets`

List available Grid Status datasets.

**Response:**
```json
{
  "datasets": [
    {
      "id": "caiso_lmp_real_time_5_min",
      "name": "CAISO LMP Real-Time 5-Min",
      "market": "CAISO",
      "resolution": "5min"
    }
  ]
}
```

**Example using cURL:**
```bash
curl http://localhost:8080/api/v1/datasets
```

---

### List Locations

#### `GET /api/v1/locations?dataset_id=:dataset_id`

List available locations for a specific dataset.

**Query Parameters:**
- `dataset_id` (string, required): Dataset ID to get locations for

**Response:**
```json
{
  "locations": [
    {
      "id": "TH_NP15_GEN-APND",
      "name": "NP15 Gen APND",
      "type": "trading_hub"
    },
    {
      "id": "TH_SP15_GEN-APND",
      "name": "SP15 Gen APND",
      "type": "trading_hub"
    }
  ],
  "updated_at": "2026-01-01T00:00:00Z",
  "count": 2
}
```

**Example using cURL:**
```bash
curl "http://localhost:8080/api/v1/locations?dataset_id=caiso_lmp_real_time_5_min"
```

---

### Rank Locations

#### `GET /api/v1/rank`

Rank locations by arbitrage potential using oracle strategy (perfect foresight).

**Query Parameters:**
- `api_key` (string, required): Your Grid Status API key
- `dataset_id` (string, required): Grid Status dataset ID
- `start_date` (string, required): Start date in `YYYY-MM-DD` format
- `end_date` (string, required): End date in `YYYY-MM-DD` format
- `location_ids` (string, optional): Comma-separated list of location IDs to rank
- `limit` (int, optional): Maximum number of results (default: 10)

**Response:**
```json
{
  "rankings": [
    {
      "rank": 1,
      "location": "TH_NP15_GEN-APND",
      "market": "CAISO",
      "count": 2016,
      "spread_p95_p05": 85.50,
      "min_lmp": 12.25,
      "max_lmp": 125.75,
      "oracle_profit": 250000.0
    },
    {
      "rank": 2,
      "location": "TH_SP15_GEN-APND",
      "market": "CAISO",
      "count": 2016,
      "spread_p95_p05": 78.25,
      "min_lmp": 15.50,
      "max_lmp": 120.00,
      "oracle_profit": 220000.0
    }
  ]
}
```

**Example using cURL:**
```bash
curl "http://localhost:8080/api/v1/rank?api_key=your-api-key&dataset_id=caiso_lmp_real_time_5_min&start_date=2026-01-01&end_date=2026-01-07&location_ids=TH_NP15_GEN-APND,TH_SP15_GEN-APND&limit=10"
```

---

## Strategies

### Schedule Strategy

Time-based strategy that charges and discharges at fixed times each day.

**Parameters:**
- `charge_start` (string): Start time for charging in `HH:MM` format (default: `"10:00"`)
- `charge_end` (string): End time for charging in `HH:MM` format (default: `"17:00"`)
- `discharge_start` (string): Start time for discharging in `HH:MM` format (default: `"17:00"`)
- `discharge_end` (string): End time for discharging in `HH:MM` format (default: `"23:59"`)
- `charge_power_mw` (float): Power to charge at in MW (default: battery's power capacity)
- `discharge_power_mw` (float): Power to discharge at in MW (default: battery's power capacity)

**Example:**
```json
{
  "name": "schedule",
  "params": {
    "charge_start": "10:00",
    "charge_end": "17:00",
    "discharge_start": "17:00",
    "discharge_end": "23:59",
    "charge_power_mw": 750.0,
    "discharge_power_mw": 750.0
  }
}
```

### Oracle Strategy

Perfect foresight optimizer that uses dynamic programming to find optimal dispatch with full knowledge of future prices. This provides an upper bound on profitability.

**Parameters:**
- `soc_steps` (int): Number of SOC discretization steps (higher = more accurate but slower, default: `200`)
- `power_steps` (int): Number of power discretization steps (default: `10`)

**Example:**
```json
{
  "name": "oracle",
  "params": {
    "soc_steps": 200,
    "power_steps": 10
  }
}
```

---

## Error Handling

All errors follow a consistent format:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": {
      "additional": "context"
    }
  }
}
```

### Common Error Codes

- `INVALID_REQUEST`: Request body is malformed or missing required fields
- `INVALID_API_KEY`: API key is missing or invalid
- `INVALID_CONFIG`: Battery or strategy configuration is invalid
- `DATA_FETCH_ERROR`: Failed to fetch data from Grid Status API
- `BACKTEST_ERROR`: Error occurred during backtest execution
- `NOT_IMPLEMENTED`: Endpoint or feature not yet implemented
- `MISSING_PARAM`: Required query parameter is missing
- `INVALID_DATE`: Date format is invalid (must be `YYYY-MM-DD`)
- `LOCATIONS_REQUIRED`: Location IDs are required for ranking

### Grid Status API Errors

When the Grid Status API returns an error, it will be passed through with additional context:

```json
{
  "error": {
    "code": "GRID_STATUS_ERROR",
    "message": "Grid Status API error message",
    "details": {
      "status_code": 401,
      "retry_after": 60
    }
  }
}
```

Common Grid Status error codes:
- `401` / `403`: Unauthorized - invalid API key
- `429`: Too many requests - rate limit exceeded
- `400`: Bad request - invalid parameters

---

## Usage Examples

### Example 1: Simple Schedule Backtest

Run a backtest with a schedule strategy using a battery preset:

```bash
curl -X POST http://localhost:8080/api/v1/backtest \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-api-key",
    "data_source": {
      "type": "gridstatus",
      "dataset_id": "caiso_lmp_real_time_5_min",
      "location_id": "TH_NP15_GEN-APND",
      "start_date": "2026-01-01",
      "end_date": "2026-01-07"
    },
    "config": {
      "battery_file": "1_moss_landing",
      "strategy": {
        "name": "schedule",
        "params": {
          "charge_start": "10:00",
          "discharge_start": "17:00"
        }
      }
    }
  }'
```

### Example 2: Custom Battery Configuration

Run a backtest with a custom battery configuration:

```bash
curl -X POST http://localhost:8080/api/v1/backtest \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-api-key",
    "data_source": {
      "type": "gridstatus",
      "dataset_id": "caiso_lmp_real_time_5_min",
      "location_id": "TH_NP15_GEN-APND",
      "start_date": "2026-01-01",
      "end_date": "2026-01-07"
    },
    "config": {
      "battery": {
        "name": "Custom Battery",
        "energy_capacity_mwh": 1000.0,
        "power_capacity_mw": 250.0,
        "charge_efficiency": 0.95,
        "discharge_efficiency": 0.95,
        "min_soc": 0.10,
        "max_soc": 0.90,
        "initial_soc": 0.10,
        "degradation_cost_per_mwh": 2.0
      },
      "strategy": {
        "name": "schedule",
        "params": {
          "charge_start": "08:00",
          "charge_end": "16:00",
          "discharge_start": "16:00",
          "discharge_end": "22:00"
        }
      }
    },
    "options": {
      "include_ledger": true
    }
  }'
```

### Example 3: Oracle Strategy with High Resolution

Run a backtest with oracle strategy using high-resolution discretization:

```bash
curl -X POST http://localhost:8080/api/v1/backtest \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-api-key",
    "data_source": {
      "type": "gridstatus",
      "dataset_id": "caiso_lmp_real_time_5_min",
      "location_id": "TH_NP15_GEN-APND",
      "start_date": "2026-01-01",
      "end_date": "2026-01-07"
    },
    "config": {
      "battery_file": "1_moss_landing",
      "strategy": {
        "name": "oracle",
        "params": {
          "soc_steps": 500,
          "power_steps": 20
        }
      }
    }
  }'
```

### Example 4: Compare Multiple Strategies

Compare schedule vs oracle strategies:

```bash
curl -X POST http://localhost:8080/api/v1/backtest/compare \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-api-key",
    "data_source": {
      "type": "gridstatus",
      "dataset_id": "caiso_lmp_real_time_5_min",
      "location_id": "TH_NP15_GEN-APND",
      "start_date": "2026-01-01",
      "end_date": "2026-01-07"
    },
    "base_config": {
      "battery_file": "1_moss_landing"
    },
    "variations": [
      {
        "name": "Schedule 10am-5pm",
        "config": {
          "strategy": {
            "name": "schedule",
            "params": {
              "charge_start": "10:00",
              "discharge_start": "17:00"
            }
          }
        }
      },
      {
        "name": "Oracle Optimal",
        "config": {
          "strategy": {
            "name": "oracle"
          }
        }
      }
    ]
  }'
```

### Example 5: Rank Locations

Find the best locations for arbitrage:

```bash
curl "http://localhost:8080/api/v1/rank?api_key=your-api-key&dataset_id=caiso_lmp_real_time_5_min&start_date=2026-01-01&end_date=2026-01-07&location_ids=TH_NP15_GEN-APND,TH_SP15_GEN-APND,TH_ZP26_GEN-APND&limit=5"
```

### Example 6: Python Client

```python
import requests

BASE_URL = "http://localhost:8080/api/v1"

# Run a backtest
response = requests.post(f"{BASE_URL}/backtest", json={
    "api_key": "your-api-key",
    "data_source": {
        "type": "gridstatus",
        "dataset_id": "caiso_lmp_real_time_5_min",
        "location_id": "TH_NP15_GEN-APND",
        "start_date": "2026-01-01",
        "end_date": "2026-01-07"
    },
    "config": {
        "battery_file": "1_moss_landing",
        "strategy": {
            "name": "schedule",
            "params": {
                "charge_start": "10:00",
                "discharge_start": "17:00"
            }
        }
    },
    "options": {
        "include_ledger": True
    }
})

result = response.json()
print(f"Total P&L: ${result['summary']['total_pnl']:,.2f}")
print(f"Energy Charged: {result['summary']['energy_charged_mwh']:.2f} MWh")
print(f"Energy Discharged: {result['summary']['energy_discharged_mwh']:.2f} MWh")
```

### Example 7: JavaScript/Node.js Client

```javascript
const axios = require('axios');

const BASE_URL = 'http://localhost:8080/api/v1';

async function runBacktest() {
  try {
    const response = await axios.post(`${BASE_URL}/backtest`, {
      api_key: 'your-api-key',
      data_source: {
        type: 'gridstatus',
        dataset_id: 'caiso_lmp_real_time_5_min',
        location_id: 'TH_NP15_GEN-APND',
        start_date: '2026-01-01',
        end_date: '2026-01-07'
      },
      config: {
        battery_file: '1_moss_landing',
        strategy: {
          name: 'schedule',
          params: {
            charge_start: '10:00',
            discharge_start: '17:00'
          }
        }
      },
      options: {
        include_ledger: true
      }
    });

    const result = response.data;
    console.log(`Total P&L: $${result.summary.total_pnl.toLocaleString()}`);
    console.log(`Energy Charged: ${result.summary.energy_charged_mwh} MWh`);
    console.log(`Energy Discharged: ${result.summary.energy_discharged_mwh} MWh`);
  } catch (error) {
    console.error('Error:', error.response?.data || error.message);
  }
}

runBacktest();
```

---

## Rate Limiting

The API does not implement rate limiting itself, but Grid Status API requests are subject to Grid Status rate limits. If you receive a `429 Too Many Requests` error, check the `retry_after` field in the error details for when to retry.

---

## Best Practices

1. **API Key Security**: Never commit API keys to version control. Use environment variables or secure credential storage.

2. **Date Ranges**: Start with shorter date ranges (1-7 days) for testing, then expand to longer periods once validated.

3. **Ledger Data**: Only request ledger data (`include_ledger: true`) when needed, as it significantly increases response size.

4. **Interval Limits**: Use `limit_intervals` for quick testing or when working with very large datasets.

5. **Strategy Selection**:
   - Use `schedule` for simple time-based strategies
   - Use `oracle` for finding optimal profitability upper bounds
   - Compare both to understand the gap between simple and optimal strategies

6. **Battery Presets**: Use `battery_file` to reference predefined battery configurations, then override specific parameters as needed.

7. **Error Handling**: Always check for error responses and handle them appropriately in your client code.

---

## Support

For issues, questions, or contributions, please refer to the main project README or open an issue in the repository.
