package sqlite

import (
	"database/sql"
	"errors"

	"yandex-practicum-final-task-go/pkg/service"
)

type TaskStorage struct {
	db *sql.DB
}

func NewTaskStorage(db *sql.DB) *TaskStorage {
	return &TaskStorage{db: db}
}

func (s *TaskStorage) ListTasks(limit int) ([]service.Task, error) {
	rows, err := s.db.Query(
		`SELECT id, date, title, comment, "repeat" FROM scheduler ORDER BY date LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (s *TaskStorage) ListTasksByDate(date string, limit int) ([]service.Task, error) {
	rows, err := s.db.Query(
		`SELECT id, date, title, comment, "repeat" FROM scheduler WHERE date = ? ORDER BY date LIMIT ?`,
		date, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (s *TaskStorage) ListTasksByPattern(pattern string, limit int) ([]service.Task, error) {
	rows, err := s.db.Query(
		`SELECT id, date, title, comment, "repeat"
         FROM scheduler
         WHERE title LIKE ? OR comment LIKE ?
         ORDER BY date
         LIMIT ?`,
		pattern, pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (s *TaskStorage) GetTask(id int64) (service.Task, error) {
	var task service.Task
	err := s.db.QueryRow(
		`SELECT id, date, title, comment, "repeat" FROM scheduler WHERE id = ?`,
		id,
	).Scan(&task.ID, &task.Date, &task.Title, &task.Comment, &task.Repeat)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.Task{}, service.ErrTaskNotFound
		}
		return service.Task{}, err
	}

	return task, nil
}

func (s *TaskStorage) CreateTask(task service.Task) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO scheduler (date, title, comment, "repeat") VALUES (?, ?, ?, ?)`,
		task.Date, task.Title, task.Comment, task.Repeat,
	)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func (s *TaskStorage) UpdateTask(task service.Task) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE scheduler SET date = ?, title = ?, comment = ?, "repeat" = ? WHERE id = ?`,
		task.Date, task.Title, task.Comment, task.Repeat, task.ID,
	)
	if err != nil {
		return false, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func (s *TaskStorage) DeleteTask(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM scheduler WHERE id = ?`, id)
	if err != nil {
		return false, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func (s *TaskStorage) UpdateTaskDate(id int64, date string) (bool, error) {
	res, err := s.db.Exec(`UPDATE scheduler SET date = ? WHERE id = ?`, date, id)
	if err != nil {
		return false, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func scanTasks(rows *sql.Rows) ([]service.Task, error) {
	tasks := make([]service.Task, 0)
	for rows.Next() {
		var task service.Task
		if err := rows.Scan(&task.ID, &task.Date, &task.Title, &task.Comment, &task.Repeat); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}
