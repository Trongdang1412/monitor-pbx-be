package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type API struct {
	db    *pgxpool.Pool
	ready atomic.Bool // true khi DB đã kết nối và tạo bảng xong
}

// requireDB: khi DB chưa sẵn sàng thì trả 503 thay vì lỗi 500, để frontend hiện thông báo và tự thử lại.
func (a *API) requireDB(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.ready.Load() && r.URL.Path != "/api/health" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Database chưa sẵn sàng"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- tiện ích đọc tham số ----------

// parseRange đọc from/to (RFC3339). Mặc định: 24 giờ gần nhất.
func parseRange(r *http.Request) (time.Time, time.Time) {
	to := time.Now()
	from := to.Add(-24 * time.Hour)
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}

// where giúp ghép điều kiện SQL với tham số $1, $2...
type where struct {
	conds []string
	args  []any
}

func (w *where) add(cond string, val any) {
	w.args = append(w.args, val)
	w.conds = append(w.conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(w.args))))
}

func (w *where) sql() string {
	if len(w.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.conds, " AND ")
}

func addCustomer(w *where, r *http.Request, col string) {
	if v := r.URL.Query().Get("customer_id"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			w.add(col+" = ?", id)
		}
	}
}

func addNum(w *where, r *http.Request, param, cond string) {
	if v := r.URL.Query().Get(param); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			w.add(cond, n)
		}
	}
}

// ---------- handlers ----------

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if !a.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "starting", "error": "Database chưa sẵn sàng"})
		return
	}
	if err := a.db.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_down", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type Customer struct {
	ID         int     `json:"id"`
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	RatePerMin float64 `json:"rate_per_min"`
}

func (a *API) customers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT id, code, name, rate_per_min::float8 FROM customers ORDER BY code`)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	list := []Customer{}
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.RatePerMin); err != nil {
			writeErr(w, err)
			return
		}
		list = append(list, c)
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) hangupCauses(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT DISTINCT hangup_cause FROM cdr ORDER BY 1`)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	list := []string{}
	for rows.Next() {
		var s string
		rows.Scan(&s)
		list = append(list, s)
	}
	writeJSON(w, http.StatusOK, list)
}

// GET /api/ccu — số cuộc gọi đồng thời hiện tại
func (a *API) ccu(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type byCustomer struct {
		CustomerID int    `json:"customer_id"`
		Name       string `json:"name"`
		CCU        int    `json:"ccu"`
	}
	type activeCall struct {
		ID       int64     `json:"id"`
		Customer string    `json:"customer"`
		Caller   string    `json:"caller"`
		Callee   string    `json:"callee"`
		Start    time.Time `json:"start_time"`
		State    string    `json:"state"`
	}
	res := struct {
		Total      int          `json:"total"`
		Ringing    int          `json:"ringing"`
		Answered   int          `json:"answered"`
		ByCustomer []byCustomer `json:"by_customer"`
		Calls      []activeCall `json:"calls"`
		At         time.Time    `json:"at"`
	}{ByCustomer: []byCustomer{}, Calls: []activeCall{}, At: time.Now()}

	err := a.db.QueryRow(ctx, `SELECT count(*),
		count(*) FILTER (WHERE state='RINGING'),
		count(*) FILTER (WHERE state='ANSWERED') FROM active_calls`).
		Scan(&res.Total, &res.Ringing, &res.Answered)
	if err != nil {
		writeErr(w, err)
		return
	}

	rows, err := a.db.Query(ctx, `SELECT c.id, c.name, count(a.id)
		FROM customers c LEFT JOIN active_calls a ON a.customer_id = c.id
		GROUP BY c.id, c.name ORDER BY count(a.id) DESC, c.name`)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var b byCustomer
		rows.Scan(&b.CustomerID, &b.Name, &b.CCU)
		res.ByCustomer = append(res.ByCustomer, b)
	}
	rows.Close()

	rows, err = a.db.Query(ctx, `SELECT a.id, COALESCE(c.name,''), a.caller, a.callee, a.start_time, a.state
		FROM active_calls a LEFT JOIN customers c ON c.id = a.customer_id
		ORDER BY a.start_time LIMIT 100`)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var c activeCall
		rows.Scan(&c.ID, &c.Customer, &c.Caller, &c.Callee, &c.Start, &c.State)
		res.Calls = append(res.Calls, c)
	}
	rows.Close()

	writeJSON(w, http.StatusOK, res)
}

