package server

import (
	"reflect"
	"testing"

	"github.com/azuradara/bobr/internal/config"
)

func TestHandler_selectOrigins(t *testing.T) {
	tests := []struct {
		name        string
		hostCfg     config.HostConfig
		requestPath string
		want        []string
	}{
		{
			name: "root origins match everything",
			hostCfg: config.HostConfig{
				Origins: []config.OriginConfig{
					{Name: "root1", Prefix: "/"},
					{Name: "root2", Prefix: ""},
				},
			},
			requestPath: "/foo/bar",
			want:        []string{"root1", "root2"},
		},
		{
			name: "specific prefix matches exactly",
			hostCfg: config.HostConfig{
				Origins: []config.OriginConfig{
					{Name: "images", Prefix: "/images"},
				},
			},
			requestPath: "/images",
			want:        []string{"images"},
		},
		{
			name: "specific prefix matches subpath",
			hostCfg: config.HostConfig{
				Origins: []config.OriginConfig{
					{Name: "images", Prefix: "/images"},
				},
			},
			requestPath: "/images/logo.png",
			want:        []string{"images"},
		},
		{
			name: "specific prefix does not match partial segment",
			hostCfg: config.HostConfig{
				Origins: []config.OriginConfig{
					{Name: "images", Prefix: "/images"},
				},
			},
			requestPath: "/images_backup/logo.png",
			want:        nil,
		},
		{
			name: "multiple origins with same prefix (fallback)",
			hostCfg: config.HostConfig{
				Origins: []config.OriginConfig{
					{Name: "images1", Prefix: "/images"},
					{Name: "images2", Prefix: "/images"},
				},
			},
			requestPath: "/images/foo.png",
			want:        []string{"images1", "images2"},
		},
		{
			name: "no match returns nil",
			hostCfg: config.HostConfig{
				Origins: []config.OriginConfig{
					{Name: "images", Prefix: "/images"},
				},
			},
			requestPath: "/docs/foo.txt",
			want:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(nil, map[string]config.HostConfig{
				"test.com": tt.hostCfg,
			})

			router := h.hosts["test.com"]
			got := h.selectOrigins(router, tt.requestPath)

			var gotNames []string
			for _, o := range got {
				gotNames = append(gotNames, o.Name)
			}

			if len(got) == 0 && len(tt.want) == 0 {
				return
			}

			if !reflect.DeepEqual(gotNames, tt.want) {
				t.Errorf("selectOrigins() = %v, want %v", gotNames, tt.want)
			}
		})
	}
}
