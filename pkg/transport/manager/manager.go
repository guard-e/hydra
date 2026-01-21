package manager

import (
	"context"
	"fmt"
	"hydra/pkg/transport"
	"hydra/pkg/transport/fronting"
	"hydra/pkg/transport/mesh"
	"log"
	"strings"
	"sync"
)

// TransportManager управляет переключением между разными транспортами
type TransportManager struct {
	transports   []transport.Transport
	currentIndex int
	mu           sync.Mutex
}

func New() *TransportManager {
	// Создаем транспорты в порядке приоритета с разными CDN доменами для retry:

	// Domain Fronting транспорты с разными CDN для retry
	frontingTransports := []*fronting.Transport{
		fronting.New(
			"ajax.googleapis.com",     // Google CDN
			"secret-chat.appspot.com", // Скрытый сервис
		),
		fronting.New(
			"cdn.cloudflare.com",      // Cloudflare CDN
			"secret-chat.appspot.com", // Скрытый сервис
		),
		fronting.New(
			"d3a2p9q8.stackpathcdn.com", // StackPath CDN
			"secret-chat.appspot.com",   // Скрытый сервис
		),
		fronting.New(
			"assets.buymeacoffee.com", // BuyMeACoffee CDN
			"secret-chat.appspot.com", // Скрытый сервис
		),
	}

	// Mesh транспорт как последний резерв
	meshTransport := mesh.New([]string{
		"192.168.1.100:8080", // Пример пиров в сети
		"192.168.1.101:8080",
		"192.168.1.102:8080",
	})

	// Конвертируем в интерфейс Transport
	transports := make([]transport.Transport, len(frontingTransports)+1)
	for i, ft := range frontingTransports {
		transports[i] = ft
	}
	transports[len(frontingTransports)] = meshTransport

	return &TransportManager{
		transports: transports,
	}
}

// Name возвращает имя менеджера
func (m *TransportManager) Name() string {
	return "transport-manager"
}

// Connect пытается подключиться к доступным транспортам
func (m *TransportManager) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Пробуем подключиться к текущему или всем
	for _, t := range m.transports {
		if err := t.Connect(ctx); err != nil {
			log.Printf("Предупреждение: не удалось подключиться к %s: %v", t.Name(), err)
		}
	}
	return nil
}

// IsAvailable проверяет доступность хотя бы одного транспорта
func (m *TransportManager) IsAvailable() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, t := range m.transports {
		if t.IsAvailable() {
			return true
		}
	}
	return false
}

// Send пытается отправить сообщение через доступные транспорты
// Автоматически переключается при ошибках
func (m *TransportManager) Send(ctx context.Context, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Пробуем все транспорты по порядку приоритета
	for i, t := range m.transports {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if !t.IsAvailable() {
				log.Printf("Транспорт %s недоступен, пропускаем", t.Name())
				continue
			}

			log.Printf("Попытка отправки через %s...", t.Name())

			err := t.Send(ctx, data)
			if err == nil {
				// Успех! Запоминаем этот транспорт для следующих отправок
				m.currentIndex = i
				log.Printf("✓ Сообщение отправлено через %s", t.Name())
				return nil
			}

			log.Printf("✗ Ошибка в транспорте %s: %v", t.Name(), err)

			// Если это Domain Fronting и ошибка 502 (блокировка CDN),
			// сразу переключаемся на следующий транспорт
			if t.Name() == "fronting" && isBlockingError(err) {
				log.Printf("Обнаружена блокировка CDN, переключаемся на Mesh...")
				continue
			}
		}
	}

	return fmt.Errorf("все транспорты недоступны")
}

// isBlockingError проверяет, является ли ошибка блокировкой CDN
func isBlockingError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Проверяем типичные признаки блокировки CDN
	return strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "Bad Gateway") ||
		strings.Contains(errStr, "blocked") ||
		strings.Contains(errStr, "certificate")
}

// GetCurrentTransport возвращает текущий активный транспорт
func (m *TransportManager) GetCurrentTransport() transport.Transport {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.transports) == 0 {
		return nil
	}

	return m.transports[m.currentIndex]
}

// SwitchTo принудительно переключает на указанный транспорт
func (m *TransportManager) SwitchTo(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, t := range m.transports {
		if t.Name() == name {
			m.currentIndex = i
			log.Printf("Принудительно переключились на %s", name)
			return nil
		}
	}

	return fmt.Errorf("транспорт %s не найден", name)
}

// GetStatus возвращает статус всех транспортов
func (m *TransportManager) GetStatus() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := make(map[string]string)
	for _, t := range m.transports {
		status[t.Name()] = "available"
		if !t.IsAvailable() {
			status[t.Name()] = "unavailable"
		}
	}

	return status
}
