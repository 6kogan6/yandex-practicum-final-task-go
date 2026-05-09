package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	db       *sql.DB
	password string
	token    string
}

type taskPayload struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Comment string `json:"comment"`
	Repeat  string `json:"repeat"`
}

type taskRecord struct {
	ID      int64  `db:"id"`
	Date    string `db:"date"`
	Title   string `db:"title"`
	Comment string `db:"comment"`
	Repeat  string `db:"repeat"`
}

func NewHandler(db *sql.DB, password, webDir string) http.Handler {
	h := &Handler{
		db:       db,
		password: password,
		token:    makeToken(password),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/nextdate", h.handleNextDate)
	mux.HandleFunc("/api/signin", h.handleSignIn)
	mux.HandleFunc("/api/tasks", h.withAuth(h.handleTasks))
	mux.HandleFunc("/api/task", h.withAuth(h.handleTask))
	mux.HandleFunc("/api/task/done", h.withAuth(h.handleDone))

	fileServer := http.FileServer(http.Dir(webDir))
	mux.Handle("/", fileServer)
	return mux
}

func makeToken(password string) string {
	if password == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) withAuth(next http.HandlerFunc) http.HandlerFunc {
	if h.password == "" {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("token")
		if err != nil || cookie.Value != h.token {
			writeError(w, http.StatusUnauthorized, "401 Unauthorized")
			return
		}
		next(w, r)
	}
}

func (h *Handler) handleNextDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeText(w, "invalid method")
		return
	}

	dateRaw := r.URL.Query().Get("date")
	repeatRaw := r.URL.Query().Get("repeat")
	nowRaw := r.URL.Query().Get("now")

	date, err := parseDateYYYYMMDD(dateRaw)
	if err != nil {
		writeText(w, "invalid date")
		return
	}

	now := dayOnly(time.Now())
	if nowRaw != "" {
		now, err = parseDateYYYYMMDD(nowRaw)
		if err != nil {
			writeText(w, "invalid now")
			return
		}
	}

	nextDate, err := calcNextDate(now, date, repeatRaw)
	if err != nil {
		writeText(w, err.Error())
		return
	}
	writeText(w, nextDate)
}

func (h *Handler) handleSignIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusBadRequest, "invalid method")
		return
	}

	if h.password == "" {
		writeError(w, http.StatusBadRequest, "password is not set")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.Password != h.password {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": h.token})
}

func (h *Handler) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusBadRequest, "invalid method")
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	var (
		rows *sql.Rows
		err  error
	)

	switch {
	case search == "":
		rows, err = h.db.Query(`SELECT id, date, title, comment, "repeat" FROM scheduler ORDER BY date LIMIT 50`)
	default:
		if dt, parseErr := time.Parse("02.01.2006", search); parseErr == nil {
			rows, err = h.db.Query(
				`SELECT id, date, title, comment, "repeat" FROM scheduler WHERE date = ? ORDER BY date LIMIT 50`,
				dt.Format(dateLayout),
			)
		} else {
			pattern := "%" + search + "%"
			rows, err = h.db.Query(
				`SELECT id, date, title, comment, "repeat"
                 FROM scheduler
                 WHERE title LIKE ? OR comment LIKE ?
                 ORDER BY date
                 LIMIT 50`,
				pattern, pattern,
			)
		}
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to load tasks")
		return
	}
	defer rows.Close()

	tasks := make([]map[string]string, 0)
	for rows.Next() {
		var t taskRecord
		if err := rows.Scan(&t.ID, &t.Date, &t.Title, &t.Comment, &t.Repeat); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse tasks")
			return
		}
		tasks = append(tasks, map[string]string{
			"id":      strconv.FormatInt(t.ID, 10),
			"date":    t.Date,
			"title":   t.Title,
			"comment": t.Comment,
			"repeat":  t.Repeat,
		})
	}

	if err := rows.Err(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to read tasks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (h *Handler) handleTask(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleTaskGet(w, r)
	case http.MethodPost:
		h.handleTaskCreate(w, r)
	case http.MethodPut:
		h.handleTaskUpdate(w, r)
	case http.MethodDelete:
		h.handleTaskDelete(w, r)
	default:
		writeError(w, http.StatusBadRequest, "invalid method")
	}
}

