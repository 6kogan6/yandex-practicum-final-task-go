package main

import (
	"log"
	"os"
	"strconv"

	"yandex-practicum-final-task-go/pkg/api"
	"yandex-practicum-final-task-go/pkg/db"
	"yandex-practicum-final-task-go/pkg/server"
)

const (
	defaultPort   = 7540
	defaultDBFile = "scheduler.db"
	defaultWebDir = "web"
)

func main() {
	if err := run(); err != nil {
		log.Println(err)
	}
}

func run() error {
	port := envInt("TODO_PORT", defaultPort)
	dbFile := envString("TODO_DBFILE", defaultDBFile)
	password := os.Getenv("TODO_PASSWORD")
	webDir := envString("TODO_WEBDIR", defaultWebDir)

	database, err := db.Init(dbFile)
	if err != nil {
		return err
	}
	defer database.Close()

	handler := api.NewHandler(database, password, webDir)

	if err := server.Run(port, handler); err != nil {
		return err
	}

	return nil
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envString(name, fallback string) string {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	return raw
}
