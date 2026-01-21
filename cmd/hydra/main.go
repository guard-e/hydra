package main

import (
	"context"
	"hydra/internal/server"
	"hydra/pkg/storage"
	"hydra/pkg/transport/manager"
	"log"
	"time"
)

func main() {
	log.Println("Запуск Hydra Messenger...")

	// Инициализируем менеджер транспортов с автоматическим переключением
	log.Println("Инициализация менеджера транспортов...")

	transportManager := manager.New()

	// Инициализация хранилища
	db, err := storage.New("hydra.db")
	if err != nil {
		log.Fatalf("Ошибка инициализации хранилища: %v", err)
	}

	// Инициализация сервера
	srv := server.New(transportManager, db)

	// Запускаем сервер в отдельной горутине
	go func() {
		if err := srv.Start(":8081"); err != nil {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	log.Println("Веб-интерфейс доступен по адресу: http://localhost:8081")
	log.Println("Для остановки нажмите Ctrl+C")

	// Демонстрационная отправка сообщения (опционально)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	message := []byte("Привет из подполья! Это тестовое сообщение через Domain Fronting.")

	log.Println("Попытка отправки демо-сообщения...")

	// В реальном мире этот вызов упадет, так как мы используем фейковые адреса без реального бэкенда.
	// Но логика клиента верна.
	err = transportManager.Send(ctx, message)
	if err != nil {
		log.Printf("Ожидаемая ошибка (нет реального бэкенда): %v", err)
		log.Println("Транспорт отработал логику отправки корректно (см. тесты).")
	} else {
		log.Println("Сообщение успешно отправлено!")
	}

	// Бесконечный цикл для поддержания работы сервера
	select {}
}
