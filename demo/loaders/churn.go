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
		connString = "postgres://localhost:5432/xtdb?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		log.Fatal("connect to XTDB:", err)
	}
	defer pool.Close()

	// Churn predictions with valid-time backdated to simulate ML model lag
	predictions := []struct {
		CustomerID string
		ChurnScore float64
		RiskLevel  string
		ValidFrom  time.Time
	}{
		{"CUST-001", 0.85, "high", time.Now().Add(-2 * time.Hour)},
		{"CUST-003", 0.92, "high", time.Now().Add(-90 * time.Minute)},
		{"CUST-005", 0.78, "high", time.Now().Add(-45 * time.Minute)},
		{"CUST-007", 0.88, "high", time.Now().Add(-30 * time.Minute)},
		{"CUST-009", 0.95, "high", time.Now().Add(-15 * time.Minute)},
		{"CUST-002", 0.25, "low", time.Now().Add(-1 * time.Hour)},
		{"CUST-004", 0.35, "low", time.Now().Add(-1 * time.Hour)},
		{"CUST-006", 0.42, "medium", time.Now().Add(-1 * time.Hour)},
		{"CUST-008", 0.15, "low", time.Now().Add(-1 * time.Hour)},
		{"CUST-010", 0.38, "low", time.Now().Add(-1 * time.Hour)},
	}

	log.Println("Loading churn predictions into XTDB...")

	for _, p := range predictions {
		sql := fmt.Sprintf(`
			INSERT INTO churn_predictions RECORDS {
				_id: %s,
				customer_id: %s,
				churn_score: %f,
				risk_level: %s,
				model_version: 'v2.1',
				_valid_from: TIMESTAMP %s
			}
		`, quote(fmt.Sprintf("churn:%s", p.CustomerID)),
			quote(p.CustomerID),
			p.ChurnScore,
			quote(p.RiskLevel),
			quote(p.ValidFrom.Format(time.RFC3339Nano)))

		if _, err := pool.Exec(ctx, sql); err != nil {
			log.Printf("Failed to insert prediction for %s: %v", p.CustomerID, err)
			continue
		}
		log.Printf("Loaded churn prediction: %s (score: %.2f, risk: %s)", p.CustomerID, p.ChurnScore, p.RiskLevel)
	}

	log.Println("Done loading churn predictions")
}

func quote(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}
