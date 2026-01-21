package mesh

import (
	"context"
	"fmt"
	"hydra/pkg/transport"
	"log"
	"net"
	"sync"
	"time"
)

// MeshTransport реализует P2P mesh сеть через TCP
// В реальном приложении здесь был бы Bluetooth/Wi-Fi Direct
// Для демонстрации используем простой TCP

type MeshTransport struct {
	peers     []string // Список пиров в сети
	listener  net.Listener
	currentIP string
	mu        sync.Mutex
}

func New(peers []string) *MeshTransport {
	return &MeshTransport{
		peers: peers,
	}
}

func (m *MeshTransport) Name() string {
	return "mesh"
}

func (m *MeshTransport) Connect(ctx context.Context) error {
	// Получаем наш локальный IP для демонстрации
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return fmt.Errorf("failed to get interface addresses: %v", err)
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				m.currentIP = ipnet.IP.String()
				break
			}
		}
	}

	// Запускаем TCP сервер для приема сообщений
	m.listener, err = net.Listen("tcp", ":0") // Случайный порт
	if err != nil {
		return fmt.Errorf("failed to start mesh listener: %v", err)
	}

	log.Printf("Mesh транспорт запущен на %s", m.listener.Addr().String())
	return nil
}

func (m *MeshTransport) Send(ctx context.Context, data []byte) error {
	if len(m.peers) == 0 {
		return fmt.Errorf("no peers available in mesh network")
	}

	// Пытаемся отправить всем доступным пирам
	var lastError error
	for _, peer := range m.peers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			conn, err := net.DialTimeout("tcp", peer, 3*time.Second)
			if err != nil {
				lastError = err
				continue
			}

			_, err = conn.Write(data)
			conn.Close()

			if err == nil {
				log.Printf("Сообщение успешно отправлено через Mesh к %s", peer)
				return nil
			}
			lastError = err
		}
	}

	return fmt.Errorf("failed to send to any peer: %v", lastError)
}

func (m *MeshTransport) IsAvailable() bool {
	// Mesh всегда доступен (локальная сеть)
	return true
}

// UpdatePeers динамически обновляет список пиров
func (m *MeshTransport) UpdatePeers(newPeers []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.peers = newPeers
	log.Printf("Mesh peers updated: %v", newPeers)
}

// GetPeers возвращает текущий список пиров
func (m *MeshTransport) GetPeers() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.peers
}

// Ensure interface compliance
var _ transport.Transport = (*MeshTransport)(nil)
