package logs

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Sugar *zap.SugaredLogger

func init() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		panic("Failed to load .env file: " + err.Error())
	}

	logger, err := InitLogger()
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync() // flushes buffer, if any

	Sugar = logger.Sugar()
}

func InitLogger() (*zap.Logger, error) {
	// Set log level based on LOG_LEVEL environment variable
	logLevel, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		//fmt.Println("LOG_LEVEL not set, defaulting to INFO")
		logLevel = "info" // default log level
	}
	fmt.Println("Log level: " + logLevel)

	logEncoding, ok := os.LookupEnv("LOG_ENCODING")
	if !ok {
		//fmt.Println("LOG_ENCODING not set, defaulting to JSON")
		logEncoding = "json" // default log encoding
	}
	fmt.Println("Log encoding: " + logEncoding)

	var level zapcore.Level
	err := level.UnmarshalText([]byte(logLevel))
	if err != nil {
		return nil, err
	}

	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(level),
		Development: true,
		Encoding:    logEncoding,
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:   "message",
			LevelKey:     "level",
			TimeKey:      "time",
			EncodeTime:   zapcore.ISO8601TimeEncoder,
			CallerKey:    "caller",
			EncodeCaller: zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return logger, nil
}
