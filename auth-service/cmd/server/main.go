package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ahmadhaffoudhi/auth-service/internal/adapter/httpapi"
	"github.com/ahmadhaffoudhi/auth-service/internal/adapter/memory"
	"github.com/ahmadhaffoudhi/auth-service/internal/usecase"
)

func main() {
	repo := memory.NewUserRepository()
	service := usecase.NewUserService(repo)
	handler := httpapi.NewHandler(service)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("auth-service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler.Router()))
}
