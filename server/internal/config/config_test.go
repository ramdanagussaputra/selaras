package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ramdanaguss/selaras/server/internal/config"
)

func TestLoad(t *testing.T) {
	const validDB = "postgres://selaras:selaras@localhost:5432/selaras?sslmode=disable"
	const validSecret = "0123456789abcdef0123456789abcdef" // exactly 32 bytes

	tests := []struct {
		name     string
		env      map[string]string
		want     config.Config
		wantErrs []string // substrings that must all appear in the error
	}{
		{
			name: "defaults applied when required vars set",
			env:  map[string]string{"DATABASE_URL": validDB, "JWT_SECRET": validSecret},
			want: config.Config{
				Port:            8080,
				DatabaseURL:     validDB,
				Env:             config.EnvDevelopment,
				CORSOrigin:      "",
				JWTSecret:       validSecret,
				AccessTokenTTL:  15 * time.Minute,
				RefreshTokenTTL: 168 * time.Hour,
			},
		},
		{
			name: "all variables set",
			env: map[string]string{
				"PORT":              "9090",
				"DATABASE_URL":      validDB,
				"ENV":               "production",
				"CORS_ORIGIN":       "http://localhost:5173",
				"JWT_SECRET":        validSecret,
				"ACCESS_TOKEN_TTL":  "30m",
				"REFRESH_TOKEN_TTL": "72h",
			},
			want: config.Config{
				Port:            9090,
				DatabaseURL:     validDB,
				Env:             config.EnvProduction,
				CORSOrigin:      "http://localhost:5173",
				JWTSecret:       validSecret,
				AccessTokenTTL:  30 * time.Minute,
				RefreshTokenTTL: 72 * time.Hour,
			},
		},
		{
			name:     "missing DATABASE_URL",
			env:      map[string]string{"JWT_SECRET": validSecret},
			wantErrs: []string{"DATABASE_URL is required"},
		},
		{
			name:     "missing JWT_SECRET",
			env:      map[string]string{"DATABASE_URL": validDB},
			wantErrs: []string{"JWT_SECRET"},
		},
		{
			name:     "too-short JWT_SECRET",
			env:      map[string]string{"DATABASE_URL": validDB, "JWT_SECRET": "tooshort"},
			wantErrs: []string{"JWT_SECRET"},
		},
		{
			name:     "non-numeric PORT",
			env:      map[string]string{"DATABASE_URL": validDB, "JWT_SECRET": validSecret, "PORT": "http"},
			wantErrs: []string{"PORT"},
		},
		{
			name:     "PORT out of range",
			env:      map[string]string{"DATABASE_URL": validDB, "JWT_SECRET": validSecret, "PORT": "70000"},
			wantErrs: []string{"PORT"},
		},
		{
			name:     "unknown ENV",
			env:      map[string]string{"DATABASE_URL": validDB, "JWT_SECRET": validSecret, "ENV": "staging"},
			wantErrs: []string{"ENV"},
		},
		{
			name:     "invalid ACCESS_TOKEN_TTL",
			env:      map[string]string{"DATABASE_URL": validDB, "JWT_SECRET": validSecret, "ACCESS_TOKEN_TTL": "nope"},
			wantErrs: []string{"ACCESS_TOKEN_TTL"},
		},
		{
			name: "all problems reported at once",
			env:  map[string]string{"PORT": "nope", "ENV": "staging"},
			wantErrs: []string{
				"DATABASE_URL is required",
				"PORT",
				"ENV",
				"JWT_SECRET",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			for _, key := range []string{
				"PORT", "DATABASE_URL", "ENV", "CORS_ORIGIN",
				"JWT_SECRET", "ACCESS_TOKEN_TTL", "REFRESH_TOKEN_TTL",
			} {
				t.Setenv(key, "")
				if v, ok := testCase.env[key]; ok {
					t.Setenv(key, v)
				}
			}

			got, err := config.Load()

			if len(testCase.wantErrs) > 0 {
				if err == nil {
					t.Fatalf("Load() = %+v, want error containing %v", got, testCase.wantErrs)
				}
				for _, want := range testCase.wantErrs {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err, want)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Errorf("Load() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
