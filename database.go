package main

import (
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/jmoiron/sqlx"
)

// Database - Base database struct, containing pointer to DB
type Database struct {
	db *sqlx.DB
}

type response struct {
	Status  bool
	Message string
	Data    interface{}
}

func (d *Database) initDB() {
	dbUser := os.Getenv("DB_USER")
	dbIP := os.Getenv("DB_IP")
	dbName := os.Getenv("DB_NAME")
	dbPort := os.Getenv("DB_PORT")
	dbPassword := os.Getenv("DB_PASSWORD")

	var err error

	url := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s", dbIP, dbPort, dbName, dbUser, dbPassword)
	fmt.Println(url)
	d.db, err = sqlx.Connect("pgx", url)

	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to connect to database:", err)
		os.Exit(1)
	}
}
