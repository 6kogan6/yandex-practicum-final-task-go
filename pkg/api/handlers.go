package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"yandex-practicum-final-task-go/pkg/service"
	sqliteStorage "yandex-practicum-final-task-go/pkg/storage/sqlite"
)

type Handler struct {
	tasks    *service.Service
	password string
}

type taskPayload struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Comment string `json:"comment"`
	Repeat  string `json:"repeat"`
}

type taskResponse struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Comment string `json:"comment"`
	Repeat  string `json:"repeat"`
}

type tasksResponse struct {
	Tasks []taskResponse `json:"tasks"`
}

type idResponse struct {
	ID string `json:"id"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

func NewHandler(db *sql.DB, password, webDir string) http.Handler {
	tasks := service.New(sqliteStorage.NewTaskStorage(db))
	return newHandler(tasks, password, webDir)
}

func newHandler(tasks *service.Service, password, webDir string) http.Handler {
	h := &Handler{
		tasks:    tasks,
		password: password,
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

const jwtHeaderJSON = `{"alg":"HS256","typ":"JWT"}`

func passwordHash(password string) string {
	if password == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func makeToken(password string) string {
	if password == "" {
		return ""
	}

	payload, _ := json.Marshal(map[string]string{"hash": passwordHash(password)})
	headerPart := base64.RawURLEncoding.EncodeToString([]byte(jwtHeaderJSON))
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	unsigned := headerPart + "." + payloadPart

	mac := hmac.New(sha256.New, []byte(password))
	mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return unsigned + "." + signature
}

func validateToken(token, password string) bool {
	if token == "" || password == "" {
		return false
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(headerBytes) != jwtHeaderJSON {
		return false
	}

	unsigned := parts[0] + "." + parts[1]
	signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(password))
	mac.Write([]byte(unsigned))
	expectedSignature := mac.Sum(nil)
	if !hmac.Equal(signatureBytes, expectedSignature) {
		return false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	var claims struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return false
	}

	return claims.Hash == passwordHash(password)
}

func (h *Handler) withAuth(next http.HandlerFunc) http.HandlerFunc {
	if h.password == "" {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("token")
		if err != nil || !validateToken(cookie.Value, h.password) {
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

	nextDate, err := h.tasks.NextDate(
		r.URL.Query().Get("date"),
		r.URL.Query().Get("repeat"),
		r.URL.Query().Get("now"),
		time.Now(),
	)
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

	writeJSON(w, http.StatusOK, tokenResponse{Token: makeToken(h.password)})
}

func (h *Handler) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusBadRequest, "invalid method")
		return
	}

	tasks, err := h.tasks.ListTasks(r.URL.Query().Get("search"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to load tasks")
		return
	}

	items := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, taskResponse{
			ID:      strconv.FormatInt(task.ID, 10),
			Date:    task.Date,
			Title:   task.Title,
			Comment: task.Comment,
			Repeat:  task.Repeat,
		})
	}

	writeJSON(w, http.StatusOK, tasksResponse{Tasks: items})
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
		writeError(w, http.StatusMethodNotAllowed, "invalid method")
	}
}

func (h *Handler) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	id, err := service.ParseID(r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	task, err := h.tasks.GetTask(id)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusBadRequest, "failed to load task")
		return
	}

	writeJSON(w, http.StatusOK, taskResponse{
		ID:      strconv.FormatInt(task.ID, 10),
		Date:    task.Date,
		Title:   task.Title,
		Comment: task.Comment,
		Repeat:  task.Repeat,
	})
}

func (h *Handler) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	var req taskPayload
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	id, err := h.tasks.CreateTask(service.TaskPayload{
		Date:    req.Date,
		Title:   req.Title,
		Comment: req.Comment,
		Repeat:  req.Repeat,
	}, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, idResponse{ID: strconv.FormatInt(id, 10)})
}

func (h *Handler) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	var req taskPayload
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	id, err := service.ParseID(req.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	updated, err := h.tasks.UpdateTask(id, service.TaskPayload{
		Date:    req.Date,
		Title:   req.Title,
		Comment: req.Comment,
		Repeat:  req.Repeat,
	}, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !updated {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, struct{}{})
}

func (h *Handler) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	id, err := service.ParseID(r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	deleted, err := h.tasks.DeleteTask(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to delete task")
		return
	}

	if !deleted {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, struct{}{})
}

func (h *Handler) handleDone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusBadRequest, "invalid method")
		return
	}

	id, err := service.ParseID(r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	completed, err := h.tasks.CompleteTask(id, time.Now())
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !completed {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, struct{}{})
}

func decodeJSON(r *http.Request, dst any) error {
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
