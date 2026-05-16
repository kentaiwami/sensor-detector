package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

const rssiThreshold = -60

func main() {
	godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	// 1分間、10秒ごとに記録（cronjobが毎分起動するため）
	for range 6 {
		record(db)
		time.Sleep(10 * time.Second)
	}
}

func record(db *sql.DB) {
	rows, err := db.Query(`
		SELECT location, AVG(rssi) AS avg_rssi
		FROM ble_rssi
		WHERE recorded_at >= NOW() - INTERVAL 1 MINUTE
		GROUP BY location
		HAVING avg_rssi >= ?
		ORDER BY avg_rssi DESC
		LIMIT 1
	`, rssiThreshold)
	if err != nil {
		log.Printf("failed to query: %v", err)
		return
	}
	defer rows.Close()

	location := "other"
	if rows.Next() {
		var loc string
		var avgRSSI float64
		if err := rows.Scan(&loc, &avgRSSI); err == nil {
			location = loc
			log.Printf("present at %s (avg RSSI: %.1f)", loc, avgRSSI)
		}
	} else {
		log.Println("not present in any known location")
	}

	if _, err := db.Exec("INSERT INTO presence_logs (location) VALUES (?)", location); err != nil {
		log.Printf("failed to insert: %v", err)
	}
}
