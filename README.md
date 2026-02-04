# Fluxnova Decision Observability

A CDC connector that streams Fluxnova (FINOS Camunda 7 fork) process history into XTDB v2 for bitemporal audit and compliance queries.

## Overview

This project solves the "decision observability" problem for BPMN workflow orchestration: **what did the process know when it made that decision?** and **what should it have known to make a better decision?**

Fluxnova/Camunda is excellent at process orchestration but has limitations for long-term audit:
- History data can be purged based on retention policies
- Querying across process instances and external data is complex
- Point-in-time reconstruction requires custom tooling

XTDB's bitemporal model enables:
- **Point-in-time queries**: `FOR VALID_TIME AS OF timestamp`
- **Audit queries**: `FOR SYSTEM_TIME AS OF timestamp`
- **Unlimited retention**: Keep data for years without complex archival
- **Cross-source joins**: Combine process events with ML predictions, CRM data, etc.

## Quick Start

The easiest way to run the demo is with the provided script:

```bash
./run-demo.sh
```

This will:
1. Start all Docker infrastructure (Fluxnova, XTDB, Kafka, Kafka Connect)
2. Configure the Kafka Connect XTDB sink connector
3. Start the external task worker with XTDB integration
4. Load demo data (churn predictions, customer outcomes)
5. Launch the demo UI

Then open http://localhost:3001 to see the demo UI.

### Script Options

```bash
./run-demo.sh --help        # Show all options
./run-demo.sh --fresh       # Fresh start: remove volumes and restart everything
./run-demo.sh --skip-infra  # Skip Docker startup (if already running)
./run-demo.sh --skip-data   # Skip loading demo data
```

## Architecture

```
┌─────────────┐                              ┌─────────────┐
│  Fluxnova   │──┬─── CDC via Kafka ────────▶│    XTDB     │
│ (processes) │  │    Connect                │   (audit)   │
└─────────────┘  │                           └─────────────┘
                 │                                  │
                 └─── Direct Activity ─────────────┘
                      Writes (xtdb.Save)
                                                    │
                                                    ▼
                                             ┌─────────────┐
                                             │   Demo UI   │
                                             │ (localhost  │
                                             │   :3000)    │
                                             └─────────────┘
```

### Two Complementary Data Capture Approaches

1. **CDC via Kafka Connect**: Streams Fluxnova history events automatically. Captures *what the process did*: activity instances, variable changes, and state transitions.
   - Tables: `fluxnova_events`, `fluxnova_processes`

2. **Direct Activity Writes**: External tasks call `xtdb.Save()` explicitly to capture decision context. Records *what external systems believed* at the exact moment a decision was made.
   - Tables: `activity_churn_signals`, `activity_routing_decisions`

Together, they answer: "what did the process do?" *and* "what did it know?"

## What is Fluxnova?

