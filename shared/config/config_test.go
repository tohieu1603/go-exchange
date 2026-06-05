package config

import (
	"strings"
	"testing"
)

func TestEnv_FallbackAndOverride(t *testing.T) {
	t.Setenv("CFG_TEST_KEY", "")
	if got := Env("CFG_TEST_KEY", "fallback"); got != "fallback" {
		t.Fatalf("empty env should yield fallback, got %q", got)
	}
	t.Setenv("CFG_TEST_KEY", "real")
	if got := Env("CFG_TEST_KEY", "fallback"); got != "real" {
		t.Fatalf("set env should yield value, got %q", got)
	}
}

func TestBuildDSN_SSLMode(t *testing.T) {
	t.Setenv("DB_SSL_MODE", "")
	if dsn := buildDSN(); !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("default DSN should be sslmode=disable, got %q", dsn)
	}
	t.Setenv("DB_SSL_MODE", "require")
	if dsn := buildDSN(); !strings.Contains(dsn, "sslmode=require") {
		t.Fatalf("DB_SSL_MODE should override sslmode, got %q", dsn)
	}
}

func TestKafkaBrokerList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a:9092", []string{"a:9092"}},
		{"a:9092,b:9092,c:9092", []string{"a:9092", "b:9092", "c:9092"}},
	}
	for _, tt := range tests {
		got := BaseConfig{KafkaBrokers: tt.in}.KafkaBrokerList()
		if len(got) != len(tt.want) {
			t.Fatalf("KafkaBrokerList(%q) = %v, want %v", tt.in, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("KafkaBrokerList(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

func TestUseKafkaAndUseElastic(t *testing.T) {
	if (BaseConfig{}).UseKafka() {
		t.Fatal("empty brokers must report UseKafka() == false")
	}
	if !(BaseConfig{KafkaBrokers: "broker:9092"}).UseKafka() {
		t.Fatal("non-empty brokers must report UseKafka() == true")
	}
	if (BaseConfig{}).UseElastic() {
		t.Fatal("empty ElasticURL must report UseElastic() == false")
	}
	if !(BaseConfig{ElasticURL: "http://es:9200"}).UseElastic() {
		t.Fatal("non-empty ElasticURL must report UseElastic() == true")
	}
}
