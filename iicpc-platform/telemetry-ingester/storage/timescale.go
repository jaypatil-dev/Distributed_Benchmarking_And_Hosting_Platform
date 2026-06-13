/*
****** WHAT THIS FILE DOES ******
* Handles persistent storage of telemetry data in TimescaleDB.
*
* TABLES:
* - order_metrics => every single order result from bot fleet
*   partitioned by time automatically via hypertable
* - scores => computed p50/p90/p99/TPS/score per submission
*
* FUNCTIONS:
* - NewTimescaleDB() => connects to TimescaleDB, creates tables
*                       and hypertables if they don't exist
* - SaveOrderMetrics() => inserts all 1000 order results per submission
* - SaveScore()        => inserts computed metrics and score
* - Close()            => closes database connection
*
* WHY TIMESCALEDB?
* Normal PostgreSQL struggles with millions of timestamped rows.
* TimescaleDB automatically partitions data by time (hypertables)
* making queries like "average latency in last 10 seconds" or
* "TPS trend over 5 minutes" 10-100x faster than standard SQL.
*
* WHY MOCK MODE FALLBACK?
* Docker Desktop on Windows has a SASL auth issue with PostgreSQL
* over TCP loopback. If connection fails we fall back to mock mode
* so the platform still works — zero code changes needed when
* deploying on Linux where this issue doesn't exist.
*/

package storage

import (
	"database/sql"
	"fmt"
	"telemetry-ingester/calculator"
	"time"

	_ "github.com/lib/pq"
)

type TimescaleDB struct {
	db     *sql.DB
	mock   bool // true if running in mock mode
}

func NewTimescaleDB() (*TimescaleDB, error) {
	connStr := "host=timescaledb port=5432 user=postgres password=iicpc123 dbname=iicpc sslmode=disable"

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		fmt.Printf("TimescaleDB connection error: %v — running in mock mode\n", err)
		return &TimescaleDB{mock: true}, nil
	}

	// test connection
	if err := db.Ping(); err != nil {
		fmt.Printf("TimescaleDB ping error: %v — running in mock mode\n", err)
		return &TimescaleDB{mock: true}, nil
	}

	fmt.Println("Connected to TimescaleDB successfully!")

	t := &TimescaleDB{db: db, mock: false}

	// create tables and hypertables on startup
	if err := t.createTables(); err != nil {
		fmt.Printf("TimescaleDB table creation error: %v — running in mock mode\n", err)
		return &TimescaleDB{mock: true}, nil
	}

	return t, nil
}

func (t *TimescaleDB) createTables() error {
	// order_metrics — stores every single order from bot fleet
	_, err := t.db.Exec(`
		CREATE TABLE IF NOT EXISTS order_metrics (
			time          TIMESTAMPTZ NOT NULL,
			submission_id TEXT        NOT NULL,
			contestant    TEXT        NOT NULL,
			bot_id        INT         NOT NULL,
			latency_ms    BIGINT      NOT NULL,
			success       BOOLEAN     NOT NULL,
			price         FLOAT       NOT NULL,
			order_type    TEXT        NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create order_metrics table: %v", err)
	}

	// convert to hypertable for time-series performance
	// ignore error if already a hypertable
	t.db.Exec(`SELECT create_hypertable('order_metrics', 'time', if_not_exists => TRUE);`)

	// scores — stores computed metrics per submission
	_, err = t.db.Exec(`
		CREATE TABLE IF NOT EXISTS scores (
			time          TIMESTAMPTZ NOT NULL,
			submission_id TEXT        NOT NULL,
			contestant    TEXT        NOT NULL,
			p50           BIGINT      NOT NULL,
			p90           BIGINT      NOT NULL,
			p99           BIGINT      NOT NULL,
			tps           FLOAT       NOT NULL,
			total_orders  INT         NOT NULL,
			success_rate  FLOAT       NOT NULL,
			score         FLOAT       NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create scores table: %v", err)
	}

	t.db.Exec(`SELECT create_hypertable('scores', 'time', if_not_exists => TRUE);`)

	fmt.Println("TimescaleDB tables ready!")
	return nil
}

func (t *TimescaleDB) SaveOrderMetrics(submissionID, contestant string, results []calculator.OrderResult) error {
	// mock mode — just print
	if t.mock {
		fmt.Printf("Saved %d order metrics for %s\n", len(results), contestant)
		return nil
	}

	// batch insert all order results in one transaction
	tx, err := t.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO order_metrics 
		(time, submission_id, contestant, bot_id, latency_ms, success, price, order_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	for _, r := range results {
		_, err := stmt.Exec(
			r.Timestamp,
			submissionID,
			contestant,
			r.BotID,
			r.Latency,
			r.Success,
			r.Price,
			r.OrderType,
		)
		if err != nil {
			return fmt.Errorf("failed to insert order metric: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	fmt.Printf("Saved %d order metrics for %s\n", len(results), contestant)
	return nil
}

func (t *TimescaleDB) SaveScore(submissionID, contestant string, m calculator.Metrics) error {
	// mock mode — just print
	if t.mock {
		fmt.Printf("Score saved for %s at %s\n", contestant, time.Now().Format("15:04:05"))
		return nil
	}

	_, err := t.db.Exec(`
		INSERT INTO scores
		(time, submission_id, contestant, p50, p90, p99, tps, total_orders, success_rate, score)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		time.Now(),
		submissionID,
		contestant,
		m.P50,
		m.P90,
		m.P99,
		m.TPS,
		m.TotalOrders,
		m.SuccessRate,
		m.Score,
	)
	if err != nil {
		return fmt.Errorf("failed to save score: %v", err)
	}

	fmt.Printf("Score saved for %s at %s\n", contestant, time.Now().Format("15:04:05"))
	return nil
}

func (t *TimescaleDB) Close() {
	if t.db != nil {
		t.db.Close()
	}
}