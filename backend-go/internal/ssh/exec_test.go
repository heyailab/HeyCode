package ssh

import (
	"testing"

	"github.com/heycode/backend-go/internal/types"
)

// TestShellQuote 覆盖 spec §4.6 规定的所有边界：
//   - 空串 → ''
//   - 纯安全字符 → 原样
//   - 含空格 → 单引号包裹
//   - 含单引号 → 转义
//   - 含特殊字符（$、;、& 等）→ 单引号包裹
func TestShellQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "''"},
		{"abc", "abc"},
		{"abc-123", "abc-123"},
		{"a/b/c", "a/b/c"},
		{"a.b", "a.b"},
		{"A_B@1:2,3", "A_B@1:2,3"},
		{"hello world", "'hello world'"},
		{"a'b", "'a'\\''b'"},
		{"$HOME", "'$HOME'"},
		{"a;rm -rf /", "'a;rm -rf /'"},
		{"a&&b", "'a&&b'"},
		{"`whoami`", "'`whoami`'"},
		{"/path/with space/file", "'/path/with space/file'"},
		{`"quoted"`, `'"quoted"'`},
		{"中文", "'中文'"}, // 非 ASCII 必然不在安全字符集
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := ShellQuote(c.in)
			if got != c.want {
				t.Errorf("ShellQuote(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestConfigKey 验证配置变更检测：相同配置 → 相同 key；任一字段变化 → key 不同。
func TestConfigKey(t *testing.T) {
	info := ServerAuthInfo{
		Host: "1.2.3.4", Port: 22, Username: "root",
		Auth: types.SshAuth{Kind: types.AuthPassword, Password: "secret"},
	}
	k1 := configKey(info)

	// 相同输入 → 相同 key
	if k2 := configKey(info); k2 != k1 {
		t.Errorf("configKey not deterministic: %q vs %q", k1, k2)
	}

	// host 变化
	changed := info
	changed.Host = "5.6.7.8"
	if k := configKey(changed); k == k1 {
		t.Errorf("configKey should change when host changes")
	}

	// port 变化
	changed = info
	changed.Port = 2222
	if k := configKey(changed); k == k1 {
		t.Errorf("configKey should change when port changes")
	}

	// username 变化
	changed = info
	changed.Username = "ubuntu"
	if k := configKey(changed); k == k1 {
		t.Errorf("configKey should change when username changes")
	}

	// auth.password 变化
	changed = info
	changed.Auth.Password = "different"
	if k := configKey(changed); k == k1 {
		t.Errorf("configKey should change when password changes")
	}
}
