package dependencies

import "github.com/WangYihang/Platypus/pkg/config"

// InitConfig initializes the configuration.
func InitConfig(path string) func() (*config.Config, error) {
	return func() (*config.Config, error) {
		return config.LoadConfig(path)
	}
}
