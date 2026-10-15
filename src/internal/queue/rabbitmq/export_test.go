package rabbitmq

import (
	"io"

	"go.opentelemetry.io/otel/metric"
)

// NewPublishFailuresCounter exposes newPublishFailuresCounter for unit testing.
var NewPublishFailuresCounter = newPublishFailuresCounter

// Channel exposes the channel interface so tests can supply fakes.
type Channel = channel

// NewTestPublisher builds a Publisher around a fake channel and dialer, so the
// publish and reconnect paths can run without a broker.
func NewTestPublisher(cfg PublisherConfig, failures metric.Int64Counter, ch Channel, dial func() (io.Closer, Channel, error)) *Publisher {
	return &Publisher{cfg: cfg, ch: ch, dial: dial, publishFailures: failures}
}
