package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		log.Fatal("SLACK_WEBHOOK_URL is required")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	rows, err := db.Query(`
		SELECT
			sensor_id,
			AVG(CASE WHEN recorded_at >= NOW() - INTERVAL 1 MINUTE THEN value END) -
			AVG(CASE WHEN recorded_at < NOW() - INTERVAL 1 MINUTE
				AND recorded_at >= NOW() - INTERVAL 5 MINUTE THEN value END) AS diff,
			AVG(CASE WHEN recorded_at >= NOW() - INTERVAL 1 MINUTE THEN value END) AS current_value
		FROM smells
		WHERE recorded_at >= NOW() - INTERVAL 5 MINUTE
		GROUP BY sensor_id
		HAVING diff > 0.01
	`)
	if err != nil {
		log.Fatal("failed to query:", err)
	}
	defer rows.Close()

	jst := time.FixedZone("JST", 9*60*60)

	for rows.Next() {
		var sensorID string
		var diff, currentValue float64
		if err := rows.Scan(&sensorID, &diff, &currentValue); err != nil {
			log.Fatal(err)
		}

		now := time.Now().In(jst).Format("2006-01-02 15:04:05")
		msg := fmt.Sprintf("センサーの値の変化を検知しました。猫がトイレをした可能性があります。\nsensor_id: %s\ndiff: %.6f\ncurrent_value: %.6f\n時刻: %s (JST)", sensorID, diff, currentValue, now)
		log.Println(msg)

		body, _ := json.Marshal(map[string]string{"text": msg})
		resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Printf("failed to send slack notification: %v", err)
			continue
		}
		resp.Body.Close()
	}
}
