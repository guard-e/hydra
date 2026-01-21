package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// TestServer для демонстрации рабочего Domain Fronting
func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Логируем все заголовки для отладки
		log.Printf("=== ВХОДЯЩИЙ ЗАПРОС ===")
		log.Printf("Method: %s", r.Method)
		log.Printf("Host: %s", r.Host)
		log.Printf("URL: %s", r.URL.String())
		
		for name, values := range r.Header {
			log.Printf("Header %s: %s", name, strings.Join(values, ", "))
		}
		
		// Проверяем Domain Fronting
		if r.Host == "secret-chat.appspot.com" {
			// Это запрос через Domain Fronting!
			log.Printf("✓ Обнаружен Domain Fronting запрос!")
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":   "success",
				"message":  "Domain Fronting работает! Сообщение доставлено.",
				"technique": "SNI: ajax.googleapis.com, Host: secret-chat.appspot.com",
			})
			return
		}
		
		// Обычный запрос
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "direct",
			"message": "Прямое соединение с сервером",
			"host":    r.Host,
		})
	})
	
	log.Printf("Тестовый сервер запущен на порту %s", port)
	log.Printf("Для теста Domain Fronting используйте:")
	log.Printf("  Front Domain: ajax.googleapis.com")
	log.Printf("  Hidden Domain: secret-chat.appspot.com")
	log.Printf("  Real Server: localhost:%s", port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}