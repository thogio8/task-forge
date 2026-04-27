package telemetry

import kafkago "github.com/segmentio/kafka-go"

// KafkaHeaderCarrier adapts a slice of kafka-go headers to the OTel
// propagation.TextMapCarrier interface, allowing the W3C Trace Context to be
// injected into outgoing Kafka messages and extracted from incoming ones.
type KafkaHeaderCarrier []kafkago.Header

func (c *KafkaHeaderCarrier) Get(key string) string {
	for _, h := range *c {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *KafkaHeaderCarrier) Set(key, value string) {
	for i, h := range *c {
		if h.Key == key {
			(*c)[i].Value = []byte(value)
			return
		}
	}
	*c = append(*c, kafkago.Header{Key: key, Value: []byte(value)})
}

func (c *KafkaHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(*c))
	for _, h := range *c {
		keys = append(keys, h.Key)
	}
	return keys
}
