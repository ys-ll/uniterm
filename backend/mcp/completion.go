package mcp

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"strings"
)

// CompletionCommand adds an in-band acknowledgement issued by the shell only
// after the submitted command returns. eval runs in the current shell, so cd,
// exports and shell variables survive. The input contains escaped control
// characters: terminal echo cannot impersonate the acknowledgement.
// For shells outside the known list, a degraded acknowledgement is built from
// bare echo lines, each on its own input line: a newline is the one statement
// separator every shell accepts, so a command trailing in &, | or an open
// quote cannot swallow it (a "; printf" join would, and needs exit-code
// syntax too). The leading echo guarantees the marker starts on a fresh line
// even when the command's own output lacks a trailing newline. Detection
// anchors on line start, and the typed occurrence always has "echo " before
// the command text, so terminal echo cannot impersonate it. Exit code is
// unknown in this mode.
//
// filterEncoding is non-empty only for shells whose piped native-program
// output is legacy-encoded (see session.LocalSession.ExecFilterEncoding): the
// job's output is then routed through a Perl filter that re-encodes lines
// which are not valid UTF-8 from that charset, while valid UTF-8 lines pass
// through untouched. The filter lives inside the shell pipeline because the
// garbling is unrecoverable after the MSYS console writer has run; the fifo
// keeps eval in the current shell so cd/exports still survive, and the wait
// before the marker guarantees the filter has flushed before completion is
// acknowledged. If mkfifo or perl is unavailable the wrapper degrades to the
// plain form.
func CompletionCommand(command, shell, filterEncoding, token, newline string) (string, *completionScanner, bool) {
	quoted := "'" + strings.ReplaceAll(command, "'", "'\"'\"'") + "'"
	// Use concealed screen text rather than OSC: ConPTY passes OSC through
	// ahead of pending screen updates, which would acknowledge completion
	// before the last output frame. Text shares the renderer's output order.
	format := `\n\033[8m__UT_` + token + `;%d__\033[0m\n`
	switch shell {
	case "bash", "zsh", "sh", "dash", "ksh":
		if filterEncoding == "" {
			return " eval " + quoted + "; printf '" + format + `' "$?"`,
				&completionScanner{prefix: []byte("__UT_" + token + ";")}, true
		}
		// The Perl filter validates each line against a strict UTF-8 byte
		// grammar (not Encode::decode/decode_utf8: their in-place UTF-8
		// optimization destroys the argument scalar, silently swallowing
		// lines) and re-encodes invalid ones from the legacy charset. A line
		// mixing both charsets is re-encoded as a whole — rare, since a line
		// is normally written by one program.
		filter := `BEGIN{$|=1} while (<>) { print m{\A(?:[\x00-\x7F]|[\xC2-\xDF][\x80-\xBF]|` +
			`[\xE0-\xEF][\x80-\xBF]{2}|[\xF0-\xF4][\x80-\xBF]{3})*\z} ? $_ : ` +
			`Encode::encode_utf8(Encode::decode("` + filterEncoding + `", $_)); }`
		return " __utc=" + quoted + "; __utd=; " +
				"if __utd=$(mktemp -d) && mkfifo \"$__utd/p\" 2>/dev/null && command -v perl >/dev/null 2>&1; then " +
				"perl -MEncode -e '" + filter + "' < \"$__utd/p\" & __utp=$!; " +
				"eval \"$__utc\" > \"$__utd/p\" 2>&1; __utr=$?; wait $__utp; rm -rf \"$__utd\"; " +
				"else [ -n \"$__utd\" ] && rm -rf \"$__utd\"; eval \"$__utc\"; __utr=$?; fi; " +
				"printf '" + format + "' \"$__utr\"; unset __utc __utd __utp __utr",
			&completionScanner{prefix: []byte("__UT_" + token + ";")}, true
	case "powershell", "pwsh":
		// $LASTEXITCODE is sticky across PowerShell cases. Reset it before
		// evaluation. Capture $? INSIDE the script block: Windows PowerShell
		// 5 resets it when an invocation/expression returns to its caller.
		name := "__ut_" + strings.ReplaceAll(token, "-", "_")
		// Keep terminal input on one line: PSReadLine does not treat a bulk
		// write containing literal LF inside a quoted string as a script paste.
		encoded := base64.StdEncoding.EncodeToString([]byte(command + "\n; $" + name + "=@($?, $global:LASTEXITCODE)"))
		psQuoted := "[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + encoded + "'))"
		return ". { $global:LASTEXITCODE=0; try { . ([scriptblock]::Create(" + psQuoted + ")); if ($" + name + "[0]) { $" + name + "=0 } elseif ($" + name + "[1]) { $" + name + "=$" + name + "[1] } else { $" + name + "=1 } } catch { Write-Error $_ -ErrorAction Continue; $" + name + "=1 } } | Out-Default; [Console]::Write([Environment]::NewLine+([string][char]27)+'[8m__UT_" + token + ";'+$" + name + "+'__'+([string][char]27)+'[0m'+[Environment]::NewLine); Remove-Variable " + name,
			&completionScanner{prefix: []byte("__UT_" + token + ";")}, true
	default:
		return command + newline + "echo" + newline + "echo __UT_" + token + "__",
			&completionScanner{prefix: []byte("\n__UT_" + token + "__"), plain: true}, true
	}
}

// completionScanner tolerates arbitrary read boundaries, including a split
// UTF-8 character next to the control sequence. Only this job's nonce matches.
// plain marks the echo fallback: the acknowledgement carries no exit code and
// the prefix already includes the trailing "__", so a prefix match completes
// immediately.
type completionScanner struct {
	prefix  []byte
	plain   bool
	pending []byte
}

func (s *completionScanner) feed(data []byte) (cleaned []byte, code int, complete bool) {
	buf := append(s.pending, data...)
	s.pending = nil
	for {
		i := bytes.Index(buf, s.prefix)
		if i < 0 {
			keep := 0
			for n := min(len(buf), len(s.prefix)-1); n > 0; n-- {
				if bytes.Equal(buf[len(buf)-n:], s.prefix[:n]) {
					keep = n
					break
				}
			}
			cleaned = append(cleaned, buf[:len(buf)-keep]...)
			s.pending = append([]byte(nil), buf[len(buf)-keep:]...)
			return
		}
		cleaned = append(cleaned, buf[:i]...)
		if s.plain {
			// The echo fallback cannot report an exit code. The anchor newline
			// is the marker line's own line break — keep it in the output.
			return append(cleaned, '\n'), -1, true
		}
		rest := buf[i+len(s.prefix):]
		end := bytes.Index(rest, []byte("__"))
		if end < 0 && len(rest) <= 16 {
			s.pending = append([]byte(nil), buf[i:]...)
			return
		}
		if end >= 0 {
			if status, err := strconv.Atoi(string(rest[:end])); err == nil {
				// Do not include the next prompt (or later shell output).
				return cleaned, status, true
			}
		}
		cleaned = append(cleaned, buf[i])
		buf = buf[i+1:]
	}
}
