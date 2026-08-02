package adapter

import (
	"regexp"
	"strings"

	"github.com/heycode/backend-go/internal/types"
)

// ptyEndMarker 是 pty 适配器约定的会话结束标记（见 §4.11）。
const ptyEndMarker = "__PTY_END__"

// ansiRegex 匹配 ANSI 转义序列（颜色码等），见 §4.7。
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// stripAnsi 剥离 ANSI 转义序列。
func stripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// PtyAdapter 是降级模式适配器，用于无专用适配器的 CLI（gemini/lingma）。
//
// 模式：PTY 终端型。
//   - 命令：lingma --cwd <cwd> "<prompt>"，spawn 时 pty:true
//   - 无续接（重启丢上下文）
//   - 首行发 session.init + progress + command.exec
//   - 后续每行原样发 command.exec
//   - __PTY_END__ → streaming.done + session.end
type PtyAdapter struct{}

func (a *PtyAdapter) Kind() types.CliKind { return types.CliPty }

// BuildStartCommand 构造 lingma 启动命令。
// 注意：pty 适配器用于 gemini/lingma 等无结构化输出的 CLI，
// 这里默认用 lingma 作为可执行文件名（用户可在 server 配置中调整）。
func (a *PtyAdapter) BuildStartCommand(opts BuildCommandOpts) StartCommand {
	args := []string{"--cwd", opts.Cwd}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	// prompt 作为命令行参数
	args = append(args, opts.Prompt)
	return StartCommand{Command: "lingma", Args: args}
}

// BuildUserInput pty 不支持 stdin 多轮，返回 ""。
func (a *PtyAdapter) BuildUserInput(prompt string) string { return "" }

// ParseLine 解析 pty 输出的一行。
//   - 首行：发 session.init + progress
//   - 每行（含首行）：发 command.exec（输出原样，剥离 ANSI）
//   - __PTY_END__：发 streaming.done + session.end
func (a *PtyAdapter) ParseLine(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	cleaned := stripAnsi(strings.TrimRight(line, "\r\n"))
	var events []types.UnifiedEvent

	// 首行发 session.init + progress（pty 无 cliSessionId）
	if !ctx.sessionInitSent {
		ctx.sessionInitSent = true
		events = append(events, types.NewSessionInit(ts, ctx.SessionId, "", string(ctx.Cli), ctx.Model, ctx.Cwd))
		events = append(events, types.NewProgress(ts, nil, nil, "PTY 会话已启动"))
	}

	// 结束标记
	if strings.Contains(cleaned, ptyEndMarker) {
		events = append(events, types.NewStreamingDone(ts, ctx.CurrentMessageId))
		events = append(events, types.NewSessionEnd(ts, nil))
		return events
	}

	// 每行原样作为 command.exec 输出（无 exitCode，表示进行中）
	events = append(events, types.NewCommandExec(ts, "", ctx.Cwd, nil, cleaned, "", ""))
	return events
}
