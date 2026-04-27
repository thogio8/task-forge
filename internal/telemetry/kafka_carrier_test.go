package telemetry

import "testing"

func TestKafkaHeaderCarrier_Get(t *testing.T) {
	c := KafkaHeaderCarrier{
		{Key: "traceparent", Value: []byte("00-abc-def-01")},
		{Key: "other", Value: []byte("x")},
	}

	if got := c.Get("traceparent"); got != "00-abc-def-01" {
		t.Errorf("Get(traceparent) = %q, want %q", got, "00-abc-def-01")
	}
	if got := c.Get("missing"); got != "" {
		t.Errorf("Get(missing) = %q, want empty string", got)
	}
}

func TestKafkaHeaderCarrier_Set_NewKey(t *testing.T) {
	c := KafkaHeaderCarrier{}
	c.Set("k1", "v1")

	if len(c) != 1 {
		t.Fatalf("len = %d, want 1", len(c))
	}
	if c[0].Key != "k1" || string(c[0].Value) != "v1" {
		t.Errorf("got %+v, want {k1, v1}", c[0])
	}
}

func TestKafkaHeaderCarrier_Set_OverwriteExisting(t *testing.T) {
	c := KafkaHeaderCarrier{
		{Key: "k1", Value: []byte("old")},
		{Key: "k2", Value: []byte("keep")},
	}
	c.Set("k1", "new")

	if len(c) != 2 {
		t.Fatalf("len = %d, want 2 (overwrite must not append)", len(c))
	}
	if string(c[0].Value) != "new" {
		t.Errorf("c[0].Value = %q, want %q", c[0].Value, "new")
	}
	if string(c[1].Value) != "keep" {
		t.Errorf("c[1].Value = %q, want %q", c[1].Value, "keep")
	}
}

func TestKafkaHeaderCarrier_Keys(t *testing.T) {
	c := KafkaHeaderCarrier{
		{Key: "a"},
		{Key: "b"},
	}

	keys := c.Keys()

	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Errorf("Keys() = %v, want [a b]", keys)
	}
}
