package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// File SQL được nhúng thẳng vào binary, không cần copy kèm khi đóng gói.
//
//go:embed db/schema.sql db/seed.sql
var sqlFiles embed.FS

// initDB chạy nền: thử kết nối + tạo bảng cho tới khi thành công (mỗi 5 giây).
// Trong lúc chờ, HTTP server vẫn chạy và trả 503 cho các API cần DB.
func initDB(ctx context.Context, db *pgxpool.Pool, seed bool, onReady func()) {
	for {
		err := db.Ping(ctx)
		if err == nil {
			err = migrate(ctx, db, seed)
		}
		if err == nil {
			log.Println("DB sẵn sàng")
			onReady()
			return
		}
		log.Printf("DB chưa sẵn sàng, thử lại sau 5s: %v", err)
		time.Sleep(5 * time.Second)
	}
}

// migrate tạo bảng nếu chưa có. Nếu seed=true và DB chưa có khách hàng nào thì nạp dữ liệu mẫu.
func migrate(ctx context.Context, db *pgxpool.Pool, seed bool) error {
	schema, _ := sqlFiles.ReadFile("db/schema.sql")
	if _, err := db.Exec(ctx, string(schema)); err != nil {
		return fmt.Errorf("tạo bảng: %w", err)
	}
	log.Println("Schema OK")

	if !seed {
		return nil
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM customers`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		log.Println("DB đã có dữ liệu, bỏ qua seed")
		return nil
	}
	data, _ := sqlFiles.ReadFile("db/seed.sql")
	if _, err := db.Exec(ctx, string(data)); err != nil {
		return fmt.Errorf("nạp dữ liệu mẫu: %w", err)
	}
	log.Println("Đã nạp dữ liệu mẫu")
	return nil
}
