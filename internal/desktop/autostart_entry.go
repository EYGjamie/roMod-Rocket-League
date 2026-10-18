package desktop

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// The autostart entries of macOS and Linux, kept apart from the system code
// so they can be tested anywhere.

const launchAgent = "com.github.eygjamie.auto-replay-upload"

// agentPlist describes the launch agent: start args once at sign-in in the
// graphical session, without restarting it after "Beenden".
func agentPlist(args []string) []byte {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	b.WriteString("\t<key>Label</key>\n\t<string>" + launchAgent + "</string>\n")
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, a := range args {
		b.WriteString("\t\t<string>")
		xml.EscapeText(&b, []byte(a))
		b.WriteString("</string>\n")
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>LimitLoadToSessionType</key>\n\t<string>Aqua</string>\n")
	b.WriteString("\t<key>ProcessType</key>\n\t<string>Interactive</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

// entrySwitchedOff reports whether the desktop's settings turned the entry off
// without removing it.
func entrySwitchedOff(entry string) bool {
	for _, line := range strings.Split(entry, "\n") {
		key, value, _ := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "Hidden" && value == "true" || key == "X-GNOME-Autostart-enabled" && value == "false" {
			return true
		}
	}
	return false
}

func desktopEntry(args []string, off bool) string {
	s := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=auto-replay-upload\n" +
		"Comment=Lädt Rocket-League-Replays auf ballchasing.com hoch\n" +
		"Exec=" + execLine(args) + "\n" +
		"Terminal=false\n"
	if off {
		return s + "Hidden=true\nX-GNOME-Autostart-enabled=false\n"
	}
	return s + "X-GNOME-Autostart-enabled=true\n"
}

// execLine quotes args for the Exec key: arguments with special characters
// in double quotes, % doubled, and on top the escapes of every string value
// (so a backslash in a quoted argument ends up as four).
func execLine(args []string) string {
	quote := strings.NewReplacer(`"`, `\"`, "`", "\\`", `$`, `\$`, `\`, `\\`)
	parts := make([]string, len(args))
	for i, a := range args {
		a = strings.ReplaceAll(a, "%", "%%")
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`") {
			a = `"` + quote.Replace(a) + `"`
		}
		parts[i] = a
	}
	escape := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
	return escape.Replace(strings.Join(parts, " "))
}
