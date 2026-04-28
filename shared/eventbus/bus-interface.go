package eventbus

import "context"

// EventPublisher is the write side: publish events to a topic.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload interface{}) error
	PublishWS(ctx context.Context, channel string, data interface{})
}

// EventConsumer is the read side: subscribe handlers and start consuming.
type EventConsumer interface {
	Subscribe(topic string, handler Handler)
	StartConsumer(ctx context.Context, topic, group, consumer string)
}

// EventBus combines publisher and consumer into a unified bus.
type EventBus interface {
	EventPublisher
	EventConsumer
}
