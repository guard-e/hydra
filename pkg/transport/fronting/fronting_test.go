package fronting

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDomainFrontingLogic проверяет, что клиент действительно отправляет разные Host header и SNI/URL.
// Поскольку мы не можем легко проверить SNI в httptest (он слушает localhost), 
// мы проверим, что Host заголовок отличается от адреса подключения, и что он корректно доходит до сервера.
func TestDomainFrontingLogic(t *testing.T) {
	// 1. Создаем тестовый сервер, который притворяется CDN/Front-ом
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем, что Host заголовок - это наш СКРЫТЫЙ домен
		expectedHost := "hidden-service.com"
		if r.Host != expectedHost {
			t.Errorf("Expected Host header %s, got %s", expectedHost, r.Host)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		// Читаем тело
		body, _ := io.ReadAll(r.Body)
		if string(body) != "test-payload" {
			t.Errorf("Expected body 'test-payload', got %s", string(body))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Извлекаем адрес тестового сервера (IP:Port), который играет роль "Front Domain"
	// В реальной жизни здесь был бы cdn.example.com
	
	// Нам нужно "обмануть" транспорт, чтобы он думал, что server.URL это frontDomain.
	// Но server.URL содержит "https://127.0.0.1:xxxxx".
	// Мы передадим адрес сервера как FrontDomain, но нам нужно отключить проверку сертификата для теста,
	// так как httptest генерирует самоподписанный сертификат для "example.com" или localhost.

	hiddenDomain := "hidden-service.com"
	
	// Инициализируем транспорт
	// Важно: в тесте мы не можем проверить SNI легко без wireshark/tcpdump логики,
	// но мы можем проверить Host header.
	tr := New("127.0.0.1", hiddenDomain)
	
	// Хак для теста: подменяем EndpointUrl на реальный адрес тестового сервера, 
	// иначе он попытается постучаться на реальный 127.0.0.1:443
	tr.EndpointUrl = server.URL // https://127.0.0.1:xxxxx

	// Хак для теста: разрешаем самоподписанные сертификаты
	tr.client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true

	// Выполняем отправку
	err := tr.Send(context.Background(), []byte("test-payload"))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}
