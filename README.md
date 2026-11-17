# monitor-pbx-be

Backend Go cho Monitor PBX, kết nối PostgreSQL (driver `pgx`).

## Cấu hình (biến môi trường)

| Biến | Mặc định | Ghi chú |
|---|---|---|
| `PORT` | `8080` | Cổng HTTP |
| `DATABASE_URL` | — | Nếu đặt sẽ bỏ qua các biến `DB_*` |
| `DB_HOST` / `DB_PORT` | `localhost` / `5432` | |
| `DB_NAME` | `monitor_pbx` | |
| `DB_USER` / `DB_PASSWORD` | `postgres` / rỗng | |
| `CORS_ORIGIN` | `*` | |
| `SIMULATE` | `false` | `true` = giả lập cuộc gọi để CCU thay đổi realtime |

## Khởi tạo DB

```bash
createdb monitor_pbx
psql -d monitor_pbx -f db/schema.sql
psql -d monitor_pbx -f db/seed.sql   # dữ liệu mẫu: 5 KH, 5000 CDR / 7 ngày
```

## Chạy

```bash
DB_USER=postgres DB_PASSWORD=secret SIMULATE=true go run .
# hoặc build: go build -o monitor-pbx-be . && ./monitor-pbx-be
```

## API

| Method | Path | Tham số |
|---|---|---|
| GET | `/api/health` | |
| GET | `/api/customers` | |
| GET | `/api/hangup-causes` | |
| GET | `/api/ccu` | CCU hiện tại, theo KH, danh sách cuộc gọi đang diễn ra |
| GET | `/api/ccu/history` | `from`, `to` (RFC3339), `customer_id` |
| GET | `/api/summary` | `from`, `to`, `customer_id` → tổng cuộc gọi, ASR, ACD, Fee, theo hangup cause, theo KH |
| GET | `/api/cdr` | `from`, `to`, `customer_id`, `caller`, `callee`, `hangup_cause`, `hangup_side`, `duration_min/max`, `connecting_min/max`, `fee_min/max`, `page`, `page_size` |

- **ASR** = số cuộc gọi có `duration > 0` / tổng cuộc gọi × 100
- **ACD** = tổng `duration` / số cuộc gọi thành công (giây)