// GET /api/ccu/history — CCU theo thời gian, tính từ CDR (số cuộc gọi chồng lấn mỗi mốc)
func (a *API) ccuHistory(w http.ResponseWriter, r *http.Request) {
	from, to := parseRange(r)
	// Chọn bước thời gian để có khoảng ~100-300 điểm
	span := to.Sub(from)
	step := 5 * time.Minute
	switch {
	case span <= 2*time.Hour:
		step = time.Minute
	case span <= 24*time.Hour:
		step = 5 * time.Minute
	case span <= 3*24*time.Hour:
		step = 15 * time.Minute
	default:
		step = time.Hour
	}

	q := `SELECT t, (SELECT count(*) FROM cdr c
			WHERE c.start_time <= t AND c.end_time > t %s) AS ccu
		  FROM generate_series($1::timestamptz, $2::timestamptz, $3::interval) t ORDER BY t`
	args := []any{from, to, fmt.Sprintf("%d seconds", int(step.Seconds()))}
	extra := ""
	if v := r.URL.Query().Get("customer_id"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			extra = "AND c.customer_id = $4"
			args = append(args, id)
		}
	}

	rows, err := a.db.Query(r.Context(), fmt.Sprintf(q, extra), args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	type point struct {
		T   time.Time `json:"t"`
		CCU int       `json:"ccu"`
	}
	points := []point{}
	peak := 0
	for rows.Next() {
		var p point
		rows.Scan(&p.T, &p.CCU)
		if p.CCU > peak {
			peak = p.CCU
		}
		points = append(points, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"step_seconds": int(step.Seconds()), "peak": peak, "points": points})
}

// GET /api/summary — tổng hợp CDR: tổng cuộc gọi, ASR, ACD, cước
func (a *API) summary(w http.ResponseWriter, r *http.Request) {
	from, to := parseRange(r)
	wh := &where{}
	wh.add("start_time >= ?", from)
	wh.add("start_time < ?", to)
	addCustomer(wh, r, "customer_id")

	type sum struct {
		TotalCalls    int     `json:"total_calls"`
		AnsweredCalls int     `json:"answered_calls"`
		FailedCalls   int     `json:"failed_calls"`
		TotalDuration int64   `json:"total_duration"` // giây
		ASR           float64 `json:"asr"`            // % cuộc gọi thành công
		ACD           float64 `json:"acd"`            // thời lượng TB cuộc gọi thành công (giây)
		TotalFee      float64 `json:"total_fee"`
	}
	var s sum
	err := a.db.QueryRow(r.Context(), `SELECT count(*),
			count(*) FILTER (WHERE duration > 0),
			COALESCE(sum(duration),0),
			COALESCE(sum(fee),0)::float8
		FROM cdr`+wh.sql(), wh.args...).
		Scan(&s.TotalCalls, &s.AnsweredCalls, &s.TotalDuration, &s.TotalFee)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.FailedCalls = s.TotalCalls - s.AnsweredCalls
	if s.TotalCalls > 0 {
		s.ASR = float64(s.AnsweredCalls) * 100 / float64(s.TotalCalls)
	}
	if s.AnsweredCalls > 0 {
		s.ACD = float64(s.TotalDuration) / float64(s.AnsweredCalls)
	}

	// Phân bổ theo nguyên nhân ngắt máy
	type kv struct {
		Key   string `json:"key"`
		Count int    `json:"count"`
	}
	causes := []kv{}
	rows, err := a.db.Query(r.Context(), `SELECT hangup_cause, count(*) FROM cdr`+wh.sql()+
		` GROUP BY 1 ORDER BY 2 DESC`, wh.args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var k kv
		rows.Scan(&k.Key, &k.Count)
		causes = append(causes, k)
	}
	rows.Close()

	// Theo khách hàng
	type custSum struct {
		Name     string  `json:"name"`
		Total    int     `json:"total"`
		Answered int     `json:"answered"`
		Duration int64   `json:"duration"`
		Fee      float64 `json:"fee"`
	}
	byCust := []custSum{}
	rows, err = a.db.Query(r.Context(), `SELECT COALESCE(cu.name,'(không rõ)'), count(*),
			count(*) FILTER (WHERE duration > 0), COALESCE(sum(duration),0), COALESCE(sum(fee),0)::float8
		FROM cdr LEFT JOIN customers cu ON cu.id = cdr.customer_id`+
		strings.ReplaceAll(wh.sql(), "customer_id", "cdr.customer_id")+
		` GROUP BY 1 ORDER BY 2 DESC`, wh.args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var c custSum
		rows.Scan(&c.Name, &c.Total, &c.Answered, &c.Duration, &c.Fee)
		byCust = append(byCust, c)
	}
	rows.Close()

	writeJSON(w, http.StatusOK, map[string]any{
		"from": from, "to": to, "summary": s, "hangup_causes": causes, "by_customer": byCust,
	})
}

