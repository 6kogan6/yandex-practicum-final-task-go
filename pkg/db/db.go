package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE scheduler (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	date CHAR(8) NOT NULL DEFAULT "",
	title VARCHAR(256) NOT NULL DEFAULT "",
	comment TEXT NOT NULL DEFAULT "",
	"repeat" VARCHAR(128) NOT NULL DEFAULT "",
	CHECK(length("repeat") <= 128)
);

CREATE INDEX scheduler_date_idx ON scheduler(date);
`

func Open(dbFile string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return db, nil
}

func Init(dbFile string) (*sql.DB, error) {
	install := false
	if _, err := os.Stat(dbFile); err != nil {
		if os.IsNotExist(err) {
			install = true
		} else {
			return nil, fmt.Errorf("stat db file: %w", err)
		}
	}

	db, err := Open(dbFile)
	if err != nil {
		return nil, err
	}

	if install {
		if _, err := db.Exec(schema); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("init schema: %w", err)
		}
	}

	return db, nil
}
