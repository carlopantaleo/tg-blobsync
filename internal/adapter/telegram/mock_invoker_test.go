package telegram

import (
	"context"
	"errors"
	"reflect"

	"github.com/gotd/td/bin"
)

type MockInvoker struct {
	Handlers map[reflect.Type]func(ctx context.Context, input bin.Encoder, output bin.Decoder) error
}

func NewMockInvoker() *MockInvoker {
	return &MockInvoker{
		Handlers: make(map[reflect.Type]func(ctx context.Context, input bin.Encoder, output bin.Decoder) error),
	}
}

func (m *MockInvoker) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	t := reflect.TypeOf(input)
	// input is usually a pointer to the request struct
	if handler, ok := m.Handlers[t]; ok {
		return handler(ctx, input, output)
	}
	return errors.New("unexpected call: " + t.String())
}

// Register adds a handler for a specific request type.
// requestType should be an instance of the request struct pointer, e.g. &tg.MessagesGetHistoryRequest{}
func (m *MockInvoker) Register(requestType interface{}, handler func(ctx context.Context, input bin.Encoder, output bin.Decoder) error) {
	t := reflect.TypeOf(requestType)
	m.Handlers[t] = handler
}
