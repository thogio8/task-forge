package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thogio8/task-forge/internal/model"
)

func CreateTask(ctx context.Context, db *pgxpool.Pool, task model.Task) error {
	query := `
	INSERT INTO tasks (id, status, payload, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5)
	`

	_, err := db.Exec(ctx, query,
		task.ID,
		task.Status,
		task.Payload,
		task.CreatedAt,
		task.UpdatedAt,
	)

	if err != nil {
		return err
	}

	return nil
}

func GetTasks(ctx context.Context, db *pgxpool.Pool) ([]model.Task, error) {
	query := `
		SELECT * FROM tasks
	`

	rows, err := db.Query(ctx, query)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var tasks []model.Task

	for rows.Next() {
		var task model.Task
		if err := rows.Scan(&task.ID, &task.Status, &task.Payload, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}
