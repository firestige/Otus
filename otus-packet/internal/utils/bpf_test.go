package utils

import (
	"testing"
)

func TestCompileBpf(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		hasErr bool
	}{
		{
			name:   "empty filter",
			filter: "",
			hasErr: false,
		},
		{
			name:   "ipv4 protocol only",
			filter: "ip",
			hasErr: false,
		},
		{
			name:   "ipv6 protocol only",
			filter: "ip6",
			hasErr: false,
		},
		{
			name:   "source IP address",
			filter: "src 192.168.1.1",
			hasErr: false,
		},
		{
			name:   "destination IP address",
			filter: "dst 10.0.0.1",
			hasErr: false,
		},
		{
			name:   "host IP address",
			filter: "host 172.16.0.1",
			hasErr: false,
		},
		{
			name:   "network CIDR",
			filter: "net 192.168.0.0/24",
			hasErr: false,
		},
		{
			name:   "IPv6 source address",
			filter: "src 2001:db8::1",
			hasErr: false,
		},
		{
			name:   "complex filter (only IP parts processed)",
			filter: "host 192.168.1.1 and port 80",
			hasErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instructions, err := CompileBpf(tt.filter)

			if tt.hasErr && err == nil {
				t.Errorf("compileBpf() error = %v, wantErr %v", err, tt.hasErr)
				return
			}

			if !tt.hasErr && err != nil {
				t.Errorf("compileBpf() error = %v, wantErr %v", err, tt.hasErr)
				return
			}

			if !tt.hasErr && len(instructions) == 0 {
				t.Errorf("compileBpf() returned empty instructions for valid filter")
			}

			t.Logf("Filter '%s' compiled to %d BPF instructions", tt.filter, len(instructions))
		})
	}
}

func TestValidateFilter(t *testing.T) {
	validFilters := []string{
		"",
		"ip",
		"ip6",
		"src 192.168.1.1",
		"dst 10.0.0.1",
		"host 172.16.0.1",
		"net 192.168.0.0/24",
		"src 2001:db8::1",
	}

	for _, filter := range validFilters {
		t.Run("valid_"+filter, func(t *testing.T) {
			err := ValidateFilter(filter)
			if err != nil {
				t.Errorf("ValidateFilter(%q) = %v, want nil", filter, err)
			}
		})
	}
}

func TestParseIPConditions(t *testing.T) {
	tests := []struct {
		name          string
		filter        string
		expectedCount int
		expectedType  string
	}{
		{
			name:          "IPv4 protocol",
			filter:        "ip",
			expectedCount: 1,
			expectedType:  "ipv4",
		},
		{
			name:          "IPv6 protocol",
			filter:        "ip6",
			expectedCount: 1,
			expectedType:  "ipv6",
		},
		{
			name:          "source IP",
			filter:        "src 192.168.1.1",
			expectedCount: 1,
			expectedType:  "ipv4",
		},
		{
			name:          "destination IP",
			filter:        "dst 10.0.0.1",
			expectedCount: 1,
			expectedType:  "ipv4",
		},
		{
			name:          "host IP",
			filter:        "host 172.16.0.1",
			expectedCount: 1,
			expectedType:  "ipv4",
		},
		{
			name:          "IPv6 source",
			filter:        "src 2001:db8::1",
			expectedCount: 1,
			expectedType:  "ipv6",
		},
		{
			name:          "no IP conditions",
			filter:        "port 80",
			expectedCount: 0,
			expectedType:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions, err := parseIPConditions(tt.filter)
			if err != nil {
				t.Errorf("parseIPConditions() error = %v", err)
				return
			}

			if len(conditions) != tt.expectedCount {
				t.Errorf("parseIPConditions() got %d conditions, want %d", len(conditions), tt.expectedCount)
				return
			}

			if tt.expectedCount > 0 && conditions[0].Protocol != tt.expectedType {
				t.Errorf("parseIPConditions() got protocol %s, want %s", conditions[0].Protocol, tt.expectedType)
			}
		})
	}
}
