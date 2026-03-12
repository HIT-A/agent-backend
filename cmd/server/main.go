package main

import (
	"log"

	"hoa-agent-backend/internal/httpserver"
)

func main() {
	s := httpserver.New(":8080")
	log.Fatal(s.ListenAndServe())
}
