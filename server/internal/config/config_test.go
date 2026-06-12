package config_test

import (
	"strings"
	"testing"

	"github.com/ramdanaguss/selaras/server/internal/config"
)

func TestLoad(t *testing.T) {
	const validDB = "postgres://selaras:selaras@localhost:5432/selaras?sslmode=disable"

	tests := []struct {
		name     string
		env      map[string]string
		want     config.Config
		wantErrs []string // substrings that must all appear in the error
	}{
		{
			name: "defaults applied when only DATABASE_URL set",
			env:  map[string]string{"DATABASE_URL": validDB},
			want: config.Config{
				Port:        8080,
				DatabaseURL: validDB,
				Env:         config.EnvDevelopment,
				CORSOrigin:  "",
			},
		},
		{
			name: "all variables set",
			env: map[string]string{
				"PORT":         "9090",
				"DATABASE_URL": validDB,
				"ENV":          "production",
				"CORS_ORIGIN":  "http://localhost:5173",
			},
			want: config.Config{
				Port:        9090,
				DatabaseURL: validDB,
				Env:         config.EnvProduction,
				CORSOrigin:  "http://localhost:5173",
			},
		},
		{
			name:     "missing DATABASE_URL",
			env:      map[string]string{},
			wantErrs: []string{"DATABASE_URL is required"},
		},
		{
			name:     "non-numeric PORT",
			env:      map[string]string{"DATABASE_URL": validDB, "PORT": "http"},
			wantErrs: []string{"PORT"},
		},
		{
			name:     "PORT out of range",
			env:      map[string]string{"DATABASE_URL": validDB, "PORT": "70000"},
			wantErrs: []string{"PORT"},
		},
		{
			name:     "unknown ENV",
			env:      map[string]string{"DATABASE_URL": validDB, "ENV": "staging"},
			wantErrs: []string{"ENV"},
		},
		{
			name: "all problems reported at once",
			env:  map[string]string{"PORT": "nope", "ENV": "staging"},
			wantErrs: []string{
				"DATABASE_URL is required",
				"PORT",
				"ENV",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"PORT", "DATABASE_URL", "ENV", "CORS_ORIGIN"} {
				t.Setenv(key, "")
				if v, ok := tt.env[key]; ok {
					t.Setenv(key, v)
				}
			}

			got, err := config.Load()

			if len(tt.wantErrs) > 0 {
				if err == nil {
					t.Fatalf("Load() = %+v, want error containing %v", got, tt.wantErrs)
				}
				for _, want := range tt.wantErrs {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err, want)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
