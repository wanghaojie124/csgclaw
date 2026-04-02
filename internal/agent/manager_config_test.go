package agent

import (
	"net"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestRenderManagerSecurityConfig(t *testing.T) {
	got := renderManagerSecurityConfig(config.LLMConfig{
		ModelID: "minimax-m2.7",
		APIKey:  "sk-1234567890",
	})

	for _, want := range []string{
		"model_list:\n",
		"  minimax-m2.7:0:\n",
		"    api_keys:\n",
		"      - sk-1234567890\n",
		"channels: {}\n",
		"web: {}\n",
		"skills: {}\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderManagerSecurityConfig() missing %q in:\n%s", want, got)
		}
	}
}

func TestIPv4FromAddr(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want string
	}{
		{
			name: "ipv4 net",
			addr: &net.IPNet{IP: net.ParseIP("192.168.1.20"), Mask: net.CIDRMask(24, 32)},
			want: "192.168.1.20",
		},
		{
			name: "ipv4 addr",
			addr: &net.IPAddr{IP: net.ParseIP("10.0.0.8")},
			want: "10.0.0.8",
		},
		{
			name: "loopback",
			addr: &net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			want: "",
		},
		{
			name: "ipv6",
			addr: &net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ipv4FromAddr(tt.addr); got != tt.want {
				t.Fatalf("ipv4FromAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}