type CDR struct {
	ID                 int64      `json:"id"`
	CustomerID         *int       `json:"customer_id"`
	Customer           *string    `json:"customer"`
	Caller             string     `json:"caller"`
	Callee             string     `json:"callee"`
	StartTime          time.Time  `json:"start_time"`
	AnswerTime         *time.Time `json:"answer_time"`
	EndTime            time.Time  `json:"end_time"`
	ConnectingDuration int        `json:"connecting_duration"`
	Duration           int        `json:"duration"`
	HangupCause        string     `json:"hangup_cause"`
	HangupSide         string     `json:"hangup_side"`
	Fee                float64    `json:"fee"`
}

// GET /api/cdr — danh sách CDR có lọc + phân trang
func (a *API) cdrList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	wh := &where{}
	if q.Get("from") != "" || q.Get("to") != "" {
		from, to := parseRange(r)
		wh.add("c.start_time >= ?", from)
		wh.add("c.start_time < ?", to)
	}
	addCustomer(wh, r, "c.customer_id")
	if v := strings.TrimSpace(q.Get("caller")); v != "" {
		wh.add("c.caller ILIKE ?", "%"+v+"%")
	}
	if v := strings.TrimSpace(q.Get("callee")); v != "" {
		wh.add("c.callee ILIKE ?", "%"+v+"%")
	}
	if v := q.Get("hangup_cause"); v != "" {
		wh.add("c.hangup_cause = ?", v)
	}
	if v := q.Get("hangup_side"); v != "" {
		wh.add("c.hangup_side = ?", v)
	}
	addNum(wh, r, "duration_min", "c.duration >= ?")
	addNum(wh, r, "duration_max", "c.duration <= ?")
	addNum(wh, r, "connecting_min", "c.connecting_duration >= ?")
	addNum(wh, r, "connecting_max", "c.connecting_duration <= ?")
	addNum(wh, r, "fee_min", "c.fee >= ?")
	addNum(wh, r, "fee_max", "c.fee <= ?")

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(q.Get("page_size"))
	if size < 1 || size > 500 {
		size = 50
	}

	var total int
	var totalFee float64
	if err := a.db.QueryRow(r.Context(), `SELECT count(*), COALESCE(sum(fee),0)::float8 FROM cdr c`+wh.sql(), wh.args...).
		Scan(&total, &totalFee); err != nil {
		writeErr(w, err)
		return
	}

	args := append(wh.args, size, (page-1)*size)
	rows, err := a.db.Query(r.Context(), fmt.Sprintf(`SELECT c.id, c.customer_id, cu.name, c.caller, c.callee,
			c.start_time, c.answer_time, c.end_time, c.connecting_duration, c.duration,
			c.hangup_cause, c.hangup_side, c.fee::float8
		FROM cdr c LEFT JOIN customers cu ON cu.id = c.customer_id %s
		ORDER BY c.start_time DESC LIMIT $%d OFFSET $%d`, wh.sql(), len(args)-1, len(args)), args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []CDR{}
	for rows.Next() {
		var c CDR
		if err := rows.Scan(&c.ID, &c.CustomerID, &c.Customer, &c.Caller, &c.Callee, &c.StartTime,
			&c.AnswerTime, &c.EndTime, &c.ConnectingDuration, &c.Duration,
			&c.HangupCause, &c.HangupSide, &c.Fee); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "total_fee": totalFee, "page": page, "page_size": size, "items": items,
	})
}
