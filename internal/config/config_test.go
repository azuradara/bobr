package config

import (
	"strings"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		origins []OriginConfig
		wantErr bool
	}{
		{
			name: "valid root only",
			origins: []OriginConfig{
				{Prefix: "/"},
			},
			wantErr: false,
		},
		{
			name: "valid empty prefix (root)",
			origins: []OriginConfig{
				{Prefix: ""},
			},
			wantErr: false,
		},
		{
			name: "valid specific prefixes",
			origins: []OriginConfig{
				{Prefix: "/images"},
				{Prefix: "/docs"},
			},
			wantErr: false,
		},
		{
			name: "invalid mixed root and specific",
			origins: []OriginConfig{
				{Prefix: "/"},
				{Prefix: "/images"},
			},
			wantErr: true,
		},
		{
			name: "invalid mixed empty and specific",
			origins: []OriginConfig{
				{Prefix: ""},
				{Prefix: "/images"},
			},
			wantErr: true,
		},
		{
			name: "invalid deep prefix",
			origins: []OriginConfig{
				{Prefix: "/images/foo"},
			},
			wantErr: true,
		},
		{
			name: "invalid no leading slash",
			origins: []OriginConfig{
				{Prefix: "images"},
			},
			wantErr: true,
		},
		{
			name: "invalid root inside specific",
			origins: []OriginConfig{
				{Prefix: "//"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Hosts: map[string]HostConfig{
					"test.com": {
						Origins: tt.origins,
					},
				},
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErr {
				if strings.Contains(tt.name, "mixed") &&
					!strings.Contains(err.Error(), "cannot mix") {
					t.Errorf("expected mixing error, got %v", err)
				}
			}
		})
	}
}
