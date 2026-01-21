package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"hydra/pkg/transport"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// VoiceMessage представляет голосовое сообщение
type VoiceMessage struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
	Duration  float64   `json:"duration"`  // Длительность в секундах
	Format    string    `json:"format"`    // audio/webm, audio/mp3, etc.
	Data      []byte    `json:"-"`         // Бинарные данные аудио
	FilePath  string    `json:"file_path"` // Путь к файлу (если сохранено)
}

// VoiceProcessor обрабатывает голосовые сообщения
type VoiceProcessor struct {
	transport     transport.Transport
	storageDir    string
	maxFileSizeMB int
	mu            sync.Mutex
}

func New(transport transport.Transport, storageDir string) *VoiceProcessor {
	// Создаем директорию для хранения аудио файлов
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Printf("Warning: failed to create voice storage directory: %v", err)
	}

	return &VoiceProcessor{
		transport:     transport,
		storageDir:    storageDir,
		maxFileSizeMB: 10, // Максимальный размер файла 10MB
	}
}

// Record записывает голосовое сообщение из multipart формы
func (vp *VoiceProcessor) Record(ctx context.Context, fileHeader *multipart.FileHeader) (*VoiceMessage, error) {
	vp.mu.Lock()
	defer vp.mu.Unlock()

	// Проверяем размер файла
	if fileHeader.Size > int64(vp.maxFileSizeMB*1024*1024) {
		return nil, fmt.Errorf("file too large: %dMB max", vp.maxFileSizeMB)
	}

	// Открываем файл
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %v", err)
	}
	defer file.Close()

	// Читаем данные
	audioData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %v", err)
	}

	// Создаем уникальное имя файла
	filename := fmt.Sprintf("voice_%d_%s", time.Now().UnixNano(), fileHeader.Filename)
	filePath := filepath.Join(vp.storageDir, filename)

	// Сохраняем файл
	if err := os.WriteFile(filePath, audioData, 0644); err != nil {
		return nil, fmt.Errorf("failed to save audio file: %v", err)
	}

	// Создаем объект голосового сообщения
	voiceMsg := &VoiceMessage{
		ID:        generateID(),
		Timestamp: time.Now(),
		Duration:  estimateDuration(len(audioData)), // Примерная оценка длительности
		Format:    fileHeader.Header.Get("Content-Type"),
		Data:      audioData,
		FilePath:  filePath,
	}

	return voiceMsg, nil
}

// Send отправляет голосовое сообщение через транспорт
func (vp *VoiceProcessor) Send(ctx context.Context, voiceMsg *VoiceMessage) error {
	// Сериализуем метаданные и данные
	message := map[string]interface{}{
		"type":      "voice",
		"id":        voiceMsg.ID,
		"user_id":   voiceMsg.UserID,
		"timestamp": voiceMsg.Timestamp,
		"duration":  voiceMsg.Duration,
		"format":    voiceMsg.Format,
		"data":      voiceMsg.Data, // Бинарные данные
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal voice message: %v", err)
	}

	// Отправляем через транспорт
	return vp.transport.Send(ctx, jsonData)
}

// Receive обрабатывает входящее голосовое сообщение
func (vp *VoiceProcessor) Receive(ctx context.Context, data []byte) (*VoiceMessage, error) {
	var message struct {
		Type      string    `json:"type"`
		ID        string    `json:"id"`
		UserID    string    `json:ser_id"`
		Timestamp time.Time `json:"timestamp"`
		Duration  float64   `json:"duration"`
		Format    string    `json:"format"`
		Data      []byte    `json:"data"`
	}

	if err := json.Unmarshal(data, &message); err != nil {
		return nil, fmt.Errorf("failed to unmarshal voice message: %v", err)
	}

	if message.Type != "voice" {
		return nil, fmt.Errorf("not a voice message")
	}

	// Сохраняем полученное аудио
	filename := fmt.Sprintf("received_voice_%s_%s", message.ID, message.Format)
	filePath := filepath.Join(vp.storageDir, filename)

	if err := os.WriteFile(filePath, message.Data, 0644); err != nil {
		return nil, fmt.Errorf("failed to save received audio: %v", err)
	}

	voiceMsg := &VoiceMessage{
		ID:        message.ID,
		UserID:    message.UserID,
		Timestamp: message.Timestamp,
		Duration:  message.Duration,
		Format:    message.Format,
		Data:      message.Data,
		FilePath:  filePath,
	}

	return voiceMsg, nil
}

// Play воспроизводит голосовое сообщение
func (vp *VoiceProcessor) Play(voiceMsg *VoiceMessage) error {
	// В реальном приложении здесь была бы логика воспроизведения аудио
	// Для демонстрации просто логируем
	log.Printf("Playing voice message %s (%.1fs) from %s",
		voiceMsg.ID, voiceMsg.Duration, voiceMsg.UserID)

	return nil
}

// GetAudioFile возвращает путь к аудио файлу
func (vp *VoiceProcessor) GetAudioFile(voiceMsg *VoiceMessage) (string, error) {
	if _, err := os.Stat(voiceMsg.FilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("audio file not found: %s", voiceMsg.FilePath)
	}
	return voiceMsg.FilePath, nil
}

// GetVoiceMessagePathByID ищет путь к файлу по ID
func (vp *VoiceProcessor) GetVoiceMessagePathByID(voiceID string) (string, error) {
	vp.mu.Lock()
	defer vp.mu.Unlock()

	// Ищем файл, который содержит voiceID в названии
	files, err := os.ReadDir(vp.storageDir)
	if err != nil {
		return "", fmt.Errorf("could not read storage directory: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.Contains(file.Name(), voiceID) {
			return filepath.Join(vp.storageDir, file.Name()), nil
		}
	}

	return "", fmt.Errorf("voice message with ID %s not found", voiceID)
}

// Cleanup удаляет старые аудио файлы
func (vp *VoiceProcessor) Cleanup(maxAge time.Duration) {
	files, err := os.ReadDir(vp.storageDir)
	if err != nil {
		log.Printf("Failed to read voice storage directory: %v", err)
		return
	}

	now := time.Now()
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > maxAge {
			filePath := filepath.Join(vp.storageDir, file.Name())
			if err := os.Remove(filePath); err != nil {
				log.Printf("Failed to delete old audio file %s: %v", file.Name(), err)
			} else {
				log.Printf("Deleted old audio file: %s", file.Name())
			}
		}
	}
}

// generateID генерирует уникальный ID для сообщения
func generateID() string {
	return fmt.Sprintf("vm_%d", time.Now().UnixNano())
}

// estimateDuration оценивает длительность аудио на основе размера
func estimateDuration(dataSize int) float64 {
	// Примерная оценка: 1KB ≈ 0.06 секунды для opus/ogg
	return float64(dataSize) / 1024 * 0.06
}
