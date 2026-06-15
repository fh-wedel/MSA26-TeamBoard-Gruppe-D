package eventbus

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	DefaultExchange    = "teamboard.events"
	DefaultDLXSuffix   = ".dlx"
	defaultMessageTTL  = int32(24 * 60 * 60 * 1000) // 24h in ms
)

// TopologyOptions configures exchange and queue declarations.
type TopologyOptions struct {
	Exchange    string
	Queue       string
	BindingKeys []string
}

// SetupTopology declares the exchange, DLX, and queue idempotently.
func SetupTopology(ch *amqp.Channel, opts TopologyOptions) error {
	exchange := opts.Exchange
	if exchange == "" {
		exchange = DefaultExchange
	}
	dlx := exchange + DefaultDLXSuffix

	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", exchange, err)
	}
	if err := ch.ExchangeDeclare(dlx, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare DLX %q: %w", dlx, err)
	}

	if opts.Queue != "" {
		args := amqp.Table{
			"x-dead-letter-exchange": dlx,
			"x-message-ttl":          defaultMessageTTL,
		}
		if _, err := ch.QueueDeclare(opts.Queue, true, false, false, false, args); err != nil {
			return fmt.Errorf("declare queue %q: %w", opts.Queue, err)
		}
		for _, key := range opts.BindingKeys {
			if err := ch.QueueBind(opts.Queue, key, exchange, false, nil); err != nil {
				return fmt.Errorf("bind queue %q with key %q: %w", opts.Queue, key, err)
			}
		}
	}
	return nil
}
