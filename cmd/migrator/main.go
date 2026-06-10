package main

import (
	"flag"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	dir := flag.String("dir", "migrations", "директория с миграциями")
	dsn := flag.String("dsn", "", "строка подключения к БД")
	flag.Parse()

	if *dsn == "" {
		*dsn = os.Getenv("POSTGRES_DSN")
	}
	if *dsn == "" {
		log.Fatal("необходимо указать DSN через --dsn или POSTGRES_DSN")
	}

	db, err := goose.OpenDBWithDriver("pgx", *dsn)
	if err != nil {
		log.Fatalf("подключение к БД: %v", err)
	}
	defer db.Close()

	if err := goose.Up(db, *dir); err != nil {
		log.Fatalf("применение миграций: %v", err)
	}

	log.Println("миграции применены успешно.")
}
