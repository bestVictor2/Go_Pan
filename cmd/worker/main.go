package main

import (
	"Go_Pan/config"
	"Go_Pan/internal/repo"
	"Go_Pan/internal/storage"
	"Go_Pan/internal/worker"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	config.InitConfig()
	repo.InitMysql()
	repo.InitRedis()
	storage.InitMinio()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("download worker started")
	if err := worker.RunDownloadWorker(ctx); err != nil {
		log.Fatalf("download worker stopped: %v", err)
	}
}
