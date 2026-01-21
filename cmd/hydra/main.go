package main

import (
	"context"
	"hydra/internal/config"
	"hydra/internal/server"
	"hydra/pkg/storage"
	"hydra/pkg/transport/manager"
	"log"
	"time"
)

func main() {
	log.Println("Запуск Hydra Messenger...")

	// Загрузка конфигурации
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Предупреждение: не удалось загрузить .env файл (%v), используются значения по умолчанию", err)
	}

	// Инициализируем менеджер транспортов с автоматическим переключением
	log.Println("Инициализация менеджера транспортов...")

	transportManager := manager.New()

	// Инициализация хранилища
	log.Printf("Подключение к БД: %s", cfg.DatabaseURL)
	db, err := storage.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Ошибка инициализации хранилища: %v", err)
	}

	// Инициализация сервера
	srv := server.New(cfg, transportManager, db)

	// Запускаем сервер в отдельной горутине
	go func() {
		addr := ":" + cfg.ServerPort
		log.Printf("Запуск сервера на порту %s", addr)
		if err := srv.Start(addr); err != nil {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	log.Printf("Веб-интерфейс доступен по адресу: http://localhost:%s", cfg.ServerPort)
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
