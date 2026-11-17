-- Dữ liệu mẫu: 5 khách hàng, ~5000 CDR trong 7 ngày gần nhất, một số cuộc gọi đang diễn ra

TRUNCATE active_calls, cdr, customers RESTART IDENTITY CASCADE;

INSERT INTO customers (code, name, rate_per_min) VALUES
('KH001', 'Công ty An Phát',     800),
('KH002', 'Ngân hàng Bình Minh', 650),
('KH003', 'Shop Cà Phê Sáng',    900),
('KH004', 'Logistics Đông Á',    700),
('KH005', 'Bảo hiểm Hưng Thịnh', 750);

-- Sinh CDR ngẫu nhiên
WITH raw AS (
    SELECT
        g,
        1 + floor(random() * 5)::int                                   AS customer_id,
        now() - (random() * interval '7 days')                          AS start_time,
        random()                                                        AS r_result,
        2 + floor(random() * 20)::int                                   AS connecting,
        floor(random() * 3)::int                                        AS r_side
    FROM generate_series(1, 5000) g
),
calls AS (
    SELECT
        r.*,
        CASE
            WHEN r_result < 0.70 THEN 'NORMAL_CLEARING'
            WHEN r_result < 0.80 THEN 'NO_ANSWER'
            WHEN r_result < 0.88 THEN 'USER_BUSY'
            WHEN r_result < 0.94 THEN 'ORIGINATOR_CANCEL'
            WHEN r_result < 0.97 THEN 'CALL_REJECTED'
            ELSE 'NO_ROUTE_DESTINATION'
        END AS hangup_cause,
        CASE WHEN r_result < 0.70 THEN 5 + floor(random() * 600)::int ELSE 0 END AS billsec
    FROM raw r
)
INSERT INTO cdr (customer_id, caller, callee, start_time, answer_time, end_time,
                 connecting_duration, duration, hangup_cause, hangup_side, fee)
SELECT
    c.customer_id,
    '09' || lpad((floor(random() * 100000000))::text, 8, '0'),
    CASE WHEN random() < 0.5
         THEN '028' || lpad((floor(random() * 10000000))::text, 7, '0')
         ELSE '1900' || lpad((floor(random() * 10000))::text, 4, '0') END,
    c.start_time,
    CASE WHEN c.billsec > 0 THEN c.start_time + make_interval(secs => c.connecting) END,
    c.start_time + make_interval(secs => c.connecting + c.billsec),
    c.connecting,
    c.billsec,
    c.hangup_cause,
    CASE
        WHEN c.hangup_cause = 'NORMAL_CLEARING'      THEN (ARRAY['CALLER','CALLEE'])[1 + c.r_side % 2]
        WHEN c.hangup_cause = 'ORIGINATOR_CANCEL'    THEN 'CALLER'
        WHEN c.hangup_cause IN ('USER_BUSY','CALL_REJECTED') THEN 'CALLEE'
        ELSE 'SYSTEM'
    END,
    -- Cước: làm tròn lên theo phút * giá của khách hàng
    ceil(c.billsec / 60.0) * cu.rate_per_min
FROM calls c
JOIN customers cu ON cu.id = c.customer_id;

-- Cuộc gọi đang diễn ra
INSERT INTO active_calls (customer_id, caller, callee, start_time, state)
SELECT
    1 + floor(random() * 5)::int,
    '09' || lpad((floor(random() * 100000000))::text, 8, '0'),
    '028' || lpad((floor(random() * 10000000))::text, 7, '0'),
    now() - (random() * interval '10 minutes'),
    CASE WHEN random() < 0.2 THEN 'RINGING' ELSE 'ANSWERED' END
FROM generate_series(1, 37);
