package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeErr(w http.ResponseWriter, err error) {
	log.Printf("lỗi: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

// CORS đơn giản để frontend chạy ở domain/cổng khác vẫn gọi được API
func cors(next http.Handler) http.Handler {
	origin := getenv("CORS_ORIGIN", "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.RequestURI(), time.Since(start).Round(time.Millisecond))
	})
}

func main() {
	dsn := getenv("DATABASE_URL", fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		getenv("DB_USER", "postgres"),
		getenv("DB_PASSWORD", ""),
		getenv("DB_HOST", "localhost"),
		getenv("DB_PORT", "5432"),
		getenv("DB_NAME", "monitor_pbx"),
	))

	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("không tạo được kết nối DB: %v", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		log.Printf("CẢNH BÁO: chưa kết nối được DB: %v", err)
	}

	api := &API{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", api.health)
	mux.HandleFunc("GET /api/customers", api.customers)
	mux.HandleFunc("GET /api/hangup-causes", api.hangupCauses)
	mux.HandleFunc("GET /api/ccu", api.ccu)
	mux.HandleFunc("GET /api/ccu/history", api.ccuHistory)
	mux.HandleFunc("GET /api/summary", api.summary)
	mux.HandleFunc("GET /api/cdr", api.cdrList)

	// Bộ giả lập cuộc gọi (tùy chọn) để CCU thay đổi theo thời gian khi chạy lab
	if getenv("SIMULATE", "false") == "true" {
		go runSimulator(ctx, db)
	}

	port := getenv("PORT", "8080")
	log.Printf("monitor-pbx-be đang chạy tại :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, cors(mux)))
}
