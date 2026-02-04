package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	connString := os.Getenv("XTDB_CONN_STRING")
	if connString == "" {
		connString = "postgres://localhost:15432/xtdb?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		log.Fatal("connect to XTDB:", err)
	}
	defer pool.Close()

	// Mock Fluxnova process instances - these match the customer IDs in churn_predictions
	processes := []struct {
		ProcessInstanceID    string
		ProcessDefinitionKey string
		BusinessKey          string
		State                string
		StartTime            time.Time
	}{
		{"proc-001", "customer-service", "CUST-001", "COMPLETED", time.Now().Add(-1 * time.Hour)},
		{"proc-002", "customer-service", "CUST-002", "COMPLETED", time.Now().Add(-55 * time.Minute)},
		{"proc-003", "customer-service", "CUST-003", "COMPLETED", time.Now().Add(-50 * time.Minute)},
		{"proc-004", "customer-service", "CUST-004", "COMPLETED", time.Now().Add(-45 * time.Minute)},
		{"proc-005", "customer-service", "CUST-005", "COMPLETED", time.Now().Add(-40 * time.Minute)},
		{"proc-006", "customer-service", "CUST-006", "ACTIVE", time.Now().Add(-35 * time.Minute)},
		{"proc-007", "customer-service", "CUST-007", "COMPLETED", time.Now().Add(-30 * time.Minute)},
		{"proc-008", "customer-service", "CUST-008", "ACTIVE", time.Now().Add(-25 * time.Minute)},
		{"proc-009", "customer-service", "CUST-009", "COMPLETED", time.Now().Add(-20 * time.Minute)},
		{"proc-010", "customer-service", "CUST-010", "ACTIVE", time.Now().Add(-15 * time.Minute)},
	}

	log.Println("Loading mock Fluxnova process instances into XTDB...")

	for _, p := range processes {
		sql := fmt.Sprintf(`
			INSERT INTO fluxnova_processes RECORDS {
				_id: %s,
				process_instance_id: %s,
				process_definition_key: %s,
				business_key: %s,
				state: %s,
				start_time: %s,
				_valid_from: TIMESTAMP %s
			}
		`, quote(p.ProcessInstanceID),
			quote(p.ProcessInstanceID),
			quote(p.ProcessDefinitionKey),
			quote(p.BusinessKey),
			quote(p.State),
			quote(p.StartTime.Format(time.RFC3339Nano)),
			quote(p.StartTime.Format(time.RFC3339Nano)))

		if _, err := pool.Exec(ctx, sql); err != nil {
			log.Printf("Failed to insert process %s: %v", p.ProcessInstanceID, err)
			continue
		}
		log.Printf("Loaded process: %s (customer: %s, state: %s)", p.ProcessInstanceID, p.BusinessKey, p.State)
	}

	log.Println("Done loading mock Fluxnova process instances")
}

func quote(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}
