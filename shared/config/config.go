package config

import (
	"database/sql"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type BaseConfig struct {
	Port         string
	DSN          string
	RedisURL     string
	JWTSecret    string
	KafkaBrokers string
	ElasticURL   string
}

func LoadBase() BaseConfig {
	_ = godotenv.Load() // load .env if exists, ignore error
	return BaseConfig{
		Port:         getEnv("PORT", "8080"),
		DSN:          buildDSN(),
		RedisURL:     getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", ""),
		ElasticURL:   getEnv("ELASTIC_URL", ""),
	}
}

func buildDSN() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	pass := getEnv("DB_PASSWORD", "postgres")
	name := getEnv("DB_NAME", "exchange")
	return "host=" + host + " port=" + port + " user=" + user + " password=" + pass + " dbname=" + name + " sslmode=disable"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (c BaseConfig) UseKafka() bool { return c.KafkaBrokers != "" }

// ConfigureDBPool sets connection pool limits on a *sql.DB instance
func ConfigureDBPool(sqlDB *sql.DB) {
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
}

func (c BaseConfig) UseElastic() bool { return c.ElasticURL != "" }

func (c BaseConfig) KafkaBrokerList() []string {
	if c.KafkaBrokers == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(c.KafkaBrokers); i++ {
		if c.KafkaBrokers[i] == ',' {
			result = append(result, c.KafkaBrokers[start:i])
			start = i + 1
		}
	}
	result = append(result, c.KafkaBrokers[start:])
	return result
}
