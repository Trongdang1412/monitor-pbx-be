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

// waitForDB thử kết nối lại trong ~60 giây (DB có thể khởi động chậm hơn backend).
func waitForDB(ctx context.Context, db *pgxpool.Pool) error {
	var err error
	for i := 0; i < 30; i++ {
		if err = db.Ping(ctx); err == nil {
			return nil
		}
		log.Printf("chờ DB... (%v)", err)
		time.Sleep(2 * time.Second)
	}
	return err
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
