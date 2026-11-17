package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// runSimulator giả lập tổng đài: mỗi 3 giây thêm vài cuộc gọi mới vào active_calls,
// kết thúc vài cuộc gọi cũ và ghi chúng vào bảng cdr. Bật bằng SIMULATE=true.
func runSimulator(ctx context.Context, db *pgxpool.Pool) {
	log.Println("Bộ giả lập cuộc gọi đã bật")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// RINGING -> ANSWERED sau vài giây
		db.Exec(ctx, `UPDATE active_calls SET state='ANSWERED'
			WHERE state='RINGING' AND start_time < now() - interval '5 seconds'`)

		// Thêm cuộc gọi mới
		n := rand.Intn(6)
		for i := 0; i < n; i++ {
			_, err := db.Exec(ctx, `INSERT INTO active_calls (customer_id, caller, callee, state)
				SELECT id, '09' || lpad((floor(random()*100000000))::text, 8, '0'),
				       '028' || lpad((floor(random()*10000000))::text, 7, '0'), 'RINGING'
				FROM customers ORDER BY random() LIMIT 1`)
			if err != nil {
				log.Printf("simulator insert: %v", err)
			}
		}

		// Kết thúc ~1/12 số cuộc gọi đang có (giữ CCU dao động ổn định) và chuyển sang CDR
		_, err := db.Exec(ctx, `WITH ended AS (
				DELETE FROM active_calls WHERE id IN (
					SELECT id FROM active_calls ORDER BY random()
					LIMIT (SELECT count(*) / 12 + floor(random() * 3)::int FROM active_calls))
				RETURNING *)
			INSERT INTO cdr (customer_id, caller, callee, start_time, answer_time, end_time,
				connecting_duration, duration, hangup_cause, hangup_side, fee)
			SELECT e.customer_id, e.caller, e.callee, e.start_time,
				CASE WHEN e.state='ANSWERED' THEN e.start_time + interval '5 seconds' END,
				now(),
				CASE WHEN e.state='ANSWERED' THEN 5 ELSE GREATEST(extract(epoch FROM now()-e.start_time)::int,0) END,
				CASE WHEN e.state='ANSWERED' THEN GREATEST(extract(epoch FROM now()-e.start_time)::int - 5, 1) ELSE 0 END,
				CASE WHEN e.state='ANSWERED' THEN 'NORMAL_CLEARING' ELSE 'NO_ANSWER' END,
				CASE WHEN e.state='ANSWERED' THEN (ARRAY['CALLER','CALLEE'])[1+floor(random()*2)::int] ELSE 'SYSTEM' END,
				CASE WHEN e.state='ANSWERED'
					THEN ceil(GREATEST(extract(epoch FROM now()-e.start_time) - 5, 1) / 60.0) * cu.rate_per_min
					ELSE 0 END
			FROM ended e LEFT JOIN customers cu ON cu.id = e.customer_id`)
		if err != nil {
			log.Printf("simulator end: %v", err)
		}
	}
}
