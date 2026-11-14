package config

import (
	"os"

	"github.com/caarlos0/env/v11"
)

func ProcessEnvironment() map[string]string {
	return env.ToMap(os.Environ())
}
