-- Schema cho Monitor PBX (PostgreSQL)
-- Backend tự chạy file này khi khởi động; an toàn khi chạy lại (không xoá dữ liệu).

-- Khách hàng (KH)
CREATE TABLE IF NOT EXISTS customers (
    id             SERIAL PRIMARY KEY,
    code           VARCHAR(20)  NOT NULL UNIQUE,
    name           VARCHAR(100) NOT NULL,
    rate_per_min   NUMERIC(10,2) NOT NULL DEFAULT 800   -- Giá cước VND / phút
);

-- Call Detail Record
CREATE TABLE IF NOT EXISTS cdr (
    id                   BIGSERIAL PRIMARY KEY,
    customer_id          INT REFERENCES customers(id),
    caller               VARCHAR(30) NOT NULL,          -- Số gọi đi
    callee               VARCHAR(30) NOT NULL,          -- Số bị gọi
    start_time           TIMESTAMPTZ NOT NULL,          -- Thời điểm bắt đầu cuộc gọi
    answer_time          TIMESTAMPTZ,                   -- Thời điểm nhấc máy (NULL nếu không nghe)
    end_time             TIMESTAMPTZ NOT NULL,          -- Thời điểm kết thúc
    connecting_duration  INT NOT NULL DEFAULT 0,        -- Thời gian đổ chuông/kết nối (giây)
    duration             INT NOT NULL DEFAULT 0,        -- Thời gian đàm thoại (billsec, giây)
    hangup_cause         VARCHAR(50) NOT NULL,          -- NORMAL_CLEARING, USER_BUSY, ...
    hangup_side          VARCHAR(10) NOT NULL,          -- CALLER / CALLEE / SYSTEM
    fee                  NUMERIC(12,2) NOT NULL DEFAULT 0  -- Cước phí (VND)
);

CREATE INDEX IF NOT EXISTS idx_cdr_start_time  ON cdr (start_time);
CREATE INDEX IF NOT EXISTS idx_cdr_customer    ON cdr (customer_id);
CREATE INDEX IF NOT EXISTS idx_cdr_caller      ON cdr (caller);
CREATE INDEX IF NOT EXISTS idx_cdr_callee      ON cdr (callee);

-- Cuộc gọi đang diễn ra (dùng để tính CCU realtime)
CREATE TABLE IF NOT EXISTS active_calls (
    id           BIGSERIAL PRIMARY KEY,
    customer_id  INT REFERENCES customers(id),
    caller       VARCHAR(30) NOT NULL,
    callee       VARCHAR(30) NOT NULL,
    start_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
    state        VARCHAR(20) NOT NULL DEFAULT 'ANSWERED'  -- RINGING / ANSWERED
);
