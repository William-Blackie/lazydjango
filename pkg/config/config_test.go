package config

import (
	"testing"
)

// TestGetDefaultConfig tests default configuration
func TestGetDefaultConfig(t *testing.T) {
	config := GetDefaultConfig()
	
	if config == nil {
		t.Fatal("GetDefaultConfig returned nil")
	}
	
	// Test default PageSize
	if config.Gui.PageSize != 20 {
		t.Errorf("Expected default PageSize to be 20, got %d", config.Gui.PageSize)
	}
}

// TestConfigStructure tests config structure
func TestConfigStructure(t *testing.T) {
	config := &AppConfig{
		Gui: GuiConfig{
			PageSize: 50,
		},
	}
	
	if config.Gui.PageSize != 50 {
		t.Errorf("Expected PageSize to be 50, got %d", config.Gui.PageSize)
	}
}

// TestGuiConfig tests GUI configuration
func TestGuiConfig(t *testing.T) {
	tests := []struct {
		name     string
		pageSize int
	}{
		{"Default", 20},
		{"Small", 10},
		{"Medium", 50},
		{"Large", 100},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AppConfig{
				Gui: GuiConfig{
					PageSize: tt.pageSize,
				},
			}
			
			if config.Gui.PageSize != tt.pageSize {
				t.Errorf("Expected PageSize to be %d, got %d", tt.pageSize, config.Gui.PageSize)
			}
		})
	}
}

// TestConfigModification tests modifying config values
func TestConfigModification(t *testing.T) {
	config := GetDefaultConfig()
	
	// Verify initial value
	if config.Gui.PageSize != 20 {
		t.Errorf("Expected initial PageSize to be 20, got %d", config.Gui.PageSize)
	}
	
	// Modify value
	config.Gui.PageSize = 100
	
	// Verify modification
	if config.Gui.PageSize != 100 {
		t.Errorf("Expected modified PageSize to be 100, got %d", config.Gui.PageSize)
	}
}

// TestConfigZeroValues tests handling of zero values
func TestConfigZeroValues(t *testing.T) {
	config := &AppConfig{}
	
	// Zero value should be 0
	if config.Gui.PageSize != 0 {
		t.Errorf("Expected zero value PageSize to be 0, got %d", config.Gui.PageSize)
	}
}

// TestMultipleConfigs tests multiple config instances
func TestMultipleConfigs(t *testing.T) {
	config1 := GetDefaultConfig()
	config2 := GetDefaultConfig()
	
	// Modify one
	config1.Gui.PageSize = 50
	
	// Other should be unchanged
	if config2.Gui.PageSize != 20 {
		t.Error("Config instances should be independent")
	}
}

