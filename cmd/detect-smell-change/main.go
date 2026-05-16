package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
	slackToken := os.Getenv("SLACK_BOT_TOKEN")
	if slackToken == "" {
		log.Fatal("SLACK_BOT_TOKEN is required")
	}
	slackChannel := os.Getenv("SLACK_CHANNEL_ID")
	if slackChannel == "" {
		log.Fatal("SLACK_CHANNEL_ID is required")
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
		HAVING diff > 0.005 AND diff < 0.013
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

		// 5分以内に通知済みならスキップ
		var count int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM smell_notifications
			WHERE sensor_id = ? AND notified_at >= NOW() - INTERVAL 5 MINUTE
		`, sensorID).Scan(&count); err != nil {
			log.Printf("failed to check cooldown: %v", err)
			continue
		}
		if count > 0 {
			log.Printf("skipping %s (cooldown)", sensorID)
			continue
		}

		now := time.Now().In(jst).Format("2006-01-02 15:04:05")
		msg := fmt.Sprintf("センサーの値の変化を検知しました。猫がトイレをした可能性があります。\nsensor_id: %s\ndiff: %.6f\ncurrent_value: %.6f\n時刻: %s (JST)", sensorID, diff, currentValue, now)
		log.Println(msg)

		body, _ := json.Marshal(map[string]string{"channel": slackChannel, "text": msg})
		req, _ := http.NewRequest(http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+slackToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("failed to send slack notification: %v", err)
			continue
		}
		var slackResp struct {
			OK bool   `json:"ok"`
			TS string `json:"ts"`
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		json.Unmarshal(respBody, &slackResp)
		log.Printf("slack response: %s", string(respBody))

		var slackTS *string
		if slackResp.OK && slackResp.TS != "" {
			slackTS = &slackResp.TS
		}

		// 通知時刻を記録
		if _, err := db.Exec(`
			INSERT INTO smell_notifications (sensor_id, slack_ts, notified_at) VALUES (?, ?, NOW())
		`, sensorID, slackTS); err != nil {
			log.Printf("failed to record notification: %v", err)
		}
	}
}
