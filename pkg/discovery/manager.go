package discovery

import (
	"context"
	"fmt"
	"hydra/pkg/transport/mesh"
	"log"
	"sync"
	"time"
)

// AutoPeerManager автоматически управляет пирами в Mesh сети
type AutoPeerManager struct {
	discovery    *ServiceDiscovery
	mesh         *mesh.MeshTransport
	updateTicker *time.Ticker
	stopChan     chan struct{}
	mu           sync.Mutex
}

func NewAutoPeerManager(meshPort int) (*AutoPeerManager, error) {
	// Создаем discovery сервис
	discovery := New("_hydra-messenger._tcp", meshPort)

	// Создаем Mesh транспорт с пустым списком пиров (будет обновляться автоматически)
	meshTransport := mesh.New([]string{})

	manager := &AutoPeerManager{
		discovery:    discovery,
		mesh:         meshTransport,
		updateTicker: time.NewTicker(15 * time.Second), // Обновляем пиры каждые 15 секунд
		stopChan:     make(chan struct{}),
	}

	// Запускаем discovery
	if err := discovery.Start(); err != nil {
		return nil, fmt.Errorf("failed to start discovery: %v", err)
	}

	// Запускаем автоматическое обновление пиров
	go manager.autoUpdatePeers()

	return manager, nil
}

// Start запускает автоматическое управление пирами
func (m *AutoPeerManager) Start() error {
	// Подключаем Mesh транспорт
	if err := m.mesh.Connect(context.Background()); err != nil {
		return fmt.Errorf("failed to connect mesh transport: %v", err)
	}

	log.Println("Auto peer manager started")
	return nil
}

// Stop останавливает автоматическое управление
func (m *AutoPeerManager) Stop() {
	m.updateTicker.Stop()
	close(m.stopChan)
	m.discovery.Stop()
}

// GetMeshTransport возвращает Mesh транспорт с автоматически обновляемыми пирами
func (m *AutoPeerManager) GetMeshTransport() *mesh.MeshTransport {
	return m.mesh
}

// autoUpdatePeers автоматически обновляет список пиров в Mesh транспорте
func (m *AutoPeerManager) autoUpdatePeers() {
	for {
		select {
		case <-m.stopChan:
			return
		case <-m.updateTicker.C:
			m.updatePeerList()
		}
	}
}

// updatePeerList обновляет список пиров на основе обнаруженных сервисов
func (m *AutoPeerManager) updatePeerList() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Получаем обнаруженные пиры
	discoveredPeers := m.discovery.GetPeers()

	if len(discoveredPeers) > 0 {
		log.Printf("Discovered %d peers: %v", len(discoveredPeers), discoveredPeers)

		// Обновляем пиры в Mesh транспорте
		m.mesh.UpdatePeers(discoveredPeers)
	}
}

// AddStaticPeer добавляет статический peer (ручное подключение)
func (m *AutoPeerManager) AddStaticPeer(peerAddr string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Здесь будет логика добавления статического пира
	log.Printf("Added static peer: %s", peerAddr)
}

// RemovePeer удаляет peer из списка
func (m *AutoPeerManager) RemovePeer(peerAddr string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Здесь будет логика удаления пира
	log.Printf("Removed peer: %s", peerAddr)
}

// GetPeerList возвращает текущий список пиров
func (m *AutoPeerManager) GetPeerList() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Здесь будет возвращаться актуальный список пиров
	return []string{} // Заглушка
}