func (h *Handler) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var t taskRecord
	err = h.db.QueryRow(
		`SELECT id, date, title, comment, "repeat" FROM scheduler WHERE id = ?`,
		id,
	).Scan(&t.ID, &t.Date, &t.Title, &t.Comment, &t.Repeat)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusBadRequest, "failed to load task")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":      strconv.FormatInt(t.ID, 10),
		"date":    t.Date,
		"title":   t.Title,
		"comment": t.Comment,
		"repeat":  t.Repeat,
	})
}

func (h *Handler) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	var req taskPayload
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	date, repeat, err := validateAndPrepareTask(req.Date, req.Title, req.Repeat, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.db.Exec(
		`INSERT INTO scheduler (date, title, comment, "repeat") VALUES (?, ?, ?, ?)`,
		date, req.Title, req.Comment, repeat,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to create task")
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to get id")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": strconv.FormatInt(id, 10)})
}

func (h *Handler) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	var req taskPayload
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	id, err := parseID(req.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	date, repeat, err := validateAndPrepareTask(req.Date, req.Title, req.Repeat, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.db.Exec(
		`UPDATE scheduler SET date = ?, title = ?, comment = ?, "repeat" = ? WHERE id = ?`,
		date, req.Title, req.Comment, repeat, id,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to update task")
		return
	}

	affected, err := res.RowsAffected()
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to update task")
		return
	}
	if affected == 0 {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{})
}

func (h *Handler) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	res, err := h.db.Exec(`DELETE FROM scheduler WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to delete task")
		return
	}

	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{})
}

func (h *Handler) handleDone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusBadRequest, "invalid method")
		return
	}

	id, err := parseID(r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var t taskRecord
	err = h.db.QueryRow(
		`SELECT id, date, title, comment, "repeat" FROM scheduler WHERE id = ?`,
		id,
	).Scan(&t.ID, &t.Date, &t.Title, &t.Comment, &t.Repeat)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusBadRequest, "failed to load task")
		return
	}

	if strings.TrimSpace(t.Repeat) == "" {
		_, err = h.db.Exec(`DELETE FROM scheduler WHERE id = ?`, id)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to delete task")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	date, err := parseDateYYYYMMDD(t.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task date")
		return
	}

	nextDate, err := calcNextDate(dayOnly(time.Now()), date, t.Repeat)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = h.db.Exec(`UPDATE scheduler SET date = ? WHERE id = ?`, nextDate, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to update task")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{})
}

func validateAndPrepareTask(dateRaw, title, repeatRaw string, now time.Time) (string, string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", fmt.Errorf("title is required")
	}

	repeatRaw = strings.TrimSpace(repeatRaw)
	if err := validateRepeat(repeatRaw); err != nil {
		return "", "", err
	}

	nowDate := dayOnly(now)

	if strings.TrimSpace(dateRaw) == "" {
		return formatDateYYYYMMDD(nowDate), repeatRaw, nil
	}

	taskDate, err := parseDateYYYYMMDD(dateRaw)
	if err != nil {
		return "", "", fmt.Errorf("invalid date")
	}

	taskDate = dayOnly(taskDate)
	if taskDate.Before(nowDate) {
		if repeatRaw == "" {
			return formatDateYYYYMMDD(nowDate), repeatRaw, nil
		}
		nextDate, err := calcNextDate(nowDate, taskDate, repeatRaw)
		if err != nil {
			return "", "", err
		}
		return nextDate, repeatRaw, nil
	}

	return formatDateYYYYMMDD(taskDate), repeatRaw, nil
}

func parseID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("empty id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		return errors.New("invalid method")
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeText(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(message))
}
