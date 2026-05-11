package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	tasksLimit       = 50
	searchDateLayout = "02.01.2006"
)

var (
	ErrInvalidID    = errors.New("invalid id")
	ErrTaskNotFound = errors.New("task not found")
)

type Task struct {
	ID      int64
	Date    string
	Title   string
	Comment string
	Repeat  string
}

type TaskPayload struct {
	Date    string
	Title   string
	Comment string
	Repeat  string
}

type TaskStorage interface {
	ListTasks(limit int) ([]Task, error)
	ListTasksByDate(date string, limit int) ([]Task, error)
	ListTasksByPattern(pattern string, limit int) ([]Task, error)
	GetTask(id int64) (Task, error)
	CreateTask(task Task) (int64, error)
	UpdateTask(task Task) (bool, error)
	DeleteTask(id int64) (bool, error)
	UpdateTaskDate(id int64, date string) (bool, error)
}

type Service struct {
	storage TaskStorage
}

func New(storage TaskStorage) *Service {
	return &Service{storage: storage}
}

func ParseID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, ErrInvalidID
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrInvalidID
	}

	return id, nil
}

func (s *Service) NextDate(dateRaw, repeatRaw, nowRaw string, now time.Time) (string, error) {
	date, err := parseDateYYYYMMDD(strings.TrimSpace(dateRaw))
	if err != nil {
		return "", fmt.Errorf("invalid date")
	}

	current := dayOnly(now)
	if nowRaw != "" {
		current, err = parseDateYYYYMMDD(strings.TrimSpace(nowRaw))
		if err != nil {
			return "", fmt.Errorf("invalid now")
		}
	}

	return calcNextDate(current, date, repeatRaw)
}

func (s *Service) ListTasks(search string) ([]Task, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return s.storage.ListTasks(tasksLimit)
	}

	if date, ok := parseSearchDate(search); ok {
		return s.storage.ListTasksByDate(formatDateYYYYMMDD(date), tasksLimit)
	}

	pattern := "%" + search + "%"
	return s.storage.ListTasksByPattern(pattern, tasksLimit)
}

func (s *Service) GetTask(id int64) (Task, error) {
	return s.storage.GetTask(id)
}

func (s *Service) CreateTask(payload TaskPayload, now time.Time) (int64, error) {
	date, repeat, err := validateAndPrepareTask(payload.Date, payload.Title, payload.Repeat, now)
	if err != nil {
		return 0, err
	}

	return s.storage.CreateTask(Task{
		Date:    date,
		Title:   strings.TrimSpace(payload.Title),
		Comment: payload.Comment,
		Repeat:  repeat,
	})
}

func (s *Service) UpdateTask(id int64, payload TaskPayload, now time.Time) (bool, error) {
	date, repeat, err := validateAndPrepareTask(payload.Date, payload.Title, payload.Repeat, now)
	if err != nil {
		return false, err
	}

	return s.storage.UpdateTask(Task{
		ID:      id,
		Date:    date,
		Title:   strings.TrimSpace(payload.Title),
		Comment: payload.Comment,
		Repeat:  repeat,
	})
}

func (s *Service) DeleteTask(id int64) (bool, error) {
	return s.storage.DeleteTask(id)
}

func (s *Service) CompleteTask(id int64, now time.Time) (bool, error) {
	task, err := s.storage.GetTask(id)
	if err != nil {
		return false, err
	}

	if strings.TrimSpace(task.Repeat) == "" {
		deleted, err := s.storage.DeleteTask(id)
		if err != nil {
			return false, err
		}
		if !deleted {
			return false, ErrTaskNotFound
		}
		return true, nil
	}

	date, err := parseDateYYYYMMDD(task.Date)
	if err != nil {
		return false, fmt.Errorf("invalid task date")
	}

	nextDate, err := calcNextDate(dayOnly(now), date, task.Repeat)
	if err != nil {
		return false, err
	}

	updated, err := s.storage.UpdateTaskDate(id, nextDate)
	if err != nil {
		return false, err
	}
	if !updated {
		return false, ErrTaskNotFound
	}

	return true, nil
}

func parseSearchDate(search string) (time.Time, bool) {
	dt, err := time.Parse(searchDateLayout, search)
	if err != nil {
		return time.Time{}, false
	}

	if dt.Format(searchDateLayout) != search {
		return time.Time{}, false
	}

	return dt, true
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
