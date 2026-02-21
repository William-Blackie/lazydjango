package config

// AppConfig contains application configuration
type AppConfig struct {
	Gui GuiConfig
}

// GuiConfig contains GUI-specific configuration
type GuiConfig struct {
	PageSize int // Number of records per page in data viewer
}

// GetDefaultConfig returns default configuration
func GetDefaultConfig() *AppConfig {
	return &AppConfig{
		Gui: GuiConfig{
			PageSize: 20,
		},
	}
}