[Fluxnova](https://fluxnova.finos.org/) is a FINOS-governed open-source workflow and process automation platform. It is a fork of the battle-tested Camunda 7 Community Edition, created by a collaboration between Fidelity Investments, NatWest Group, Deutsche Bank, Capital One, and BMO.

Key features:
- Native BPMN 2.0 and DMN support
- REST API compatible with Camunda 7
- Java-based, runs embedded in Spring Boot
- Open governance under FINOS/Linux Foundation

## Project Structure

```
fluxnova-decision-observability/
├── run-demo.sh                  # One-command demo startup script
├── main.go                      # CDC connector entry point
├── internal/
│   ├── config/config.go         # Configuration (env vars + YAML)
│   ├── fluxnova/
│   │   ├── client.go            # Fluxnova REST API client
│   │   └── poller.go            # Poll history API for events
│   ├── kafka/
│   │   └── producer.go          # Kafka producer for CDC events
│   └── pipeline/pipeline.go     # Source→Sink orchestration
├── kafka-connect-xtdb/          # Kafka Connect XTDB sink connector (Java)
├── demo/
│   ├── workflow/
│   │   ├── types.go             # Workflow data types
│   │   └── activities.go        # External task handlers (with XTDB)
│   ├── worker/main.go           # External task worker
│   ├── xtdb/client.go           # XTDB client helper for activities
│   ├── loaders/
│   │   ├── churn.go             # Load churn predictions
│   │   └── outcomes.go          # Load customer outcomes
│   └── ui/
│       ├── main.go              # Web server with audit APIs
│       └── templates/index.html # Dashboard
├── docker-compose.yml           # Full infrastructure stack
├── Dockerfile                   # CDC connector Docker image
└── config.yaml                  # Default configuration
```

## Configuration

Environment variables (or `config.yaml`):

| Variable | Default | Description |
|----------|---------|-------------|
| `FLUXNOVA_BASE_URL` | `http://localhost:18080/engine-rest` | Fluxnova REST API endpoint |
| `FLUXNOVA_USERNAME` | (empty) | Basic auth username |
| `FLUXNOVA_PASSWORD` | (empty) | Basic auth password |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker addresses |
| `XTDB_CONN_STRING` | `postgres://localhost:15432/xtdb?sslmode=disable` | XTDB connection |
| `POLL_INTERVAL` | `10s` | How often to poll Fluxnova history |

## XTDB Tables

### CDC Tables (populated via Kafka Connect)

#### `fluxnova_processes`
Process instance state with bitemporal tracking.

```sql
SELECT * FROM fluxnova_processes
FOR VALID_TIME AS OF TIMESTAMP '2024-01-15T10:00:00Z'
WHERE state = 'ACTIVE'
```

#### `fluxnova_events`
Activity instance history with process variables.

```sql
SELECT * FROM fluxnova_events
WHERE process_instance_id = 'abc-123'
ORDER BY start_time
```

### Activity Context Tables (populated via direct writes)

#### `activity_churn_signals`
Churn risk scores captured at the moment external tasks executed.

#### `activity_routing_decisions`
Full decision context including all inputs that informed routing decisions.

## Example Bitemporal Queries

### What processes were running at a specific time?

```sql
SELECT * FROM fluxnova_processes
FOR VALID_TIME AS OF TIMESTAMP '2024-06-15T10:00:00Z'
WHERE state = 'ACTIVE'
```

### Cross-source join (demo)

```sql
SELECT p.process_instance_id, p.start_time, c.churn_score
FROM fluxnova_processes p
JOIN churn_predictions FOR VALID_TIME ALL AS c
  ON c.customer_id = p.business_key
WHERE c._valid_from <= CAST(p.start_time AS TIMESTAMPTZ)
  AND (c._valid_to IS NULL OR c._valid_to > CAST(p.start_time AS TIMESTAMPTZ))
```

## Development

### Prerequisites

- Go 1.21+
- Docker & Docker Compose
- (Optional) `psql` for querying XTDB directly

### Building

```bash
go build -o cdc-connector .
```

### Querying XTDB Directly

```bash
psql "postgres://localhost:15432/xtdb?sslmode=disable"

SELECT COUNT(*) FROM fluxnova_processes;
SELECT COUNT(*) FROM fluxnova_events;
```

## Related Projects

- [Fluxnova](https://github.com/finos/fluxnova-bpm-platform) - FINOS fork of Camunda 7
- [XTDB](https://xtdb.com/) - Bitemporal database
- [temporal-decision-observability](https://github.com/jtaylorgrid/temporal-decision-observability) - Similar project for Temporal.io

## License

Apache License 2.0

## Sources

- [Fluxnova FINOS Website](https://fluxnova.finos.org/faqs)
- [Fluxnova GitHub](https://github.com/finos/fluxnova-bpm-platform)
- [Why the Future of API Orchestration is Open Source](https://www.finos.org/blog/why-the-future-of-api-orchestration-is-open-source-meet-fluxnova-at-apidays-paris)
- [Scott Logic's Contribution to FINOS Fluxnova](https://blog.scottlogic.com/2025/10/22/accelerating-financial-process-automation-scott-logic-finos-fluxnova.html)
