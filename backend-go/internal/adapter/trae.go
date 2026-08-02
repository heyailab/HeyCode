package adapter

import "github.com/heycode/backend-go/internal/types"

// TraeAdapter 继承 claude-code 的全部解析逻辑，仅 Kind() 返回 trae。
// 见 SPEC-GO-REWRITE.md §2.6.3：完全继承 claude-code 的 parseLine/buildUserInput。
type TraeAdapter struct {
	ClaudeCodeAdapter
}

// Kind 返回 trae。
func (a *TraeAdapter) Kind() types.CliKind { return types.CliTrae }

// BuildStartCommand 同 claude-code，仅 command 改为 "trae"。
func (a *TraeAdapter) BuildStartCommand(opts BuildCommandOpts) StartCommand {
	cmd := a.ClaudeCodeAdapter.BuildStartCommand(opts)
	cmd.Command = "trae"
	return cmd
}
