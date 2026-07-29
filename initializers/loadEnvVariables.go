package initializers

import (
	"github.com/MartinHell/overlord/logs"
	"github.com/joho/godotenv"
)

// LoadEnvVariables loads a .env file when one is present. It is optional:
// containerised deployments configure overlord through the environment
// directly, and missing values either fall back to a default or fail loudly at
// the point they are actually needed.
func LoadEnvVariables() {
	if err := godotenv.Load(); err != nil {
		logs.Sugar.Debugln("No .env file found, reading configuration from the environment")
	}
}
