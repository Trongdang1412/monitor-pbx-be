# monitor-pbx-be

Backend Go (>= 1.25) cho Monitor PBX, dùng PostgreSQL.

Khi khởi động, backend tự chờ DB sẵn sàng (tối đa ~60s) và tự tạo bảng nếu chưa có.
Chỉ cần tạo sẵn một database rỗng, không phải chạy file SQL bằng tay.

## Build & chạy

```bash
CGO_ENABLED=0 go build -o monitor-pbx-be .
DB_HOST=localhost DB_USER=postgres DB_PASSWORD=secret DB_NAME=monitor_pbx SEED=true ./monitor-pbx-be
```

## Biến môi trường

| Biến | Mặc định | Ghi chú |
|---|---|---|
| `PORT` | `8080` | Cổng HTTP |
| `DB_HOST` / `DB_PORT` | `localhost` / `5432` | Trong container không dùng `localhost` |
| `DB_NAME` | `monitor_pbx` | |
| `DB_USER` / `DB_PASSWORD` | `postgres` / rỗng | |
| `DATABASE_URL` | — | Nếu đặt sẽ thay cho các biến `DB_*` |
| `SEED` | `false` | `true` = nạp dữ liệu mẫu nếu DB chưa có dữ liệu |
| `SIMULATE` | `false` | `true` = giả lập cuộc gọi để CCU thay đổi realtime |
| `CORS_ORIGIN` | `*` | |

File SQL (`db/schema.sql`, `db/seed.sql`) được nhúng sẵn trong binary.

## API

| Path | Tham số |
|---|---|
| `GET /api/health` | |
| `GET /api/customers` | |
| `GET /api/hangup-causes` | |
| `GET /api/ccu` | |
| `GET /api/ccu/history` | `from`, `to` (RFC3339), `customer_id` |
| `GET /api/summary` | `from`, `to`, `customer_id` |
| `GET /api/cdr` | `from`, `to`, `customer_id`, `caller`, `callee`, `hangup_cause`, `hangup_side`, `duration_min/max`, `connecting_min/max`, `fee_min/max`, `page`, `page_size` |

ASR = cuộc gọi có `duration > 0` / tổng cuộc gọi × 100. ACD = tổng `duration` / số cuộc gọi thành công.
