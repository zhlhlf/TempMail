package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	SMTPAddr          string
	SMTPDomain        string
	DBDSN             string
	RateLimit         int
	RateWindow        int
	SMTPServerIP      string
	AdminKeyFile      string
	SMTPMaxMessage    int64
	SMTPReadTimeout   time.Duration
	SMTPWriteTimeout  time.Duration
	SMTPMaxRecipients int
}

func Load() *Config {
	rl, _ := strconv.Atoi(getEnv("RATE_LIMIT", "500"))
	rw, _ := strconv.Atoi(getEnv("RATE_WINDOW", "60"))
	maxRecipients, _ := strconv.Atoi(getEnv("SMTP_MAX_RECIPIENTS", "100"))
	maxMessageBytes, _ := strconv.ParseInt(getEnv("SMTP_MAX_MESSAGE_BYTES", "10240000"), 10, 64)
	readTimeoutSeconds, _ := strconv.Atoi(getEnv("SMTP_READ_TIMEOUT", "30"))
	writeTimeoutSeconds, _ := strconv.Atoi(getEnv("SMTP_WRITE_TIMEOUT", "30"))

	httpAddr := getEnv("HTTP_ADDR", "")
	if httpAddr == "" {
		httpAddr = ":" + getEnv("PORT", "8080")
	}

	smtpAddr := getEnv("SMTP_ADDR", "")
	if smtpAddr == "" {
		smtpAddr = ":" + getEnv("SMTP_PORT", "25")
	}

	smtpDomain := getEnv("SMTP_DOMAIN", "localhost")
	if host := os.Getenv("SMTP_HOSTNAME"); host != "" {
		smtpDomain = host
	}

	adminKeyFile := getEnv("ADMIN_KEY_FILE", "./data/admin.key")

	return &Config{
		HTTPAddr:          httpAddr,
		SMTPAddr:          smtpAddr,
		SMTPDomain:        smtpDomain,
		DBDSN:             getEnv("DB_DSN", getEnv("API_DB_PATH", "./data/tempmail.db")),
		RateLimit:         rl,
		RateWindow:        rw,
		SMTPServerIP:      os.Getenv("SMTP_SERVER_IP"),
		AdminKeyFile:      adminKeyFile,
		SMTPMaxMessage:    maxMessageBytes,
		SMTPReadTimeout:   time.Duration(readTimeoutSeconds) * time.Second,
		SMTPWriteTimeout:  time.Duration(writeTimeoutSeconds) * time.Second,
		SMTPMaxRecipients: maxRecipients,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
