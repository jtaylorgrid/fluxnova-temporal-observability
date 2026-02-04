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

	// Customer outcomes - what actually happened
	outcomes := []struct {
		CustomerID  string
		Churned     bool
		ChurnReason string
		FinalLTV    float64
	}{
		{"CUST-001", true, "poor_support_experience", 2500},
		{"CUST-003", true, "competitor_switch", 8500},
		{"CUST-005", true, "price_sensitivity", 1200},
		{"CUST-007", true, "product_issues", 4500},
		{"CUST-009", true, "unresolved_complaint", 4300},
		{"CUST-002", false, "", 3200},
		{"CUST-004", false, "", 1800},
		{"CUST-006", false, "", 2100},
		{"CUST-008", false, "", 950},
		{"CUST-010", false, "", 1500},
	}

	log.Println("Loading customer outcomes into XTDB...")

	for _, o := range outcomes {
		churnReasonSQL := "NULL"
		if o.ChurnReason != "" {
			churnReasonSQL = quote(o.ChurnReason)
		}

		sql := fmt.Sprintf(`
			INSERT INTO customer_outcomes RECORDS {
				_id: %s,
				customer_id: %s,
				churned: %t,
				churn_reason: %s,
				final_ltv: %f,
				_valid_from: TIMESTAMP %s
			}
		`, quote(fmt.Sprintf("outcome:%s", o.CustomerID)),
			quote(o.CustomerID),
			o.Churned,
			churnReasonSQL,
			o.FinalLTV,
			quote(time.Now().Format(time.RFC3339Nano)))

		if _, err := pool.Exec(ctx, sql); err != nil {
			log.Printf("Failed to insert outcome for %s: %v", o.CustomerID, err)
			continue
		}
		status := "retained"
		if o.Churned {
			status = fmt.Sprintf("churned (%s)", o.ChurnReason)
		}
		log.Printf("Loaded outcome: %s - %s (LTV: $%.0f)", o.CustomerID, status, o.FinalLTV)
	}

	log.Println("Done loading customer outcomes")
}

func quote(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}
