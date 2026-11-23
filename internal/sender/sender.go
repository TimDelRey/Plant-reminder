// Package sender — отдельная доменная область отправки уведомлений.
// Ничего не знает про растения и события ухода: видит только generic-задачи из notification_jobs.
package sender

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Channel string

const (
	ChannelMAX      Channel = "max"
	ChannelVK       Channel = "vk"
	ChannelTelegram Channel = "telegram"
)

var AllChannels = []Channel{ChannelMAX, ChannelVK, ChannelTelegram}

func (c Channel) Valid() bool {
	for _, k := range AllChannels {
		if c == k {
			return true
		}
	}
	return false
}

type Message struct {
	Title string
	Body  string
}

// Text — сообщение одной строкой для платформ без заголовков.
func (m Message) Text() string {
	if m.Body == "" {
		return m.Title
	}
	return m.Title + "\n\n" + m.Body
}

// Sender — адаптер одной платформы.
type Sender interface {
	Send(ctx context.Context, externalID string, msg Message) error
}

var ErrChannelNotConfigured = errors.New("канал не настроен на сервере")

// Dispatcher — единая точка входа для отправки в любой канал.
type Dispatcher struct {
	mu      sync.RWMutex
	senders map[Channel]Sender
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{senders: map[Channel]Sender{}}
}

func (d *Dispatcher) Register(ch Channel, s Sender) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.senders[ch] = s
}

func (d *Dispatcher) Has(ch Channel) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.senders[ch]
	return ok
}

func (d *Dispatcher) Send(ctx context.Context, ch Channel, externalID string, msg Message) error {
	d.mu.RLock()
	s, ok := d.senders[ch]
	d.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrChannelNotConfigured, ch)
	}
	return s.Send(ctx, externalID, msg)
}
