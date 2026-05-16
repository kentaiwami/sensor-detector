package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

const rssiThreshold = -68

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

	// 直近1分の平均RSSIがthreshold以上のlocationを取得
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
		log.Fatal("failed to query:", err)
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
		log.Fatal("failed to insert:", err)
	}
}
