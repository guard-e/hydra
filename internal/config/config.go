package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	ServerPort  string

	// Paths
	VoiceStoragePath string
	WebStaticPath    string

	// WebRTC
	ICEServers []string

	// SMTP Configuration
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	// SMS Configuration (Placeholder for future)
	SMSProvider string
	SMSAPIKey   string
}

func Load() (*Config, error) {
	// Загружаем .env файл, если он существует
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:      getEnv("DATABASE_URL", "user=postgres password=postgres dbname=hydra sslmode=disable"),
		ServerPort:       getEnv("SERVER_PORT", "8081"),
		VoiceStoragePath: getEnv("VOICE_STORAGE_PATH", "./voice_storage"),
		WebStaticPath:    getEnv("WEB_STATIC_PATH", "./web"),
		SMTPHost:         getEnv("SMTP_HOST", "smtp.example.com"),
		SMTPPort:         getEnv("SMTP_PORT", "587"),
		SMTPUser:         getEnv("SMTP_USER", ""),
		SMTPPassword:     getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:         getEnv("SMTP_FROM", "noreply@example.com"),
		SMSProvider:      getEnv("SMS_PROVIDER", "console"), // "console" means log to stdout
		SMSAPIKey:        getEnv("SMS_API_KEY", ""),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
