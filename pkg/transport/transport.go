package transport

import "context"

// Transport определяет общий интерфейс для всех способов связи (Fronting, Mesh, Direct).
type Transport interface {
	// Name возвращает название транспорта (например, "domain-fronting", "mesh")
	Name() string

	// Connect устанавливает соединение (если применимо)
	Connect(ctx context.Context) error

	// Send отправляет данные.
	// address может быть ID получателя или специфичный для транспорта адрес.
	Send(ctx context.Context, data []byte) error

	// IsAvailable проверяет, доступен ли данный транспорт в текущий момент.
	IsAvailable() bool
}
