package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/btse/hedwig/internal/storage"
)

func main() {
	dbPath := flag.String("db", "hedwig-dev.db", "path to sqlite database")
	flag.Parse()

	db, err := storage.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("migrations applied successfully:", *dbPath)
}
